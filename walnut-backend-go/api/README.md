# api/ — 接口契约规范资产

⚠️ 本目录是**契约规范文件**（OpenAPI schema、路由清单），**不是接口实现**。

接口实现在 `internal/module/*/handler.go`（端点处理）+ `routes.go`（路由声明），
统一由 `internal/bootstrap/router.go` 组合根挂载。

> 命名遵循 [golang-standards/project-layout](https://github.com/golang-standards/project-layout)
> 约定：`/api` 存放 OpenAPI/Swagger 规范、JSON schema、协议定义文件，而非 API 代码。

## 子目录

| 目录 | 内容 | 地位 |
|---|---|---|
| `golden/` | contract.json（OpenAPI 3.1）+ routes.txt（125 条）+ modules.json | **契约事实源**，2026-11-20 冻结，Go 自有，变更须先评审 |
| `openapi/` | 同三件套 | 历史 Python 导出存档，`make contract-export` 仅作参照复现 |

## 怎么用

- **浏览接口**：VS Code OpenAPI 插件预览 `golden/contract.json`（离线）；或看 `make routes` 生成的 `docs/go/路由清单.md`
- **路由防漂移门禁**：`go test ./test/contract/`（读 `golden/routes.txt`，Go 出现基线没有的路由即硬失败）
- **契约协议说明**：`docs/go/04-接口契约.md`（信封 / 认证 / 权限 / 校验范式）