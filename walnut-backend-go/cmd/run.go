package cmd

import (
	"context"
	"errors"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"walnut-seed-go/internal/bootstrap"
	"walnut-seed-go/internal/config"
)

// shutdownTimeout 优雅停机的收尾时限。
//
// ⚠️ compose 的 stop_grace_period 必须**大于**此值，否则容器会在收尾途中被 SIGKILL。
// Java 端把宽限期放宽到 35s 对应其 30s 的 Spring 收尾；Go 侧 15s 足够排空在途请求，
// compose 中相应设置 stop_grace_period: 20s。
const shutdownTimeout = 15 * time.Second

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "启动 HTTP 服务",
	Long: "启动 WalnutSeed Go 后端 HTTP 服务。\n" +
		"启动顺序：装载配置 → 初始化日志 → 安全校验（不通过则拒绝启动）→\n" +
		"建库与连接 DB/Redis（失败仅告警）→ 注册路由 → 路由认证审计 → 监听端口。",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runServer()
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func runServer() error {
	cfg, err := config.Load(resolveEnv())
	if err != nil {
		return fmt.Errorf("配置装载失败: %w", err)
	}

	app, err := bootstrap.New(cfg)
	if err != nil {
		// 安全校验失败、路由审计失败等致命问题在此拦截，服务不对外暴露
		return err
	}

	// 监听终止信号：SIGINT（Ctrl-C）与 SIGTERM（docker stop / k8s 滚动更新）
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Run(ctx)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		// 收到信号：给应用一个有时限的收尾窗口
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		done := make(chan error, 1)
		go func() { done <- app.Shutdown(shutdownCtx) }()

		select {
		case err := <-done:
			if err != nil {
				return err
			}
			// Shutdown 完成后 app.Run 会因 http.ErrServerClosed 正常返回，
			// 排空 errCh 避免 goroutine 泄漏（非阻塞）
			select {
			case runErr := <-errCh:
				if runErr != nil && !errors.Is(runErr, context.Canceled) {
					return runErr
				}
			default:
			}
			return nil
		case <-time.After(shutdownTimeout + 2*time.Second):
			return errors.New("优雅停机超时，强制退出（可能有在途请求被中断）")
		}
	}
}
