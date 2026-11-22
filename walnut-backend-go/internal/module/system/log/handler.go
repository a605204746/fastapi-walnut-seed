package log

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/cachex"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/envelope"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/excelx"
	"walnut-seed-go/internal/pkg/idempotentx"
	"walnut-seed-go/internal/pkg/permguard"
)

type Handler struct {
	db    *gorm.DB
	errs  *errs.Handler
	perm  *permguard.Guard
	idem  *idempotentx.Guard
	cache *cachex.Client
}

func NewHandler(db *gorm.DB, eh *errs.Handler, perm *permguard.Guard, idem *idempotentx.Guard, cache *cachex.Client) *Handler {
	return &Handler{db: db, errs: eh, perm: perm, idem: idem, cache: cache}
}

func (h *Handler) service(c *gin.Context) (*Service, bool) {
	ac, ok := authctx.From(c)
	if !ok {
		h.errs.Abort(c, errs.NotLogin())
		return nil, false
	}
	return NewService(h.db, ac), true
}

func (h *Handler) OperLogList(c *gin.Context) {
	q, err := bindOperLogQuery(c)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.OperLogPage(c.Request.Context(), q)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, data)
}

func (h *Handler) OperLogExport(c *gin.Context) {
	merged, err := collectFilterParams(c)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	q, err := bindOperLogExportFilter(merged)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}

	svc, ok := h.service(c)
	if !ok {
		return
	}
	rows, err := svc.OperLogExportRows(c.Request.Context(), q)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	cols := make([]excelx.Column, 0)
	for _, ec := range OperLogExportColumns() {
		cols = append(cols, excelx.Column{Header: ec.Header, Value: pickSnake(ec.Key)})
	}
	anyRows := make([]any, 0, len(rows))
	for _, r := range rows {
		anyRows = append(anyRows, r)
	}
	if err := excelx.Export(c, anyRows, cols, "操作日志"); err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgSystemError))
		return
	}
}

func (h *Handler) OperLogClean(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	if err := h.runWithCleanLock(c, "lock:operlog:clean", svc.OperLogClean); err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, nil)
}

func (h *Handler) OperLogDelete(c *gin.Context) {
	ids, err := parseLogIDs(c.Param("oper_ids"))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	n, err := svc.OperLogDelete(c.Request.Context(), ids)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if n <= 0 {
		envelope.Fail(c, "操作失败")
		return
	}
	envelope.OK(c, nil)
}

func (h *Handler) LogininforList(c *gin.Context) {
	q, err := bindLogininforQuery(c)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.LogininforPage(c.Request.Context(), q)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, data)
}

func (h *Handler) LogininforExport(c *gin.Context) {
	merged, err := collectFilterParams(c)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	q, err := bindLogininforExportFilter(merged)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}

	svc, ok := h.service(c)
	if !ok {
		return
	}
	rows, err := svc.LogininforExportRows(c.Request.Context(), q)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	cols := make([]excelx.Column, 0)
	for _, ec := range LogininforExportColumns() {
		cols = append(cols, excelx.Column{Header: ec.Header, Value: pickSnake(ec.Key)})
	}
	anyRows := make([]any, 0, len(rows))
	for _, r := range rows {
		anyRows = append(anyRows, r)
	}
	if err := excelx.Export(c, anyRows, cols, "登录日志"); err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgSystemError))
		return
	}
}

func (h *Handler) LogininforClean(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	if err := h.runWithCleanLock(c, "lock:logininfor:clean", svc.LogininforClean); err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, nil)
}

func (h *Handler) LogininforDelete(c *gin.Context) {
	ids, err := parseLogIDs(c.Param("info_ids"))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	n, err := svc.LogininforDelete(c.Request.Context(), ids)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if n <= 0 {
		envelope.Fail(c, "操作失败")
		return
	}
	envelope.OK(c, nil)
}

func (h *Handler) LogininforUnlock(c *gin.Context) {
	userName := c.Param("user_name")
	if userName == "" {
		h.errs.Abort(c, errs.BadRequest("用户名不能为空"))
		return
	}
	if h.cache != nil {
		// 键前缀必须与写入侧 auth.keyPwdErrCount 一致（pwd_err_cnt:，无 global: 段），
		// 否则模式永远匹配不到，解锁返回成功但账户仍锁定。
		pattern := h.cache.Key("pwd_err_cnt:" + userName + ":*")
		keys, err := h.cache.Client.Keys(c.Request.Context(), pattern).Result()
		if err == nil {
			for _, key := range keys {
				h.cache.Del(c.Request.Context(), key)
			}
		}
	}
	envelope.OK(c, nil)
}

