// Package operlogx 实现操作日志的异步采集，对应 Python 端 OperationLogRoute + consume_oper_log。
//
// 架构：
//   - 请求级中间件捕获请求/响应 → 构建 Event → 发送到 channel
//   - worker pool 从 channel 消费 → 写入 sys_oper_log
//   - App.Shutdown 关闭 channel → worker 排空后退出
package operlogx

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/idgen"
)

// 字段长度上限（与 Python 端 3800 一致）：超限整体替换为固定文案，不是截断。
const (
	truncParam  = 3800
	truncResult = 3800
	truncURL    = 255
)

// 密码类字段，与 Python 端 SystemConstants.EXCLUDE_PROPERTIES 一致。
var scrubKeys = map[string]bool{
	"password":        true,
	"oldPassword":     true,
	"newPassword":     true,
	"confirmPassword": true,
}

// skipPrefixes 不采集的 URL 前缀。
var skipPrefixes = []string{
	"/common/health",
	"/resource/sse",
}

// Event 操作日志事件。
type Event struct {
	Title         string
	BusinessType  int
	Method        string
	RequestMethod string
	OperatorType  int
	OperName      string
	DeptName      string
	OperURL       string
	OperIP        string
	OperParam     string
	JSONResult    string
	Status        int
	ErrorMsg      string
	OperTime      time.Time
	CostTime      int64
}

// Logger 操作日志异步采集器。
type Logger struct {
	db      *gorm.DB
	log     *slog.Logger
	ch      chan Event
	wg      sync.WaitGroup
	closeCh chan struct{}
}

// NewLogger 构造操作日志采集器。worker 数量由 Start 决定。
func NewLogger(db *gorm.DB, log *slog.Logger, chanBuf int) *Logger {
	if chanBuf < 1 {
		chanBuf = 1024
	}
	return &Logger{
		db:      db,
		log:     log,
		ch:      make(chan Event, chanBuf),
		closeCh: make(chan struct{}),
	}
}

// Start 启动 worker pool。
func (l *Logger) Start(workerCount int) {
	for i := 0; i < workerCount; i++ {
		l.wg.Add(1)
		go l.consume()
	}
	l.log.Info("✅ 操作日志采集器已启动", slog.Int("workers", workerCount))
}

// Stop 关闭 channel 并等待 worker 排空。
func (l *Logger) Stop() {
	close(l.closeCh)
	close(l.ch)
	l.wg.Wait()
	l.log.Info("操作日志采集器已停止")
}

// Send 向 channel 投递一个事件。
func (l *Logger) Send(evt Event) {
	select {
	case l.ch <- evt:
	default:
		l.log.Warn("操作日志 channel 已满，事件被丢弃",
			slog.String("url", evt.OperURL),
			slog.String("method", evt.RequestMethod))
	}
}

// Middleware 返回 gin 中间件，自动采集写方法的操作日志。
func (l *Logger) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		// 只采集写方法（对齐 OPERATION_RECORD_METHOD）
		switch method {
		case "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS":
		default:
			c.Next()
			return
		}
		// 跳过不采集的路径
		path := c.Request.URL.Path
		for _, pfx := range skipPrefixes {
			if strings.HasPrefix(path, pfx) {
				c.Next()
				return
			}
		}

		// 读取请求体（后续 handler 也要用，需还原）
		var reqBody []byte
		if c.Request.Body != nil {
			reqBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewReader(reqBody))
		}

		// 包装 response writer 以捕获响应体
		rw := &responseWriter{ResponseWriter: c.Writer, buf: &bytes.Buffer{}}
		c.Writer = rw

		start := time.Now()

		// 执行 handler
		c.Next()

		costMs := time.Since(start).Milliseconds()

		// 标题/业务类型/method 列取注册表（Python @log 装饰器 + summary 兜底），
		// 未登记路由退回路径/方法推导
		title := pathToTitle(path)
		btype := methodToBusinessType(method)
		methodCol := method + " " + path
		if meta, ok := operlogRegistry[method+" "+c.FullPath()]; ok {
			title = meta.Title
			btype = meta.BusinessType
			methodCol = meta.Method
		}

		// 构建事件
		evt := Event{
			Title:         title,
			BusinessType:  btype,
			Method:        methodCol,
			RequestMethod: method,
			OperatorType:  1, // 后台用户
			OperURL:       truncate(path, truncURL),
			OperIP:        c.ClientIP(),
			OperTime:      start,
			CostTime:      costMs,
			Status:        statusFromHTTP(c.Writer.Status()),
		}

		// 操作人
		if ac, ok := authctx.From(c); ok {
			evt.OperName = ac.User.UserName
		}

		// 请求参数（脱敏；超长整体替换，对齐 Python「请求参数过长」）
		param := buildParam(c, reqBody)
		if utf8.RuneCountInString(param) > truncParam {
			param = "请求参数过长"
		}
		evt.OperParam = param

		// 响应体：仅 JSON 响应记录原文，非 JSON（如 Excel 导出）记 "{}"，
		// 否则二进制截断后写入 utf8mb4 列会触发 Error 1366 整行丢失。
		result := "{}"
		if strings.Contains(c.Writer.Header().Get("Content-Type"), "application/json") && rw.buf.Len() > 0 {
			result = rw.buf.String()
		}
		if utf8.RuneCountInString(result) > truncResult {
			result = "响应结果过长"
		}
		evt.JSONResult = result

		// error_msg：Python log_data 不含该字段，恒 NULL，这里不采集

		l.Send(evt)
	}
}

