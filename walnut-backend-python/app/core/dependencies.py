import json
from collections.abc import AsyncGenerator
from datetime import UTC, datetime

from fastapi import Depends, Request
from redis.asyncio.client import Redis
from sqlalchemy.ext.asyncio import AsyncSession

from app.common.constant import SystemConstants
from app.common.enums import CacheNames, RedisInitKeyConfig
from app.config.setting import settings
from app.core.base_schema import AuthSchema, CoreUserSchema, JWTPayloadSchema
from app.core.database import async_db_session
from app.core.exceptions import NotLoginException, NotPermissionException, NotRoleException
from app.core.logger import logger
from app.core.redis_crud import RedisUtils
from app.core.security import OAuth2Schema, decode_access_token


async def db_getter(request: Request) -> AsyncGenerator[AsyncSession, None]:
    """数据库会话 — 请求级生命周期管理（一个请求内共享同一事务）。

    会话挂到 request.state，由 OperationLogRoute 在**响应发送前**显式 commit。
    不能只靠 session.begin() 的 teardown：FastAPI 的 yield 依赖清理在响应发送
    **之后**才执行，客户端「写后立即读」会读到未提交的旧数据（竞态）。
    异常路径仍由 begin() 上下文负责 rollback。
    """
    async with async_db_session() as session, session.begin():
        request.state.db_session = session
        yield session


async def redis_getter(request: Request) -> Redis:
    """获取 Redis 连接。"""
    return request.app.state.redis


def _extract_clientid(request: Request) -> str | None:
    """读取请求中的 clientid（请求头或查询参数）。"""
    return request.headers.get(settings.CLIENT_ID_HEADER) or request.query_params.get(settings.CLIENT_ID_HEADER)


async def get_current_user(
    request: Request,
    redis: Redis = Depends(redis_getter),
    token: str = Depends(OAuth2Schema),
) -> AuthSchema:
    """获取当前登录用户（校验 JWT 并从 Redis 加载会话）。"""
    return await _authenticate(request, token, redis)


async def _authenticate(request: Request, token: str, redis: Redis) -> AuthSchema:
    if not token:
        raise NotLoginException("认证已失效")

    # JWT exp 始终校验固定硬过期；滑动过期只延长 Redis 会话的活跃 TTL。
    payload: JWTPayloadSchema = decode_access_token(token, verify_exp=True)
    if payload.is_refresh:
        raise NotLoginException("非法凭证")

    session_id = payload.sub
    if not session_id:
        raise NotLoginException("认证已失效")

    # clientid 一致性校验：请求携带的 clientid 必须与 JWT 载荷中的一致
    if payload.clientid is not None:
        request_clientid = _extract_clientid(request)
        if request_clientid != payload.clientid:
            raise NotLoginException("客户端ID与Token不匹配")

    # 从 Redis 加载会话（键结构 user_session:<会话ID>，存储登录用户信息）
    session_key = f"{RedisInitKeyConfig.USER_SESSION.key}:{session_id}"
    raw = await RedisUtils(redis).get(session_key)
    if not raw:
        raise NotLoginException("认证已失效")
    try:
        session_info = json.loads(raw)
    except (TypeError, json.JSONDecodeError):
        raise NotLoginException("认证已失效")

    # 滑动过期续期：以会话内保存的 active_timeout 为准，且不能超过 JWT
    # 的固定 exp。旧会话没有该字段时不再续期，避免沿用错误的全局默认值。
    active_timeout = session_info.get("_active_timeout")
    if settings.TOKEN_SLIDING_EXPIRE and active_timeout is not None:
        try:
            active_timeout = int(active_timeout)
        except (TypeError, ValueError) as exc:
            raise NotLoginException("认证已失效") from exc
        if active_timeout <= 0:
            raise NotLoginException("认证已失效")

        access_key = f"{RedisInitKeyConfig.ACCESS_TOKEN.key}:{session_id}"
        ttl = await RedisUtils(redis).ttl(session_key)
        if 0 < ttl < max(1, active_timeout // 2):
            exp = payload.exp
            exp_timestamp = int(exp.timestamp()) if isinstance(exp, datetime) else int(exp)
            remaining_hard_expire = exp_timestamp - int(datetime.now(UTC).timestamp())
            renew_seconds = min(active_timeout, remaining_hard_expire)
            if renew_seconds > 0:
                redis_utils = RedisUtils(redis)
                await redis_utils.expire(session_key, renew_seconds)
                await redis_utils.expire(access_key, renew_seconds)
                await redis_utils.expire(f"{CacheNames.ONLINE_TOKEN_KEY}{token}", renew_seconds)

    user_id = session_info.get("user_id") or payload.user_id
    if not user_id:
        raise NotLoginException("认证已失效")
    if session_info.get("user_status") == SystemConstants.DISABLE:
        raise NotLoginException("用户已被停用")

    user = CoreUserSchema(
        id=user_id,
        username=session_info.get("user_name") or payload.user_name,
        nickname=session_info.get("nickname"),
        dept_id=session_info.get("dept_id") or payload.dept_id,
        is_superuser=(user_id == SystemConstants.SUPER_ADMIN_ID),
    )

    auth = AuthSchema(
        user=user,
        permissions=session_info.get("menu_permission", []),
        roles=session_info.get("role_permission", []),
        menu_ids=session_info.get("menu_ids", []),
    )
    # 挂载到 request.state，供 SSE/WebSocket/操作日志等读取
    request.state.login_user = session_info
    request.state.auth = auth
    return auth


class AuthPermission:
    """权限验证依赖。

    用法:
        @router.get("/list", dependencies=[Depends(AuthPermission(permissions=["system:user:list"]))])
        async def list(...): ...

    - ``permissions``：所需权限标识列表；``mode`` = "AND"(全部满足) / "OR"(满足其一)，默认 AND。
    - ``roles``：所需角色标识列表；``role_mode`` 默认 OR。
    - 超级管理员（user_id==1）直接放行；持有 ``*:*:*`` 权限直接放行。
    """

    def __init__(
        self,
        permissions: list[str] | None = None,
        roles: list[str] | None = None,
        mode: str = "AND",
        role_mode: str = "OR",
    ) -> None:
        self.permissions = permissions or []
        self.roles = roles or []
        self.mode = mode.upper()
        self.role_mode = role_mode.upper()

    async def __call__(self, auth: AuthSchema = Depends(get_current_user)) -> AuthSchema:
        user = auth.user
        # 超级管理员放行
        if user.id == SystemConstants.SUPER_ADMIN_ID or user.is_superuser:
            return auth

        # 角色校验
        if self.roles:
            user_roles = set(auth.roles)
            role_ok = any(r in user_roles for r in self.roles) if self.role_mode == "OR" else all(r in user_roles for r in self.roles)
            if not role_ok:
                logger.error(f"用户缺少所需角色: {self.roles}")
                raise NotRoleException()

        # 权限校验
        if self.permissions:
            user_permissions = set(auth.permissions)
            # 持有全部权限通配符直接放行
            if "*" in user_permissions or "*:*:*" in user_permissions:
                return auth
            if not user_permissions:
                raise NotPermissionException()
            perm_ok = any(p in user_permissions for p in self.permissions) if self.mode == "OR" else all(p in user_permissions for p in self.permissions)
            if not perm_ok:
                logger.error(f"用户缺少所需权限: {self.permissions}")
                raise NotPermissionException()

        return auth
