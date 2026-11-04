// Package resource 实现 SSE/WebSocket 端点，对应 Python 端 module_resource/controller.py。
package resource

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/config"
	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/cryptox"
	"walnut-seed-go/internal/pkg/envelope"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/routex"
	"walnut-seed-go/internal/pkg/ssex"
)

// Handler 资源域端点。
type Handler struct {
	sse *ssex.Manager
	cfg *config.Config
	err *errs.Handler
}

// NewHandler 构造资源 handler。
func NewHandler(sse *ssex.Manager, cfg *config.Config, eh *errs.Handler) *Handler {
	return &Handler{sse: sse, cfg: cfg, err: eh}
}

// Mount 注册资源路由。/sse 需要登录，/sse/close 在白名单内无需登录。
func (h *Handler) Mount(protected routex.Binder, public routex.Binder) {
	protected.GET("/sse", h.SSEConnect)
	public.GET("/sse/close", h.SSEClose)
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
		fmt.Sscanf(userIDStr, "%d", &userID)
		if userID > 0 {
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
