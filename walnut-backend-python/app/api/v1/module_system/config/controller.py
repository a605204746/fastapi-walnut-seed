"""参数设置接口。

对外路径为 ``/system/config/**``（本文件路由前缀为 ``/config``，由主线装配 ``/system`` 前缀）。
"""

from typing import Annotated

from fastapi import APIRouter, Depends, Request
from redis.asyncio.client import Redis
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.v1.module_system.config.schema import ConfigCreateSchema, ConfigOutSchema, ConfigQueryParam, ConfigUpdateByKeySchema, ConfigUpdateSchema
from app.api.v1.module_system.config.service import ConfigService
from app.common.enums import BusinessType, HttpStatus
from app.common.response import ErrorResponse, SuccessResponse
from app.core.base_schema import AuthSchema
from app.core.dependencies import AuthPermission, db_getter, get_current_user
from app.core.exceptions import ServiceException
from app.core.router_class import OperationLogRoute, log
from app.utils.excel_util import ExcelUtil

ConfigRouter = APIRouter(route_class=OperationLogRoute, prefix="/config", tags=["参数设置"])

DbSession = Annotated[AsyncSession, Depends(db_getter)]
LoginUser = Annotated[AuthSchema, Depends(get_current_user)]

# sys_yes_no 字典值→标签（导出时使用）
_YES_NO_LABEL = {"Y": "是", "N": "否"}

# 导出列（顺序即列序；键为驼峰出参键名）
_CONFIG_EXPORT_HEADERS = {"id": "参数主键", "configName": "参数名称", "configKey": "参数键名", "configValue": "参数键值", "configType": "系统内置", "remark": "备注", "createTime": "创建时间"}


def _get_redis(request: Request) -> Redis | None:
    """安全获取 Redis 连接；Redis 不可用时返回 None（降级为直接查库）。"""
    return getattr(request.app.state, "redis", None)


def _parse_ids(ids: str) -> list[int]:
    """解析逗号分隔的 ID 串。"""
    try:
        return [int(part) for part in ids.split(",") if part.strip()]
    except ValueError:
        raise ServiceException("参数格式错误", code=HttpStatus.BAD_REQUEST)


@ConfigRouter.get("/list", summary="获取参数配置列表")
async def list_config(
    param: Annotated[ConfigQueryParam, Depends()],
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:config:list"]))],
) -> SuccessResponse:
    page = await ConfigService(auth, db).select_page_config_list(param)
    page["rows"] = [ConfigOutSchema.model_validate(row).model_dump(by_alias=True, mode="json") for row in page["rows"]]
    return SuccessResponse(data=page)


@ConfigRouter.post("/export", summary="导出参数配置列表")
@log(title="参数管理", business_type=BusinessType.EXPORT)
async def export_config(
    param: Annotated[ConfigQueryParam, Depends()],
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:config:export"]))],
):
    rows = await ConfigService(auth, db).select_config_list(param)
    data = []
    for row in rows:
        item = ConfigOutSchema.model_validate(row).model_dump(by_alias=True, mode="json")
        # 系统内置按 sys_yes_no 字典转换为标签
        config_type = item.get("configType") or ""
        item["configType"] = _YES_NO_LABEL.get(config_type, config_type)
        data.append(item)
    return ExcelUtil.export_excel_response(data, _CONFIG_EXPORT_HEADERS, "参数数据")


@ConfigRouter.get("/configKey/{config_key}", summary="根据参数键名查询参数值")
async def get_config_by_key(
    config_key: str,
    request: Request,
    db: DbSession,
    auth: LoginUser,
) -> SuccessResponse:
    value = await ConfigService(auth, db).select_config_by_key(config_key, _get_redis(request))
    return SuccessResponse(data=value, msg="操作成功")


@ConfigRouter.put("/updateByKey", summary="根据参数键名修改参数配置")
@log(title="参数管理", business_type=BusinessType.UPDATE)
async def update_config_by_key(
    req: ConfigUpdateByKeySchema,
    request: Request,
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:config:edit"]))],
) -> SuccessResponse:
    await ConfigService(auth, db).update_config(req, _get_redis(request))
    return SuccessResponse()


@ConfigRouter.delete("/refreshCache", summary="刷新参数缓存")
@log(title="参数管理", business_type=BusinessType.CLEAN)
async def refresh_config_cache(
    request: Request,
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:config:remove"]))],
) -> SuccessResponse:
    await ConfigService(auth, db).reset_config_cache(_get_redis(request))
    return SuccessResponse()


@ConfigRouter.get("/{config_id}", summary="根据参数编号获取详细信息")
async def get_config(
    config_id: int,
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:config:query"]))],
) -> SuccessResponse:
    instance = await ConfigService(auth, db).select_config_by_id(config_id)
    data = ConfigOutSchema.model_validate(instance).model_dump(by_alias=True, mode="json") if instance else None
    return SuccessResponse(data=data)


@ConfigRouter.post("", summary="新增参数配置")
@log(title="参数管理", business_type=BusinessType.INSERT)
async def add_config(
    req: ConfigCreateSchema,
    request: Request,
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:config:add"]))],
):
    service = ConfigService(auth, db)
    if not await service.check_config_key_unique(req):
        return ErrorResponse(msg=f"新增参数'{req.config_name or ''}'失败，参数键名已存在")
    await service.insert_config(req, _get_redis(request))
    return SuccessResponse()


@ConfigRouter.put("", summary="修改参数配置")
@log(title="参数管理", business_type=BusinessType.UPDATE)
async def update_config(
    req: ConfigUpdateSchema,
    request: Request,
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:config:edit"]))],
):
    service = ConfigService(auth, db)
    if not await service.check_config_key_unique(req):
        return ErrorResponse(msg=f"修改参数'{req.config_name or ''}'失败，参数键名已存在")
    await service.update_config(req, _get_redis(request))
    return SuccessResponse()


@ConfigRouter.delete("/{config_ids}", summary="删除参数配置")
@log(title="参数管理", business_type=BusinessType.DELETE)
async def delete_config(
    config_ids: str,
    request: Request,
    db: DbSession,
    auth: Annotated[AuthSchema, Depends(AuthPermission(permissions=["system:config:remove"]))],
) -> SuccessResponse:
    await ConfigService(auth, db).delete_config_by_ids(_parse_ids(config_ids), _get_redis(request))
    return SuccessResponse()
