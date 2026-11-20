// Package resource 实现 SSE/WebSocket 端点，对应 Python 端 module_resource/controller.py。
package resource

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/config"
	"walnut-seed-go/internal/module/web/auth"
	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/cachex"
	"walnut-seed-go/internal/pkg/cryptox"
	"walnut-seed-go/internal/pkg/envelope"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/routex"
	"walnut-seed-go/internal/pkg/ssex"
	"walnut-seed-go/internal/pkg/wsx"
)

// Handler 资源域端点。
type Handler struct {
	sse   *ssex.Manager
	ws    *wsx.Manager
	cache *cachex.Client
	cfg   *config.Config
	err   *errs.Handler
	log   *slog.Logger
}

// NewHandler 构造资源 handler。
func NewHandler(sse *ssex.Manager, ws *wsx.Manager, cache *cachex.Client, cfg *config.Config, eh *errs.Handler, log *slog.Logger) *Handler {
	return &Handler{sse: sse, ws: ws, cache: cache, cfg: cfg, err: eh, log: log}
}

// Mount 注册资源路由。/sse 需要登录，/sse/close 在白名单内无需登录。
// /websocket 在白名单内（浏览器 WebSocket 无法设自定义头）。
func (h *Handler) Mount(protected routex.Binder, public routex.Binder) {
	protected.GET("/sse", h.SSEConnect)
	public.GET("/sse/close", h.SSEClose)
	public.GET("/websocket", h.WebSocket)
}

// WebSocket GET /resource/websocket — WebSocket 连接。
//
// 认证：从查询参数或请求头读 token，解码 → Redis 会话 → clientid 一致性。
// 任一失败在 accept 前拒绝握手：HTTP 403 空体（uvicorn 对 close(4401) 的呈现，
// 实测 content-type: text/plain、connection: close）。Redis 缺失同样拒绝（fail-closed）。
func (h *Handler) WebSocket(c *gin.Context) {
	// 从 query 或 header 取 token
	token := c.Query("token")
	if token == "" {
		auth := c.GetHeader(h.cfg.TokenName)
		if strings.HasPrefix(auth, h.cfg.TokenPrefix+" ") {
			token = strings.TrimSpace(strings.TrimPrefix(auth, h.cfg.TokenPrefix+" "))
		}
	}

	reject := func(reason string) {
		h.log.Warn("WebSocket 认证失败", slog.String("reason", reason))
		c.Header("Content-Type", "text/plain")
		c.Header("Connection", "close")
		c.Status(http.StatusForbidden)
	}

	// JWT 解码（exp 始终按硬过期校验）
	claims, err := cryptox.ParseToken(token, h.cfg.SecretKey, true)
	if err != nil || claims.Sub == "" {
		reject(errString(err))
		return
	}

	// Redis 会话检查（缺失/不可达一律拒绝，对齐 Python try/except fail-closed）
	if h.cache == nil {
		reject("redis 不可用")
		return
	}
	raw, ok := h.cache.Get(c.Request.Context(), "user_session:"+claims.Sub)
	if !ok || raw == "" {
		reject("认证已失效")
		return
	}
	var sess auth.SessionInfo
	if jsonErr := json.Unmarshal([]byte(raw), &sess); jsonErr != nil {
		reject("会话快照损坏")
		return
	}

	// clientid 一致性检查
	if claims.ClientID != nil {
		requestClient := c.Query(h.cfg.ClientIDHeader)
		if requestClient == "" {
			requestClient = c.GetHeader(h.cfg.ClientIDHeader)
		}
		if requestClient != *claims.ClientID {
			reject("客户端ID与Token不匹配")
			return
		}
	}

	// Python：user_id 优先取会话快照，回退 JWT claims
	userID := sess.UserID
	if userID == 0 && claims.UserID != nil {
		userID = *claims.UserID
	}
	if userID == 0 {
		reject("认证已失效")
		return
	}

	// 升级为 WebSocket
	conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		InsecureSkipVerify: h.cfg.WebSocketAllowedOrigin == "*",
	})
	if err != nil {
		h.log.Warn("WebSocket 升级失败", slog.String("err", err.Error()))
		return
	}

	h.ws.Sessions().Add(userID, conn)
	h.log.Info("WebSocket 连接建立", slog.Int64("user_id", userID))

	defer func() {
		h.ws.Sessions().Remove(userID)
		conn.Close(websocket.StatusNormalClosure, "")
		h.log.Info("WebSocket 连接关闭", slog.Int64("user_id", userID))
	}()

	ctx := c.Request.Context()
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				return
			}
			h.log.Warn("WebSocket 读取异常", slog.String("err", err.Error()))
			return
		}
		msg := string(data)
		if msg == "ping" {
			if err := conn.Write(ctx, websocket.MessageText, []byte("pong")); err != nil {
				h.log.Warn("WebSocket 发送 pong 失败", slog.String("err", err.Error()))
				return
			}
			continue
		}
		// 其他消息通过 pub/sub 管道回发
		dto := wsx.Message{SessionKeys: []int64{userID}, Message: msg}
		if err := h.ws.PublishMessage(ctx, dto); err != nil {
			h.log.Warn("WebSocket 消息发布失败", slog.String("err", err.Error()))
		}
	}
}