// ---- Query binding ----

func bindOperLogQuery(c *gin.Context) (OperLogQueryParam, error) {
	page, pageErr := crudx.BindPageQuery(c)
	qp := OperLogQueryParam{
		Page: page,
		Filter: OperLogFilterParam{
			Title:    strings.TrimSpace(c.Query("title")),
			OperName: strings.TrimSpace(c.Query("operName")),
			OperIP:   strings.TrimSpace(c.Query("operIp")),
		},
	}
	// 校验错误按 pydantic 字段序收集后以 ", " 拼接：分页字段在前，过滤字段在后，
	// body（businessTypes）错误最后。空串也算显式传参，同样报错。
	var msgs []string
	if pageErr != nil {
		msgs = append(msgs, errMsgs(pageErr)...)
	}
	if raw, ok := c.GetQuery("businessType"); ok {
		if n, valid := pyParseInt(raw); valid {
			qp.Filter.BusinessType = &n
		} else {
			msgs = append(msgs, msgIntParsing)
		}
	}
	if raw, ok := c.GetQuery("status"); ok {
		if n, valid := pyParseInt(raw); valid {
			qp.Filter.Status = &n
		} else {
			msgs = append(msgs, msgIntParsing)
		}
	}
	if raw, ok := c.GetQuery("beginTime"); ok {
		if t, derr := pyParseDate(raw); derr == nil {
			qp.Filter.BeginTime = &t
		} else {
			msgs = append(msgs, derr.msg)
		}
	}
	if raw, ok := c.GetQuery("endTime"); ok {
		if t, derr := pyParseDate(raw); derr == nil {
			qp.Filter.EndTime = &t
		} else {
			msgs = append(msgs, derr.msg)
		}
	}
	bts, btsMsgs := bindBusinessTypes(c)
	qp.Filter.BusinessTypes = bts
	msgs = append(msgs, btsMsgs...)
	if len(msgs) > 0 {
		return OperLogQueryParam{}, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return qp, nil
}

func bindLogininforQuery(c *gin.Context) (LogininforQueryParam, error) {
	page, pageErr := crudx.BindPageQuery(c)
	qp := LogininforQueryParam{
		Page: page,
		Filter: LogininforFilterParam{
			UserName: strings.TrimSpace(c.Query("userName")),
			IPAddr:   strings.TrimSpace(c.Query("ipaddr")),
			Status:   strings.TrimSpace(c.Query("status")),
		},
	}
	var msgs []string
	if pageErr != nil {
		msgs = append(msgs, errMsgs(pageErr)...)
	}
	if raw, ok := c.GetQuery("beginTime"); ok {
		if t, derr := pyParseDate(raw); derr == nil {
			qp.Filter.BeginTime = &t
		} else {
			msgs = append(msgs, derr.msg)
		}
	}
	if raw, ok := c.GetQuery("endTime"); ok {
		if t, derr := pyParseDate(raw); derr == nil {
			qp.Filter.EndTime = &t
		} else {
			msgs = append(msgs, derr.msg)
		}
	}
	if len(msgs) > 0 {
		return LogininforQueryParam{}, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return qp, nil
}

func errMsgs(err error) []string {
	if se := errs.As(err); se != nil {
		return strings.Split(se.Msg, ", ")
	}
	return []string{err.Error()}
}

// bindBusinessTypes 读取 GET operlog/list 的 businessTypes：契约为 JSON body 数组
// （FastAPI 把 list 字段视为 body 参数，见 contract.json）；无 body 时兼容 query
// 逗号串。body 的 Content-Type 判定与 FastAPI 一致：仅 application/json 或
// application/*+json 才做 JSON 解析，否则原始字节按 list_type 报错。
func bindBusinessTypes(c *gin.Context) ([]int64, []string) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil || len(body) == 0 {
		raw := c.Query("businessTypes")
		if raw == "" {
			return nil, nil
		}
		return parseBusinessTypesCSV(raw)
	}
	if !isJSONContentType(c.GetHeader("Content-Type")) {
		return nil, []string{msgListType}
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, []string{errs.MsgJSONInvalid}
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, []string{errs.MsgJSONInvalid}
	}
	if v == nil {
		return nil, nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, []string{msgListType}
	}
	var out []int64
	var msgs []string
	for _, item := range arr {
		switch t := item.(type) {
		case json.Number:
			if n, ok := pyJSONNumberInt(t); ok {
				out = append(out, n)
			} else {
				msgs = append(msgs, msgIntFromFloat)
			}
		case string:
			if n, ok := pyParseInt(t); ok {
				out = append(out, n)
			} else {
				msgs = append(msgs, msgIntParsing)
			}
		case bool:
			if t {
				out = append(out, 1)
			} else {
				out = append(out, 0)
			}
		default:
			msgs = append(msgs, msgIntType)
		}
	}
	return out, msgs
}

func isJSONContentType(ct string) bool {
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil || !strings.HasPrefix(mt, "application/") {
		return false
	}
	sub := strings.TrimPrefix(mt, "application/")
	return sub == "json" || strings.HasSuffix(sub, "+json")
}

// pyJSONNumberInt 处理 body 数组里的 JSON 数字：整数字面量直接取（Python int 无上限，
// 溢出饱和）；浮点字面量须为零小数（1.0/1e2 可用，1.5 报 int_from_float，超 int64 报 size）。
func pyJSONNumberInt(n json.Number) (int64, bool) {
	s := n.String()
	if !strings.ContainsAny(s, ".eE") {
		v, err := strconv.ParseInt(s, 10, 64)
		if err == nil {
			return v, true
		}
		if strings.HasPrefix(s, "-") {
			return math.MinInt64, true
		}
		return math.MaxInt64, true
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) || f != math.Trunc(f) {
		return 0, false
	}
	if f >= 9223372036854775808.0 || f < -9223372036854775808.0 {
		return 0, false
	}
	return int64(f), true
}

func parseBusinessTypesCSV(raw string) ([]int64, []string) {
	var out []int64
	var msgs []string
	for _, item := range strings.Split(raw, ",") {
		it := strings.TrimSpace(item)
		if it == "" {
			continue
		}
		if n, ok := pyParseInt(it); ok {
			out = append(out, n)
		} else {
			msgs = append(msgs, msgIntParsing)
		}
	}
	return out, msgs
}

// ---- Export filter binding ----

// collectFilterParams 对应 controller._collect_filter_params：合并 URL query 与
// urlencoded 表单，表单覆盖 query；重复 query 键取末值；重复表单键成数组；
// 丢弃空串值（仅精确 ""，空白串保留并参与校验）。
func collectFilterParams(c *gin.Context) (map[string]any, error) {
	if err := c.Request.ParseForm(); err != nil {
		return nil, errs.BadRequest(errs.MsgValidationFailed)
	}
	merged := make(map[string]any)
	for k, vs := range c.Request.URL.Query() {
		if len(vs) > 0 {
			merged[k] = vs[len(vs)-1]
		}
	}
	for k, vs := range c.Request.PostForm {
		if len(vs) == 1 {
			merged[k] = vs[0]
		} else if len(vs) > 1 {
			merged[k] = vs
		}
	}
	for k, v := range merged {
		if s, ok := v.(string); ok && s == "" {
			delete(merged, k)
		}
	}
	return merged, nil
}

func bindOperLogExportFilter(m map[string]any) (OperLogFilterParam, error) {
	var f OperLogFilterParam
	var ferrs []pyFieldErr
	f.Title = exportStrField(m, "title", &ferrs)
	if v, ok := m["businessType"]; ok {
		f.BusinessType = exportIntField(v, "businessType", &ferrs)
	}
	if v, ok := m["businessTypes"]; ok {
		f.BusinessTypes = exportBusinessTypes(v, &ferrs)
	}
	if v, ok := m["status"]; ok {
		f.Status = exportIntField(v, "status", &ferrs)
	}
	f.OperName = exportStrField(m, "operName", &ferrs)
	f.OperIP = exportStrField(m, "operIp", &ferrs)
	if v, ok := m["beginTime"]; ok {
		f.BeginTime = exportDateField(v, "beginTime", &ferrs)
	}
	if v, ok := m["endTime"]; ok {
		f.EndTime = exportDateField(v, "endTime", &ferrs)
	}
	if len(ferrs) > 0 {
		return OperLogFilterParam{}, errs.BadRequest(pyValidationDump("OperLogFilterSchema", ferrs))
	}
	return f, nil
}

func bindLogininforExportFilter(m map[string]any) (LogininforFilterParam, error) {
	var f LogininforFilterParam
	var ferrs []pyFieldErr
	f.UserName = exportStrField(m, "userName", &ferrs)
	f.IPAddr = exportStrField(m, "ipaddr", &ferrs)
	f.Status = exportStrField(m, "status", &ferrs)
	if v, ok := m["beginTime"]; ok {
		f.BeginTime = exportDateField(v, "beginTime", &ferrs)
	}
	if v, ok := m["endTime"]; ok {
		f.EndTime = exportDateField(v, "endTime", &ferrs)
	}
	if len(ferrs) > 0 {
		return LogininforFilterParam{}, errs.BadRequest(pyValidationDump("LogininforFilterSchema", ferrs))
	}
	return f, nil
}

func exportStrField(m map[string]any, key string, ferrs *[]pyFieldErr) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case []string:
		*ferrs = append(*ferrs, pyFieldErr{key, "string_type", msgStringType, pyListRepr(t), "list"})
	}
	return ""
}

