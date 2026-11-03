package cmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"walnut-seed-go/internal/config"
	"walnut-seed-go/internal/pkg/databasex"
	"walnut-seed-go/internal/pkg/migratex"
)

// migrationsDir 迁移脚本目录，可用 --dir 覆盖。
// 容器内镜像把它放在 /walnut-backend/migrations（见 Dockerfile 的 COPY）。
var migrationsDir string

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "数据库迁移（对应 Python 端 main.py upgrade/downgrade/current）",
	Long: "管理版本化 schema 迁移。\n\n" +
		"Schema 的唯一事实源是 migrations/ 下的 SQL 文件，**生产环境严禁使用 GORM AutoMigrate**——\n" +
		"它会绕过迁移历史直接改表，造成三端 schema 漂移且无法回溯。\n\n" +
		"子命令：up / down / current / force",
}

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "应用全部未执行的迁移",
	Long: "按版本号顺序应用所有待执行迁移。\n\n" +
		"执行前会自动引导建库（Go 的 mysql 驱动没有 JDBC 的 createDatabaseIfNotExist，\n" +
		"而中间件编排的 MYSQL_DATABASE 只能建一个库，故由 databasex.EnsureDatabase 负责）。",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMigrate(func(r *migratex.Runner) error { return r.Up() }, "已应用全部迁移")
	},
}

var migrateDownSteps int

var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "回滚迁移",
	Long: "回滚迁移。默认回滚**全部**（会删表）。\n\n" +
		"⚠️ MySQL 的 DDL 非事务性：回滚中途失败无法自动恢复，需人工检查残留。\n" +
		"用 -r N 只回滚 N 个版本（N 为负数表示向下，与 Alembic 的 -r -1 语义一致）。",
	RunE: func(cmd *cobra.Command, args []string) error {
		if migrateDownSteps == 0 {
			if !assumeYes {
				fmt.Print("⚠️  即将回滚**全部**迁移并删除所有表，确认继续？(y/N) ")
				var ans string
				if _, err := fmt.Scanln(&ans); err != nil || (ans != "y" && ans != "Y") {
					fmt.Println("已取消。")
					return nil
				}
			}
			return runMigrate(func(r *migratex.Runner) error { return r.Down() }, "已回滚全部迁移")
		}
		n := migrateDownSteps
		if n > 0 {
			n = -n // 统一成负数表示向下
		}
		return runMigrate(func(r *migratex.Runner) error { return r.Steps(n) },
			fmt.Sprintf("已回滚 %d 个版本", -n))
	},
}

var migrateCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "查看当前迁移版本与 dirty 状态",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadCfg()
		if err != nil {
			return err
		}
		if err := ensureDB(cfg); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		r, err := newRunner(ctx, cfg)
		if err != nil {
			return err
		}
		defer func() { _ = r.Close() }()

		version, dirty, err := r.Version()
		if errors.Is(err, migratex.ErrNilVersion) {
			fmt.Printf("数据库 %s 尚未执行任何迁移（无 schema_migrations 记录）\n", cfg.DatabaseName)
			return nil
		}
		if err != nil {
			return err
		}
		fmt.Printf("数据库 %-20s 版本 %d", cfg.DatabaseName, version)
		if dirty {
			fmt.Printf("  ⚠️ DIRTY（上次迁移中途失败，需人工核对实际结构后用 migrate force -v N 清理）")
		}
		fmt.Println()
		return nil
	},
}

var migrateForceVersion int

var migrateForceCmd = &cobra.Command{
	Use:   "force",
	Short: "强制设置版本号（清理 dirty 状态，不执行任何 SQL）",
	Long: "仅修改 schema_migrations 表里的版本标记，**不执行任何 DDL**。\n\n" +
		"用途：某次迁移中途失败导致 dirty=true 后，人工核对实际表结构，\n" +
		"确认它等同于某个版本，再用本命令把标记修正过去。\n\n" +
		"⚠️ 用错版本号会让后续迁移建立在错误的基线上，务必先核对。",
	RunE: func(cmd *cobra.Command, args []string) error {
		if migrateForceVersion < 0 {
			return errors.New("-v 不能为负数（用 -v 0 表示清空版本记录）")
		}
		return runMigrate(func(r *migratex.Runner) error { return r.Force(migrateForceVersion) },
			fmt.Sprintf("已强制标记为版本 %d", migrateForceVersion))
	},
}

