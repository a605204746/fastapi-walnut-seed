// Package cmd 提供 WalnutSeed Go 后端的 CLI 命令面（cobra），
// 形态对齐 Python 端 main.py 的 typer CLI：
//
//	go run . run --env dev            # 启动服务（无子命令时的默认行为）
//	go run . migrate up --env dev     # 应用迁移到最新版本
//	go run . migrate current --env dev
//	go run . seed --env dev           # 写入种子数据（P1）
//	go run . genkey                   # 生成 RSA 密钥对与 SECRET_KEY（P1）
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// envName 由 --env 全局旗标注入（dev / prod）。
var envName string

var rootCmd = &cobra.Command{
	Use:   "walnut-seed-go",
	Short: "WalnutSeed Go 后端（与 Python/Java 端同接口契约）",
	Long: "WalnutSeed 全栈脚手架的 Go 后端实现。\n" +
		"与 Python(FastAPI) / Java(Spring Boot) 共享同一套接口契约，前端无感互换。",
	// 根命令本身不执行动作，仅打印帮助；无参数时由 Execute 补 run 子命令
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute 执行根命令；无子命令时默认 run
// （对齐 Python 端「裸跑 uv run main.py 等价于 run --env dev」的行为）。
func Execute() {
	if len(os.Args) == 1 {
		os.Args = append(os.Args, "run")
	}
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&envName, "env", "",
		"运行环境 (dev, prod)，缺省读环境变量 ENVIRONMENT，再缺省 dev")
}

// resolveEnv 返回生效的环境名，优先级：--env 旗标 > 环境变量 ENVIRONMENT > dev。
//
// 与配置加载优先级一致（真实环境变量高于 env 文件），但注意 --env 旗标在这里
// **高于**环境变量：显式命令行意图应当压过环境继承，这与 Python 端 typer 的
// `run --env dev` 行为一致。
func resolveEnv() string {
	if envName != "" {
		return envName
	}
	if v := os.Getenv("ENVIRONMENT"); v != "" {
		return v
	}
	return "dev"
}
