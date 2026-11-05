// Package idgen 实现雪花 ID 生成器，与 Python 端 app/utils/snowflake.py **逐位对齐**。
//
// 位分配（共 64 位）：
//
//	 0 | 41 位时间戳（毫秒，相对 2020-01-01 epoch） | 10 位机器位 | 12 位序列号
//	MSB                                                          LSB
//
//	ID = ((now_ms - EPOCH) << 22) | (workerID << 12) | sequence
//
// ⚠️ 位分配必须与另两端完全一致，否则同一个 ID 在三端会被解析出不同的
// 时间戳/机器位，跨端引用（如日志里记录的 ID）就无法互认。
package idgen

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"
)

const (
	// Epoch 起始时间戳：2020-01-01 00:00:00 UTC 的毫秒值。
	// 与 Python 端 _EPOCH 相同。改动会让新旧 ID 的时间戳段错位。
	Epoch int64 = 1577836800000

	workerIDBits = 10
	sequenceBits = 12

	// MaxWorkerID 机器位上限（1023）。
	MaxWorkerID = (1 << workerIDBits) - 1
	// MaxSequence 同一毫秒内的序列号上限（4095）。
	MaxSequence = (1 << sequenceBits) - 1

	workerIDShift  = sequenceBits                // 12
	timestampShift = sequenceBits + workerIDBits // 22
)

// ErrWorkerIDOutOfRange 机器位越界。
//
// ⚠️ 不能静默掩码：越界值取模后会回绕成**另一个实例**的机器位，
// 两个实例产生相同 ID 却毫无报错，是最难排查的一类故障。
var ErrWorkerIDOutOfRange = errors.New("worker_id 越界")

// Generator 线程安全的雪花 ID 生成器。
//
// 用 sync.Mutex 而非 atomic：next_id 内部是「读时间戳 → 比较 → 更新序列号」
// 的复合操作，需要互斥保证原子性。Python 端同样用 threading.Lock。
type Generator struct {
	workerID int64

	mu            sync.Mutex
	sequence      int64
	lastTimestamp int64
}

// New 用显式机器位构造生成器。
func New(workerID int) (*Generator, error) {
	if workerID < 0 || workerID > MaxWorkerID {
		return nil, fmt.Errorf("%w: 必须在 0-%d 范围内，实际为 %d", ErrWorkerIDOutOfRange, MaxWorkerID, workerID)
	}
	return &Generator{workerID: int64(workerID), lastTimestamp: -1}, nil
}

// NewAuto 按 Python 端 _resolve_worker_id 的规则解析机器位：
// 配置优先；未配置（configured < 0）时回退「本机出口 IP + 进程号」派生并告警一次。
//
// 回退派生是**尽力而为**：crc32(IP) 区分不同机器/容器副本，叠加 PID 区分
// 同一机器上的多个进程。但 IP 可能随 DHCP 或容器重建变化，
// 故正式多实例部署必须显式配置 SNOWFLAKE_WORKER_ID 且各实例互不相同。
func NewAuto(configured *int, logger *slog.Logger) (*Generator, error) {
	if configured != nil {
		return New(*configured)
	}

	warnFallbackOnce(logger)
	return &Generator{workerID: deriveWorkerID(), lastTimestamp: -1}, nil
}

// fallbackWarned 保证回退告警只打一次（对应 Python 端的 _FALLBACK_WARNED 模块级标志）。
//
// ⚠️ 这是**共享可变状态**，而 Go 的初始化与请求处理是真并发的
// （Python 端在单事件循环里读写模块级全局天然安全）。故必须用 sync.Once，
// 否则多协程同时构造生成器会打出重复告警。见实施计划 R8。
var fallbackOnce sync.Once

func warnFallbackOnce(logger *slog.Logger) {
	fallbackOnce.Do(func() {
		msg := "⚠️ SNOWFLAKE_WORKER_ID 未配置，雪花机器位回退为「本机 IP + 进程号」派生；" +
			"多 worker/多副本部署必须显式配置且各实例互不相同"
		if logger != nil {
			logger.Warn(msg)
			return
		}
		slog.Warn(msg)
	})
}

// deriveWorkerID 计算回退机器位：(crc32(本机IP) + PID) & 1023。
func deriveWorkerID() int64 {
	ip := localIP()
	// Go 的 crc32.ChecksumIEEE 与 Python 的 zlib.crc32 是同一算法（CRC-32/ISO-HDLC），
	// 故两端对同一 IP 字符串得到相同校验值——这保证了混部时机器位不撞车
	checksum := crc32.ChecksumIEEE([]byte(ip))
	return int64((checksum + uint32(os.Getpid())) & MaxWorkerID)
}

