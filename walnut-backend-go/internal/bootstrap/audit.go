package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"walnut-seed-go/internal/pkg/databasex"
)

// AuditRoutes 启动期路由认证审计，对应 Python 端 init_app.audit_routes_auth。
//
// 规则：白名单（WHITE_API_LIST_PATH）之外的路由必须携带认证，违规**直接启动失败**。
// 这是 fail-fast 的强安全约束，杜绝「漏挂认证中间件导致接口裸奔」。
//
// 比 Python 端多做一层：除了校验经 Builder 登记的路由，还反向比对
// engine.Routes() 的全集，捕获任何绕过 Builder（直接调 engine.GET 等）注册的路由。
// 否则日后新增模块时忘记走 Builder，审计就会静默失效。
func (a *App) AuditRoutes() error {
	if a.builder == nil {
		return fmt.Errorf("路由审计失败：Builder 未初始化")
	}

	recorded := a.builder.Routes()
	violations := make([]string, 0)

	// ---- 1. 已登记路由：公开组的路由必须命中白名单 ----
	recordedSet := make(map[string]struct{}, len(recorded))
	for _, r := range recorded {
		key := r.Method + " " + r.Path
		recordedSet[key] = struct{}{}
		if r.Authed {
			continue
		}
		if !a.cfg.IsWhiteListed(r.Path) {
			violations = append(violations, fmt.Sprintf("%s（登记为公开但不在白名单）", key))
		}
	}

	// ---- 2. 引擎实际路由：任何未登记的路由都视为可疑 ----
	// gin 注册路由时会自动生成 HEAD（用于 GET），这类派生路由按同路径的登记状态判定
	for _, ri := range a.engine.Routes() {
		key := ri.Method + " " + ri.Path
		if _, ok := recordedSet[key]; ok {
			continue
		}
		if ri.Method == "HEAD" {
			if _, ok := recordedSet["GET "+ri.Path]; ok {
				continue
			}
		}
		// 未登记但命中白名单的路由放行（例如 gin 内部派生的重定向路由）
		if a.cfg.IsWhiteListed(ri.Path) {
			continue
		}
		violations = append(violations, fmt.Sprintf("%s（未经 Builder 登记，认证状态未知）", key))
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		return fmt.Errorf(
			"路由认证审计失败：以下路由未携带认证且不在公开白名单（WHITE_API_LIST_PATH，%d 条）内：%s",
			len(a.cfg.WhiteAPIListPath), strings.Join(violations, "; "),
		)
	}

	a.log.Info("✅ 路由认证审计通过",
		slog.Int("whitelist_count", len(a.cfg.WhiteAPIListPath)),
		slog.Int("registered_routes", len(recorded)),
	)
	return nil
}

// checkDatabase 供 /common/health/ready 探测数据库连通性。
func (a *App) checkDatabase(ctx context.Context) error {
	return databasex.Ping(ctx, a.db)
}

// checkRedis 供 /common/health/ready 探测 Redis 连通性。
func (a *App) checkRedis(ctx context.Context) error {
	if a.cache == nil {
		return fmt.Errorf("Redis 未初始化")
	}
	return a.cache.Ping(ctx)
}
