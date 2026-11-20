"""角色管理端点。

URL 前缀 /system/role，本文件 prefix="/role"。包含：list/export/{roleId}/增删改/changeStatus/dataScope/
optionselect/authUser(allocatedList/unallocatedList/cancel/cancelAll/selectAll)/deptTree 全部端点。
"""

from __future__ import annotations

from typing import Annotated

from fastapi import APIRouter, Depends, Query, Request
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.v1.module_system.role.schema import (
    AuthUserCancelSchema,
    AuthUserQueryParam,
    RoleCreateSchema,
    RoleDataScopeSchema,
    RoleQueryParam,
    RoleStatusSchema,
    RoleUpdateSchema,
)
from app.api.v1.module_system.role.service import RoleService
from app.common.enums import BusinessType, HttpStatus
from app.common.response import EnvelopeResponse, ErrorResponse, SuccessResponse
from app.core.base_schema import AuthSchema
from app.core.dependencies import AuthPermission, db_getter, get_current_user
from app.core.exceptions import ServiceException
from app.core.idempotent import RepeatSubmit
from app.core.router_class import OperationLogRoute, log
from app.utils.excel_util import ExcelUtil
from app.utils.string_util import str2list

RoleRouter = APIRouter(route_class=OperationLogRoute, prefix="/role", tags=["角色管理"])

DbSession = Annotated[AsyncSession, Depends(db_getter)]
AuthUser = Annotated[AuthSchema, Depends(get_current_user)]

# 数据范围字典（导出转换）
_DATA_SCOPE_LABEL = {
    "1": "全部数据权限",
    "2": "自定义数据权限",
    "3": "本部门数据权限",
    "4": "本部门及以下数据权限",
    "5": "仅本人数据权限",
    "6": "部门及以下或本人数据权限",
}
# sys_normal_disable 字典（角色状态导出转换）
_STATUS_LABEL = {"0": "正常", "1": "停用"}

# 导出列（顺序即列序）
_ROLE_EXPORT_HEADERS = {
    "id": "角色序号",
    "role_name": "角色名称",
    "role_key": "角色权限",
    "role_sort": "角色排序",
    "data_scope": "数据范围",
    "menu_check_strictly": "菜单树选择项是否关联显示",
    "dept_check_strictly": "部门树选择项是否关联显示",
    "status": "角色状态",
    "remark": "备注",
    "create_time": "创建时间",
}


def _parse_ids(ids: str) -> list[int]:
    """解析逗号分隔的 ID 串。"""
    try:
        id_list = [int(item) for item in str2list(ids)]
    except ValueError:
        raise ServiceException("角色ID格式有误", code=HttpStatus.BAD_REQUEST)
    if not id_list:
        raise ServiceException("角色ID不能为空", code=HttpStatus.BAD_REQUEST)
    return id_list


def _bool_or_none(value: int | None) -> bool | None:
    """实体列 1/0 转布尔（导出 menuCheckStrictly/deptCheckStrictly）。"""
    if value is None:
        return None
    return bool(value)


# ==================== 查询 ====================
@RoleRouter.get("/list", summary="获取角色信息列表", dependencies=[Depends(AuthPermission(permissions=["system:role:list"]))])
async def list_role(param: Annotated[RoleQueryParam, Depends()], auth: AuthUser, db: DbSession) -> SuccessResponse:
    return SuccessResponse(data=await RoleService(auth, db).page_list(param))


@RoleRouter.get("/optionselect", summary="获取角色选择框列表", dependencies=[Depends(AuthPermission(permissions=["system:role:query"]))])
async def optionselect_role(
    auth: AuthUser,
    db: DbSession,
    role_ids: Annotated[str | None, Query(alias="roleIds", description="角色ID串")] = None,
) -> SuccessResponse:
    try:
        ids = [int(item) for item in str2list(role_ids)] if role_ids else None
    except ValueError:
        raise ServiceException("角色ID格式有误", code=HttpStatus.BAD_REQUEST)
    return SuccessResponse(data=await RoleService(auth, db).option_select(ids))


