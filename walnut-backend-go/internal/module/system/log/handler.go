package log

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/cachex"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/envelope"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/excelx"
	"walnut-seed-go/internal/pkg/idempotentx"
	"walnut-seed-go/internal/pkg/permguard"
	"walnut-seed-go/internal/pkg/routex"
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

func (h *Handler) Mount(b routex.Binder) {
	require, repeat := h.guards()

	b.GET("/operlog/list", require("monitor:operlog:list"), h.OperLogList)
	b.POST("/operlog/export", require("monitor:operlog:export"), h.OperLogExport)
	b.DELETE("/operlog/clean", require("monitor:operlog:remove"), h.OperLogClean)
	b.DELETE("/operlog/:oper_ids", require("monitor:operlog:remove"), h.OperLogDelete)

	b.GET("/logininfor/list", require("monitor:logininfor:list"), h.LogininforList)
	b.POST("/logininfor/export", require("monitor:logininfor:export"), h.LogininforExport)
	b.DELETE("/logininfor/clean", require("monitor:logininfor:remove"), h.LogininforClean)
	b.DELETE("/logininfor/:info_ids", require("monitor:logininfor:remove"), h.LogininforDelete)
	b.GET("/logininfor/unlock/:user_name", require("monitor:logininfor:unlock"), repeat(), h.LogininforUnlock)
}

func (h *Handler) guards() (func(string) gin.HandlerFunc, func() gin.HandlerFunc) {
	pass := func(c *gin.Context) { c.Next() }
	require := func(perm string) gin.HandlerFunc {
		if h.perm == nil {
			return pass
		}
		return h.perm.Require(perm)
	}
	repeat := func() gin.HandlerFunc {
		if h.idem == nil {
			return pass
		}
		return h.idem.Default()
	}
	return require, repeat
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
	if err := c.Request.ParseForm(); err != nil {
		h.errs.Abort(c, errs.BadRequest(errs.MsgValidationFailed))
		return
	}
	q := OperLogFilterParam{}
	q.Title = strings.TrimSpace(c.PostForm("title"))
	q.OperName = strings.TrimSpace(c.PostForm("operName"))
	q.OperIP = strings.TrimSpace(c.PostForm("operIp"))
	q.BeginTime = strings.TrimSpace(c.PostForm("beginTime"))
	q.EndTime = strings.TrimSpace(c.PostForm("endTime"))
	if raw := c.PostForm("businessType"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err == nil {
			q.BusinessType = &n
		}
	}
	if raw := c.PostForm("status"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err == nil {
			q.Status = &n
		}
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
	if err := svc.OperLogClean(c.Request.Context()); err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, nil)
}

func (h *Handler) OperLogDelete(c *gin.Context) {
	ids, err := crudx.ParseIDs(c.Param("oper_ids"), "日志ID")
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
	if err := c.Request.ParseForm(); err != nil {
		h.errs.Abort(c, errs.BadRequest(errs.MsgValidationFailed))
		return
	}
	q := LogininforFilterParam{}
	q.UserName = strings.TrimSpace(c.PostForm("userName"))
	q.IPAddr = strings.TrimSpace(c.PostForm("ipaddr"))
	q.Status = strings.TrimSpace(c.PostForm("status"))
	q.BeginTime = strings.TrimSpace(c.PostForm("beginTime"))
	q.EndTime = strings.TrimSpace(c.PostForm("endTime"))

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
	if err := svc.LogininforClean(c.Request.Context()); err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, nil)
}

func (h *Handler) LogininforDelete(c *gin.Context) {
	ids, err := crudx.ParseIDs(c.Param("info_ids"), "日志ID")
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
		pattern := h.cache.Key("global:pwd_err_cnt:" + userName + ":*")
		keys, err := h.cache.Client.Keys(c.Request.Context(), pattern).Result()
		if err == nil {
			for _, key := range keys {
				h.cache.Client.Del(c.Request.Context(), key)
			}
		}
	}
	envelope.OK(c, nil)
}

// ---- Query binding ----

func bindOperLogQuery(q crudx.Querier) (OperLogQueryParam, error) {
	page, err := crudx.BindPageQuery(q)
	if err != nil {
		return OperLogQueryParam{}, err
	}
	qp := OperLogQueryParam{
		Page: page,
		Filter: OperLogFilterParam{
			Title:     strings.TrimSpace(q.Query("title")),
			OperName:  strings.TrimSpace(q.Query("operName")),
			OperIP:    strings.TrimSpace(q.Query("operIp")),
			BeginTime: strings.TrimSpace(q.Query("beginTime")),
			EndTime:   strings.TrimSpace(q.Query("endTime")),
		},
	}
	if raw := q.Query("businessType"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err == nil {
			qp.Filter.BusinessType = &n
		}
	}
	if raw := q.Query("status"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err == nil {
			qp.Filter.Status = &n
		}
	}
	return qp, nil
}

func bindLogininforQuery(q crudx.Querier) (LogininforQueryParam, error) {
	page, err := crudx.BindPageQuery(q)
	if err != nil {
		return LogininforQueryParam{}, err
	}
	return LogininforQueryParam{
		Page: page,
		Filter: LogininforFilterParam{
			UserName:  strings.TrimSpace(q.Query("userName")),
			IPAddr:    strings.TrimSpace(q.Query("ipaddr")),
			Status:    strings.TrimSpace(q.Query("status")),
			BeginTime: strings.TrimSpace(q.Query("beginTime")),
			EndTime:   strings.TrimSpace(q.Query("endTime")),
		},
	}, nil
}
