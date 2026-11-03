// 本文件实现通知公告域的 HTTP 端点，对应 Python 端 module_system/notice/controller.py。
//
// ⚠️ POST/PUT 挂 RepeatSubmit（防重提交）；DELETE 的 ID 串文案是
// 「公告ID格式有误 / 公告ID不能为空」（crudx.ParseIDs 同款）。
// ⚠️ 新增成功后的全站 SSE 广播属 P4（/resource/sse 未迁移），当前无契约影响，留 TODO。

package notice

import (
	"io"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/envelope"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/idempotentx"
	"walnut-seed-go/internal/pkg/permguard"
	"walnut-seed-go/internal/pkg/routex"
)

// Handler 通知公告域端点。
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

// Mount 注册通知公告路由。挂载点是 /system/notice（由 bootstrap 决定）。
func (h *Handler) Mount(b routex.Binder) {
	require, repeat := h.guards()

	b.GET("/list", require("system:notice:list"), h.List)
	b.GET("/:notice_id", require("system:notice:query"), h.GetByID)
	b.POST("", require("system:notice:add"), repeat(), h.Add)
	b.PUT("", require("system:notice:edit"), repeat(), h.Update)
	b.DELETE("/:notice_ids", require("system:notice:remove"), h.Delete)
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

// List GET /system/notice/list
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

// GetByID GET /system/notice/{notice_id}
//
// ⚠️ 路径参数是 `notice_id: int`：非整数报 Pydantic 原文（code=400），
// 与 DELETE 的 ID 串文案（公告ID格式有误）不同，别混用。
func (h *Handler) GetByID(c *gin.Context) {
	id, err := pathInt(c, "notice_id")
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

// Add POST /system/notice
//
// TODO(P4): 新增成功后 Python 端会 sse_manager.publish_all 广播
// "[类型标签] 标题"；/resource/sse 迁移时一并补上。
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
	done, err := svc.Insert(c.Request.Context(), in)
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

// Update PUT /system/notice
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
	done, err := svc.Update(c.Request.Context(), in)
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

// Delete DELETE /system/notice/{notice_ids}
func (h *Handler) Delete(c *gin.Context) {
	ids, err := crudx.ParseIDs(c.Param("notice_ids"), "公告ID")
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	n, err := svc.DeleteByIDs(c.Request.Context(), ids)
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

// pathInt 解析整型路径参数，失败时给出 Pydantic 同款文案。
func pathInt(c *gin.Context, name string) (int64, error) {
	raw := strings.TrimSpace(c.Param(name))
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, errs.BadRequest(msgValidInt)
	}
	return n, nil
}
