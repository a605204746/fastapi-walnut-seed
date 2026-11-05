// Package resource 实现 SSE/WebSocket 端点，对应 Python 端 module_resource/controller.py。
package resource

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/config"
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
// 认证：从查询参数或请求头读 token，解码并校验 clientid，
// 通过后注册会话。认证失败关闭连接（code=4401）。
func (h *Handler) WebSocket(c *gin.Context) {
	// 从 query 或 header 取 token
	token := c.Query("token")
	if token == "" {
		auth := c.GetHeader(h.cfg.TokenName)
		if strings.HasPrefix(auth, h.cfg.TokenPrefix+" ") {
			token = strings.TrimPrefix(auth, h.cfg.TokenPrefix+" ")
		}
	}

	// JWT 解码（exp 始终按硬过期校验）
	claims, err := cryptox.ParseToken(token, h.cfg.SecretKey, true)
	if err != nil || claims.UserID == nil {
		h.log.Warn("WebSocket 认证失败", slog.String("err", errString(err)))
		c.JSON(http.StatusOK, gin.H{"code": 4401, "msg": "认证失败"})
		return
	}

	// Redis 会话检查
	if h.cache != nil && claims.Sub != "" {
		sessionKey := h.cache.Key("user_session:" + claims.Sub)
		exists, _ := h.cache.Client.Exists(c.Request.Context(), sessionKey).Result()
		if exists == 0 {
			h.log.Warn("WebSocket 会话不存在", slog.String("sub", claims.Sub))
			c.JSON(http.StatusOK, gin.H{"code": 4401, "msg": "认证已失效"})
			return
		}
	}

	// clientid 一致性检查
	if claims.ClientID != nil {
		requestClient := c.Query(h.cfg.ClientIDHeader)
		if requestClient == "" {
			requestClient = c.GetHeader(h.cfg.ClientIDHeader)
		}
		if requestClient != *claims.ClientID {
			h.log.Warn("WebSocket clientid 不匹配")
			c.JSON(http.StatusOK, gin.H{"code": 4401, "msg": "客户端ID与Token不匹配"})
			return
		}
	}

	userID := *claims.UserID

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

// SSEConnect GET /resource/sse — SSE 连接（需登录，token 可走 query 参数）。
func (h *Handler) SSEConnect(c *gin.Context) {
	ac, ok := authctx.From(c)
	if !ok {
		// 尝试从 query 参数取 token（EventSource 无法设 Authorization 头）
		token := extractQueryToken(c)
		if token == "" {
			h.err.Abort(c, errs.NotLogin())
			return
		}
		claims, err := cryptox.ParseToken(token, h.cfg.SecretKey, false)
		if err != nil || claims.UserID == nil {
			h.err.Abort(c, errs.NotLogin())
			return
		}
		ac = authctx.Context{User: authctx.User{ID: *claims.UserID}}
	}

	userID := ac.User.ID
	token := extractToken(c)
	if token == "" {
		token = fmt.Sprintf("anon_%d_%d", userID, time.Now().UnixNano())
	}

	ch := h.sse.Connect(userID, token)
	defer h.sse.Disconnect(userID, token)

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	// 发送初始 connected 事件
	fmt.Fprintf(c.Writer, "event: connected\ndata: \n\n")
	c.Writer.Flush()

	heartbeat := time.NewTicker(ssex.HeartbeatInterval)
	defer heartbeat.Stop()

	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(c.Writer, "event: message\ndata: %s\n\n", msg)
			c.Writer.Flush()
		case <-heartbeat.C:
			fmt.Fprintf(c.Writer, "event: heartbeat\ndata: \n\n")
			c.Writer.Flush()
		}
	}
}

// SSEClose GET /resource/sse/close — 断开 SSE 连接（白名单内，无需登录）。
func (h *Handler) SSEClose(c *gin.Context) {
	userIDStr := c.Query("userId")
	token := c.Query("token")

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
		if _, err := fmt.Sscanf(userIDStr, "%d", &userID); err == nil && userID > 0 {
			h.sse.Disconnect(userID, token)
		}
	}
	envelope.OK(c, nil)
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