func exportIntField(v any, key string, ferrs *[]pyFieldErr) *int64 {
	switch t := v.(type) {
	case string:
		if n, ok := pyParseInt(t); ok {
			return &n
		}
		*ferrs = append(*ferrs, pyFieldErr{key, "int_parsing", msgIntParsing, pyRepr(t), "str"})
	case []string:
		*ferrs = append(*ferrs, pyFieldErr{key, "int_type", msgIntType, pyListRepr(t), "list"})
	}
	return nil
}

func exportDateField(v any, key string, ferrs *[]pyFieldErr) *time.Time {
	switch t := v.(type) {
	case string:
		d, derr := pyParseDate(t)
		if derr == nil {
			return &d
		}
		*ferrs = append(*ferrs, pyFieldErr{key, derr.typ, derr.msg, pyRepr(t), "str"})
	case []string:
		*ferrs = append(*ferrs, pyFieldErr{key, "date_type", msgDateType, pyListRepr(t), "list"})
	}
	return nil
}

// exportBusinessTypes 对应 schema.split_business_types：逗号串逐项 strip 并过滤空项；
// 重复表单键已是数组，原样逐项解析（空串元素也报错，下标即原位置）。
func exportBusinessTypes(v any, ferrs *[]pyFieldErr) []int64 {
	var items []string
	switch t := v.(type) {
	case string:
		for _, item := range strings.Split(t, ",") {
			if it := strings.TrimSpace(item); it != "" {
				items = append(items, it)
			}
		}
	case []string:
		items = t
	}
	var out []int64
	for i, item := range items {
		if n, ok := pyParseInt(item); ok {
			out = append(out, n)
		} else {
			*ferrs = append(*ferrs, pyFieldErr{fmt.Sprintf("businessTypes.%d", i), "int_parsing", msgIntParsing, pyRepr(item), "str"})
		}
	}
	return out
}

