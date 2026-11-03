// Package bootstrap 是应用组装根：配置 → 日志 → 安全校验 → DB/Redis → 路由 → 生命周期。
//
// 对应 Python 端 app/init_app.py 的 create_app + lifespan。
// 依赖方向单向向内：bootstrap 依赖 config 与 internal/pkg，反之不成立。
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"walnut-seed-go/internal/config"
	"walnut-seed-go/internal/module/common/health"
	"walnut-seed-go/internal/pkg/cachex"
	"walnut-seed-go/internal/pkg/databasex"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/i18nx"
	"walnut-seed-go/internal/pkg/idgen"
	"walnut-seed-go/internal/pkg/logx"
	"walnut-seed-go/internal/pkg/middleware"
	"walnut-seed-go/internal/pkg/migratex"
	"walnut-seed-go/internal/pkg/seedx"
)

// App 持有全部运行时依赖，负责启动与优雅停机。
type App struct {
	cfg    *config.Config
	log    *slog.Logger
	engine *gin.Engine
	srv    *http.Server

	db    *gorm.DB
	cache *cachex.Client

	errHandler *errs.Handler
	health     *health.Service
	// idGen 雪花 ID 生成器。业务模块通过 idgen.Default() 使用全局单例，
	// 此处保留引用是为了让 App 成为其生命周期的持有者（便于测试与诊断）
	idGen *idgen.Generator
	// builder 保留路由登记表，供启动期认证审计与契约 diff 使用
	builder *Builder
	// logCloser 关闭日志文件句柄；不关会导致 Windows 上目录无法删除、
	// Linux 上轮转后的旧文件无法回收
	logCloser io.Closer
	// shutdownOnce 保证收尾只执行一次（信号处理与 HTTP 服务退出可能各触发一次）
	shutdownOnce sync.Once
}

// New 完成全部装配。任何**致命**问题在此返回 error（拒绝启动），
// 可降级问题只记告警——对齐 Python 端 lifespan 的容错策略：
// DB/Redis 不可用时应用仍要起来，由 /common/health/ready 返回 503 反映真实状态，
// 让 compose 的探针门禁去拦流量，而不是让容器反复重启。
func New(cfg *config.Config) (*App, error) {
	if cfg == nil {
		return nil, errors.New("配置不能为空")
	}

	// ---- 1. 日志（最先初始化，后续所有步骤都要用它）----
	logger, logCloser, err := logx.Setup(logx.Options{
		Level:       cfg.LoggerLevel,
		Dir:         resolveLogDir(cfg),
		Filename:    cfg.LogFilename,
		MaxSizeMB:   cfg.LogMaxSizeMB,
		MaxBackups:  cfg.LogMaxBackups,
		MaxAgeDays:  cfg.LogMaxAgeDays,
		ConsoleJSON: cfg.Environment.IsProd(), // 生产统一 JSON 便于采集，开发保留可读 text
		ServiceName: "walnut-seed-go",
	})
	if err != nil {
		return nil, fmt.Errorf("日志初始化失败: %w", err)
	}

	// ---- 2. 启动期安全校验（必须在注册加解密中间件之前，因为校验可能把它关掉）----
	res, err := cfg.ValidateSecurity()
	if err != nil {
		closeQuietly(logCloser)
		return nil, err // 生产环境密钥不安全 → fail-fast，拒绝启动
	}
	for _, w := range res.Warnings {
		logger.Warn(w.Message, slog.String("code", w.Code))
	}

	app := &App{
		cfg:       cfg,
		log:       logger,
		logCloser: logCloser,
		errHandler: &errs.Handler{
			// 生产环境不外泄 detail，对齐 Python 端 ENVIRONMENT != PROD 的判断
			ExposeDetail: !cfg.Environment.IsProd(),
			Logger:       logger,
			// i18n：按请求的 Accept-Language 解析 AuthErrorCode 的消息键
			// （如 user.password.not.match）。未命中时 i18nx 返回键本身，
			// errs 层会回退到 AuthErrorCode.Fallback（中文兜底文案）。
			Resolve:  i18nx.MessageIn,
			LocaleOf: middleware.LocaleFrom,
		},
	}

	// ---- 3. 雪花 ID 生成器（业务写入都依赖它，须在路由注册前就绪）----
	// 配置优先；未配置时回退「本机 IP + PID」派生并告警一次（与 Python 端同规则）
	idGen, err := idgen.NewAuto(cfg.SnowflakeWorkerID, logger)
	if err != nil {
		app.Close()
		return nil, fmt.Errorf("雪花 ID 生成器初始化失败: %w", err)
	}
	idgen.SetDefault(idGen)
	app.idGen = idGen

	// ---- 4. 数据库（失败仅告警，不阻断启动）----
	app.connectDatabase()

	// ---- 5. Redis（失败仅告警）----
	app.connectRedis()

	// ---- 6. Gin 引擎与路由 ----
	if err := app.buildEngine(); err != nil {
		app.Close()
		return nil, err
	}

	// ---- 7. 路由认证审计（白名单外缺认证即拒绝启动）----
	if err := app.AuditRoutes(); err != nil {
		app.Close()
		return nil, err
	}

	app.srv = &http.Server{
		Addr:    cfg.Addr(),
		Handler: app.engine,
		// 读超时不设或设很大：SSE 是长连接，短的 ReadTimeout 会把它掐断。
		// 写超时同理。空闲连接超时用于回收 keep-alive 连接。
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       120 * time.Second,
		// 请求体大小上限（对应 MAX_REQUEST_SIZE）
		MaxHeaderBytes: 1 << 20,
	}
	return app, nil
}

