"""操作日志与登录日志监控接口。

两个 Router 由主线挂到应用根（不带 /system 前缀），对外路径为
``/monitor/operlog/**`` 与 ``/monitor/logininfor/**``。
"""

import asyncio
import time
from typing import Annotated, Any

from fastapi import APIRouter, Depends, Request
from redis.asyncio.client import Redis
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.v1.module_system.log.schema import (
    LOGININFOR_EXPORT_HEADERS,
    OPER_LOG_EXPORT_HEADERS,
    LogininforFilterSchema,
    LogininforQueryParam,
    OperLogFilterSchema,
    OperLogQueryParam,
)
from app.api.v1.module_system.log.service import LogininforService, OperLogService
from app.common.enums import BusinessType, CacheNames, HttpStatus
from app.common.response import ErrorResponse, SuccessResponse
from app.core.base_schema import AuthSchema
from app.core.dependencies import AuthPermission, db_getter, get_current_user, redis_getter
from app.core.exceptions import ServiceException
from app.core.idempotent import RepeatSubmit
from app.core.redis_crud import RedisUtils
from app.core.router_class import OperationLogRoute, log
from app.utils.excel_util import ExcelUtil
from app.utils.string_util import str2list

OperLogRouter = APIRouter(route_class=OperationLogRoute, prefix="/monitor/operlog", tags=["操作日志记录"])
LogininforRouter = APIRouter(route_class=OperationLogRoute, prefix="/monitor/logininfor", tags=["系统访问记录"])

AuthDep = Annotated[AuthSchema, Depends(get_current_user)]
DbDep = Annotated[AsyncSession, Depends(db_getter)]
RedisDep = Annotated[Redis, Depends(redis_getter)]


# ==================== 公共工具 ====================


async def _run_with_lock(redis: Redis, lock_key: str, action) -> None:
    """获取分布式锁并执行业务（等待3秒、持有30秒）。"""
    redis_utils = RedisUtils(redis)
    deadline = time.monotonic() + 3
    acquired, lock_value = False, ""
    while True:
        acquired, lock_value = await redis_utils.lock(lock_key, expire=30)
        if acquired:
            break
        if time.monotonic() >= deadline:
            raise ServiceException(f"分布式锁 [{lock_key}] 获取失败，请稍后重试")
        await asyncio.sleep(0.1)
    try:
        await action()
    finally:
        await redis_utils.unlock(lock_key, lock_value)


async def _collect_filter_params(request: Request) -> dict[str, Any]:
    """合并查询参数与表单参数。"""
    merged: dict[str, Any] = dict(request.query_params)
    content_type = request.headers.get("Content-Type", "")
    if content_type.startswith("application/x-www-form-urlencoded"):
        form = await request.form()
        for key in form:
            values = form.getlist(key)
            merged[key] = values if len(values) > 1 else values[0]
    return {key: value for key, value in merged.items() if value not in (None, "")}


def _parse_ids(raw_ids: str) -> list[int]:
    """解析路径中的逗号分隔ID串。"""
    try:
        return [int(item) for item in str2list(raw_ids)]
    except ValueError:
        raise ServiceException("日志ID格式有误", code=HttpStatus.BAD_REQUEST)


# ==================== 操作日志 ====================


@OperLogRouter.get("/list", summary="操作日志列表", dependencies=[Depends(AuthPermission(permissions=["monitor:operlog:list"]))])
async def list_oper_log(auth: AuthDep, db: DbDep, query: Annotated[OperLogQueryParam, Depends()]) -> SuccessResponse:
    """获取操作日志记录列表。"""
    result = await OperLogService(auth, db).page_list(query)
    result["rows"] = [row.model_dump(by_alias=True, mode="json") for row in result["rows"]]
    return SuccessResponse(data=result)


@OperLogRouter.post("/export", summary="导出操作日志", dependencies=[Depends(AuthPermission(permissions=["monitor:operlog:export"]))])
@log(title="操作日志", business_type=BusinessType.EXPORT)
async def export_oper_log(request: Request, auth: AuthDep, db: DbDep):
    """导出操作日志记录列表。"""
    query = OperLogFilterSchema.model_validate(await _collect_filter_params(request))
    rows = await OperLogService(auth, db).export_rows(query)
    return ExcelUtil.export_excel_response(rows, OPER_LOG_EXPORT_HEADERS, "操作日志")


