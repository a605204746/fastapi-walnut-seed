// 本文件实现客户端域的 HTTP 端点，对应 Python 端 module_system/client/controller.py。
//
// ⚠️ 三个契约点：
//   - 导出走 **form**（request.form()），且 pop pageSize（导出不分页）；
//   - key 重复是 **raise ServiceException（500）**，不是 ErrorResponse 信封；
//   - 删除/改状态的 ID 串文案是「主键格式有误 / 主键不能为空」。

package client

import (
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/envelope"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/excelx"
	"walnut-seed-go/internal/pkg/idempotentx"
	"walnut-seed-go/internal/pkg/permguard"
	"walnut-seed-go/internal/pkg/routex"
)

// Handler 客户端域端点。
type Handler struct {
	db   *gorm.DB
	errs *errs.Handler
	perm *permguard.Guard
	idem *idempotentx.Guard
}

// NewHandler 构造 handler。perm / idem 可为 nil（仅单测）。
func NewHandler(db *gorm.DB, eh *errs.Handler, perm *permguard.Guard, idem *idempotentx.Guard) *Handler {
	return &Handler{db: db, errs: eh, perm: perm, idem: idem}
}

// Mount 注册客户端路由。挂载点是 /system/client（由 bootstrap 决定）。
func (h *Handler) Mount(b routex.Binder) {
	require, repeat := h.guards()

	b.GET("/list", require("system:client:list"), h.List)
	b.POST("/export", require("system:client:export"), h.Export)
	b.GET("/:client_id", require("system:client:query"), h.GetByID)
	b.POST("", require("system:client:add"), repeat(), h.Add)
	b.PUT("", require("system:client:edit"), repeat(), h.Update)
	b.PUT("/changeStatus", require("system:client:edit"), h.ChangeStatus)
	b.DELETE("/:ids", require("system:client:remove"), h.Delete)
}

// guards 返回「权限守卫」与「防重提交」两个中间件工厂（同 post/handler.go）。
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

// service 取出当前请求的业务层。
func (h *Handler) service(c *gin.Context) (*Service, bool) {
	ac, ok := authctx.From(c)
	if !ok {
		h.errs.Abort(c, errs.NotLogin())
		return nil, false
	}
	return NewService(h.db, ac), true
}

// List GET /system/client/list
func (h *Handler) List(c *gin.Context) {
	q, err := BindQuery(c)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	rows, total, err := svc.Page(c.Request.Context(), q)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.Page(c, rows, total)
}

// Export POST /system/client/export
//
// ⚠️ 参数走 **form**（Python 端 request.form()），并 pop pageSize；
// grantTypeList 在落 Excel 前转逗号分隔串。
func (h *Handler) Export(c *gin.Context) {
	if err := c.Request.ParseForm(); err != nil {
		h.errs.Abort(c, errs.BadRequest(errs.MsgValidationFailed))
		return
	}
	q := Query{
		ClientID:     c.PostForm("clientId"),
		ClientKey:    c.PostForm("clientKey"),
		ClientSecret: c.PostForm("clientSecret"),
		Status:       c.PostForm("status"),
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	rows, err := svc.ListAll(c.Request.Context(), q)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if err := excelx.Export(c, anySlice(rows), exportColumns(), "客户端管理"); err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgSystemError))
		return
	}
}

// GetByID GET /system/client/{client_id}
func (h *Handler) GetByID(c *gin.Context) {
	id, err := pathInt(c, "client_id")
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.GetByID(c.Request.Context(), id)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, data)
}

