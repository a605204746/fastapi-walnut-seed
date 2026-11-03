package auth

// 本文件实现认证中间件，对应 Python 端 app/core/dependencies.py 的 get_current_user。
//
// 它是 P3 全部 113 条受保护路由的**共同前置**：校验 JWT → 读 Redis 会话 →
// 滑动续期 → 组装 authctx.Context 写入请求。
//
// ⚠️ 失败文案是契约的一部分（前端会直接展示），必须与 Python 端逐字一致：
//
//	无 Authorization 头 / scheme 不是 Bearer  → 认证失败，请登录后再试
//	token 为空                               → 认证已失效
//	JWT 解析失败                             → 见 cryptox.TokenError.Message()
//	是 refresh token                         → 非法凭证
//	clientid 与 token 内不一致               → 客户端ID与Token不匹配
//	Redis 会话缺失或 JSON 损坏                → 认证已失效
//	_active_timeout 非法（<=0）              → 认证已失效
//	取不到 user_id                           → 认证已失效
//	会话里的 user_status 为停用               → 用户已被停用
//
// 全部经 errs.NotLoginMsg 渲染为 **HTTP 200 + body.code=401**（信封契约）。

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/config"
	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/cachex"
	"walnut-seed-go/internal/pkg/cryptox"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/modelx"
)

// 认证失败文案，逐字对齐 Python 端 dependencies.py / security.py 的字面量。
const (
	msgAuthFailed       = "认证失败，请登录后再试"
	msgSessionInvalid   = "认证已失效"
	msgIllegalCred      = "非法凭证"
	msgClientIDMismatch = "客户端ID与Token不匹配"
	msgUserDisabled     = "用户已被停用"
)

// NewMiddleware 构造认证中间件。
//
// cache 允许为 nil（Redis 完全不可用时）：此时按 **fail-closed** 一律判"认证已失效"，
// 与 Python 端 RedisUtils.get 吞掉异常返回 None 的效果一致——绝不因缓存挂掉而放行。
func NewMiddleware(cfg *config.Config, cache *cachex.Client, logger *slog.Logger, eh *errs.Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac, err := Authenticate(c, cfg, cache, logger)
		if err != nil {
			eh.Abort(c, err)
			return
		}
		authctx.Set(c, ac)
		c.Next()
	}
}

// Authenticate 执行一次完整的认证，返回请求级认证上下文。
//
// 导出是为了让 bootstrap 之外的场景（SSE/WebSocket 握手、P3 的操作日志采集）
// 能复用同一条认证链路，而不必挂在 gin 中间件上。
func Authenticate(c *gin.Context, cfg *config.Config, cache *cachex.Client, logger *slog.Logger) (authctx.Context, error) {
	ctx := c.Request.Context()

	// ---- 1. 取 token（对应 CustomOAuth2PasswordBearer.__call__）----
	authorization := c.GetHeader(cfg.TokenName)
	if authorization == "" {
		// SSE 用 EventSource，无法设置自定义请求头，前端只能把 token 放查询串
		authorization = c.Query(cfg.TokenName)
	}
	scheme, token := splitAuthScheme(authorization)
	if authorization == "" || !strings.EqualFold(scheme, cfg.TokenPrefix) {
		return authctx.Context{}, errs.NotLoginMsg(msgAuthFailed)
	}

	// ---- 2. 解析 JWT（exp 始终按固定硬过期校验）----
	if token == "" {
		return authctx.Context{}, errs.NotLoginMsg(msgSessionInvalid)
	}
	claims, err := cryptox.ParseToken(token, cfg.SecretKey, true)
	if err != nil {
		return authctx.Context{}, errs.NotLoginMsg(tokenErrorMessage(err))
	}
	if claims.IsRefresh {
		return authctx.Context{}, errs.NotLoginMsg(msgIllegalCred)
	}
	sessionID := claims.Sub
	if sessionID == "" {
		return authctx.Context{}, errs.NotLoginMsg(msgSessionInvalid)
	}

	// ---- 3. clientid 一致性：防止 A 客户端的 token 被 B 客户端盗用 ----
	if claims.ClientID != nil {
		if requestClientID(c, cfg.ClientIDHeader) != *claims.ClientID {
			return authctx.Context{}, errs.NotLoginMsg(msgClientIDMismatch)
		}
	}

	// ---- 4. 从 Redis 加载会话 ----
	if cache == nil {
		return authctx.Context{}, errs.NotLoginMsg(msgSessionInvalid)
	}
	raw, ok := cache.Get(ctx, keyUserSession+sessionID)
	if !ok || raw == "" {
		return authctx.Context{}, errs.NotLoginMsg(msgSessionInvalid)
	}
	var sess SessionInfo
	if jsonErr := json.Unmarshal([]byte(raw), &sess); jsonErr != nil {
		if logger != nil {
			logger.Warn("会话快照 JSON 损坏，按认证失效处理",
				slog.String("session_id", sessionID), slog.String("err", jsonErr.Error()))
		}
		return authctx.Context{}, errs.NotLoginMsg(msgSessionInvalid)
	}

	// ---- 5. 滑动过期续期 ----
	if err := renewSession(ctx, cfg, cache, claims, &sess, sessionID, token); err != nil {
		return authctx.Context{}, err
	}

	// ---- 6. 组装认证上下文 ----
	return buildAuthContext(&sess, claims)
}