// ---- 内部 ----

func (l *Logger) consume() {
	defer l.wg.Done()
	for evt := range l.ch {
		l.insert(evt)
	}
}

func (l *Logger) insert(evt Event) {
	row := sysOperLog{
		ID:            idgen.NextID(),
		Title:         nilStr(evt.Title),
		BusinessType:  intPtr(evt.BusinessType),
		Method:        nilStr(evt.Method),
		RequestMethod: nilStr(evt.RequestMethod),
		OperatorType:  intPtr(evt.OperatorType),
		OperName:      nilStr(evt.OperName),
		DeptName:      nilStr(evt.DeptName),
		OperURL:       nilStr(evt.OperURL),
		OperIP:        nilStr(evt.OperIP),
		OperParam:     nilStr(evt.OperParam),
		JsonResult:    nilStr(evt.JSONResult),
		Status:        intPtr(evt.Status),
		ErrorMsg:      nilStr(evt.ErrorMsg),
		OperTime:      &evt.OperTime,
		CostTime:      int64Ptr(evt.CostTime),
	}
	if err := l.db.Create(&row).Error; err != nil {
		l.log.Error("操作日志写入失败",
			slog.String("url", evt.OperURL),
			slog.String("err", err.Error()))
	}
}

// sysOperLog 写入用的内部实体（避免导入 module/system/log 造成循环）。
type sysOperLog struct {
	ID            int64      `gorm:"column:id;primaryKey"`
	Title         *string    `gorm:"column:title"`
	BusinessType  *int       `gorm:"column:business_type"`
	Method        *string    `gorm:"column:method"`
	RequestMethod *string    `gorm:"column:request_method"`
	OperatorType  *int       `gorm:"column:operator_type"`
	OperName      *string    `gorm:"column:oper_name"`
	DeptName      *string    `gorm:"column:dept_name"`
	OperURL       *string    `gorm:"column:oper_url"`
	OperIP        *string    `gorm:"column:oper_ip"`
	OperParam     *string    `gorm:"column:oper_param"`
	JsonResult    *string    `gorm:"column:json_result"`
	Status        *int       `gorm:"column:status"`
	ErrorMsg      *string    `gorm:"column:error_msg"`
	OperTime      *time.Time `gorm:"column:oper_time"`
	CostTime      *int64     `gorm:"column:cost_time"`
}

func (sysOperLog) TableName() string { return "sys_oper_log" }

// responseWriter 包装 gin.ResponseWriter 以捕获响应体。
type responseWriter struct {
	gin.ResponseWriter
	buf *bytes.Buffer
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	rw.buf.Write(b)
	return rw.ResponseWriter.Write(b)
}

func (rw *responseWriter) WriteString(s string) (int, error) {
	rw.buf.WriteString(s)
	return rw.ResponseWriter.WriteString(s)
}

// buildParam 从请求体 / form 构建参数快照并脱敏，对齐 Python router_class：
// form 类 content-type → form 快照；否则有 body 就尝试 JSON 解析（失败记原文）；
// 空快照记 "{}"（Python json.dumps({})），JSON 序列化不转义 HTML（ensure_ascii=False 等价）。
func buildParam(c *gin.Context, body []byte) string {
	ct := c.ContentType()
	result := make(map[string]any)

	if strings.HasPrefix(ct, "application/x-www-form-urlencoded") || strings.HasPrefix(ct, "multipart/form-data") {
		form := make(map[string]string)
		for k, v := range c.Request.PostForm {
			if _, skip := scrubKeys[k]; !skip && len(v) > 0 {
				form[k] = v[0]
			}
		}
		if len(form) > 0 {
			result["form"] = form
		}
	} else if len(body) > 0 {
		var obj any
		if err := json.Unmarshal(body, &obj); err == nil {
			result["body"] = scrub(obj)
		} else {
			result["body"] = string(body)
		}
	}

	if len(c.Params) > 0 {
		pp := make(map[string]string)
		for _, p := range c.Params {
			pp[p.Key] = p.Value
		}
		result["path_params"] = pp
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(result)
	return strings.TrimSuffix(buf.String(), "\n")
}

// scrub 递归移除密码类字段。
func scrub(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, child := range val {
			if scrubKeys[k] {
				continue
			}
			out[k] = scrub(child)
		}
		return out
	case []any:
		out := make([]any, 0, len(val))
		for _, child := range val {
			out = append(out, scrub(child))
		}
		return out
	default:
		return v
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func methodToBusinessType(method string) int {
	switch method {
	case "POST":
		return 1 // INSERT
	case "PUT":
		return 2 // UPDATE
	case "DELETE":
		return 3 // DELETE
	default:
		return 0 // OTHER
	}
}

func statusFromHTTP(code int) int {
	if code >= 400 {
		return 1 // FAIL
	}
	return 0 // SUCCESS
}

func pathToTitle(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 {
		return parts[0] + "/" + parts[1]
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return path
}

func nilStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// business_type/operator_type/status/cost_time 的 0 均为合法业务值
// （其它/其它/正常/0ms），按空值省略会写 NULL，前端字典匹配不到显示 unknown。
func intPtr(v int) *int {
	return &v
}

func int64Ptr(v int64) *int64 {
	return &v
}
