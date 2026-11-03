"""岗位管理（URL 前缀 /system/post）。"""

from typing import Annotated

from fastapi import APIRouter, Depends, Query, Request
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.v1.module_system.post.schema import DeptTreeQueryParam, PostCreateSchema, PostQueryParam, PostUpdateSchema
from app.api.v1.module_system.post.service import PostService
from app.common.constant import SystemConstants
from app.common.enums import BusinessType, HttpStatus
from app.common.response import EnvelopeResponse, ErrorResponse, SuccessResponse
from app.core.base_schema import AuthSchema
from app.core.dependencies import AuthPermission, db_getter
from app.core.exceptions import ServiceException
from app.core.idempotent import RepeatSubmit
from app.core.router_class import OperationLogRoute, log
from app.utils.excel_util import ExcelUtil
from app.utils.string_util import str2list

PostRouter = APIRouter(route_class=OperationLogRoute, prefix="/post", tags=["岗位管理"])

DbSession = Annotated[AsyncSession, Depends(db_getter)]

# 导出表头
# ⚠️ 键必须是**驼峰**：export_post 传入的 rows 来自 select_list()，
# 即 PostOutSchema.model_dump(by_alias=True) 的输出，键已被 to_camel 转成驼峰。
# build_workbook 用 item.get(field) 取值，键名对不上就取到 None → 单元格为空。
# 此前这里写的是下划线键，导致 9 列里有 6 列（部门id/岗位编码/岗位名称/
# 类别编码/岗位排序/创建时间）导出恒为空，只有 id/status/remark 三列有值
# （因为这三个词驼峰化后拼写不变）。
# 同类正确写法见 config/controller.py 的 _CONFIG_EXPORT_HEADERS、
# dict/controller.py 的 _DICT_TYPE_EXPORT_HEADERS。
# 注：user/role 两个模块的导出行是 service 层手工构造的**下划线**键 dict，
# 与它们各自的下划线表头是匹配的，不受影响、不要一起改。
_EXPORT_HEADERS = {
    "id": "岗位序号",
    "deptId": "部门id",
    "postCode": "岗位编码",
    "postName": "岗位名称",
    "postCategory": "类别编码",
    "postSort": "岗位排序",
    "status": "状态",
    "remark": "备注",
    "createTime": "创建时间",
}


def _parse_ids(ids: str) -> list[int]:
    """解析路径中的ID串。"""
    try:
        id_list = [int(item) for item in str2list(ids)]
    except ValueError:
        raise ServiceException("岗位ID格式有误", code=HttpStatus.BAD_REQUEST)
    if not id_list:
        raise ServiceException("岗位ID不能为空", code=HttpStatus.BAD_REQUEST)
    return id_list


@PostRouter.get("/list", summary="获取岗位列表")
async def list_post(
    params: Annotated[PostQueryParam, Depends()],
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:post:list"]))],
) -> SuccessResponse:
    return SuccessResponse(data=await PostService(auth, db).page_list(params))


@PostRouter.post("/export", summary="导出岗位列表")
@log(title="岗位管理", business_type=BusinessType.EXPORT)
async def export_post(
    request: Request,
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:post:export"]))],
):
    form = await request.form()
    data = {key: value for key, value in form.items() if value not in (None, "")}
    data.pop("pageSize", None)  # 导出时不分页，查询全部
    params = PostQueryParam.model_validate(data)
    rows = await PostService(auth, db).select_list(params)
    return ExcelUtil.export_excel_response(rows, _EXPORT_HEADERS, "岗位数据")


@PostRouter.get("/optionselect", summary="获取岗位选择框列表")
async def optionselect_post(
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:post:query"]))],
    post_ids: Annotated[str | None, Query(alias="postIds", description="岗位ID串")] = None,
    dept_id: Annotated[int | None, Query(alias="deptId", description="部门id")] = None,
) -> SuccessResponse:
    ids = [int(item) for item in str2list(post_ids)] if post_ids else None
    return SuccessResponse(data=await PostService(auth, db).option_select(ids, dept_id))


@PostRouter.get("/deptTree", summary="获取部门树列表")
async def dept_tree_post(
    params: Annotated[DeptTreeQueryParam, Depends()],
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:post:list"]))],
) -> SuccessResponse:
    return SuccessResponse(data=await PostService(auth, db).dept_tree_list(params))


@PostRouter.get("/{post_id}", summary="根据岗位编号获取详细信息")
async def get_post(
    post_id: int,
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:post:query"]))],
) -> SuccessResponse:
    return SuccessResponse(data=await PostService(auth, db).get_by_id(post_id))


@PostRouter.post("", summary="新增岗位", dependencies=[Depends(RepeatSubmit())])
@log(title="岗位管理", business_type=BusinessType.INSERT)
async def add_post(
    req: PostCreateSchema,
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:post:add"]))],
) -> EnvelopeResponse:
    service = PostService(auth, db)
    if not await service.check_post_name_unique(req):
        raise ServiceException(f"新增岗位'{req.post_name}'失败，岗位名称已存在")
    if not await service.check_post_code_unique(req):
        raise ServiceException(f"新增岗位'{req.post_name}'失败，岗位编码已存在")
    return SuccessResponse() if await service.insert_post(req) else ErrorResponse()


@PostRouter.put("", summary="修改岗位", dependencies=[Depends(RepeatSubmit())])
@log(title="岗位管理", business_type=BusinessType.UPDATE)
async def update_post(
    req: PostUpdateSchema,
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:post:edit"]))],
) -> EnvelopeResponse:
    service = PostService(auth, db)
    if not await service.check_post_name_unique(req):
        raise ServiceException(f"修改岗位'{req.post_name}'失败，岗位名称已存在")
    if not await service.check_post_code_unique(req):
        raise ServiceException(f"修改岗位'{req.post_name}'失败，岗位编码已存在")
    assert req.id is not None  # schema 校验（validate_default）保证非空
    if req.status == SystemConstants.DISABLE and await service.count_user_post_by_id(req.id) > 0:
        raise ServiceException("该岗位下存在已分配用户，不能禁用!")
    return SuccessResponse() if await service.update_post(req) else ErrorResponse()


@PostRouter.delete("/{post_ids}", summary="删除岗位")
@log(title="岗位管理", business_type=BusinessType.DELETE)
async def delete_post(
    post_ids: str,
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:post:remove"]))],
) -> EnvelopeResponse:
    return SuccessResponse() if await PostService(auth, db).delete_post_by_ids(_parse_ids(post_ids)) > 0 else ErrorResponse()
