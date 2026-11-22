// WalnutSeed Go 后端入口。
//
// CLI 命令面与 Python 端 main.py（typer）对齐：
//
//	go run . run --env dev        # 启动服务（默认命令）
//	go run . migrate up --env dev # 应用迁移到最新版本
//	go run . migrate current      # 查看当前迁移版本
package main

import (
	_ "embed"

	"walnut-seed-go/cmd"
)

// contractSpec 嵌入冻结契约 OpenAPI 文档，供 /docs 与 /openapi.json 使用。
// 在 main（backend 根）嵌入：go:embed 不允许 .. 路径，而 api/ 位于根下，
// 只有根包能直接嵌入 api/golden/contract.json（自包含二进制，容器无需 COPY api/）。
//
//go:embed api/golden/contract.json
var contractSpec []byte

func main() {
	cmd.ContractSpec = contractSpec
	cmd.Execute()
}
