# 冻结契约基线（Golden Baseline）

> **所有权声明**：本目录于 **2026-11-20 冻结**，是 **Go 后端自有的契约事实源**。
> Go 已是主后端，契约所有权从「活 Python 导出」转为「Go 自有冻结基线」——
> 本目录**不再从活 Python 再生成**。三个数据文件复制自冻结当日的 `api/openapi/`
> 导出快照（2026-11-02 导出，125 条路由），字节一致。
> `api/openapi/` 保留为**历史 Python 导出**，`make contract-export` 仅作参照复现用。

## 文件

| 文件 | 内容 |
|---|---|
| `contract.json` | 完整 OpenAPI 3.1 文档（路径、方法、参数、响应 schema） |
| `routes.txt` | 排序后的 `METHOD path` 清单（125 行），`routes_inventory_test.go` 读取本目录 |
| `modules.json` | 路由 → 归属模块映射 + 范围统计 |

## 冻结的语义级含义

冻结的是**语义契约**，不是逐字节文本：

- **结构 / 字段名**：信封与载荷的键集合、层级结构冻结，不得单端增删改。
- **code 数值**：业务码（200 / 400 / 401 / 500 …）的取值与含义冻结。
- **null 恒出现**：契约中应出现的键即使值为 null 也必须序列化输出，
  「键缺失」与「键为 null」语义不同，不得互相替代。
- **键序豁免**：JSON 对象键的先后顺序不在契约内
  （diff 的 canon 已 `sort_keys=True`，两端对称归一化）。
- **校验 msg 按 i18n 策略**：阶段 4 起 Go 的校验错误文案（code=400 信封的 msg）
  从 Pydantic 英文改为 i18n 中文，不再与 Python 原文逐字一致。
  diff 对这类 msg 放宽为语义级检查（非空字符串、无 Pydantic 残留标记），
  见仓库根 `scripts/contract/diff_system.py` 的 `MSG_SEMANTIC`（`diff_contract.py` 同款）。
  结构 / code / 字段存在性不受此放宽影响，仍严格冻结。

## 变更流程

基线冻结后，任何契约变更（新增路由、改字段、改 code 语义）必须**先经评审更新
本目录**，再在 Go 端实现——不能单端先行。更新时同步：

- `test/contract/routes_inventory_test.go` 的 `expectedAtExport` 常量与豁免清单
  （当前已知豁免：`/docs`、`/docs/oauth2-redirect`、`/redoc` 三条文档路由，
  以及 OpenAPI 不导出的 WS 路由 `GET /resource/websocket`，均以测试文件内注释为准）。
- 本 README 的统计与说明。