// Add POST /system/client
func (h *Handler) Add(c *gin.Context) {
	in, err := DecodeCreate(readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	if err := svc.CheckKeyUnique(ctx, in.ClientKey, nil); err != nil {
		if errors.Is(err, errDupKey) {
			h.errs.Abort(c, errs.Newf("新增客户端'%s'失败，客户端key已存在", deref(in.ClientKey)))
			return
		}
		h.errs.Abort(c, err)
		return
	}
	done, err := svc.Insert(ctx, in)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if !done {
		envelope.Fail(c, envelope.MsgFail)
		return
	}
	envelope.OK(c, nil)
}

// Update PUT /system/client
func (h *Handler) Update(c *gin.Context) {
	in, err := DecodeUpdate(readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	if err := svc.CheckKeyUnique(ctx, in.ClientKey, in.ID); err != nil {
		if errors.Is(err, errDupKey) {
			h.errs.Abort(c, errs.Newf("修改客户端'%s'失败，客户端key已存在", deref(in.ClientKey)))
			return
		}
		h.errs.Abort(c, err)
		return
	}
	done, err := svc.Update(ctx, in)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if !done {
		envelope.Fail(c, envelope.MsgFail)
		return
	}
	envelope.OK(c, nil)
}

// ChangeStatus PUT /system/client/changeStatus
func (h *Handler) ChangeStatus(c *gin.Context) {
	in, err := DecodeStatus(readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	n, err := svc.ChangeStatus(c.Request.Context(), *in.ClientID, *in.Status)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if n <= 0 {
		envelope.Fail(c, envelope.MsgFail)
		return
	}
	envelope.OK(c, nil)
}

// Delete DELETE /system/client/{ids}
func (h *Handler) Delete(c *gin.Context) {
	ids, err := crudx.ParseIDs(c.Param("ids"), "主键")
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	done, err := svc.DeleteByIDs(c.Request.Context(), ids)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if !done {
		envelope.Fail(c, envelope.MsgFail)
		return
	}
	envelope.OK(c, nil)
}

// ---- 导出列（顺序即列序；注意首列表头就是小写 "id"）----

func exportColumns() []excelx.Column {
	return []excelx.Column{
		{Header: "id", Value: pick("id")},
		{Header: "客户端id", Value: pick("clientId")},
		{Header: "客户端key", Value: pick("clientKey")},
		{Header: "客户端秘钥", Value: pick("clientSecret")},
		{Header: "授权类型", Value: pick("grantTypeList")},
		{Header: "token活跃超时时间", Value: pick("activeTimeout")},
		{Header: "token固定超时时间", Value: pick("timeout")},
		{Header: "状态", Value: pick("status")},
	}
}

func pick(field string) func(any) any {
	return func(row any) any {
		m, ok := row.(map[string]any)
		if !ok {
			return nil
		}
		return m[field]
	}
}

// anySlice 出参 → 驼峰键 map；grantTypeList 转逗号串（导出契约）。
func anySlice(rows []Out) []any {
	out := make([]any, 0, len(rows))
	for i := range rows {
		m := map[string]any{
			"id":            nil,
			"clientId":      nil,
			"clientKey":     nil,
			"clientSecret":  nil,
			"grantTypeList": nil,
			"grantType":     nil,
			"deviceType":    nil,
			"activeTimeout": nil,
			"timeout":       nil,
			"status":        nil,
		}
		if rows[i].ID != nil {
			m["id"] = rows[i].ID.Int64()
		}
		if rows[i].ClientID != nil {
			m["clientId"] = *rows[i].ClientID
		}
		if rows[i].ClientKey != nil {
			m["clientKey"] = *rows[i].ClientKey
		}
		if rows[i].ClientSecret != nil {
			m["clientSecret"] = *rows[i].ClientSecret
		}
		m["grantTypeList"] = strings.Join(rows[i].GrantTypeList, ",")
		if rows[i].GrantType != nil {
			m["grantType"] = *rows[i].GrantType
		}
		if rows[i].DeviceType != nil {
			m["deviceType"] = *rows[i].DeviceType
		}
		if rows[i].ActiveTimeout != nil {
			m["activeTimeout"] = rows[i].ActiveTimeout.Int64()
		}
		if rows[i].Timeout != nil {
			m["timeout"] = rows[i].Timeout.Int64()
		}
		if rows[i].Status != nil {
			m["status"] = *rows[i].Status
		}
		out = append(out, m)
	}
	return out
}

// ---- 内部工具 ----

func readBody(c *gin.Context) []byte {
	if c.Request.Body == nil {
		return nil
	}
	buf, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil
	}
	return buf
}

func pathInt(c *gin.Context, name string) (int64, error) {
	raw := strings.TrimSpace(c.Param(name))
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, errs.BadRequest(msgValidInt)
	}
	return n, nil
}