// SSEConnect GET /resource/sse — SSE 连接（需登录）。
// 认证由 Protected 分组的 auth 中间件完成（支持 Authorization 头与同名 query，
// 含 JWT+Redis 会话+clientid+滑动续期），handler 内不再二次解 token。
func (h *Handler) SSEConnect(c *gin.Context) {
	ac, ok := authctx.From(c)
	if !ok {
		h.err.Abort(c, errs.NotLogin())
		return
	}

	userID := ac.User.ID
	token := extractToken(c)
	if token == "" {
		token = fmt.Sprintf("anon_%d_%d", userID, time.Now().UnixNano())
	}

	ch := h.sse.Connect(userID, token)
	defer h.sse.Disconnect(userID, token)

	// 头与 Python EventSourceResponse 实测一致（无 Connection 头）
	c.Writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	writeFrame := func(s string) {
		fmt.Fprint(c.Writer, s)
		c.Writer.Flush()
	}

	// connected 事件 raw_data="" → 无 data: 行（fastapi build_sse_bytes 语义）
	writeFrame("event: connected\n\n")

	// 双计时器复刻 Python 两层空闲语义：
	//   heartbeat：生成器 60s 拿不到消息 → 发 heartbeat（消息/心跳后均重置）
	//   ping：路由层 15s 拿不到任何事件 → 发 ": ping" 注释（仅事件后重置，心跳也算事件）
	const pingInterval = 15 * time.Second
	pingTimer := time.NewTimer(pingInterval)
	defer pingTimer.Stop()
	hbTimer := time.NewTimer(ssex.HeartbeatInterval)
	defer hbTimer.Stop()
	resetTimer := func(t *time.Timer, d time.Duration) {
		if !t.Stop() {
			select {
			case <-t.C:
			default:
			}
		}
		t.Reset(d)
	}

	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			writeFrame(sseMessageFrame(msg))
			resetTimer(pingTimer, pingInterval)
			resetTimer(hbTimer, ssex.HeartbeatInterval)
		case <-hbTimer.C:
			writeFrame("event: heartbeat\n\n")
			resetTimer(pingTimer, pingInterval)
			resetTimer(hbTimer, ssex.HeartbeatInterval)
		case <-pingTimer.C:
			writeFrame(": ping\n\n")
			resetTimer(pingTimer, pingInterval)
		}
	}
}

// sseMessageFrame 按 fastapi build_sse_bytes 渲染 message 事件：
// 多行消息拆成多条 data: 行；空消息无 data: 行。
func sseMessageFrame(msg string) string {
	var b strings.Builder
	b.WriteString("event: message\n")
	if msg != "" {
		normalized := strings.ReplaceAll(strings.ReplaceAll(msg, "\r\n", "\n"), "\r", "\n")
		// str.splitlines 不产生末尾空元素："abc\n" → ["abc"]
		normalized = strings.TrimSuffix(normalized, "\n")
		for _, line := range strings.Split(normalized, "\n") {
			b.WriteString("data: ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	b.WriteByte('\n')
	return b.String()
}

// SSEClose GET /resource/sse/close — 断开 SSE 连接（白名单内，无需登录）。
// 查询参数名与 Python 一致：user_id / token；user_id 非整数 → 400（pydantic 文案）。
func (h *Handler) SSEClose(c *gin.Context) {
	userIDStr, userIDPresent := c.GetQuery("user_id")
	token := c.Query("token")

	if userIDPresent {
		if _, err := strconv.ParseInt(userIDStr, 10, 64); err != nil {
			h.err.Abort(c, errs.BadRequest("Input should be a valid integer, unable to parse string as an integer"))
			return
		}
	}

	// 如果 query 没有，尝试从 header 解析
	if userIDStr == "" || token == "" {
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			rawToken := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := cryptox.ParseToken(rawToken, h.cfg.SecretKey, false)
			if err == nil && claims.UserID != nil {
				userIDStr = fmt.Sprintf("%d", *claims.UserID)
				if token == "" {
					token = rawToken
				}
			}
		}
	}

	if userIDStr != "" && token != "" {
		var userID int64
		if _, err := fmt.Sscanf(userIDStr, "%d", &userID); err == nil {
			h.sse.Disconnect(userID, token)
		}
	}
	envelope.OKMsg(c, nil, "SSE 连接已断开")
}

func extractQueryToken(c *gin.Context) string {
	auth := c.Query("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return auth
}

func extractToken(c *gin.Context) string {
	auth := c.GetHeader("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return extractQueryToken(c)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
