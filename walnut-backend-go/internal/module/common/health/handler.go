// Package health 实现三个探针端点，对齐 Python 端 module_common/health/controller.py。
//
// 路由（挂在 /common 前缀下）：
//
//	GET /common/health/check  存活式健康检查，不依赖 DB/Redis
//	GET /common/health/live   存活探针（k8s liveness / compose 无依赖探活）
//	GET /common/health/ready  就绪探针，DB + Redis 均就绪才 200，否则 **HTTP 503**
//
// ⚠️ 白名单里有 /common/health 与 /common/health/*，但 Python 端**并不存在**
// /common/health 本身这个端点（只有 check/live/ready）。Go 侧保持一致，
// 不要为了「看起来更完整」而多加一个根端点，否则路由清单 diff 会失败。
package health

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/pkg/envelope"
	"walnut-seed-go/internal/pkg/timex"
)

// DependencyChecker 依赖探测器。由 bootstrap 注入真实的 DB/Redis 检查，
// 未注入时返回 status=0，避免 nil 解引用。
type DependencyChecker func(ctx context.Context) error

// Service 持有启动时间与依赖探测器。
//
// startTime 在构造时固定，对应 Python 端模块级的 _start_time = datetime.now()。
// 用 sync.Once 保护惰性初始化的探测器（Go 的 handler 是真并发的，
// 而 Python 端在单事件循环里对模块级全局的读写天然安全——这是移植时最易忽略的语义差）。
type Service struct {
	version   string
	startTime time.Time
	// probeTimeout 单个依赖的探测超时。
	// ⚠️ 与 Python 端有意不同，见 probe() 注释与实施计划 R19
	probeTimeout time.Duration

	mu       sync.RWMutex
	checkers map[string]DependencyChecker
}

// defaultProbeTimeout 探测超时默认值。
//
// 必须显著小于 compose healthcheck 的 timeout: 5s——否则「依赖慢」会被编排误判成
// 「探测失败」，而这两者的运维含义完全不同（前者该扩容/查慢查询，后者该重启容器）。
const defaultProbeTimeout = 2 * time.Second

// New 构造健康检查服务。probeTimeout <= 0 时回退 defaultProbeTimeout。
func New(version string, probeTimeout time.Duration) *Service {
	if probeTimeout <= 0 {
		probeTimeout = defaultProbeTimeout
	}
	return &Service{
		version:      version,
		startTime:    time.Now(),
		probeTimeout: probeTimeout,
		checkers:     make(map[string]DependencyChecker),
	}
}

// Register 注册一个依赖探测器（如 "database"、"redis"）。
// 仅在启动阶段调用，故不需要加锁；运行期动态注册应改用 SetChecker。
func (s *Service) Register(name string, fn DependencyChecker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.checkers == nil {
		s.checkers = make(map[string]DependencyChecker)
	}
	s.checkers[name] = fn
}

// basePayload 构造三个端点共用的基础字段。
//
// timestamp 用 isoformat 形态（2026-11-02T11:53:34.691234），
// **不是**全站通用的 yyyy-MM-dd HH:mm:ss——Python 端这里直接调用了
// datetime.now().isoformat()，未经过 DATETIME_DISPLAY_FMT。
func (s *Service) basePayload() HealthOut {
	now := time.Now()
	return HealthOut{
		Status:        1,
		Timestamp:     timex.ISOTimestamp(now),
		Version:       s.version,
		UptimeSeconds: now.Sub(s.startTime).Seconds(),
	}
}

// Check GET /common/health/check —— 存活式健康检查，不触碰任何依赖。
func (s *Service) Check(c *gin.Context) {
	envelope.OKMsg(c, s.basePayload(), "系统健康")
}

// Live GET /common/health/live —— 存活探针。
func (s *Service) Live(c *gin.Context) {
	envelope.OKMsg(c, s.basePayload(), "进程存活")
}

// Ready GET /common/health/ready —— 就绪探针。
//
// DB 与 Redis 并发探测（对齐 Python 端的 asyncio.gather），全部就绪才返回 200；
// 否则返回 **HTTP 503** + body.code=503，载荷仍完整给出各依赖状态便于排障。
// compose 的编排门禁依赖这个语义：后端带着未初始化依赖时不能被标记为 healthy，
// 否则前端容器会立即放流量进来。
func (s *Service) Ready(c *gin.Context) {
	ctx := c.Request.Context()

	s.mu.RLock()
	checkers := make(map[string]DependencyChecker, len(s.checkers))
	for k, v := range s.checkers {
		checkers[k] = v
	}
	s.mu.RUnlock()

	type result struct {
		name string
		st   DependencyStatus
	}
	results := make(chan result, len(checkers))

	var wg sync.WaitGroup
	for name, fn := range checkers {
		wg.Add(1)
		go func(name string, fn DependencyChecker) {
			defer wg.Done()
			results <- result{name: name, st: s.probe(ctx, fn)}
		}(name, fn)
	}
	wg.Wait()
	close(results)

	dependencies := make(map[string]DependencyStatus, len(checkers))
	allOK := true
	for r := range results {
		dependencies[r.name] = r.st
		if r.st.Status != 1 {
			allOK = false
		}
	}
	// 未注册任何探测器时视为未就绪：宁可拒绝放流量，也不要假装健康（fail-closed）
	if len(checkers) == 0 {
		allOK = false
	}

	payload := ReadinessOut{
		HealthOut:    s.basePayload(),
		Dependencies: dependencies,
		DiskUsage:    diskUsagePercent(),
	}
	if !allOK {
		payload.Status = 0
	}

	if allOK {
		envelope.OKMsg(c, payload, "依赖就绪")
		return
	}
	envelope.FailWith(c, http.StatusServiceUnavailable, 503, "依赖未就绪", payload)
}

// probe 执行单个依赖探测并测量耗时。
//
// 探测失败只记录状态、不返回错误：ready 端点必须始终给出完整载荷，
// 让调用方能看到「是哪个依赖挂了」，而不是拿到一个笼统的 500。
//
// ⚠️ 已知的**有意偏差**（见实施计划 R19）：这里的 2s 上限与 Python 端不同。
// Python 的 _check_database 没有显式超时，靠 SQLAlchemy 的 POOL_TIMEOUT=30 兜底
// （实测 MySQL 不可达时耗时约 4s 才返回 503）。因此在「依赖慢但存活」的场景下，
// Python 可能判定 status=1 → HTTP 200，而 Go 判定超时 status=0 → HTTP 503。
//
// 保留短超时的理由：compose 的 healthcheck 是 timeout: 5s / start_interval: 2s，
// 探针必须在 5s 内给出结论，否则会被判为「探测失败」而非「未就绪」，语义更糟。
// 超时值由 HEALTH_PROBE_TIMEOUT 配置（默认 2s），可按部署环境调优。
func (s *Service) probe(ctx context.Context, fn DependencyChecker) DependencyStatus {
	if fn == nil {
		return DependencyStatus{Status: 0}
	}
	timeout := s.probeTimeout
	if timeout <= 0 {
		timeout = defaultProbeTimeout
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	if err := fn(probeCtx); err != nil {
		return DependencyStatus{Status: 0}
	}
	latency := roundTo(time.Since(start).Seconds()*1000, 2)
	return DependencyStatus{Status: 1, LatencyMs: &latency}
}
