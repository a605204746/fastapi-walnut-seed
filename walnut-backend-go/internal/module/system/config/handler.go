// 本文件实现参数配置域的 HTTP 端点，对应 Python 端 module_system/config/controller.py。
//
// ⚠️ 三个反直觉点（照抄）：
//   - refreshCache 是 DELETE 方法；
//   - /configKey/{key} 只挂登录、不挂权限串，且未找到时 data 是**空串**不是 null；
//   - 导出参数走查询串（Depends 注入），不是 form（与 post 导出不同）。

package config

import (
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/cachex"
	"walnut-seed-go/internal/pkg/envelope"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/excelx"
	"walnut-seed-go/internal/pkg/permguard"
	"walnut-seed-go/internal/pkg/routex"
)

// Handler 参数配置域端点。
type Handler struct {
	db    *gorm.DB
	errs  *errs.Handler
	perm  *permguard.Guard
	cache *cachex.Client
}

// NewHandler 构造 handler。perm / cache 可为 nil。
func NewHandler(db *gorm.DB, eh *errs.Handler, perm *permguard.Guard, cache *cachex.Client) *Handler {
	return &Handler{db: db, errs: eh, perm: perm, cache: cache}
}

// Mount 注册参数配置路由。挂载点是 /system/config（由 bootstrap 决定）。
func (h *Handler) Mount(b routex.Binder) {
	require := func(perm string) gin.HandlerFunc {
		if h.perm == nil {
			return func(c *gin.Context) { c.Next() }
		}
		return h.perm.Require(perm)
	}

	b.GET("/list", require("system:config:list"), h.List)
	b.POST("/export", require("system:config:export"), h.Export)
	b.GET("/configKey/:config_key", h.GetByKey)
	b.PUT("/updateByKey", require("system:config:edit"), h.UpdateByKey)
	b.DELETE("/refreshCache", require("system:config:remove"), h.RefreshCache)
	b.GET("/:config_id", require("system:config:query"), h.GetByID)
	b.POST("", require("system:config:add"), h.Add)
	b.PUT("", require("system:config:edit"), h.Update)
	b.DELETE("/:config_ids", require("system:config:remove"), h.Delete)
}

// service 取出当前请求的业务层。
func (h *Handler) service(c *gin.Context) (*Service, bool) {
	ac, ok := authctx.From(c)
	if !ok {
		h.errs.Abort(c, errs.NotLogin())
		return nil, false
	}
	return NewService(h.db, ac, h.cache), true
}

// List GET /system/config/list
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

// Export POST /system/config/export
//
// ⚠️ configType 按 sys_yes_no 转标签（Y→是 / N→否）后再导出。
func (h *Handler) Export(c *gin.Context) {
	q, err := BindQuery(c)
	if err != nil {
		h.errs.Abort(c, err)
		return
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
	if err := excelx.Export(c, anySlice(rows), exportColumns(), "参数数据"); err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgSystemError))
		return
	}
}

// GetByKey GET /system/config/configKey/{config_key}（仅登录）
func (h *Handler) GetByKey(c *gin.Context) {
	key := c.Param("config_key")
	svc, ok := h.service(c)
	if !ok {
		return
	}
	value, err := svc.GetByKey(c.Request.Context(), key)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, value)
}

// UpdateByKey PUT /system/config/updateByKey
func (h *Handler) UpdateByKey(c *gin.Context) {
	in, err := DecodeUpdateByKey(readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	err = svc.Update(c.Request.Context(), updateParam(in))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, nil)
}

// RefreshCache DELETE /system/config/refreshCache
func (h *Handler) RefreshCache(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	svc.ResetCache(c.Request.Context())
	envelope.OK(c, nil)
}

// GetByID GET /system/config/{config_id}
func (h *Handler) GetByID(c *gin.Context) {
	id, err := pathInt(c, "config_id")
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
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, data)
}

// Add POST /system/config
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
	if err := svc.CheckKeyUnique(ctx, in.ConfigKey, nil); err != nil {
		if errors.Is(err, errDuplicateKey) {
			h.errs.Abort(c, errs.Newf("新增参数'%s'失败，参数键名已存在", deref(in.ConfigName)))
			return
		}
		h.errs.Abort(c, err)
		return
	}
	if err := svc.Insert(ctx, in); err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, nil)
}

// Update PUT /system/config
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
	if err := svc.CheckKeyUnique(ctx, in.ConfigKey, in.ID); err != nil {
		if errors.Is(err, errDuplicateKey) {
			h.errs.Abort(c, errs.Newf("修改参数'%s'失败，参数键名已存在", deref(in.ConfigName)))
			return
		}
		h.errs.Abort(c, err)
		return
	}
	if err := svc.Update(ctx, updateParam{
		ID:          in.ID,
		ConfigName:  in.ConfigName,
		ConfigKey:   in.ConfigKey,
		ConfigValue: in.ConfigValue,
		ConfigType:  in.ConfigType,
		Remark:      in.Remark,
	}); err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, nil)
}

// Delete DELETE /system/config/{config_ids}
func (h *Handler) Delete(c *gin.Context) {
	ids, err := parseIDs(c.Param("config_ids"))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	if err := svc.Delete(c.Request.Context(), ids); err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, nil)
}

// ---- 导出列（顺序即列序）----

func exportColumns() []excelx.Column {
	return []excelx.Column{
		{Header: "参数主键", Value: pick("id")},
		{Header: "参数名称", Value: pick("configName")},
		{Header: "参数键名", Value: pick("configKey")},
		{Header: "参数键值", Value: pick("configValue")},
		{Header: "系统内置", Value: pick("configType")},
		{Header: "备注", Value: pick("remark")},
		{Header: "创建时间", Value: pick("createTime")},
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

// anySlice 出参 → 驼峰键 map；configType 先转 是/否 标签（导出契约）。
func anySlice(rows []Out) []any {
	yesNo := map[string]string{"Y": "是", "N": "否"}
	out := make([]any, 0, len(rows))
	for i := range rows {
		m := camelMap(rows[i])
		if v, ok := m["configType"].(string); ok {
			if label, hit := yesNo[v]; hit {
				m["configType"] = label
			}
		}
		out = append(out, m)
	}
	return out
}

func camelMap(o Out) map[string]any {
	m := map[string]any{
		"id":          nil,
		"configName":  nil,
		"configKey":   nil,
		"configValue": nil,
		"configType":  nil,
		"remark":      nil,
		"createTime":  nil,
	}
	if o.ID != nil {
		m["id"] = o.ID.Int64()
	}
	if o.ConfigName != nil {
		m["configName"] = *o.ConfigName
	}
	if o.ConfigKey != nil {
		m["configKey"] = *o.ConfigKey
	}
	if o.ConfigValue != nil {
		m["configValue"] = *o.ConfigValue
	}
	if o.ConfigType != nil {
		m["configType"] = *o.ConfigType
	}
	if o.Remark != nil {
		m["remark"] = *o.Remark
	}
	if o.CreateTime != nil {
		m["createTime"] = o.CreateTime.Time().Format("2006-01-02 15:04:05")
	}
	return m
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

// parseIDs 对应 controller._parse_ids：空串 → 空列表（不报错）；非整数 → 400「参数格式错误」。
func parseIDs(raw string) ([]int64, error) {
	ids := make([]int64, 0)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, errs.BadRequest("参数格式错误")
		}
		ids = append(ids, n)
	}
	return ids, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