// ---- Delete / clean helpers ----

// parseLogIDs 对应 controller._parse_ids：逐项 int()。空串/纯逗号 → 空列表
// （删除 0 行 → code 500「操作失败」）；非数字 → code 400 透传 ValueError 原文。
// int() 接受数字间下划线（"1_0"=10）。
func parseLogIDs(raw string) ([]int64, error) {
	items := crudx.Str2List(raw)
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		n, ok := pyIntStrict(item)
		if !ok {
			// Python _parse_ids 已改为 ServiceException（两端同步，不再透传 traceback）
			return nil, errs.BadRequest("日志ID格式有误")
		}
		ids = append(ids, n)
	}
	return ids, nil
}

// runWithCleanLock 对应 controller._run_with_lock：等待 3 秒、持有 30 秒、每 0.1 秒重试，
// 失败 → code 500「分布式锁 [键] 获取失败，请稍后重试」（文案用逻辑键，不含根前缀）；
// 结束按锁值原子释放（Lua），与 Python RedisUtils.lock/unlock 一致。
func (h *Handler) runWithCleanLock(c *gin.Context, lockKey string, action func(context.Context) error) error {
	ctx := c.Request.Context()
	if h.cache == nil {
		return errs.Newf("分布式锁 [%s] 获取失败，请稍后重试", lockKey)
	}
	lockValue := uuid.NewString()
	deadline := time.Now().Add(3 * time.Second)
	for !h.cache.SetIfAbsent(ctx, lockKey, lockValue, 30*time.Second) {
		if !time.Now().Before(deadline) {
			return errs.Newf("分布式锁 [%s] 获取失败，请稍后重试", lockKey)
		}
		time.Sleep(100 * time.Millisecond)
	}
	defer func() {
		script := "if redis.call('get', KEYS[1]) == ARGV[1] then return redis.call('del', KEYS[1]) else return 0 end"
		_ = h.cache.Client.Eval(ctx, script, []string{h.cache.Key(lockKey)}, lockValue).Err()
	}()
	return action(ctx)
}
