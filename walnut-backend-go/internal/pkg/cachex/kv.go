package cachex

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// 本文件实现与 Python 端 app/core/redis_crud.py 的 RedisUtils 等价的 KV 操作。
//
// ⚠️ 错误处理语义必须一致：**吞掉异常并记日志**，而不是向上返回 error。
//   - Get 失败返回 ("", false)，调用方按「缓存未命中」处理
//   - Set/Delete 失败返回 false，调用方按需降级
//
// 这是刻意的：缓存不可用时业务应当降级继续（例如验证码取不到就当过期），
// 而不是让整个请求 500。Python 端每个方法都包了 try/except 记日志后返回默认值，
// Go 侧若改成返回 error，调用方会被迫处理一堆无意义的错误分支，
// 且很容易在某个分支上把降级写成失败。

// SetLogger 注入日志器。未注入时用 slog.Default()。
//
// 之所以用包级注入而非 Client 字段：这些方法的签名要与 Python 端一致
// （返回 bool 而非 error），没有位置传 logger，而给每个方法加 logger 参数
// 会让调用点极其啰嗦。
func (c *Client) SetLogger(l *slog.Logger) {
	if l != nil {
		c.logger = l
	}
}

func (c *Client) log() *slog.Logger {
	if c.logger != nil {
		return c.logger
	}
	return slog.Default()
}

// Get 读取字符串值。ok=false 表示未命中或出错（两者对调用方等价）。
func (c *Client) Get(ctx context.Context, key string) (value string, ok bool) {
	v, err := c.Client.Get(ctx, c.Key(key)).Result()
	if err != nil {
		if err != redis.Nil {
			c.log().Error("获取缓存失败", slog.String("key", c.Key(key)), slog.String("err", err.Error()))
		}
		return "", false
	}
	return v, true
}

// Set 写入字符串值；ttl <= 0 表示永不过期。
func (c *Client) Set(ctx context.Context, key string, value any, ttl time.Duration) bool {
	if err := c.Client.Set(ctx, c.Key(key), value, ttlOrNil(ttl)).Err(); err != nil {
		c.log().Error("设置缓存失败", slog.String("key", c.Key(key)), slog.String("err", err.Error()))
		return false
	}
	return true
}

// SetIfAbsent 即 SETNX，用于防重提交与分布式锁。
func (c *Client) SetIfAbsent(ctx context.Context, key string, value any, ttl time.Duration) bool {
	ok, err := c.Client.SetNX(ctx, c.Key(key), value, ttlOrNil(ttl)).Result()
	if err != nil {
		c.log().Error("SETNX 失败", slog.String("key", c.Key(key)), slog.String("err", err.Error()))
		return false
	}
	return ok
}

// Delete 删除一个或多个键。
func (c *Client) Delete(ctx context.Context, keys ...string) bool {
	if len(keys) == 0 {
		return true
	}
	full := make([]string, len(keys))
	for i, k := range keys {
		full[i] = c.Key(k)
	}
	if err := c.Client.Del(ctx, full...).Err(); err != nil {
		c.log().Error("删除缓存失败", slog.String("keys", c.Key(keys[0])), slog.String("err", err.Error()))
		return false
	}
	return true
}

// IncrWithTTL 自增计数并在**首次创建时**设置过期时间。
//
// 用于登录失败计数：必须在键首次出现时就设 TTL，否则计数永不过期，
// 用户会被永久锁定。用 Pipeline 保证 INCR 与 EXPIRE 的原子提交，
// 避免 INCR 成功后进程崩溃导致留下一个无 TTL 的键。
func (c *Client) IncrWithTTL(ctx context.Context, key string, ttl time.Duration) (int64, bool) {
	full := c.Key(key)
	pipe := c.Client.TxPipeline()
	incr := pipe.Incr(ctx, full)
	// EXPIRE 每次刷新：与 Python 端 `set(error_key, n, expire=lock_time*60)` 的
	// 行为一致——每次失败都重置锁定窗口（滑动而非固定）
	pipe.Expire(ctx, full, ttlOrNil(ttl))
	if _, err := pipe.Exec(ctx); err != nil {
		c.log().Error("计数自增失败", slog.String("key", full), slog.String("err", err.Error()))
		return 0, false
	}
	return incr.Val(), true
}

// Exists 判断键是否存在。
func (c *Client) Exists(ctx context.Context, key string) bool {
	n, err := c.Client.Exists(ctx, c.Key(key)).Result()
	if err != nil {
		c.log().Error("检查键存在失败", slog.String("key", c.Key(key)), slog.String("err", err.Error()))
		return false
	}
	return n > 0
}

// TTL 返回键的剩余存活**秒数**，对应 Python 端 RedisUtils.ttl。
//
// 负值语义与 Redis 原生命令一致：-1 = 键存在但未设过期，-2 = 键不存在。
// 出错时也返回 -1（与 Python 端 except 分支相同）——调用方的 `0 < ttl` 判断
// 会把这三种情况一并排除，无需额外分支。
func (c *Client) TTL(ctx context.Context, key string) int64 {
	d, err := c.Client.TTL(ctx, c.Key(key)).Result()
	if err != nil {
		c.log().Error("获取过期时间失败", slog.String("key", c.Key(key)), slog.String("err", err.Error()))
		return -1
	}
	// go-redis 把 Redis 的 -1/-2 原样存成 -1ns/-2ns（见 DurationCmd.readReply），
	// 不做秒→Duration 换算，故负值可直接当秒数返回
	if d < 0 {
		return int64(d)
	}
	return int64(d / time.Second)
}

// Expire 重设键的过期时间，对应 Python 端 RedisUtils.expire。
// 用于登录态的滑动续期；ttl <= 0 时不做任何事（EXPIRE 0 会直接删键）。
func (c *Client) Expire(ctx context.Context, key string, ttl time.Duration) bool {
	if ttl <= 0 {
		return false
	}
	if err := c.Client.Expire(ctx, c.Key(key), ttl).Err(); err != nil {
		c.log().Error("设置过期时间失败", slog.String("key", c.Key(key)), slog.String("err", err.Error()))
		return false
	}
	return true
}

// ttlOrNil 把 ttl <= 0 映射为 redis.KeepTTL 之外的「永不过期」语义。
//
// go-redis 中 expiration=0 表示不设过期，与 Python 端 `expire=None` 一致。
func ttlOrNil(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return 0
	}
	return ttl
}
