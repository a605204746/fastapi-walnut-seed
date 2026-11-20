"""字典类型与字典数据接口。

对外路径为 ``/system/dict/type/**`` 与 ``/system/dict/data/**``
（本文件路由前缀为 ``/dict/type`` 与 ``/dict/data``，由主线装配 ``/system`` 前缀）。
"""

from typing import Annotated

from fastapi import APIRouter, Depends, Request
from redis.asyncio.client import Redis
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.v1.module_system.dict.schema import (
    DictDataCreateSchema,
    DictDataOutSchema,
    DictDataQueryParam,
    DictDataUpdateSchema,
    DictTypeCreateSchema,
    DictTypeOutSchema,
    DictTypeQueryParam,
    DictTypeUpdateSchema,
)
from app.api.v1.module_system.dict.service import DictDataService, DictTypeService
from app.common.enums import BusinessType, HttpStatus
from app.common.response import ErrorResponse, SuccessResponse
from app.core.base_schema import AuthSchema
from app.core.dependencies import AuthPermission, db_getter, get_current_user
from app.core.exceptions import ServiceException
from app.core.redis_crud import RedisUtils
from app.core.router_class import OperationLogRoute, log
from app.utils.excel_util import ExcelUtil

DictTypeRouter = APIRouter(route_class=OperationLogRoute, prefix="/dict/type", tags=["字典类型"])
DictDataRouter = APIRouter(route_class=OperationLogRoute, prefix="/dict/data", tags=["字典数据"])

DbSession = Annotated[AsyncSession, Depends(db_getter)]
LoginUser = Annotated[AuthSchema, Depends(get_current_user)]

# sys_yes_no 字典值→标签（导出时使用）
_YES_NO_LABEL = {"Y": "是", "N": "否"}

# 导出列（顺序即列序；键为驼峰出参键名）
_DICT_TYPE_EXPORT_HEADERS = {"id": "字典主键", "dictName": "字典名称", "dictType": "字典类型", "remark": "备注", "createTime": "创建时间"}
_DICT_DATA_EXPORT_HEADERS = {
    "id": "字典编码",
    "dictSort": "字典排序",
    "dictLabel": "字典标签",
    "dictValue": "字典键值",
    "dictType": "字典类型",
    "isDefault": "是否默认",
    "remark": "备注",
    "createTime": "创建时间",
}


def _get_redis(request: Request) -> Redis | None:
    """安全获取 Redis 连接；Redis 不可用时返回 None（降级为直接查库）。"""
    return getattr(request.app.state, "redis", None)


def _parse_ids(ids: str) -> list[int]:
    """解析逗号分隔的 ID 串。"""
    try:
        return [int(part) for part in ids.split(",") if part.strip()]
    except ValueError:
        raise ServiceException("参数格式错误", code=HttpStatus.BAD_REQUEST)


# ==================== 字典类型 ====================
@DictTypeRouter.get("/list", summary="查询字典类型列表")
async def list_dict_type(
    param: Annotated[DictTypeQueryParam, Depends()],
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:dict:list"]))],
) -> SuccessResponse:
    page = await DictTypeService(auth, db).select_page_dict_type_list(param)
    page["rows"] = [DictTypeOutSchema.model_validate(row).model_dump(by_alias=True, mode="json") for row in page["rows"]]
    return SuccessResponse(data=page)


@DictTypeRouter.post("/export", summary="导出字典类型列表")
@log(title="字典类型", business_type=BusinessType.EXPORT)
async def export_dict_type(
    param: Annotated[DictTypeQueryParam, Depends()],
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:dict:export"]))],
):
    rows = await DictTypeService(auth, db).select_dict_type_list(param)
    data = [DictTypeOutSchema.model_validate(row).model_dump(by_alias=True, mode="json") for row in rows]
    return ExcelUtil.export_excel_response(data, _DICT_TYPE_EXPORT_HEADERS, "字典类型")


@DictTypeRouter.get("/optionselect", summary="获取字典选择框列表")
async def optionselect_dict_type(db: DbSession, auth: LoginUser) -> SuccessResponse:
    rows = await DictTypeService(auth, db).select_dict_type_all()
    data = [DictTypeOutSchema.model_validate(row).model_dump(by_alias=True, mode="json") for row in rows]
    return SuccessResponse(data=data)


@DictTypeRouter.delete("/refreshCache", summary="刷新字典缓存")
@log(title="字典类型", business_type=BusinessType.CLEAN)
async def refresh_dict_cache(
    request: Request,
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:dict:remove"]))],
) -> SuccessResponse:
    redis = _get_redis(request)
    service = DictTypeService(auth, db)
    if redis is None:
        await service.reset_dict_cache(None)
        return SuccessResponse()
    # 未抢到锁时直接执行（清缓存操作可重入）
    ru = RedisUtils(redis)
    acquired, lock_value = await ru.lock("lock:dict:refreshCache")
    try:
        await service.reset_dict_cache(redis)
    finally:
        if acquired:
            await ru.unlock("lock:dict:refreshCache", lock_value)
    return SuccessResponse()


