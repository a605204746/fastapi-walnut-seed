// Package ratelimitx 实现接口限流，对齐 Python 端 app/core/rate_limiter.py。
//
// 键组成（与 Python 端逐字符一致，便于跨端排查）：
//
//	{REDIS_KEY_PREFIX}:global:rate_limit:{请求路径}{IP}:{附加key}
//
// ⚠️ 路径与 IP 之间**没有分隔符**，IP 后面带一个冒号——这是 Python 端
// `"".join([RATE_LIMIT_KEY, path, f"{ip}:"])` 的实际结果。看着别扭，
// 但既然三端各自有独立的 REDIS_KEY_PREFIX、键不会互相碰撞，
// 保持同构更利于「同一条限流规则在两个后端上表现一致」的排查。
//
// 算法：**固定窗口计数**，用单条 Lua 脚本原子执行 INCR + EXPIRE。
// 原子性是必需的：若 INCR 与 EXPIRE 分两次调用，进程在中间崩溃会留下一个
// **永不过期**的键，该 IP 就此被永久限流。脚本还额外处理了
// 「键存在但无 TTL」的历史遗留情况（TTL == -1 时补设）。
//
// ⚠️ Redis 故障时 **fail-closed**（拒绝请求），不是 fail-open。
// 限流是安全控制，缓存挂了就放行等于把限流形同虚设。
package ratelimitx

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimitKeyPrefix 限流键的固定前缀段，对应 Python 端 CacheNames.RATE_LIMIT_KEY。
const RateLimitKeyPrefix = "global:rate_limit:"

// limitLua 原子计数脚本，与 Python 端 _RATE_LIMIT_LUA 逐字一致。
const limitLua = `
local current = redis.call('INCR', KEYS[1])
if current == 1 or redis.call('TTL', KEYS[1]) == -1 then
    redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return current
`

// LimitType 限流维度，对应 Python 端 LimitType 枚举。
type LimitType string

const (
	// LimitDefault 全局计数（不区分来源）
	LimitDefault LimitType = "DEFAULT"
	// LimitIP 按客户端 IP 计数
	LimitIP LimitType = "IP"
	// LimitCluster 按后端实例计数
	LimitCluster LimitType = "CLUSTER"
)

// instanceID 本进程的实例标识（CLUSTER 模式用），对应 Python 端 _INSTANCE_ID。
var instanceID = newInstanceID()

// Limiter 限流器。
type Limiter struct {
	rdb    *redis.Client
	prefix string // Redis 根前缀（如 walnut-seed-go），由 cachex 提供
	logger *slog.Logger
	script *redis.Script
}

// New 构造限流器。prefix 应为 cachex.Client.Prefix() 的返回值（已含末尾冒号）。
func New(rdb *redis.Client, prefix string, logger *slog.Logger) *Limiter {
	if logger == nil {
		logger = slog.Default()
	}
	return &Limiter{
		rdb:    rdb,
		prefix: prefix,
		logger: logger,
		// redis.Script 会先尝试 EVALSHA，未命中再回退 EVAL，避免每次传脚本正文
		script: redis.NewScript(limitLua),
	}
}

// Rule 一条限流规则。
type Rule struct {
	// Window 时间窗口
	Window time.Duration
	// Count 窗口内最大请求数
	Count int64
	// Type 限流维度
	Type LimitType
	// Key 附加键（可为空）
	Key string
	// Message 超限提示；为空时用 i18n 的 rate.limiter.message
	Message string
}

// Result 限流判定结果。
type Result struct {
	// Allowed 是否放行
	Allowed bool
	// Current 窗口内当前计数
	Current int64
	// Remaining 剩余配额（可能为负，表示已超限多少次）
	Remaining int64
	// Message 拒绝时应返回的提示文案
	Message string
	// Err 限流器自身故障（Redis 不可用等）。
	// ⚠️ 非 nil 时必须 fail-closed 拒绝请求，不能因为限流器坏了就放行
	Err error
}

// Allow 判定一次请求是否放行。
//
// path 与 clientIP 由调用方传入（而非从 *http.Request 取），
// 这样限流逻辑不依赖 gin，便于单测。
func (l *Limiter) Allow(ctx context.Context, rule Rule, path, clientIP string) Result {
	key := l.combineKey(rule, path, clientIP)

	res, err := l.script.Run(ctx, l.rdb, []string{key}, int(rule.Window.Seconds())).Int64()
	if err != nil {
		// fail-closed：Redis 故障时拒绝请求
		l.logger.Error("限流器异常，按 fail-closed 拒绝请求",
			slog.String("key", key), slog.String("err", err.Error()))
		return Result{Allowed: false, Err: err, Message: "服务器限流异常，请稍候再试"}
	}

	if res > rule.Count {
		msg := rule.Message
		l.logger.Warn("限流拦截",
			slog.String("path", path),
			slog.String("client_ip", clientIP),
			slog.Int64("current", res),
			slog.Int64("limit", rule.Count))
		return Result{Allowed: false, Current: res, Remaining: rule.Count - res, Message: msg}
	}
	return Result{Allowed: true, Current: res, Remaining: rule.Count - res}
}

// combineKey 拼装完整 Redis 键（含根前缀）。
func (l *Limiter) combineKey(rule Rule, path, clientIP string) string {
	var b []byte
	b = append(b, l.prefix...)
	b = append(b, RateLimitKeyPrefix...)
	b = append(b, path...)
	switch rule.Type {
	case LimitIP:
		b = append(b, clientIP...)
		b = append(b, ':')
	case LimitCluster:
		b = append(b, instanceID...)
		b = append(b, ':')
	}
	b = append(b, rule.Key...)
	return string(b)
}

// ErrRateLimited 供调用方用 errors.Is 判断"因限流被拒"。
var ErrRateLimited = errors.New("请求过于频繁")

// newInstanceID 生成 8 位十六进制实例标识。
//
// 用 crypto/rand 而非 math/rand：虽然实例 ID 不是密钥，但可预测的实例 ID
// 会让攻击者能定向构造 CLUSTER 维度的限流键。
func newInstanceID() string {
	const hexDigits = "0123456789abcdef"
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		// 极端情况下退化为时间戳，保证不为空（空实例 ID 会让所有实例共用一个计数键）
		return fmt.Sprintf("%08x", time.Now().UnixNano()&0xffffffff)
	}
	out := make([]byte, 8)
	for i, v := range buf {
		out[i] = hexDigits[int(v)%len(hexDigits)]
	}
	return string(out)
}
