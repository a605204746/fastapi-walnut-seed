package health

// DependencyStatus 单个依赖的探测结果。
//
// 字段名是 **snake_case**，与全站其余接口的 camelCase 不同——
// 健康探针的 schema（app/api/v1/module_common/health/schema.py）直接用 Pydantic
// 字段名输出，没有配 alias_generator。Go 侧必须照抄，否则 compose 的
// ready 探针门禁与监控面板都会失效。
type DependencyStatus struct {
	// Status 1=正常 0=异常
	Status int `json:"status"`
	// LatencyMs 探测耗时（毫秒）。用指针以区分「未探测」(null) 与「0ms」，
	// 对应 Python 端 latency_ms: float | None = None
	LatencyMs *float64 `json:"latency_ms"`
}

// HealthOut /check 与 /live 的载荷。
type HealthOut struct {
	Status        int     `json:"status"`
	Timestamp     string  `json:"timestamp"`
	Version       string  `json:"version"`
	UptimeSeconds float64 `json:"uptime_seconds"`
}

// ReadinessOut /ready 的载荷，在 HealthOut 基础上追加依赖明细与磁盘使用率。
//
// 用内联嵌入而非字段复制：Go 的匿名嵌入 + 无 tag 会让 encoding/json 把
// HealthOut 的字段平铺到同一层，与 Python 端 ReadinessOut(HealthOut) 的继承一致。
type ReadinessOut struct {
	HealthOut
	Dependencies map[string]DependencyStatus `json:"dependencies"`
	DiskUsage    float64                     `json:"disk_usage"`
}
