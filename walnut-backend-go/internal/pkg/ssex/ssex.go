// Package ssex 实现 SSE（Server-Sent Events）连接管理，对应 Python 端 app/core/sse.py。
//
// 支持多副本广播：同一用户可在多个浏览器标签页同时连接，
// 跨实例通过 Redis pub/sub topic "global:sse" 扇出。
package ssex

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"walnut-seed-go/internal/pkg/cachex"
)

// 心跳间隔，与 Python 端 _HEARTBEAT_INTERVAL 一致。
const HeartbeatInterval = 60 * time.Second

// Topic Redis pub/sub 频道名，对应 Python 端 CacheNames.SSE_TOPIC。
const Topic = "global:sse"

// Message SSE 消息体，对应 Python 端 SseMessageDto。
type Message struct {
	UserIDs []int64 `json:"user_ids"` // 空 = 广播
	Message string  `json:"message"`
}

// Manager 管理全部 SSE 连接。
type Manager struct {
	mu     sync.RWMutex
	queues map[int64]map[string]chan string // userID → token → queue
	cache  *cachex.Client
	log    *slog.Logger
}

// NewManager 构造 SSE 管理器。
func NewManager(cache *cachex.Client, log *slog.Logger) *Manager {
	return &Manager{
		queues: make(map[int64]map[string]chan string),
		cache:  cache,
		log:    log,
	}
}

// Connect 建立 SSE 连接，返回事件流 channel。
// 调用方应在 goroutine 中消费 returned channel，直到它被关闭。
func (m *Manager) Connect(userID int64, token string) <-chan string {
	m.mu.Lock()
	if m.queues[userID] == nil {
		m.queues[userID] = make(map[string]chan string)
	}
	ch := make(chan string, 64)
	m.queues[userID][token] = ch
	m.mu.Unlock()
	m.log.Debug("SSE connect", slog.Int64("user_id", userID))
	return ch
}

// Disconnect 断开指定连接。
func (m *Manager) Disconnect(userID int64, token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if tokens, ok := m.queues[userID]; ok {
		if ch, ok := tokens[token]; ok {
			close(ch)
			delete(tokens, token)
		}
		if len(tokens) == 0 {
			delete(m.queues, userID)
		}
	}
	m.log.Debug("SSE disconnect", slog.Int64("user_id", userID))
}

// SendMessage 向指定用户的所有连接推送消息。
func (m *Manager) SendMessage(userID int64, message string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if tokens, ok := m.queues[userID]; ok {
		for _, ch := range tokens {
			select {
			case ch <- message:
			default:
				// queue full, drop message
			}
		}
	}
}

// Broadcast 向全部连接推送消息。
func (m *Manager) Broadcast(message string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, tokens := range m.queues {
		for _, ch := range tokens {
			select {
			case ch <- message:
			default:
			}
		}
	}
}

// Publish 通过 Redis pub/sub 向全部实例推送消息。
func (m *Manager) Publish(ctx context.Context, msg Message) error {
	if m.cache == nil {
		// 无 Redis 时退化为本地广播
		if len(msg.UserIDs) == 0 {
			m.Broadcast(msg.Message)
		} else {
			for _, uid := range msg.UserIDs {
				m.SendMessage(uid, msg.Message)
			}
		}
		return nil
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化 SSE 消息失败: %w", err)
	}
	topicKey := m.cache.Key(Topic)
	return m.cache.Client.Publish(ctx, topicKey, data).Err()
}

// OnTopicMessage 处理 Redis pub/sub 收到的消息。
func (m *Manager) OnTopicMessage(raw string) {
	var msg Message
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		m.log.Warn("SSE 消息解析失败", slog.String("raw", raw), slog.String("err", err.Error()))
		return
	}
	if len(msg.UserIDs) > 0 {
		for _, uid := range msg.UserIDs {
			m.SendMessage(uid, msg.Message)
		}
	} else {
		m.Broadcast(msg.Message)
	}
}

// Subscribe 启动 Redis pub/sub 订阅，阻塞直到 ctx 取消。
// 应在 App 启动时作为 goroutine 调用。
func (m *Manager) Subscribe(ctx context.Context) {
	if m.cache == nil {
		m.log.Warn("⚠️ Redis 不可用，SSE 跨实例广播禁用")
		return
	}
	topicKey := m.cache.Key(Topic)
	sub := m.cache.Subscribe(ctx, topicKey)
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