// assumeYes 跳过交互确认（CI / 容器 entrypoint 场景）。
var assumeYes bool

func init() {
	migrateCmd.PersistentFlags().StringVar(&migrationsDir, "dir", "migrations",
		"迁移脚本目录（容器内为 /walnut-backend/migrations）")
	migrateCmd.PersistentFlags().BoolVarP(&assumeYes, "yes", "y", false,
		"跳过交互确认（用于 CI 与容器 entrypoint）")

	migrateDownCmd.Flags().IntVarP(&migrateDownSteps, "revision", "r", 0,
		"回滚的版本数；0 表示全部回滚（会删表）")
	migrateForceCmd.Flags().IntVarP(&migrateForceVersion, "version", "v", -1,
		"要强制标记的版本号")
	_ = migrateForceCmd.MarkFlagRequired("version")

	migrateCmd.AddCommand(migrateUpCmd, migrateDownCmd, migrateCurrentCmd, migrateForceCmd)
	rootCmd.AddCommand(migrateCmd)
}

// loadCfg 装载配置（CLI 命令共用）。
func loadCfg() (*config.Config, error) {
	cfg, err := config.Load(resolveEnv())
	if err != nil {
		return nil, fmt.Errorf("配置装载失败: %w", err)
	}
	return cfg, nil
}

// dbParams 从配置构造数据库参数。
func dbParams(cfg *config.Config) databasex.Params {
	return databasex.Params{
		Host:        cfg.DatabaseHost,
		Port:        cfg.DatabasePort,
		User:        cfg.DatabaseUser,
		Password:    cfg.DatabasePassword,
		Name:        cfg.DatabaseName,
		PoolSize:    cfg.PoolSize,
		MaxOverflow: cfg.MaxOverflow,
		PoolTimeout: cfg.PoolTimeout,
		PoolRecycle: cfg.PoolRecycle,
		Echo:        cfg.DatabaseEcho,
	}
}

// ensureDB 引导建库。
func ensureDB(cfg *config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := databasex.EnsureDatabase(ctx, dbParams(cfg)); err != nil {
		return fmt.Errorf("建库引导失败: %w", err)
	}
	return nil
}

// newRunner 建立迁移器（含迁移专用连接，Close 时一并释放）。
func newRunner(ctx context.Context, cfg *config.Config) (*migratex.Runner, error) {
	return migratex.Open(ctx, dbParams(cfg), migrationsDir)
}

// runMigrate 是各迁移子命令的公共骨架：装载配置 → 建库 → 执行 → 报告版本。
func runMigrate(action func(*migratex.Runner) error, successMsg string) error {
	cfg, err := loadCfg()
	if err != nil {
		return err
	}
	if err := ensureDB(cfg); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	r, err := newRunner(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := r.Close(); cerr != nil {
			fmt.Println("⚠️ 关闭迁移连接异常:", cerr)
		}
	}()

	if err := action(r); err != nil {
		return fmt.Errorf("迁移失败: %w", err)
	}

	version, dirty, verr := r.Version()
	if verr != nil && !errors.Is(verr, migratex.ErrNilVersion) {
		return fmt.Errorf("%s，但读取版本号失败: %w", successMsg, verr)
	}
	if dirty {
		return fmt.Errorf("%s，但数据库处于 DIRTY 状态（版本 %d）——"+
			"说明有迁移中途失败，请人工核对实际表结构后用 migrate force -v N 修正",
			successMsg, version)
	}
	fmt.Printf("✅ %s（数据库 %s，当前版本 %d）\n", successMsg, cfg.DatabaseName, version)
	return nil
}
