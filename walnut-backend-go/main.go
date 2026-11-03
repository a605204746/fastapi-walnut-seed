// WalnutSeed Go 后端入口。
//
// CLI 命令面与 Python 端 main.py（typer）对齐：
//
//	go run . run --env dev        # 启动服务（默认命令）
//	go run . migrate up --env dev # 应用迁移到最新版本
//	go run . migrate current      # 查看当前迁移版本
package main

import "walnut-seed-go/cmd"

func main() {
	cmd.Execute()
}
