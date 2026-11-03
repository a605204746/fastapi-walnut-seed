// Package envelope 实现三端统一的响应信封 {"code", "msg", "data"} 与分页载荷 {"rows", "total"}。
//
// 契约要点（对齐 Python 端 app/common/response.py）：
//   - 业务异常几乎都返回 HTTP 200，真实结果由 body.code 表达
//   - code: 200=成功、500=失败、601=警告、400=参数校验失败，认证模块用 10000 段
//   - 401/403/404/405 返回真实 HTTP 状态码，但**仍带信封体**
//   - 分页空结果 rows 必须是 [] 而非 null（前端 v-for 会崩）
package envelope

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// 业务状态码，与 Python 端 HttpStatus / ApiResponse 的 ClassVar 对齐。
const (
	CodeSuccess = 200
	CodeFail    = 500
	CodeWarn    = 601

	CodeBadRequest   = 400
	CodeUnauthorized = 401
	CodeForbidden    = 403
	CodeNotFound     = 404
	CodeBadMethod    = 405
)

// 默认提示文案。
const (
	MsgSuccess = "操作成功"
	MsgFail    = "操作失败"
)

// Response 统一响应信封。
//
// Data 用 any 且**不加 omitempty**：Python 端 ApiResponse.data 默认 None 会序列化成
// "data": null，字段必须始终存在，否则前端解构 data.xxx 会抛异常。
type Response struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data"`
}

// PageResult 分页载荷，序列化为 {"rows": [...], "total": N}。
//
// Rows 初始化为空切片而非 nil：Go 的 nil slice 会序列化成 null，
// 而 Python 端 PageResult.rows 用 default_factory=list 恒为 []。
type PageResult[T any] struct {
	Rows  []T   `json:"rows"`
	Total int64 `json:"total"`
}

// NewPage 构造分页载荷；rows 为 nil 时归一化为空切片。
func NewPage[T any](rows []T, total int64) PageResult[T] {
	if rows == nil {
		rows = []T{}
	}
	return PageResult[T]{Rows: rows, Total: total}
}

// OK 成功响应（HTTP 200，body code=200，msg="操作成功"）。
func OK(c *gin.Context, data any) {
	write(c, http.StatusOK, CodeSuccess, MsgSuccess, data)
}

// OKMsg 成功响应并自定义提示文案（对齐 SuccessResponse(data=..., msg=...)）。
func OKMsg(c *gin.Context, data any, msg string) {
	write(c, http.StatusOK, CodeSuccess, msg, data)
}

// Warn 警告响应（HTTP 200，body code=601）。
func Warn(c *gin.Context, data any, msg string) {
	write(c, http.StatusOK, CodeWarn, msg, data)
}

// Fail 业务失败（HTTP 200，body code=500）。
func Fail(c *gin.Context, msg string) {
	write(c, http.StatusOK, CodeFail, msg, nil)
}

// FailWith 业务失败并携带 data（对齐 ErrorResponse(data=..., msg=..., code=...)）。
// httpStatus 为传输层状态码；传 0 表示沿用 200（业务失败的常规形态）。
func FailWith(c *gin.Context, httpStatus, code int, msg string, data any) {
	if httpStatus == 0 {
		httpStatus = http.StatusOK
	}
	write(c, httpStatus, code, msg, data)
}

// Page 分页成功响应（HTTP 200，data={"rows","total"}）。
func Page[T any](c *gin.Context, rows []T, total int64) {
	OK(c, NewPage(rows, total))
}

// write 是唯一的出口，确保所有响应都经过同一套 JSON 编码。
//
// 不用 c.JSON：gin 默认的 JSON 渲染器在不同 build tag 下会切换到 sonic/jsoniter，
// 而本项目的 JSInt / JSONTime 等自定义 MarshalJSON 需要标准库语义保证。
func write(c *gin.Context, httpStatus, code int, msg string, data any) {
	c.Render(httpStatus, jsonRender{Payload: Response{Code: code, Msg: msg, Data: data}})
}
