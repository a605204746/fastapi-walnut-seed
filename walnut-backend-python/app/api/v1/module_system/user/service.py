"""用户域业务层。

- 数据权限：行级过滤条件复用核心组件 ``app.core.permission.Permission``
  （sys_user 按 RuoYi 契约以 ``dept_id`` 列过滤）；无角色仅本人可见、
  任一角色 ALL 不过滤、多角色条件 OR 连接。
- 角色/岗位关联均先删后插；删除用户为逻辑删除并清理关联。
"""

from __future__ import annotations

from datetime import datetime
from datetime import time as dtime

import bcrypt
from pydantic import ValidationError
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.sql.elements import ColumnElement

from app.api.v1.module_system.role.schema import RoleOutSchema
from app.api.v1.module_system.user.crud import UserCrud
from app.api.v1.module_system.user.model import UserModel
from app.api.v1.module_system.user.schema import (
    GetInfoOutSchema,
    ProfileOutSchema,
    ProfileUserOutSchema,
    UserBaseSchema,
    UserCreateSchema,
    UserInfoOutSchema,
    UserOutSchema,
    UserPasswordUpdateSchema,
    UserProfileUpdateSchema,
    UserQueryParam,
    UserUpdateSchema,
    dump_camel,
)
from app.common.constant import SystemConstants
from app.common.enums import HttpStatus
from app.core.base_schema import AuthSchema
from app.core.exceptions import ServiceException
from app.utils.string_util import is_blank, str2list


def _cell_str(row: dict, key: str) -> str | None:
    """导入单元格 → 去空白字符串（None 保持 None）。"""
    value = row.get(key)
    if value is None:
        return None
    text = str(value).strip()
    return text or None


def _cell_int(row: dict, key: str) -> int | None:
    """导入单元格 → 整数（非法/空返回 None）。"""
    value = row.get(key)
    if value is None or str(value).strip() == "":
        return None
    try:
        return int(float(value))
    except (ValueError, TypeError):
        return None


def _validation_message(exc: ValidationError) -> str:
    """提取 pydantic 校验错误消息（去掉 "Value error" 前缀，逗号拼接）。"""
    msgs = []
    for err in exc.errors():
        msg = str(err.get("msg", ""))
        if msg.startswith("Value error"):
            msg = msg[len("Value error") :].lstrip(", ")
        msgs.append(msg)
    return ", ".join(msgs) if msgs else "请求参数验证失败"


