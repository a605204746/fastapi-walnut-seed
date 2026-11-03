# WalnutSeed Go 后端
#
# 心智对齐前端 pnpm scripts 与 Python 端 uv run main.py：
#   make dev      ≈  uv run main.py run --env dev
#   make migrate  ≈  uv run main.py upgrade --env dev
#   make lint     ≈  uv run ruff check .
#   make fmt      ≈  uv run ruff format .

# 国内网络可用配方（详见 docs/go/00-Go后端实施计划.md §9.4）：
#   goproxy.cn / goproxy.io / sum.golang.google.cn 在部分网络下会被连接重置，
#   阿里云镜像直连可用。GOSUMDB 必须保持默认值，让 sumdb 查询经 GOPROXY 转发；
#   改成 sum.golang.google.cn 会让 Go 绕过代理直连（反而失败），
#   设成 off 则工具链模块无法通过校验。
GOPROXY ?= https://mirrors.aliyun.com/goproxy/,direct
GOSUMDB ?= sum.golang.org
GOTOOLCHAIN ?= auto

export GOPROXY GOSUMDB GOTOOLCHAIN

BINARY  := walnut-seed-go
ENV     ?= dev
# 与 Dockerfile 的 ARG BUILD_IMAGE 保持一致；⚠️ 必须 >= go.mod 的 go 指令
# （当前依赖链 gin v1.12 / go-redis v9.23 要求 Go >= 1.26）
BUILD_IMAGE ?= golang:1.26-alpine
# CGO_ENABLED=0 产出静态二进制，可直接跑在 alpine/scratch 上
GOFLAGS_BUILD := -trimpath -ldflags="-s -w"
# 契约 diff 的参照端（Python 后端），需另行启动并换端口避免与 Go 端冲突
PY_BASE ?= http://127.0.0.1:8012
GO_BASE ?= http://127.0.0.1:8011

.DEFAULT_GOAL := help

.PHONY: help
help: ## 显示本帮助
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

## ---------- 开发 ----------

.PHONY: dev
dev: ## 本地启动服务（等价 go run . run --env dev）
	go run . run --env $(ENV)

.PHONY: build
build: ## 编译到 bin/
	CGO_ENABLED=0 go build $(GOFLAGS_BUILD) -o bin/$(BINARY) .

.PHONY: run
run: build ## 编译并运行
	./bin/$(BINARY) run --env $(ENV)

.PHONY: clean
clean: ## 清理构建产物
	rm -rf bin/ dist/ coverage.out

## ---------- 质量门禁 ----------

.PHONY: test
test: ## 运行测试
	go test ./...

.PHONY: test-race
test-race: ## 竞态检测（CI 硬门槛）⚠️ Windows 本机需 gcc，见下方说明
	@echo "提示: -race 依赖 cgo。Windows 上若无 gcc 会报 'C compiler \"gcc\" not found'，"
	@echo "      此时请在 CI(Linux) 或用 make test-race-docker 运行；本机可用 make test 代替。"
	CGO_ENABLED=1 go test -race ./...

.PHONY: test-race-docker
test-race-docker: ## 在容器内跑竞态检测（无需本机 gcc，Windows 开发机的推荐方式）
	docker run --rm -v "$(CURDIR)":/src -w /src \
		-e GOPROXY=$(GOPROXY) -e GOSUMDB=$(GOSUMDB) -e GOTOOLCHAIN=$(GOTOOLCHAIN) \
		$(BUILD_IMAGE) go test -race ./...

.PHONY: cover
cover: ## 生成覆盖率报告
	go test -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out | tail -n 1

.PHONY: lint
lint: ## golangci-lint 检查
	golangci-lint run ./...

.PHONY: fmt
fmt: ## gofumpt 格式化（比 gofmt 更严格）
	gofumpt -l -w .

.PHONY: fmt-check
fmt-check: ## 检查格式，不改动文件（CI 用）
	@test -z "$$(gofumpt -l .)" || { echo "以下文件需要 gofumpt 格式化:"; gofumpt -l .; exit 1; }

.PHONY: vet
vet: ## go vet
	go vet ./...

.PHONY: vuln
vuln: ## 已知漏洞检查
	govulncheck ./...

.PHONY: check
check: fmt-check vet lint test-race ## 提交前完整检查

.PHONY: tidy
tidy: ## 整理依赖
	go mod tidy

## ---------- 数据库 ----------

.PHONY: migrate
migrate: ## 应用迁移到最新版本
	go run . migrate up --env $(ENV)

.PHONY: migrate-down
migrate-down: ## 回退一个迁移版本
	go run . migrate down --env $(ENV)

.PHONY: migrate-current
migrate-current: ## 查看当前迁移版本
	go run . migrate current --env $(ENV)

.PHONY: seed
seed: ## 写入种子数据（幂等）
	go run . seed --env $(ENV)

.PHONY: schema-parity
schema-parity: ## 校验本端 schema 与 Python 库逐字段一致（计划 R13 验收，需中间件在跑）
	cd ../walnut-backend-python && \
		PYTHONIOENCODING=utf-8 .venv/Scripts/python ../walnut-backend-go/scripts/verify_schema_parity.py

.PHONY: key-pairing
key-pairing: ## 校验前端三个 env 与 Python/Go 后端的 RSA 密钥配对关系
	cd ../walnut-backend-python && \
		PYTHONIOENCODING=utf-8 .venv/Scripts/python ../walnut-backend-go/scripts/verify_key_pairing.py

## ---------- 契约 ----------

.PHONY: contract-export
contract-export: ## 从 Python 端导出 golden 契约（无需启动服务，直接调 create_app()）
	cd ../walnut-backend-python && \
		ENVIRONMENT=dev PYTHONIOENCODING=utf-8 PYTHONUTF8=1 \
		../walnut-backend-python/.venv/Scripts/python ../walnut-backend-go/scripts/export_contract.py
	@echo "已导出 -> api/openapi/{contract.json,routes.txt,modules.json}"

.PHONY: contract-diff
contract-diff: ## 双后端响应 diff 跑分（需先启动 Go:8011 与 Python:8012）
	python test/contract/diff_contract.py --py $(PY_BASE) --go $(GO_BASE)

.PHONY: cryptointerop
cryptointerop: ## Python↔Go 加解密互通全链路（生成向量 → Go 测试 → Python 反向校验）
	cd ../walnut-backend-python && PYTHONIOENCODING=utf-8 \
		.venv/Scripts/python ../walnut-backend-go/test/cryptointerop/gen_vectors.py
	go test ./test/cryptointerop/ -v -count=1
	cd ../walnut-backend-python && PYTHONIOENCODING=utf-8 \
		.venv/Scripts/python ../walnut-backend-go/test/cryptointerop/verify_go_output.py

.PHONY: smoke
smoke: ## 全端点冒烟（P5 交付，对应 Python 端 scripts/smoke_all.py）
	@echo "尚未实现：P5 阶段落地 scripts/smoke（对应 Python 端 scripts/smoke_all.py）"
	@echo "当前可用的端到端校验是 make contract-diff"

## ---------- 容器 ----------

.PHONY: docker-build
docker-build: ## 构建镜像（与 compose 中的 image 名一致）
	docker build -t walnut-seed-backend-go .

.PHONY: docker-up
docker-up: ## 起全栈（中间件 + Go 后端 + 前端）
	docker compose -f ../docker/docker-compose.go.yml up -d --build

.PHONY: docker-down
docker-down: ## 停全栈（保留数据卷与日志）
	docker compose -f ../docker/docker-compose.go.yml down