// connectDatabase 引导建库并建立连接。
func (a *App) connectDatabase() {
	params := databasex.Params{
		Host:        a.cfg.DatabaseHost,
		Port:        a.cfg.DatabasePort,
		User:        a.cfg.DatabaseUser,
		Password:    a.cfg.DatabasePassword,
		Name:        a.cfg.DatabaseName,
		PoolSize:    a.cfg.PoolSize,
		MaxOverflow: a.cfg.MaxOverflow,
		PoolTimeout: a.cfg.PoolTimeout,
		PoolRecycle: a.cfg.PoolRecycle,
		Echo:        a.cfg.DatabaseEcho,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Go 的 mysql 驱动不支持 createDatabaseIfNotExist，必须显式引导
	if err := databasex.EnsureDatabase(ctx, params); err != nil {
		a.log.Warn("⚠️ 数据库建库引导失败，跳过数据库初始化", slog.String("err", err.Error()))
		return
	}
	db, err := databasex.Open(ctx, params)
	if err != nil {
		a.log.Warn("⚠️ 数据库连接失败，跳过数据库初始化", slog.String("err", err.Error()))
		return
	}
	a.db = db
	a.log.Info("✅ 数据库连接初始化完成", slog.String("database", a.cfg.DatabaseName))

	// ---- schema 迁移与种子数据 ----
	// 语义与 Python 端 init_app.lifespan 一致：
	//   - DATABASE_AUTO_MIGRATE=true（dev）→ 应用内执行迁移
	//   - prod 由 docker-entrypoint.sh 在应用启动**之前**执行 migrate up（fail-fast），
	//     此处 AutoMigrate=false 跳过，避免多副本滚动发布时的迁移竞争
	//   - **种子数据始终尝试写入**（在 AutoMigrate 判断之外），seedx 自带幂等守卫
	//   - 整段失败**只告警不阻断启动**，由 /common/health/ready 反映真实可用性
	migCtx, migCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer migCancel()

	if a.cfg.DatabaseAutoMigrate {
		if err := a.runMigrations(migCtx); err != nil {
			a.log.Warn("⚠️ 数据库迁移失败，schema 可能过期", slog.String("err", err.Error()))
		} else {
			a.log.Info("✅ 数据库迁移已应用到最新版本")
		}
	}

	if err := a.runSeed(migCtx); err != nil {
		a.log.Warn("⚠️ 种子数据初始化跳过/失败", slog.String("err", err.Error()))
	}
}

// runMigrations 应用全部待执行迁移。
//
// ctx 只用于建立迁移连接：golang-migrate 的迁移执行 API 本身没有 ctx 版本，
// 执行期的超时只能靠 DSN 连接参数与 MySQL 侧的 wait_timeout 兜底。
func (a *App) runMigrations(ctx context.Context) error {
	r, err := migratex.Open(ctx, a.dbParams(), a.cfg.MigrationsDir)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	if err := r.Up(); err != nil {
		return err
	}
	// 迁移后必须确认非 dirty：MySQL DDL 非事务，中途失败会留下部分表，
	// 带着这种状态继续服务比直接失败更危险
	if version, dirty, verr := r.Version(); verr == nil && dirty {
		return fmt.Errorf("迁移后数据库处于 DIRTY 状态（版本 %d），需人工核对表结构后用 "+
			"`migrate force -v N` 修正", version)
	}
	return nil
}

// dbParams 从配置构造数据库参数。
func (a *App) dbParams() databasex.Params {
	return databasex.Params{
		Host:        a.cfg.DatabaseHost,
		Port:        a.cfg.DatabasePort,
		User:        a.cfg.DatabaseUser,
		Password:    a.cfg.DatabasePassword,
		Name:        a.cfg.DatabaseName,
		PoolSize:    a.cfg.PoolSize,
		MaxOverflow: a.cfg.MaxOverflow,
		PoolTimeout: a.cfg.PoolTimeout,
		PoolRecycle: a.cfg.PoolRecycle,
		Echo:        a.cfg.DatabaseEcho,
	}
}

// runSeed 写入种子数据（幂等）。
func (a *App) runSeed(ctx context.Context) error {
	if a.db == nil {
		return fmt.Errorf("数据库未初始化")
	}
	res, err := seedx.Run(ctx, a.db, a.cfg.SeedFile)
	if err != nil {
		return err
	}
	if res.Skipped {
		a.log.Debug("系统表已存在数据，跳过种子数据初始化",
			slog.Int64("existing_rows", res.Existing))
		return nil
	}
	a.log.Info("✅ 种子数据初始化完成",
		slog.Int("inserts", res.Executed),
		slog.String("file", a.cfg.SeedFile))
	return nil
}

// connectRedis 建立 Redis 连接。
func (a *App) connectRedis() {
	a.cache = cachex.New(cachex.Params{
		Host:                a.cfg.RedisHost,
		Port:                a.cfg.RedisPort,
		DB:                  a.cfg.RedisDBName,
		Username:            a.cfg.RedisUser,
		Password:            a.cfg.RedisPassword,
		KeyPrefix:           a.cfg.RedisKeyPrefix,
		HealthCheckInterval: a.cfg.RedisHealthCheckInterval,
	})

	// 探测超时设 2s 而非更长：Redis 不可达时 go-redis 会重试多次拨号，
	// 超时的 ctx 能截断这个重试循环，避免启动（和测试）被无谓拖慢。
	// 启动探测失败只是告警，运行期 /common/health/ready 会继续反映真实状态。
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := a.cache.Ping(ctx); err != nil {
		// 保留 client 实例（不置 nil）：Redis 恢复后连接池会自动重连，
		// 与 Python 端 redis_connect 失败后仍保留对象的行为一致
		a.log.Warn("⚠️ Redis 未连接（认证/缓存/SSE 订阅将不可用）", slog.String("err", err.Error()))
		return
	}
	a.log.Info("✅ Redis 连接初始化完成",
		slog.String("addr", fmt.Sprintf("%s:%d", a.cfg.RedisHost, a.cfg.RedisPort)),
		slog.String("key_prefix", a.cfg.RedisKeyPrefix),
	)
}

// Run 启动 HTTP 服务并阻塞，直到 ctx 取消或服务自身异常退出。
//
// ⚠️ **Run 不负责停机**：ctx 取消时它只是返回，Shutdown 必须由调用方显式调用。
// 早期版本在这里也调了一次 Shutdown，与 cmd/run.go 的调用重复，导致 SIGTERM 时
// 停机日志打印两遍、并对已关闭的 Redis client 二次 Close 报 "client is closed"。
// 职责划分：Run = 提供 HTTP 服务；Shutdown = 调用方掌握的收尾动作（它需要控制超时窗口）。
func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		a.log.Info(fmt.Sprintf("🎉 WalnutSeed Go 后端已启动: http://%s", a.cfg.Addr()),
			slog.String("environment", string(a.cfg.Environment)),
			slog.String("version", a.cfg.Version),
		)
		// ListenAndServe 正常关闭时返回 http.ErrServerClosed，不当作错误
		if err := a.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("HTTP 服务异常退出: %w", err)
		}
		return nil
	case <-ctx.Done():
		return nil // 交由调用方执行 Shutdown
	}
}

