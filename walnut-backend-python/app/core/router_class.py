"""操作日志路由。

- ``@log(title, business_type, ...)`` 装饰器为端点标记日志元数据；
- ``OperationLogRoute`` 在写方法（POST/PUT/DELETE/PATCH）执行前后采集请求/响应，
  组装 ``OperLogEvent`` 并通过后台任务异步写出（消费者可在业务阶段注册到 sys_oper_log）。
- 截断限制：operUrl≤255，operParam/jsonResult/errorMsg≤3800。
"""

import json
import time
from collections.abc import Callable, Coroutine
from typing import Any

from fastapi import Request, Response
from fastapi.routing import APIRoute
from starlette.background import BackgroundTask

from app.common.constant import SystemConstants
from app.common.dataclasses import OperLogEvent
from app.common.enums import BusinessStatus, BusinessType, OperatorType
from app.config.setting import settings
from app.core.logger import logger
from app.utils.common_util import get_client_ip

_WRITE_METHODS = {"POST", "PUT", "DELETE", "PATCH"}

# 可插拔的操作日志消费者（业务阶段注册：写入 sys_oper_log）
_oper_log_consumer: Callable[[OperLogEvent], Coroutine[Any, Any, None]] | None = None


def set_oper_log_consumer(consumer: Callable[[OperLogEvent], Coroutine[Any, Any, None]] | None) -> None:
    global _oper_log_consumer
    _oper_log_consumer = consumer


def log(
    title: str = "",
    business_type: BusinessType = BusinessType.OTHER,
    operator_type: OperatorType = OperatorType.MANAGE,
    is_save_request_data: bool = True,
    is_save_response_data: bool = True,
    exclude_param_names: tuple[str, ...] = (),
):
    """操作日志装饰器。"""

    def decorator(func):
        func._log_meta = {
            "title": title,
            "business_type": business_type,
            "operator_type": operator_type,
            "is_save_request_data": is_save_request_data,
            "is_save_response_data": is_save_response_data,
            "exclude_param_names": set(exclude_param_names),
        }
        return func

    return decorator


async def _write_operation_log_async(log_data: dict) -> None:
    event = OperLogEvent(**log_data)
    if _oper_log_consumer is not None:
        try:
            await _oper_log_consumer(event)
        except Exception:
            logger.exception("操作日志写入失败: path={}", event.oper_url)
    else:
        logger.info("[操作日志] {} {} | type={} | status={} | cost={}ms", event.request_method, event.oper_url, event.business_type, event.status, event.cost_time)


def _scrub(obj: Any, excludes: set[str]) -> Any:
    if isinstance(obj, dict):
        return {k: _scrub(v, excludes) for k, v in obj.items() if k not in excludes and k not in SystemConstants.EXCLUDE_PROPERTIES}
    if isinstance(obj, list):
        return [_scrub(i, excludes) for i in obj]
    return obj


class OperationLogRoute(APIRoute):
    """操作日志路由（写方法自动记录请求/响应并后台异步写入）。"""

    def get_route_handler(self) -> Callable[[Request], Coroutine[Any, Any, Response]]:
        original_route_handler = super().get_route_handler()

        async def custom_route_handler(request: Request) -> Response:
            start = time.perf_counter()
            response: Response = await original_route_handler(request)

            # 响应发送前提交事务（db_getter 把会话挂在 request.state）。
            # yield 依赖的 teardown 在响应发送后才跑，只靠它 commit 会让
            # 「写后立即读」的下一个请求读到旧数据。teardown 的二次 commit
            # 因事务已关闭而为 no-op；异常路径仍由 begin() 上下文 rollback。
            session = getattr(request.state, "db_session", None)
            if session is not None:
                await session.commit()

            if request.method not in settings.OPERATION_RECORD_METHOD:
                return response
            route: APIRoute | None = request.scope.get("route", None)
            endpoint = request.scope.get("endpoint")
            meta = getattr(endpoint, "_log_meta", {}) if endpoint else {}
            excludes = meta.get("exclude_param_names", set())

            try:
                oper_param: dict[str, Any] = {}
                if meta.get("is_save_request_data", True):
                    content_type = request.headers.get("Content-Type", "")
                    if content_type.startswith(("multipart/form-data", "application/x-www-form-urlencoded")):
                        try:
                            form_data = await request.form()
                            oper_param["form"] = _scrub({k: v for k, v in form_data.items() if not hasattr(v, "read")}, excludes)
                        except Exception:
                            oper_param["form"] = {}
                    else:
                        payload = await request.body()
                        if payload:
                            try:
                                oper_param["body"] = _scrub(json.loads(payload.decode()), excludes)
                            except (json.JSONDecodeError, UnicodeDecodeError):
                                oper_param["body"] = payload.decode("utf-8", errors="ignore")
                    if request.path_params:
                        oper_param["path_params"] = dict(request.path_params)

                log_payload = json.dumps(oper_param, ensure_ascii=False)
                if len(log_payload) > 3800:
                    log_payload = "请求参数过长"

                json_result = ""
                if meta.get("is_save_response_data", True):
                    is_json = "application/json" in response.headers.get("Content-Type", "")
                    body_bytes = getattr(response, "body", b"") or b"{}"
                    json_result = bytes(body_bytes).decode("utf-8", "ignore") if is_json else "{}"
                    if len(json_result) > 3800:
                        json_result = "响应结果过长"

                auth = getattr(request.state, "auth", None)
                oper_name = getattr(getattr(auth, "user", None), "username", None) if auth else None
                status_value = BusinessStatus.SUCCESS if response.status_code < 400 else BusinessStatus.FAIL

                log_data: dict[str, Any] = {
                    "title": meta.get("title") or (route.summary if route else ""),
                    "business_type": int(meta.get("business_type", BusinessType.OTHER)),
                    "method": f"{endpoint.__module__}.{endpoint.__name__}()" if endpoint else "",
                    "request_method": request.method,
                    "operator_type": int(meta.get("operator_type", OperatorType.MANAGE)),
                    "oper_name": oper_name,
                    "oper_url": request.url.path[:255],
                    "oper_ip": get_client_ip(request),
                    "oper_param": log_payload,
                    "json_result": json_result,
                    "status": int(status_value),
                    "cost_time": int((time.perf_counter() - start) * 1000),
                }
                response.background = BackgroundTask(_write_operation_log_async, log_data)
            except Exception:
                logger.warning("操作日志采集异常: {}", request.url.path, exc_info=True)
            return response

        return custom_route_handler
