// Package wsx 实现 WebSocket 会话管理，对应 Python 端 app/core/websocket.py。
//
// 设计：
//   - 一用户一会话（Python 端同此约定）
//   - 跨实例通过 Redis pub/sub "global:websocket" 扇出
//   - 本地会话直接发送，远端会话走 Redis
package wsx

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/coder/websocket"

	"walnut-seed-go/internal/pkg/cachex"
)

// Topic Redis pub/sub 频道名，对应 Python 端 CacheNames.WEB_SOCKET_TOPIC。
const Topic = "global:websocket"

// Message WebSocket 消息 DTO，对应 Python 端 WebSocketMessageDto。
type Message struct {
	SessionKeys []int64 `json:"session_keys"`
	Message     string  `json:"message"`
}

// Holder 持有本机全部 WebSocket 会话（user_id → conn）。
type Holder struct {
	mu       sync.RWMutex
	sessions map[int64]*websocket.Conn
}

// NewHolder 构造会话持有器。
func NewHolder() *Holder {
	return &Holder{sessions: make(map[int64]*websocket.Conn)}
}

// Add 注册会话（新连接替换旧连接）。
func (h *Holder) Add(userID int64, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sessions[userID] = conn
}

// Remove 移除会话。
func (h *Holder) Remove(userID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.sessions, userID)
}

// Get 取出会话。
func (h *Holder) Get(userID int64) (*websocket.Conn, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conn, ok := h.sessions[userID]
	return conn, ok
}

// Exist 检查会话是否存在。
func (h *Holder) Exist(userID int64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.sessions[userID]
	return ok
}

// AllKeys 返回全部用户 ID。
func (h *Holder) AllKeys() []int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	keys := make([]int64, 0, len(h.sessions))
	for k := range h.sessions {
		keys = append(keys, k)
	}
	return keys
}

// Manager 管理 WebSocket 会话与跨实例通信。
type Manager struct {
	holder *Holder
	cache  *cachex.Client
	log    *slog.Logger
}

// NewManager 构造管理器。
func NewManager(cache *cachex.Client, log *slog.Logger) *Manager {
	return &Manager{
		holder: NewHolder(),
		cache:  cache,
		log:    log,
	}
}

// Holder 返回会话持有器（端点注册/注销使用）。
func (m *Manager) Sessions() *Holder {
	return m.holder
}

// SendLocal 向本地会话发送消息（容错）。
func (m *Manager) SendLocal(ctx context.Context, userID int64, message string) {
	conn, ok := m.holder.Get(userID)
	if !ok {
		return
	}
	if err := conn.Write(ctx, websocket.MessageText, []byte(message)); err != nil {
		m.log.Warn("WebSocket 发送失败",
			slog.Int64("user_id", userID),
			slog.String("err", err.Error()))
	}
}

// PublishMessage 混合路由：本地会话直发，远端会话走 Redis pub/sub。
// 对应 Python 端 WebSocketUtils.publish_message。
func (m *Manager) PublishMessage(ctx context.Context, msg Message) error {
	// 本地直发
	for _, key := range msg.SessionKeys {
		if m.holder.Exist(key) {
			m.SendLocal(ctx, key, msg.Message)
		}
	}

	// 远端或广播走 Redis
	remoteKeys := make([]int64, 0)
	for _, key := range msg.SessionKeys {
		if !m.holder.Exist(key) {
			remoteKeys = append(remoteKeys, key)
		}
	}
	if len(remoteKeys) > 0 || len(msg.SessionKeys) == 0 {
		if m.cache == nil {
			return nil // 无 Redis 时退化为本地发送
		}
		dto := Message{SessionKeys: remoteKeys, Message: msg.Message}
		data, err := json.Marshal(dto)
		if err != nil {
			return fmt.Errorf("序列化 WebSocket 消息失败: %w", err)
		}
		topicKey := m.cache.Key(Topic)
		return m.cache.Client.Publish(ctx, topicKey, data).Err()
	}
	return nil
}

// PublishAll 向全部实例广播，对应 Python 端 WebSocketUtils.publish_all。
func (m *Manager) PublishAll(ctx context.Context, message string) error {
	return m.PublishMessage(ctx, Message{SessionKeys: nil, Message: message})
}

// OnTopicMessage 处理 Redis pub/sub 收到的消息。
func (m *Manager) OnTopicMessage(raw string) {
	var msg Message
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		m.log.Warn("WebSocket topic 消息解析失败", slog.String("raw", raw))
		return
	}
	ctx := context.Background()
	if len(msg.SessionKeys) > 0 {
		for _, key := range msg.SessionKeys {
			if m.holder.Exist(key) {
				m.SendLocal(ctx, key, msg.Message)
			}
		}
	} else {
		// 广播：发给本机全部会话
		for _, key := range m.holder.AllKeys() {
			m.SendLocal(ctx, key, msg.Message)
		}
	}
}

// Subscribe 启动 Redis pub/sub 订阅（阻塞，应在 goroutine 中调用）。
func (m *Manager) Subscribe(ctx context.Context) {
	if m.cache == nil {
		m.log.Warn("⚠️ Redis 不可用，WebSocket 跨实例通信禁用")
		return
	}
	topicKey := m.cache.Key(Topic)
	sub := m.cache.Client.Subscribe(ctx, topicKey)
	defer sub.Close()

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			m.OnTopicMessage(msg.Payload)
		}
	}
}