@RoleRouter.get("/authUser/allocatedList", summary="查询已分配用户角色列表", dependencies=[Depends(AuthPermission(permissions=["system:role:list"]))])
async def allocated_list(param: Annotated[AuthUserQueryParam, Depends()], auth: AuthUser, db: DbSession) -> SuccessResponse:
    return SuccessResponse(data=await RoleService(auth, db).allocated_list(param))


@RoleRouter.get("/authUser/unallocatedList", summary="查询未分配用户角色列表", dependencies=[Depends(AuthPermission(permissions=["system:role:list"]))])
async def unallocated_list(param: Annotated[AuthUserQueryParam, Depends()], auth: AuthUser, db: DbSession) -> SuccessResponse:
    return SuccessResponse(data=await RoleService(auth, db).unallocated_list(param))


@RoleRouter.get("/deptTree/{role_id}", summary="获取指定角色部门树列表", dependencies=[Depends(AuthPermission(permissions=["system:role:list"]))])
async def role_dept_tree(role_id: int, auth: AuthUser, db: DbSession) -> SuccessResponse:
    return SuccessResponse(data=await RoleService(auth, db).role_dept_tree(role_id))


@RoleRouter.get("/{role_id}", summary="根据角色编号获取详细信息", dependencies=[Depends(AuthPermission(permissions=["system:role:query"]))])
async def get_role(role_id: int, auth: AuthUser, db: DbSession) -> SuccessResponse:
    return SuccessResponse(data=await RoleService(auth, db).get_by_id(role_id))


# ==================== 导出 ====================
@RoleRouter.post("/export", summary="导出角色信息列表", dependencies=[Depends(AuthPermission(permissions=["system:role:export"]))])
@log(title="角色管理", business_type=BusinessType.EXPORT)
async def export_role(request: Request, auth: AuthUser, db: DbSession):
    form = await request.form()
    data = {key: value for key, value in form.items() if value not in (None, "")}
    data.pop("pageSize", None)  # 导出查询全部
    data.pop("pageNum", None)
    params = RoleQueryParam.model_validate(data)
    roles = await RoleService(auth, db).select_list(params)
    rows = []
    for role in roles:
        rows.append(
            {
                "id": role.id,
                "role_name": role.role_name,
                "role_key": role.role_key,
                "role_sort": role.role_sort,
                "data_scope": _DATA_SCOPE_LABEL.get(role.data_scope or "", role.data_scope or ""),
                "menu_check_strictly": _bool_or_none(role.menu_check_strictly),
                "dept_check_strictly": _bool_or_none(role.dept_check_strictly),
                "status": _STATUS_LABEL.get(role.status, role.status or ""),
                "remark": role.remark,
                "create_time": role.create_time,
            }
        )
    return ExcelUtil.export_excel_response(rows, _ROLE_EXPORT_HEADERS, "角色数据")


# ==================== 写入 ====================
@RoleRouter.post("", summary="新增角色", dependencies=[Depends(AuthPermission(permissions=["system:role:add"])), Depends(RepeatSubmit())])
@log(title="角色管理", business_type=BusinessType.INSERT)
async def add_role(req: RoleCreateSchema, auth: AuthUser, db: DbSession) -> EnvelopeResponse:
    service = RoleService(auth, db)
    await service.check_role_allowed(None, req.role_key)
    if not await service.check_role_name_unique(req):
        raise ServiceException(f"新增角色'{req.role_name}'失败，角色名称已存在")
    if not await service.check_role_key_unique(req):
        raise ServiceException(f"新增角色'{req.role_name}'失败，角色权限已存在")
    return SuccessResponse() if await service.insert_role(req) else ErrorResponse()


@RoleRouter.put("", summary="修改保存角色", dependencies=[Depends(AuthPermission(permissions=["system:role:edit"])), Depends(RepeatSubmit())])
@log(title="角色管理", business_type=BusinessType.UPDATE)
async def update_role(req: RoleUpdateSchema, auth: AuthUser, db: DbSession) -> EnvelopeResponse:
    service = RoleService(auth, db)
    if req.id is None:
        raise ServiceException("角色ID不能为空")
    await service.check_role_allowed(req.id, req.role_key)
    await service.check_role_data_scope([req.id])
    if not await service.check_role_name_unique(req):
        raise ServiceException(f"修改角色'{req.role_name}'失败，角色名称已存在")
    if not await service.check_role_key_unique(req):
        raise ServiceException(f"修改角色'{req.role_name}'失败，角色权限已存在")
    return SuccessResponse() if await service.update_role(req) else ErrorResponse(msg=f"修改角色'{req.role_name}'失败，请联系管理员")