// Shutdown 优雅停机：停止接收新请求 → 等在途请求收尾 → 关连接池 → 关日志句柄。
//
// 幂等：重复调用是安全的（sync.Once 保护），因为信号处理与 HTTP 服务退出可能
// 从不同路径各触发一次收尾。
//
// ⚠️ compose 的 stop_grace_period 必须大于这里传入的 ctx 超时，
// 否则容器会在收尾途中被 SIGKILL（Java 端为此把宽限期放宽到了 35s；
// Go 侧应用收尾为 15s，compose 设 20s）。
func (a *App) Shutdown(ctx context.Context) error {
	a.shutdownOnce.Do(func() {
		a.log.Info("开始优雅停机...")

		if a.srv != nil {
			if err := a.srv.Shutdown(ctx); err != nil {
				a.log.Error("HTTP 服务停机异常", slog.String("err", err.Error()))
			}
		}
		a.closeResources()
		a.log.Info("✅ 应用已优雅关闭")
		// 日志句柄最后关闭：前面的停机日志还要写进去
		closeQuietly(a.logCloser)
		a.logCloser = nil
	})
	return nil
}

// closeResources 释放 DB 与 Redis 连接池。装配失败时也要调用，避免句柄泄漏。
//
// 幂等：关闭后立即置 nil，避免二次 Close 触发 "redis: client is closed" 这类噪音告警。
//
// ⚠️ **并发前提**：本方法会把 a.db / a.cache 置 nil，而健康探针会并发读它们。
// 安全性依赖调用顺序——Shutdown 里 `srv.Shutdown(ctx)` **先**返回（它关闭监听并等在途
// 请求跑完），之后才调本方法，故此时已无 handler 在跑。
// 若在服务仍在处理请求时直接调用本方法（例如测试里的 Close），就构成 data race（见计划 R8）。
func (a *App) closeResources() {
	if a.cache != nil {
		cache := a.cache
		a.cache = nil
		if err := cache.Close(); err != nil {
			a.log.Warn("Redis 连接关闭异常", slog.String("err", err.Error()))
		}
	}
	if a.db != nil {
		db := a.db
		a.db = nil
		if err := databasex.Close(db); err != nil {
			a.log.Warn("数据库连接关闭异常", slog.String("err", err.Error()))
		}
	}
}