@OperLogRouter.delete("/clean", summary="清空操作日志", dependencies=[Depends(AuthPermission(permissions=["monitor:operlog:remove"]))])
@log(title="操作日志", business_type=BusinessType.CLEAN)
async def clean_oper_log(auth: AuthDep, db: DbDep, redis: RedisDep) -> SuccessResponse:
    """清理操作日志记录（分布式锁保护）。"""
    await _run_with_lock(redis, "lock:operlog:clean", OperLogService(auth, db).clean)
    return SuccessResponse()


@OperLogRouter.delete("/{oper_ids}", summary="删除操作日志", dependencies=[Depends(AuthPermission(permissions=["monitor:operlog:remove"]))])
@log(title="操作日志", business_type=BusinessType.DELETE)
async def delete_oper_log(oper_ids: str, auth: AuthDep, db: DbDep):
    """批量删除操作日志记录。"""
    deleted = await OperLogService(auth, db).delete_by_ids(_parse_ids(oper_ids))
    return SuccessResponse() if deleted > 0 else ErrorResponse(msg="操作失败")


# ==================== 系统访问记录（登录日志） ====================


@LogininforRouter.get("/list", summary="登录日志列表", dependencies=[Depends(AuthPermission(permissions=["monitor:logininfor:list"]))])
async def list_logininfor(auth: AuthDep, db: DbDep, query: Annotated[LogininforQueryParam, Depends()]) -> SuccessResponse:
    """获取系统访问记录列表。"""
    result = await LogininforService(auth, db).page_list(query)
    result["rows"] = [row.model_dump(by_alias=True, mode="json") for row in result["rows"]]
    return SuccessResponse(data=result)


@LogininforRouter.post("/export", summary="导出登录日志", dependencies=[Depends(AuthPermission(permissions=["monitor:logininfor:export"]))])
@log(title="登录日志", business_type=BusinessType.EXPORT)
async def export_logininfor(request: Request, auth: AuthDep, db: DbDep):
    """导出系统访问记录列表。"""
    query = LogininforFilterSchema.model_validate(await _collect_filter_params(request))
    rows = await LogininforService(auth, db).export_rows(query)
    return ExcelUtil.export_excel_response(rows, LOGININFOR_EXPORT_HEADERS, "登录日志")


@LogininforRouter.delete("/clean", summary="清空登录日志", dependencies=[Depends(AuthPermission(permissions=["monitor:logininfor:remove"]))])
@log(title="登录日志", business_type=BusinessType.CLEAN)
async def clean_logininfor(auth: AuthDep, db: DbDep, redis: RedisDep) -> SuccessResponse:
    """清理系统访问记录（分布式锁保护）。"""
    await _run_with_lock(redis, "lock:logininfor:clean", LogininforService(auth, db).clean)
    return SuccessResponse()


@LogininforRouter.delete("/{info_ids}", summary="删除登录日志", dependencies=[Depends(AuthPermission(permissions=["monitor:logininfor:remove"]))])
@log(title="登录日志", business_type=BusinessType.DELETE)
async def delete_logininfor(info_ids: str, auth: AuthDep, db: DbDep):
    """批量删除登录日志。"""
    deleted = await LogininforService(auth, db).delete_by_ids(_parse_ids(info_ids))
    return SuccessResponse() if deleted > 0 else ErrorResponse(msg="操作失败")


@LogininforRouter.get(
    "/unlock/{user_name}",
    summary="账户解锁",
    dependencies=[Depends(AuthPermission(permissions=["monitor:logininfor:unlock"])), Depends(RepeatSubmit())],
)
@log(title="账户解锁", business_type=BusinessType.OTHER)
async def unlock_user(user_name: str, redis: RedisDep) -> SuccessResponse:
    """账户解锁：删除该用户所有 用户名+IP 组合的密码错误计数键。"""
    redis_utils = RedisUtils(redis)
    await redis_utils.delete_by_pattern(f"{CacheNames.PWD_ERR_CNT_KEY}{user_name}:*")
    return SuccessResponse()
