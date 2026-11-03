package health

import "strconv"

// roundTo 把浮点数四舍五入到指定小数位。
//
// 借道 strconv 而非 math.Round：Python 的 round() 用**银行家舍入**（round-half-to-even），
// 而 math.Round 是 half-away-from-zero，两者在 .5 精确落点时结果不同。
// strconv.FormatFloat 使用正确舍入且遵循 half-to-even，与 Python 的 round(x, n) 一致。
//
// disk_usage 与 latency_ms 都要经它处理，故舍入模式必须对齐，否则契约 diff 会报差异。
func roundTo(v float64, digits int) float64 {
	parsed, err := strconv.ParseFloat(strconv.FormatFloat(v, 'f', digits, 64), 64)
	if err != nil {
		return v
	}
	return parsed
}
