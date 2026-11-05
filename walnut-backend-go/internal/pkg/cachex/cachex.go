// Package cachex 封装 Redis 连接与键前缀。
//
// ⚠️ 键前缀是**三端共存的硬约束**：Python 用 walnut-seed-python、Java 用
// walnut-seed-java、Go 用 walnut-seed-go。三端共用同一个 Redis 实例，
// 前缀缺失或重复会导致登录态、验证码、限流计数互相串键。
//
// go-redis 没有内置键前缀能力，故本包提供 Key() 统一拼装，
// 业务侧**禁止**自行拼接 Redis 键名。
package cachex

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Params 连接参数。
type Params struct {
	Host     string
	Port     int
	DB       int
	Username string
	Password string
	// KeyPrefix 键根前缀，如 walnut-seed-go
	KeyPrefix string
	// HealthCheckInterval 连接健康检查间隔秒数
	HealthCheckInterval int
	// PoolSize 连接池大小
	PoolSize int
}

// Client 是带键前缀能力的 Redis 客户端包装。
type Client struct {
	*redis.Client
	prefix string
	// logger 由 SetLogger 注入；KV 操作吞掉错误只记日志（见 kv.go 的说明），
	// 故需要一个 logger。nil 时退化为 slog.Default()。
	// 仅在启动期写入、运行期只读，无需加锁。
	logger *slog.Logger
}

// New 建立连接。注意：**不在此处 Ping**，因为 Redis 不可用时应用仍应降级启动
// （对齐 Python 端 redis_connect 失败仅告警的行为），由 health/ready 反映真实状态。
func New(p Params) *Client {
	prefix := normalizePrefix(p.KeyPrefix)
	// ⚠️ go-redis v9 的 Options **没有** HealthCheckInterval 字段
	// （该名只存在于 PubSub 的 WithChannelHealthCheckInterval 选项）。
	// redis-py 的 health_check_interval 语义是「取用空闲连接前先 PING 探活」，
	// go-redis 的等价手段是 ConnMaxIdleTime——空闲超过该时长的连接直接回收重建，
	// 同样能避免把已被服务端关闭的连接交给业务。故此处做语义映射而非同名透传。
	idleTimeout := time.Duration(positive(p.HealthCheckInterval, 20)) * time.Second
	return &Client{
		Client: redis.NewClient(&redis.Options{
			Addr:     fmt.Sprintf("%s:%d", p.Host, p.Port),
			DB:       p.DB,
			Username: p.Username,
			// 密码为空时不设置，避免 go-redis 发送空 AUTH 导致连接失败
			// （Java 端 Redisson 有同类问题，compose 注释里专门提醒过）
			Password:        p.Password,
			PoolSize:        positive(p.PoolSize, 20),
			MinIdleConns:    2,
			DialTimeout:     5 * time.Second,
			ReadTimeout:     3 * time.Second,
			WriteTimeout:    3 * time.Second,
			ConnMaxIdleTime: idleTimeout,
			ConnMaxLifetime: 30 * time.Minute,
		}),
		prefix: prefix,
	}
}

// normalizePrefix 统一前缀分隔符：内部以 ':' 结尾，空值返回空串（不加前缀）。
func normalizePrefix(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	return strings.TrimSuffix(p, ":") + ":"
}

// Key 返回带前缀的完整键名。业务侧所有 Redis 操作都必须经此拼装。
func (c *Client) Key(parts ...string) string {
	return c.prefix + strings.Join(parts, ":")
}

// Prefix 暴露前缀本身，供需要按前缀批量扫描（SCAN）的场景使用。
func (c *Client) Prefix() string { return c.prefix }

// Ping 健康探测，供 /common/health/ready 使用。
func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.Client == nil {
		return errors.New("redis 未初始化")
	}
	return c.Client.Ping(ctx).Err()
}

// Close 关闭连接池。
func (c *Client) Close() error {
	if c == nil || c.Client == nil {
		return nil
	}
	return c.Client.Close()
}

func positive(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}
