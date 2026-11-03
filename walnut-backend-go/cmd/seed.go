package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"walnut-seed-go/internal/pkg/databasex"
	"walnut-seed-go/internal/pkg/seedx"
)

// seedFile 种子 SQL 路径，可用 --file 覆盖。
// 容器内镜像把它放在 /walnut-backend/seed/mysql_data.sql（见 Dockerfile 的 COPY）。
var seedFile string

var seedCmd = &cobra.Command{
	Use:   "seed",
	Short: "写入基线种子数据（幂等）",
	Long: "写入默认管理员、客户端、部门、角色、菜单、岗位、字典、参数、通知等基线数据。\n\n" +
		"幂等性双重保证：\n" +
		"  1. seed/mysql_data.sql 每条语句都是 INSERT IGNORE（主键冲突则跳过）\n" +
		"  2. 执行前先 SELECT COUNT(*) FROM sys_user，非空则整体跳过\n" +
		"     （与 Python 端 InitializeData.init_db 的守卫一致）\n\n" +
		"⚠️ 需先执行 migrate up 建好表结构。\n" +
		"⚠️ 基线含 admin/admin123，生产部署后必须立即改密。",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadCfg()
		if err != nil {
			return err
		}
		if err := ensureDB(cfg); err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		db, err := databasex.Open(ctx, dbParams(cfg))
		if err != nil {
			return fmt.Errorf("连接数据库失败: %w", err)
		}
		defer func() {
			if cerr := databasex.Close(db); cerr != nil {
				fmt.Println("⚠️ 关闭数据库连接异常:", cerr)
			}
		}()

		res, err := seedx.Run(ctx, db, seedFile)
		if err != nil {
			return fmt.Errorf("种子数据写入失败: %w", err)
		}
		if res.Skipped {
			fmt.Printf("ℹ️  %s 已有 %d 行数据，跳过种子初始化（幂等）\n", cfg.DatabaseName, res.Existing)
			return nil
		}
		fmt.Printf("✅ 种子数据写入完成：%d 条 INSERT（数据库 %s）\n", res.Executed, cfg.DatabaseName)
		fmt.Println("   默认账号 admin / admin123 —— ⚠️ 生产环境请立即修改密码")
		return nil
	},
}

func init() {
	seedCmd.Flags().StringVar(&seedFile, "file", "seed/mysql_data.sql",
		"种子 SQL 文件路径（容器内为 /walnut-backend/seed/mysql_data.sql）")
	rootCmd.AddCommand(seedCmd)
}