// tokenErrorMessage 把 JWT 解析错误翻译成与 Python 端逐字一致的文案。
func tokenErrorMessage(err error) string {
	var te *cryptox.TokenError
	if errors.As(err, &te) {
		return te.Message()
	}
	// 理论上 ParseToken 只返回 *TokenError；兜底走 Python 端 InvalidTokenError 分支
	return "token已失效，请重新登录"
}

// requestClientID 读取请求携带的 clientid，对应 Python 端 _extract_clientid。
//
// ⚠️ 请求头与查询串都取不到时返回空串：随后与 claims.ClientID 的比较必然不等，
// 从而拒绝访问——这与 Python 端 `None != payload.clientid` 的结果一致。
func requestClientID(c *gin.Context, header string) string {
	if v := c.GetHeader(header); v != "" {
		return v
	}
	return c.Query(header)
}

// splitAuthScheme 复刻 starlette 的 get_authorization_scheme_param：
// 按**第一个空格**切分，无空格时 scheme 取整串、token 为空。
//
// ⚠️ 与 service.go 的 extractBearerToken **语义不同**，不要合并：
// 那个用于登出（宽松，接受裸 token，因为登出要幂等成功），
// 这个用于鉴权（严格，scheme 不匹配就拒绝，与 OAuth2PasswordBearer 一致）。
func splitAuthScheme(header string) (scheme, token string) {
	if header == "" {
		return "", ""
	}
	s, rest, _ := strings.Cut(header, " ")
	return s, rest
}

// renewSession 滑动过期续期，对应 dependencies.py 里的 active_timeout 分支。
//
// 语义（必须逐条对齐）：
//   - 会话内没有 _active_timeout（旧会话）→ **不续期**，而不是回退全局默认值
//   - 只在「剩余 TTL 已不足活跃超时的一半」时才续，避免每个请求都写 Redis
//   - 续期时长不能超过 JWT 的固定硬过期时间——JWT 无法撤销，
//     若让 Redis 会话活得比 exp 更久，就会出现"token 名义上过期但仍可用"
//   - 同时续 user_session / access_token / online_tokens 三个键，
//     漏一个会让在线用户页与实际登录态脱节
func renewSession(
	ctx context.Context,
	cfg *config.Config,
	cache *cachex.Client,
	claims *cryptox.Claims,
	sess *SessionInfo,
	sessionID, token string,
) error {
	if !cfg.TokenSlidingExpire || sess.ActiveTimeout == nil {
		return nil
	}
	activeTimeout := *sess.ActiveTimeout
	if activeTimeout <= 0 {
		return errs.NotLoginMsg(msgSessionInvalid)
	}

	ttl := cache.TTL(ctx, keyUserSession+sessionID)
	// Python: if 0 < ttl < max(1, active_timeout // 2)
	half := int64(activeTimeout / 2)
	if half < 1 {
		half = 1
	}
	if ttl <= 0 || ttl >= half {
		return nil
	}

	remainingHard := claims.Exp - time.Now().UTC().Unix()
	renew := int64(activeTimeout)
	if remainingHard < renew {
		renew = remainingHard
	}
	if renew <= 0 {
		return nil
	}

	d := time.Duration(renew) * time.Second
	cache.Expire(ctx, keyUserSession+sessionID, d)
	cache.Expire(ctx, keyAccessToken+sessionID, d)
	cache.Expire(ctx, keyOnlineToken+token, d)
	return nil
}

// buildAuthContext 由会话快照 + JWT claims 组装认证上下文，
// 对应 dependencies.py 末尾构造 CoreUserSchema / AuthSchema 的那段。
//
// 回退规则用 Python 的 `or` 语义（会话值为假值时取 claims 里的）：
//
//	user_id  = session.user_id  or payload.user_id
//	username = session.user_name or payload.user_name
//	dept_id  = session.dept_id  or payload.dept_id
//	nickname = session.nickname            ← 无回退
func buildAuthContext(sess *SessionInfo, claims *cryptox.Claims) (authctx.Context, error) {
	userID := sess.UserID
	if userID == 0 && claims.UserID != nil {
		userID = *claims.UserID
	}
	if userID == 0 {
		return authctx.Context{}, errs.NotLoginMsg(msgSessionInvalid)
	}
	if sess.UserStatus == modelx.Disabled {
		return authctx.Context{}, errs.NotLoginMsg(msgUserDisabled)
	}

	userName := sess.UserName
	if userName == "" && claims.UserName != nil {
		userName = *claims.UserName
	}
	deptID := sess.DeptID
	if (deptID == nil || *deptID == 0) && claims.DeptID != nil {
		deptID = claims.DeptID
	}

	return authctx.Context{
		User: authctx.User{
			ID:          userID,
			UserName:    userName,
			Nickname:    sess.Nickname,
			DeptID:      deptID,
			IsSuperuser: userID == modelx.SuperAdminID,
		},
		Permissions: nonNilStrings(sess.MenuPermission),
		Roles:       nonNilStrings(sess.RolePermission),
		MenuIDs:     nonNilInts(sess.MenuIDs),
	}, nil
}

// nonNilStrings 把 nil 归一化为空切片。
//
// Python 端 `session_info.get("menu_permission", [])` 拿到的是 []，
// 且 AuthSchema 的字段是 list[str]（不允许 None）。Go 的 nil 切片在
// 权限判断里行为相同，但一旦哪天被序列化出去就会变成 null，前端遍历会崩。
func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func nonNilInts(v []int64) []int64 {
	if v == nil {
		return []int64{}
	}
	return v
}