class UserService:
    """用户服务。"""

    def __init__(self, auth: AuthSchema, db: AsyncSession) -> None:
        self.auth = auth
        self.db = db
        self.crud = UserCrud(UserModel, auth, db)

    # ==================== 通用辅助 ====================
    def _current_is_super_admin(self) -> bool:
        """当前登录用户是否超级管理员。"""
        user = self.auth.user
        return user.id == SystemConstants.SUPER_ADMIN_ID or bool(user.is_superuser)

    @staticmethod
    def _is_super_admin_id(user_id: int | None) -> bool:
        """指定用户ID是否超级管理员。"""
        return user_id is not None and user_id == SystemConstants.SUPER_ADMIN_ID

    # ==================== 数据权限 ====================
    async def _user_data_scope_condition(self) -> ColumnElement | None:
        """构建 sys_user 行级数据权限条件（委托核心组件 Permission，多角色条件 OR 连接）。

        与 core/permission.py 单一实现保持一致：部门列按 RuoYi 契约使用 ``dept_id``；
        超级管理员不过滤、任一角色 ALL 不过滤、自定义按角色授权部门、
        本部门及以下按 ancestors 子孙子查询、无角色仅本人（fail-closed）。
        """
        if self._current_is_super_admin():
            return None
        from app.core.permission import Permission

        return await Permission(UserModel, self.auth, self.db, dept_column=UserModel.dept_id).build_condition()

    # ==================== 校验 ====================
    async def check_user_allowed(self, user_id: int | None) -> None:
        """校验是否允许操作该用户（超管不可操作）。"""
        if self._is_super_admin_id(user_id):
            raise ServiceException("不允许操作超级管理员用户")

    async def check_user_data_scope(self, user_id: int | None) -> None:
        """校验是否有该用户的数据权限。"""
        if user_id is None:
            return
        if self._current_is_super_admin():
            return
        scope = await self._user_data_scope_condition()
        if await self.crud.count_user_by_id(user_id, scope) == 0:
            raise ServiceException("没有权限访问用户数据！")

    async def check_dept_data_scope(self, dept_id: int | None) -> None:
        """校验是否有该部门的数据权限（复用 DeptService.check_dept_data_scope）。"""
        from app.api.v1.module_system.dept.service import DeptService

        await DeptService(self.auth, self.db).check_dept_data_scope(dept_id)

    async def check_user_name_unique(self, user_name: str, exclude_id: int | None = None) -> bool:
        conditions = [UserModel.user_name == user_name]
        if exclude_id is not None:
            conditions.append(UserModel.id != exclude_id)
        return not await self.crud.exists_by(*conditions)

    async def check_phone_unique(self, phonenumber: str, exclude_id: int | None = None) -> bool:
        conditions = [UserModel.phonenumber == phonenumber]
        if exclude_id is not None:
            conditions.append(UserModel.id != exclude_id)
        return not await self.crud.exists_by(*conditions)

    async def check_email_unique(self, email: str, exclude_id: int | None = None) -> bool:
        conditions = [UserModel.email == email]
        if exclude_id is not None:
            conditions.append(UserModel.id != exclude_id)
        return not await self.crud.exists_by(*conditions)

    # ==================== 查询条件构建 ====================
    def _between_condition(self, begin, end) -> ColumnElement | None:
        # begin/end 同时存在才生效
        if begin is None or end is None:
            return None
        return UserModel.create_time.between(datetime.combine(begin, dtime.min), datetime.combine(end, dtime.min))

    async def _build_list_conditions(self, req: UserQueryParam) -> list[ColumnElement]:
        """列表查询条件（不含 del_flag，由 crud 统一追加）。"""
        conditions: list[ColumnElement] = []
        if req.id is not None:
            conditions.append(UserModel.id == req.id)
        if not is_blank(req.user_name):
            conditions.append(UserModel.user_name.like(f"%{req.user_name}%"))
        if not is_blank(req.nick_name):
            conditions.append(UserModel.nick_name.like(f"%{req.nick_name}%"))
        if not is_blank(req.status):
            conditions.append(UserModel.status == req.status)
        if not is_blank(req.phonenumber):
            conditions.append(UserModel.phonenumber.like(f"%{req.phonenumber}%"))
        between = self._between_condition(req.begin_time, req.end_time)
        if between is not None:
            conditions.append(between)
        if req.dept_id is not None:
            conditions.append(UserModel.dept_id.in_(await self.crud.dept_and_child_ids(req.dept_id)))
        if not is_blank(req.user_ids):
            try:
                conditions.append(UserModel.id.in_([int(x) for x in str2list(req.user_ids)]))
            except ValueError:
                raise ServiceException("用户ID格式有误", code=HttpStatus.BAD_REQUEST)
        if not is_blank(req.exclude_user_ids):
            try:
                conditions.append(UserModel.id.not_in([int(x) for x in str2list(req.exclude_user_ids)]))
            except ValueError:
                raise ServiceException("用户ID格式有误", code=HttpStatus.BAD_REQUEST)
        return conditions

    async def _build_export_conditions(self, req: UserQueryParam) -> list[ColumnElement]:
        """导出查询条件（无 id/userIds/excludeUserIds）。"""
        conditions: list[ColumnElement] = []
        if not is_blank(req.user_name):
            conditions.append(UserModel.user_name.like(f"%{req.user_name}%"))
        if not is_blank(req.nick_name):
            conditions.append(UserModel.nick_name.like(f"%{req.nick_name}%"))
        if not is_blank(req.status):
            conditions.append(UserModel.status == req.status)
        if not is_blank(req.phonenumber):
            conditions.append(UserModel.phonenumber.like(f"%{req.phonenumber}%"))
        between = self._between_condition(req.begin_time, req.end_time)
        if between is not None:
            conditions.append(between)
        if req.dept_id is not None:
            conditions.append(UserModel.dept_id.in_(await self.crud.dept_and_child_ids(req.dept_id)))
        return conditions

    # ==================== 部门名回填 / 字典映射 ====================
    async def _dept_name_map(self, users: list[UserModel]) -> dict[int, str]:
        dept_ids = sorted({u.dept_id for u in users if u.dept_id is not None})
        return await self.crud.map_dept_names(dept_ids)

    async def _dict_maps(self, dict_type: str) -> tuple[dict, dict]:
        """字典 (value→label, label→value) 映射。"""
        from app.api.v1.module_system.dict.service import DictTypeService

        data = await DictTypeService(self.auth, self.db).select_dict_data_by_type(dict_type, None) or []
        value_to_label = {item.get("dictValue"): item.get("dictLabel") for item in data}
        label_to_value = {item.get("dictLabel"): item.get("dictValue") for item in data}
        return value_to_label, label_to_value

    # ==================== 查询 ====================
    async def select_page_user_list(self, req: UserQueryParam) -> dict:
        """分页查询用户列表（带数据权限 + deptName 回填）。"""
        conditions = await self._build_list_conditions(req)
        scope = await self._user_data_scope_condition()
        if scope is not None:
            conditions.append(scope)
        result = await self.crud.page_user_list(req, conditions)
        dept_names = await self._dept_name_map(result["rows"])
        rows = []
        for user in result["rows"]:
            out = UserOutSchema.model_validate(user)
            out.dept_name = dept_names.get(user.dept_id)
            out.roles = None  # 列表不返回角色对象，仅回填 deptName
            rows.append(dump_camel(out))
        return {"rows": rows, "total": result["total"]}

    async def select_user_export_list(self, req: UserQueryParam) -> list[dict]:
        """导出用户列表（含 deptName/负责人翻译、性别/状态字典转换）。"""
        conditions = await self._build_export_conditions(req)
        scope = await self._user_data_scope_condition()
        if scope is not None:
            conditions.append(scope)
        result_rows = await self.crud.list_export_rows(conditions)
        sex_v2l, _ = await self._dict_maps("sys_user_sex")
        status_v2l, _ = await self._dict_maps("sys_normal_disable")
        rows = []
        for user, dept_name, leader_name in result_rows:
            rows.append(
                {
                    "id": user.id,
                    "user_name": user.user_name,
                    "nick_name": user.nick_name,
                    "email": user.email,
                    "phonenumber": user.phonenumber,
                    "sex": sex_v2l.get(user.sex, user.sex) if user.sex is not None else "",
                    "status": status_v2l.get(user.status, user.status) if user.status is not None else "",
                    "login_ip": user.login_ip,
                    "login_date": user.login_date,
                    "dept_name": dept_name,
                    "leader_name": leader_name,
                }
            )
        return rows

    async def select_user_by_id(self, user_id: int) -> UserOutSchema | None:
        """按ID查询用户（无数据权限，回填 roles 与 deptName）。"""
        user = await self.crud.get_user_by_id(user_id)
        if user is None:
            return None
        out = UserOutSchema.model_validate(user)
        out.roles = [RoleOutSchema.model_validate(r) for r in (user.roles or []) if r.del_flag == SystemConstants.NORMAL]
        dept_names = await self.crud.map_dept_names([user.dept_id] if user.dept_id is not None else [])
        out.dept_name = dept_names.get(user.dept_id) if user.dept_id is not None else None
        return out

    async def select_user_by_user_name(self, user_name: str) -> UserModel | None:
        """按用户账号查询（登录/导入使用）。"""
        return await self.crud.get_user_by_user_name(user_name)

    async def select_user_by_ids(self, user_ids: list[int] | None, dept_id: int | None) -> list[dict]:
        """用户选择框列表（status=0，仅输出 id/userName/nickName）。"""
        users = await self.crud.list_option(user_ids, dept_id)
        return [dump_camel(UserOutSchema(id=u.id, user_name=u.user_name, nick_name=u.nick_name)) for u in users]

    async def select_user_list_by_dept(self, dept_id: int) -> list[dict]:
        """部门下所有用户（无数据权限，回填 deptName）。"""
        users = await self.crud.list_users(UserModel.dept_id == dept_id)
        dept_names = await self._dept_name_map(users)
        out = []
        for user in users:
            item = UserOutSchema.model_validate(user)
            item.dept_name = dept_names.get(user.dept_id) if user.dept_id is not None else None
            item.roles = None
            out.append(dump_camel(item))
        return out

    # ==================== GET / 与 /{userId} ====================
    async def _select_normal_role_models(self) -> list:
        """状态正常的角色列表（复用 RoleService，含数据权限）。"""
        from app.api.v1.module_system.role.schema import RoleQueryParam
        from app.api.v1.module_system.role.service import RoleService

        return await RoleService(self.auth, self.db).select_list(RoleQueryParam(status=SystemConstants.NORMAL))

    async def get_user_info(self, user_id: int | None) -> dict:
        """GET /user 与 /user/{userId}（返回 user + roleIds + posts + postIds + roles）。"""
        info = UserInfoOutSchema()
        if user_id is not None:
            await self.check_user_data_scope(user_id)
            user_out = await self.select_user_by_id(user_id)
            info.user = user_out
            info.role_ids = await self.crud.list_role_ids_by_user_id(user_id)
            dept_id = user_out.dept_id if user_out is not None else None
            if dept_id is not None:
                info.posts = await self._select_posts_by_dept(dept_id)
                info.post_ids = await self._select_post_ids_by_user(user_id)
        roles = await self._select_normal_role_models()
        # 非超管目标用户排除超级管理员角色
        if not self._is_super_admin_id(user_id):
            roles = [r for r in roles if r.id != SystemConstants.SUPER_ADMIN_ID]
        info.roles = [RoleOutSchema.model_validate(r) for r in roles]
        return dump_camel(info)

    async def _select_posts_by_dept(self, dept_id: int) -> list[dict]:
        """部门下岗位列表（复用 PostService，已驼峰输出）。"""
        from app.api.v1.module_system.post.schema import PostQueryParam
        from app.api.v1.module_system.post.service import PostService

        return await PostService(self.auth, self.db).select_list(PostQueryParam(dept_id=dept_id))

    async def _select_post_ids_by_user(self, user_id: int) -> list[int]:
        """用户所属岗位ID列表（复用 PostService）。"""
        from app.api.v1.module_system.post.service import PostService

        return await PostService(self.auth, self.db).select_post_list_by_user_id(user_id)

    # ==================== getInfo ====================
    async def get_info(self) -> dict | None:
        """GET /user/getInfo（忽略数据权限，返回 user + permissions + roles）。

        用户不存在时返回 None，由 controller 返回 fail("没有权限访问用户数据!")。
        """
        user_out = await self.select_user_by_id(self.auth.user.id)
        if user_out is None:
            return None
        info = GetInfoOutSchema(user=user_out, permissions=list(self.auth.permissions), roles=list(self.auth.roles))
        return dump_camel(info)

    # ==================== authRole ====================
    async def get_auth_role(self, user_id: int) -> dict:
        """GET /user/authRole/{userId}（返回 user + roles，roles 带 flag）。"""
        await self.check_user_data_scope(user_id)
        user_out = await self.select_user_by_id(user_id)
        user_role_ids = set(await self.crud.list_role_ids_by_user_id(user_id))
        roles = await self._select_all_role_models()
        role_outs = []
        for role in roles:
            item = RoleOutSchema.model_validate(role)
            item.flag = role.id in user_role_ids
            role_outs.append(item)
        if not self._is_super_admin_id(user_id):
            role_outs = [r for r in role_outs if r.id != SystemConstants.SUPER_ADMIN_ID]
        info = UserInfoOutSchema(user=user_out, roles=role_outs)
        return dump_camel(info)

    async def _select_all_role_models(self) -> list:
        """全部角色列表（复用 RoleService，含数据权限）。"""
        from app.api.v1.module_system.role.schema import RoleQueryParam
        from app.api.v1.module_system.role.service import RoleService

        return await RoleService(self.auth, self.db).select_list(RoleQueryParam())

    async def insert_user_auth(self, user_id: int | None, role_ids: list[int] | None) -> None:
        """PUT /user/authRole（重建用户角色关联）。"""
        if user_id is None:
            raise ServiceException("用户ID不能为空")
        # 与其他写路径一致：先校验目标不可为超管，再校验数据权限
        await self.check_user_allowed(user_id)
        await self.check_user_data_scope(user_id)
        await self._insert_user_role(user_id, role_ids, clear=True)

    # ==================== 角色/岗位关联（先删后插） ====================
    async def _insert_user_role(self, user_id: int, role_ids: list[int] | None, clear: bool) -> None:
        """新增/重建用户角色关联（clear=True 时先删后插）。"""
        if not role_ids:
            return
        role_list = list(role_ids)
        # 非超级管理员目标用户，禁止包含超级管理员角色
        if not self._is_super_admin_id(user_id):
            role_list = [r for r in role_list if r != SystemConstants.SUPER_ADMIN_ID]
        if not role_list:
            raise ServiceException("不允许为普通用户分配超级管理员角色，请至少选择一个其他角色")
        if await self.crud.count_visible_roles(role_list) != len(role_list):
            raise ServiceException("没有权限访问角色的数据")
        if clear:
            await self.crud.delete_user_roles([user_id])
        await self.crud.insert_user_roles(user_id, role_list)

    async def _insert_user_post(self, user_id: int, post_ids: list[int] | None, clear: bool) -> None:
        """新增/重建用户岗位关联。"""
        if not post_ids:
            return
        if await self.crud.count_visible_posts(post_ids) != len(post_ids):
            raise ServiceException("没有权限访问岗位的数据")
        if clear:
            await self.crud.delete_user_posts([user_id])
        await self.crud.insert_user_posts(user_id, post_ids)

    # ==================== 写操作 ====================
    async def insert_user(self, req: UserBaseSchema | UserCreateSchema) -> int:
        """新增用户（写用户 + 岗位关联 + 角色关联，密码由调用方预先哈希）。"""
        user = UserModel(
            dept_id=req.dept_id,
            user_name=req.user_name,
            nick_name=req.nick_name,
            user_type=req.user_type or "sys_user",
            email=req.email,
            phonenumber=req.phonenumber,
            sex=req.sex,
            password=req.password,
            status=req.status,
            remark=req.remark,
        )
        await self.crud.create(user)
        await self._insert_user_post(user.id, req.post_ids, clear=False)
        await self._insert_user_role(user.id, req.role_ids, clear=False)
        return 1

    async def update_user(self, req: UserUpdateSchema) -> int:
        """修改用户（先删后插角色/岗位关联，再更新非空字段）。"""
        if req.id is None:
            raise ServiceException("用户ID不能为空")
        await self._insert_user_role(req.id, req.role_ids, clear=True)
        await self._insert_user_post(req.id, req.post_ids, clear=True)
        user = await self.crud.get_user_by_id(req.id)
        if user is None:
            raise ServiceException(f"修改用户{req.user_name}信息失败")
        # 更新时忽略 None 字段；password 不在修改用户时变更
        data = req.model_dump(exclude_none=True, exclude={"id", "password", "role_ids", "post_ids"})
        for field, value in data.items():
            setattr(user, field, value)
        await self.crud.update(user)
        return 1

    async def delete_user_by_ids(self, user_ids: list[int]) -> int:
        """批量删除用户（校验 + 清理关联 + 逻辑删除）。"""
        for user_id in user_ids:
            await self.check_user_allowed(user_id)
            await self.check_user_data_scope(user_id)
        await self.crud.delete_user_roles(user_ids)
        await self.crud.delete_user_posts(user_ids)
        flag = await self.crud.soft_delete_by_ids(user_ids)
        if flag < 1:
            raise ServiceException("删除用户失败!")
        return flag

    async def reset_user_pwd(self, user_id: int, password: str) -> int:
        """重置密码（password 由调用方预先哈希）。"""
        return await self.crud.update_password(user_id, password)

    async def update_user_status(self, user_id: int, status: str | None) -> int:
        """修改状态。"""
        return await self.crud.update_status(user_id, status or SystemConstants.NORMAL)

    # ==================== 个人中心（profile） ====================
    async def select_user_role_group(self, user_id: int) -> str:
        """用户所属角色组（角色名逗号串）。"""
        user = await self.crud.get_user_by_id(user_id)
        if user is None:
            return ""
        names = [r.role_name for r in (user.roles or []) if r.del_flag == SystemConstants.NORMAL]
        return ",".join(names)

    async def select_user_post_group(self, user_id: int) -> str:
        """用户所属岗位组（岗位名逗号串）。"""
        from app.api.v1.module_system.post.service import PostService

        posts = await PostService(self.auth, self.db).select_posts_by_user_id(user_id)
        names = [name for p in posts if (name := p.get("postName"))]
        return ",".join(names)

    async def get_profile(self) -> dict:
        """GET /user/profile（返回 user + roleGroup + postGroup）。"""
        user_id = self.auth.user.id
        user = await self.crud.get_user_by_id(user_id)
        profile_user: ProfileUserOutSchema | None = None
        if user is not None:
            profile_user = ProfileUserOutSchema.model_validate(user)
            dept_names = await self.crud.map_dept_names([user.dept_id] if user.dept_id is not None else [])
            profile_user.dept_name = dept_names.get(user.dept_id) if user.dept_id is not None else None
        info = ProfileOutSchema(
            user=profile_user,
            role_group=await self.select_user_role_group(user_id),
            post_group=await self.select_user_post_group(user_id),
        )
        return dump_camel(info)

    async def update_user_profile(self, req: UserProfileUpdateSchema, user_id: int) -> int:
        """修改个人基本资料（nickName 非空才更新，其余字段直更）。"""
        values: dict = {"phonenumber": req.phonenumber, "email": req.email, "sex": req.sex}
        if req.nick_name is not None:
            values["nick_name"] = req.nick_name
        return await self.crud.update_profile(user_id, values)

    async def update_user_avatar(self, user_id: int, avatar_url: str) -> bool:
        """修改头像。"""
        return (await self.crud.update_avatar(user_id, avatar_url)) > 0

    async def update_pwd(self, req: UserPasswordUpdateSchema, user_id: int) -> int:
        """修改个人密码（校验旧密码、新旧不同后重置）。"""
        user = await self.crud.get_user_by_id(user_id)
        if user is None or not user.password:
            raise ServiceException("修改密码失败，旧密码错误")
        if req.old_password is None:
            raise ServiceException("旧密码不能为空")
        if req.new_password is None:
            raise ServiceException("新密码不能为空")
        if not bcrypt.checkpw(req.old_password.encode("utf-8"), user.password.encode("utf-8")):
            raise ServiceException("修改密码失败，旧密码错误")
        if bcrypt.checkpw(req.new_password.encode("utf-8"), user.password.encode("utf-8")):
            raise ServiceException("新密码不能与旧密码相同")
        return await self.crud.update_password(user_id, bcrypt.hashpw(req.new_password.encode("utf-8"), bcrypt.gensalt()).decode("utf-8"))

    # ==================== 导入 ====================
    async def import_users(self, rows: list[dict], update_support: bool) -> str:
        """导入用户，返回分析结果消息；失败抛 ServiceException。"""
        from app.api.v1.module_system.config.service import ConfigService
        from app.config.setting import settings

        # 初始密码优先取系统参数 sys.user.initPassword，缺失时回退配置项（禁止使用弱口令兜底）
        init_password = await ConfigService(self.auth, self.db).select_config_by_key("sys.user.initPassword", None) or settings.USER_IMPORT_INIT_PASSWORD
        hashed_password = bcrypt.hashpw(init_password.encode("utf-8"), bcrypt.gensalt()).decode("utf-8")
        _, sex_l2v = await self._dict_maps("sys_user_sex")
        _, status_l2v = await self._dict_maps("sys_normal_disable")

        success_num = 0
        failure_num = 0
        success_msg = ""
        failure_msg = ""
        for row in rows:
            user_name = _cell_str(row, "用户账号")
            nick_name = _cell_str(row, "用户昵称")
            email = _cell_str(row, "用户邮箱")
            phonenumber = _cell_str(row, "手机号码")
            dept_id = _cell_int(row, "部门编号")
            sex_label = _cell_str(row, "用户性别")
            sex = sex_l2v.get(sex_label, sex_label) if sex_label else None
            status_label = _cell_str(row, "账号状态")
            status = status_l2v.get(status_label, status_label) if status_label else None

            sys_user = await self.crud.get_user_by_user_name(user_name) if user_name else None
            try:
                if sys_user is None:
                    req = UserBaseSchema(
                        dept_id=dept_id,
                        user_name=user_name,
                        nick_name=nick_name,
                        email=email,
                        phonenumber=phonenumber,
                        sex=sex,
                        status=status,
                        password=hashed_password,
                    )
                    await self.insert_user(req)
                    success_num += 1
                    success_msg += f"<br/>{success_num}、账号 {user_name} 导入成功"
                elif update_support:
                    req_update = UserUpdateSchema(
                        id=sys_user.id,
                        dept_id=dept_id,
                        user_name=user_name,
                        nick_name=nick_name,
                        email=email,
                        phonenumber=phonenumber,
                        sex=sex,
                        status=status,
                    )
                    await self.check_user_allowed(sys_user.id)
                    await self.check_user_data_scope(sys_user.id)
                    await self.update_user(req_update)
                    success_num += 1
                    success_msg += f"<br/>{success_num}、账号 {user_name} 更新成功"
                else:
                    failure_num += 1
                    failure_msg += f"<br/>{failure_num}、账号 {sys_user.user_name} 已存在"
            except ValidationError as exc:
                failure_num += 1
                failure_msg += f"<br/>{failure_num}、账号 {user_name} 导入失败：{_validation_message(exc)}"
            except ServiceException as exc:
                failure_num += 1
                failure_msg += f"<br/>{failure_num}、账号 {user_name} 导入失败：{exc.message}"
            except Exception as exc:  # noqa: BLE001
                failure_num += 1
                failure_msg += f"<br/>{failure_num}、账号 {user_name} 导入失败：{exc}"

        if failure_num > 0:
            raise ServiceException(f"很抱歉，导入失败！共 {failure_num} 条数据格式不正确，错误如下：{failure_msg}")
        return f"恭喜您，数据已全部导入成功！共 {success_num} 条，数据如下：{success_msg}"