@DictTypeRouter.get("/{dict_id}", summary="查询字典类型详细")
async def get_dict_type(
    dict_id: int,
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:dict:query"]))],
) -> SuccessResponse:
    instance = await DictTypeService(auth, db).select_dict_type_by_id(dict_id)
    data = DictTypeOutSchema.model_validate(instance).model_dump(by_alias=True, mode="json") if instance else None
    return SuccessResponse(data=data)


@DictTypeRouter.post("", summary="新增字典类型")
@log(title="字典类型", business_type=BusinessType.INSERT)
async def add_dict_type(
    req: DictTypeCreateSchema,
    request: Request,
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:dict:add"]))],
):
    service = DictTypeService(auth, db)
    if not await service.check_dict_type_unique(req):
        return ErrorResponse(msg=f"新增字典'{req.dict_name or ''}'失败，字典类型已存在")
    await service.insert_dict_type(req, _get_redis(request))
    return SuccessResponse()


@DictTypeRouter.put("", summary="修改字典类型")
@log(title="字典类型", business_type=BusinessType.UPDATE)
async def update_dict_type(
    req: DictTypeUpdateSchema,
    request: Request,
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:dict:edit"]))],
):
    service = DictTypeService(auth, db)
    if not await service.check_dict_type_unique(req):
        return ErrorResponse(msg=f"修改字典'{req.dict_name or ''}'失败，字典类型已存在")
    await service.update_dict_type(req, _get_redis(request))
    return SuccessResponse()


@DictTypeRouter.delete("/{dict_ids}", summary="删除字典类型")
@log(title="字典类型", business_type=BusinessType.DELETE)
async def delete_dict_type(
    dict_ids: str,
    request: Request,
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:dict:remove"]))],
) -> SuccessResponse:
    await DictTypeService(auth, db).delete_dict_type_by_ids(_parse_ids(dict_ids), _get_redis(request))
    return SuccessResponse()


# ==================== 字典数据 ====================
@DictDataRouter.get("/list", summary="查询字典数据列表")
async def list_dict_data(
    param: Annotated[DictDataQueryParam, Depends()],
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:dict:list"]))],
) -> SuccessResponse:
    page = await DictDataService(auth, db).select_page_dict_data_list(param)
    page["rows"] = [DictDataOutSchema.model_validate(row).model_dump(by_alias=True, mode="json") for row in page["rows"]]
    return SuccessResponse(data=page)


@DictDataRouter.post("/export", summary="导出字典数据列表")
@log(title="字典数据", business_type=BusinessType.EXPORT)
async def export_dict_data(
    param: Annotated[DictDataQueryParam, Depends()],
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:dict:export"]))],
):
    rows = await DictDataService(auth, db).select_dict_data_list(param)
    data = []
    for row in rows:
        item = DictDataOutSchema.model_validate(row).model_dump(by_alias=True, mode="json")
        # 是否默认按 sys_yes_no 字典转换为标签
        is_default = item.get("isDefault") or ""
        item["isDefault"] = _YES_NO_LABEL.get(is_default, is_default)
        data.append(item)
    return ExcelUtil.export_excel_response(data, _DICT_DATA_EXPORT_HEADERS, "字典数据")


@DictDataRouter.get("/type/{dict_type}", summary="根据字典类型查询字典数据信息")
async def get_dict_data_by_type(
    dict_type: str,
    request: Request,
    db: DbSession,
    auth: LoginUser,
) -> SuccessResponse:
    data = await DictTypeService(auth, db).select_dict_data_by_type(dict_type, _get_redis(request))
    return SuccessResponse(data=data or [])


@DictDataRouter.get("/{dict_code}", summary="查询字典数据详细")
async def get_dict_data(
    dict_code: int,
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:dict:query"]))],
) -> SuccessResponse:
    instance = await DictDataService(auth, db).select_dict_data_by_id(dict_code)
    data = DictDataOutSchema.model_validate(instance).model_dump(by_alias=True, mode="json") if instance else None
    return SuccessResponse(data=data)


@DictDataRouter.post("", summary="新增字典数据")
@log(title="字典数据", business_type=BusinessType.INSERT)
async def add_dict_data(
    req: DictDataCreateSchema,
    request: Request,
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:dict:add"]))],
):
    service = DictDataService(auth, db)
    if not await service.check_dict_data_unique(req):
        return ErrorResponse(msg=f"新增字典数据'{req.dict_value}'失败，字典键值已存在")
    await service.insert_dict_data(req, _get_redis(request))
    return SuccessResponse()


@DictDataRouter.put("", summary="修改保存字典数据")
@log(title="字典数据", business_type=BusinessType.UPDATE)
async def update_dict_data(
    req: DictDataUpdateSchema,
    request: Request,
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:dict:edit"]))],
):
    service = DictDataService(auth, db)
    if not await service.check_dict_data_unique(req):
        return ErrorResponse(msg=f"修改字典数据'{req.dict_value}'失败，字典键值已存在")
    await service.update_dict_data(req, _get_redis(request))
    return SuccessResponse()


@DictDataRouter.delete("/{dict_codes}", summary="删除字典数据")
@log(title="字典数据", business_type=BusinessType.DELETE)
async def delete_dict_data(
    dict_codes: str,
    request: Request,
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:dict:remove"]))],
) -> SuccessResponse:
    await DictDataService(auth, db).delete_dict_data_by_ids(_parse_ids(dict_codes), _get_redis(request))
    return SuccessResponse()