// localIP 获取本机默认出口 IP，与 Python 端 _local_ip 同一套回退链：
// UDP connect 让内核选路（不实际发包，可离线安全使用）→ 主机名解析 → 主机名本身。
func localIP() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "udp", "114.114.114.114:53")
	if err == nil {
		defer conn.Close()
		if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			return addr.IP.String()
		}
	}
	if host, err := os.Hostname(); err == nil {
		if ips, err := (&net.Resolver{}).LookupIPAddr(ctx, host); err == nil && len(ips) > 0 {
			return ips[0].IP.String()
		}
		return host
	}
	return "unknown"
}

// NextID 生成下一个 ID。
func (g *Generator) NextID() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	timestamp := currentMillis()

	// 时钟回拨：顺延到上次时间戳，避免产生重复 ID。
	// 不报错也不等待——回拨通常很小（NTP 校正），顺延的代价只是 ID 时间戳段略微超前。
	if timestamp < g.lastTimestamp {
		timestamp = g.lastTimestamp
	}

	if timestamp == g.lastTimestamp {
		g.sequence = (g.sequence + 1) & MaxSequence
		if g.sequence == 0 {
			// 同一毫秒内 4096 个 ID 已用尽，自旋等到下一毫秒
			for timestamp <= g.lastTimestamp {
				timestamp = currentMillis()
			}
		}
	} else {
		g.sequence = 0
	}
	g.lastTimestamp = timestamp

	return ((timestamp - Epoch) << timestampShift) | (g.workerID << workerIDShift) | g.sequence
}

// NextIDString 生成 ID 的字符串形式。
//
// 对应 Python 端 IdGeneratorUtil.next_id()。**面向前端的响应应始终用字符串**：
// 雪花 ID 实测 18-19 位、恒超出 JS 的 Number.MAX_SAFE_INTEGER（见 idtype.JSInt），
// 直接给数字会被 JS 抹平尾部精度。
func (g *Generator) NextIDString() string {
	return fmt.Sprintf("%d", g.NextID())
}

// WorkerID 返回当前机器位（诊断用）。
func (g *Generator) WorkerID() int { return int(g.workerID) }

func currentMillis() int64 { return time.Now().UnixMilli() }

// ---- 全局单例与包级便捷函数 ----

var (
	defaultMu   sync.RWMutex
	defaultGen  *Generator
	defaultOnce sync.Once
)

// SetDefault 显式设置全局生成器（应用启动时调用一次）。
//
// 必须在启动期设置：Default() 的惰性兜底用 workerID=0，
// 多副本部署下若忘了 SetDefault，所有实例都会用 0 号机器位而产生 ID 冲突。
//
// 用 RWMutex 保护而非裸赋值：Go 的启动装配与首个请求可能并发，
// 裸写全局指针构成 data race（见实施计划 R8），且 -race 下会直接报错。
func SetDefault(g *Generator) {
	if g == nil {
		return
	}
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultGen = g
}

// Default 返回全局生成器；未显式设置时用 workerID=0 惰性构造。
func Default() *Generator {
	defaultOnce.Do(func() {
		defaultMu.RLock()
		g := defaultGen
		defaultMu.RUnlock()
		if g == nil {
			// 忽略 error：workerID=0 必然合法
			fallback, _ := New(0)
			SetDefault(fallback)
		}
	})
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultGen
}

// NextID 用全局生成器产出 ID。
func NextID() int64 { return Default().NextID() }

// NextIDString 用全局生成器产出 ID 字符串。
func NextIDString() string { return Default().NextIDString() }

// UUID 生成 32 位无横线 UUID，对应 Python 端 IdGeneratorUtil.next_uuid()。
//
// 用途是对象存储的文件 key（{yyyy/MM/dd}/{uuid}.{ext}），不参与数据库主键。
func UUID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand 失败极其罕见，但绝不能返回空串——那会让所有文件写到同一个 key
		// 并相互覆盖。退化为时间戳+PID，保证唯一性底线。
		return fmt.Sprintf("%x%08x", time.Now().UnixNano(), os.Getpid())
	}
	return hex.EncodeToString(buf)
}

// UUIDWithPrefix / IDWithPrefix 对应 Python 端同名的带前缀变体。
func UUIDWithPrefix(prefix string) string { return prefix + UUID() }

func IDWithPrefix(prefix string) string { return prefix + NextIDString() }

// ---- 反解（排障与测试断言用）----

// TimestampFromID 从雪花 ID 反解生成时间。
func TimestampFromID(id int64) time.Time {
	return time.UnixMilli((id >> timestampShift) + Epoch)
}

// WorkerIDFromID 从雪花 ID 反解机器位。
func WorkerIDFromID(id int64) int {
	return int((id >> workerIDShift) & MaxWorkerID)
}

// SequenceFromID 从雪花 ID 反解同毫秒内的序列号。
func SequenceFromID(id int64) int {
	return int(id & MaxSequence)
}