@RoleRouter.put("/dataScope", summary="修改保存数据权限", dependencies=[Depends(AuthPermission(permissions=["system:role:edit"])), Depends(RepeatSubmit())])
@log(title="角色管理", business_type=BusinessType.UPDATE)
async def data_scope(req: RoleDataScopeSchema, auth: AuthUser, db: DbSession) -> EnvelopeResponse:
    service = RoleService(auth, db)
    if req.id is None:
        raise ServiceException("角色ID不能为空")
    await service.check_role_allowed(req.id, None)
    await service.check_role_data_scope([req.id])
    return SuccessResponse() if await service.auth_data_scope(req) else ErrorResponse()


@RoleRouter.put("/changeStatus", summary="状态修改", dependencies=[Depends(AuthPermission(permissions=["system:role:edit"])), Depends(RepeatSubmit())])
@log(title="角色管理", business_type=BusinessType.UPDATE)
async def change_status(req: RoleStatusSchema, auth: AuthUser, db: DbSession) -> EnvelopeResponse:
    service = RoleService(auth, db)
    if req.id is None:
        raise ServiceException("角色ID不能为空")
    await service.check_role_allowed(req.id, None)
    await service.check_role_data_scope([req.id])
    return SuccessResponse() if await service.update_role_status(req.id, req.status) else ErrorResponse()


@RoleRouter.delete("/{role_ids}", summary="删除角色", dependencies=[Depends(AuthPermission(permissions=["system:role:remove"]))])
@log(title="角色管理", business_type=BusinessType.DELETE)
async def delete_role(role_ids: str, auth: AuthUser, db: DbSession) -> EnvelopeResponse:
    return SuccessResponse() if await RoleService(auth, db).delete_role_by_ids(_parse_ids(role_ids)) > 0 else ErrorResponse()


# ==================== authUser 授权 ====================
@RoleRouter.put("/authUser/cancel", summary="取消授权用户", dependencies=[Depends(AuthPermission(permissions=["system:role:edit"])), Depends(RepeatSubmit())])
@log(title="角色管理", business_type=BusinessType.GRANT)
async def cancel_auth_user(req: AuthUserCancelSchema, auth: AuthUser, db: DbSession) -> EnvelopeResponse:
    return SuccessResponse() if await RoleService(auth, db).cancel_auth_user(req) > 0 else ErrorResponse()


@RoleRouter.put("/authUser/cancelAll", summary="批量取消授权用户", dependencies=[Depends(AuthPermission(permissions=["system:role:edit"])), Depends(RepeatSubmit())])
@log(title="角色管理", business_type=BusinessType.GRANT)
async def cancel_auth_user_all(
    role_id: Annotated[int, Query(alias="roleId", description="角色ID")],
    user_ids: Annotated[str, Query(alias="userIds", description="用户ID串")],
    auth: AuthUser,
    db: DbSession,
) -> EnvelopeResponse:
    return SuccessResponse() if await RoleService(auth, db).cancel_auth_users(role_id, _parse_ids(user_ids)) > 0 else ErrorResponse()


@RoleRouter.put("/authUser/selectAll", summary="批量选择用户授权", dependencies=[Depends(AuthPermission(permissions=["system:role:edit"])), Depends(RepeatSubmit())])
@log(title="角色管理", business_type=BusinessType.GRANT)
async def select_auth_user_all(
    role_id: Annotated[int, Query(alias="roleId", description="角色ID")],
    user_ids: Annotated[str, Query(alias="userIds", description="用户ID串")],
    auth: AuthUser,
    db: DbSession,
) -> EnvelopeResponse:
    return SuccessResponse() if await RoleService(auth, db).select_auth_users(role_id, _parse_ids(user_ids)) > 0 else ErrorResponse()
