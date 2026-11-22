package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"walnut-seed-go/internal/bootstrap"
	"walnut-seed-go/internal/config"
)

var routesOut string

var routesCmd = &cobra.Command{
	Use:   "routes",
	Short: "导出全局路由清单（Markdown）",
	Long: "装配应用并把路由登记表导出为 Markdown 清单，按模块前缀分组。\n" +
		"纯静态导出：固定 dev 装配且 DB/Redis 指向不可达地址，不触碰真实基础设施。",
	RunE: func(cmd *cobra.Command, args []string) error {
		return exportRoutes(routesOut)
	},
}

func init() {
	routesCmd.Flags().StringVar(&routesOut, "out", "../docs/go/路由清单.md",
		"输出文件路径（相对运行目录 walnut-backend-go/）；\"-\" 表示输出到 stdout")
	rootCmd.AddCommand(routesCmd)
}

func exportRoutes(out string) error {
	// 与 test/contract 的 newTestApp 同款装配：路由清单只需要登记表，
	// 不需要真实 DB/Redis。指向不可达地址既加速装配，也杜绝 dev 环境
	// AutoMigrate/seed 对真实库的副作用——导出工具绝不能写库。
	for k, v := range map[string]string{
		"ENVIRONMENT":         "dev",
		"LOG_DIR":             os.TempDir(),
		"DEBUG":               "false",
		"LOGGER_LEVEL":        "ERROR",
		"DATABASE_HOST":       "127.0.0.1",
		"DATABASE_PORT":       "1",
		"REDIS_HOST":          "127.0.0.1",
		"REDIS_PORT":          "1",
		"SNOWFLAKE_WORKER_ID": "",
	} {
		_ = os.Setenv(k, v)
	}

	cfg, err := config.Load("dev")
	if err != nil {
		return fmt.Errorf("配置装载失败: %w", err)
	}
	app, err := bootstrap.New(cfg)
	if err != nil {
		return err
	}
	routes := app.RegisteredRoutes()
	_ = app.Close()

	md, public, protected := renderRoutesMarkdown(routes)
	if out == "-" {
		fmt.Print(md)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}
	if err := os.WriteFile(out, []byte(md), 0o644); err != nil {
		return fmt.Errorf("写入路由清单失败: %w", err)
	}
	fmt.Printf("路由清单已生成：%s（共 %d 条：公开 %d / 受保护 %d）\n",
		out, len(routes), public, protected)
	return nil
}

// renderRoutesMarkdown 按模块前缀分组渲染。输出不含时间戳，保证重复生成
// 内容确定——清单进 git 后 diff 只反映真实路由变化，不反映生成时刻。
func renderRoutesMarkdown(routes []bootstrap.RecordedRoute) (md string, public, protected int) {
	groups := make(map[string][]bootstrap.RecordedRoute)
	for _, r := range routes {
		g := routeGroup(r.Path)
		groups[g] = append(groups[g], r)
		if r.Authed {
			protected++
		} else {
			public++
		}
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("# 路由清单\n\n")
	b.WriteString("> 生成物，勿手改。运行 `make routes`（或 `go run . routes`）重新生成。\n")
	b.WriteString("> 数据来源：bootstrap.Builder 路由登记表；认证归属以注册分组为准\n")
	b.WriteString("> （Public = 公开白名单，Protected = 受保护 JWT）。\n")
	b.WriteString("> 端点的权限码（permguard）与防重守卫见各模块 routes.go，不在本清单。\n\n")
	fmt.Fprintf(&b, "共 %d 条：公开 %d 条，受保护 %d 条。\n", len(routes), public, protected)

	for _, k := range keys {
		rs := groups[k]
		sort.Slice(rs, func(i, j int) bool {
			if rs[i].Path != rs[j].Path {
				return rs[i].Path < rs[j].Path
			}
			return rs[i].Method < rs[j].Method
		})
		fmt.Fprintf(&b, "\n## %s（%d 条）\n\n", k, len(rs))
		b.WriteString("| 方法 | 路径 | 认证 |\n|---|---|---|\n")
		for _, r := range rs {
			auth := "公开"
			if r.Authed {
				auth = "受保护"
			}
			fmt.Fprintf(&b, "| %s | %s | %s |\n", r.Method, r.Path, auth)
		}
	}
	return b.String(), public, protected
}

// routeGroupKeys 与 bootstrap/router.go 的挂载前缀保持一致，新增挂载点时同步此表。
// 不用「前两段」启发式：/auth/login 会被拆成单条组，/upload/*file_path 会把通配符带进组名。
var routeGroupKeys = []string{
	"/system/user", "/system/menu", "/system/post", "/system/dept",
	"/system/dict", "/system/config", "/system/notice", "/system/client",
	"/system/social", "/system/role",
	"/common/health", "/common/file",
	"/auth", "/monitor", "/upload", "/resource",
}

// routeGroup 按挂载前缀最长匹配分组；未命中回退「前两段」规则，根路径归 "/"。
func routeGroup(path string) string {
	if path == "/" {
		return "/"
	}
	best := ""
	for _, p := range routeGroupKeys {
		if (path == p || strings.HasPrefix(path, p+"/")) && len(p) > len(best) {
			best = p
		}
	}
	if best != "" {
		return best
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 {
		return "/" + parts[0]
	}
	return "/" + parts[0] + "/" + parts[1]
}