// closeQuietly 关闭资源并忽略错误——用于装配失败的回滚路径，
// 此时不应让清理错误掩盖真正的失败原因。
func closeQuietly(c io.Closer) {
	if c != nil {
		_ = c.Close()
	}
}

// Engine 暴露 gin 引擎，供测试（httptest）使用。
func (a *App) Engine() *gin.Engine { return a.engine }

// RegisteredRoutes 返回路由登记表快照，供启动期审计与**契约路由清单 diff** 使用。
//
// 契约 diff 用它把 Go 的路由面与 api/openapi/routes.txt（Python 端导出的 golden）
// 对比：Go 出现 Python 没有的路由 = 私自增删接口造成漂移，必须硬失败。
func (a *App) RegisteredRoutes() []RecordedRoute {
	if a.builder == nil {
		return nil
	}
	return a.builder.Routes()
}

// Close 释放全部资源，用于**未启动 HTTP 服务**的场景（如测试中只做装配与 httptest）。
// 正常停机走 Shutdown，它会先优雅关闭 HTTP 服务再调本方法做的事。
func (a *App) Close() error {
	a.closeResources()
	closeQuietly(a.logCloser)
	a.logCloser = nil
	return nil
}

// resolveLogDir 解析日志目录。
//
// 配置为空时回退到 <repo>/data/logs —— 与 Python 端一致：本地直接启动时日志写入
// 仓库根目录 data/logs/，Docker 启动时由 compose 挂载并显式设置 LOG_DIR。
// 从 walnut-backend-go/ 运行时，仓库根即上一级目录。
func resolveLogDir(cfg *config.Config) string {
	if cfg.LogDir != "" {
		return cfg.LogDir
	}
	if cfg.Environment.IsProd() {
		return "" // 生产不写文件，交给 stdout 采集
	}
	return filepath.Join("..", "data", "logs")
}
