// 本文件实现字典域的 HTTP 端点，对应 Python 端 module_system/dict/controller.py。
//
// ⚠️ 路由形态的三个反直觉点（照抄不修正）：
//   - refreshCache 是 **DELETE** 方法（不是 POST）
//   - optionselect 与 /data/type/{dict_type} 只挂登录、**不挂权限串**
//   - 删除的 ID 串解析失败文案是「参数格式错误」（不是 post 的「岗位ID格式有误」），
//     且空串解析为**空列表**（不报「不能为空」），删除 0 条也返回成功

package dict

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

// Handler 字典域端点。
type Handler struct {
	db    *gorm.DB
	errs  *errs.Handler
	perm  *permguard.Guard
	cache *cachex.Client
}

// NewHandler 构造 handler。perm 可为 nil（不施加守卫，仅单测）；cache 可为 nil（降级查库）。
func NewHandler(db *gorm.DB, eh *errs.Handler, perm *permguard.Guard, cache *cachex.Client) *Handler {
	return &Handler{db: db, errs: eh, perm: perm, cache: cache}
}

// Mount 注册字典路由。挂载点是 /system/dict（由 bootstrap 决定）。
//
// ⚠️ 动态段（/type/:dict_id 等）必须注册在同方法静态路由之后。
func (h *Handler) Mount(b routex.Binder) {
	require := func(perm string) gin.HandlerFunc {
		if h.perm == nil {
			return func(c *gin.Context) { c.Next() }
		}
		return h.perm.Require(perm)
	}

	b.GET("/type/list", require("system:dict:list"), h.TypeList)
	b.POST("/type/export", require("system:dict:export"), h.TypeExport)
	b.GET("/type/optionselect", h.TypeOptionSelect)
	b.DELETE("/type/refreshCache", require("system:dict:remove"), h.RefreshCache)
	b.GET("/type/:dict_id", require("system:dict:query"), h.TypeGet)
	b.POST("/type", require("system:dict:add"), h.TypeAdd)
	b.PUT("/type", require("system:dict:edit"), h.TypeUpdate)
	b.DELETE("/type/:dict_ids", require("system:dict:remove"), h.TypeDelete)

	b.GET("/data/list", require("system:dict:list"), h.DataList)
	b.POST("/data/export", require("system:dict:export"), h.DataExport)
	b.GET("/data/type/:dict_type", h.DataByType)
	b.GET("/data/:dict_code", require("system:dict:query"), h.DataGet)
	b.POST("/data", require("system:dict:add"), h.DataAdd)
	b.PUT("/data", require("system:dict:edit"), h.DataUpdate)
	b.DELETE("/data/:dict_codes", require("system:dict:remove"), h.DataDelete)
}

// service 取出当前请求的业务层。ok=false 表示路由未挂认证中间件（编程错误）。
func (h *Handler) service(c *gin.Context) (*Service, bool) {
	ac, ok := authctx.From(c)
	if !ok {
		h.errs.Abort(c, errs.NotLogin())
		return nil, false
	}
	return NewService(h.db, ac, h.cache), true
}

// ---- 字典类型 ----

// TypeList GET /system/dict/type/list
func (h *Handler) TypeList(c *gin.Context) {
	q, err := BindTypeQuery(c)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	rows, total, err := svc.TypePage(c.Request.Context(), q)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.Page(c, rows, total)
}

// TypeExport POST /system/dict/type/export
//
// ⚠️ 参数走**查询串**（Python 端 Depends() 注入），不是 form——与 post 导出不同。
func (h *Handler) TypeExport(c *gin.Context) {
	q, err := BindTypeQuery(c)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	rows, err := svc.TypeListAll(c.Request.Context(), q)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if err := excelx.Export(c, typeAnySlice(rows), typeExportColumns(), "字典类型"); err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgSystemError))
		return
	}
}

// TypeOptionSelect GET /system/dict/type/optionselect（仅登录，无权限串）
func (h *Handler) TypeOptionSelect(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	rows, err := svc.TypeOptionSelect(c.Request.Context())
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, rows)
}

// RefreshCache DELETE /system/dict/type/refreshCache
func (h *Handler) RefreshCache(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	svc.ResetCache(c.Request.Context())
	envelope.OK(c, nil)
}

// TypeGet GET /system/dict/type/{dict_id}
func (h *Handler) TypeGet(c *gin.Context) {
	id, err := pathInt(c, "dict_id")
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.TypeGet(c.Request.Context(), id)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, data)
}

// TypeAdd POST /system/dict/type
func (h *Handler) TypeAdd(c *gin.Context) {
	in, err := DecodeTypeCreate(readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	if err := svc.CheckTypeUnique(ctx, in.DictType, nil); err != nil {
		if errors.Is(err, errDuplicateType) {
			h.errs.Abort(c, errs.Newf("新增字典'%s'失败，字典类型已存在", deref(in.DictName)))
			return
		}
		h.errs.Abort(c, err)
		return
	}
	if err := svc.InsertType(ctx, in); err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, nil)
}

// TypeUpdate PUT /system/dict/type
func (h *Handler) TypeUpdate(c *gin.Context) {
	in, err := DecodeTypeUpdate(readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	if err := svc.CheckTypeUnique(ctx, in.DictType, in.ID); err != nil {
		if errors.Is(err, errDuplicateType) {
			h.errs.Abort(c, errs.Newf("修改字典'%s'失败，字典类型已存在", deref(in.DictName)))
			return
		}
		h.errs.Abort(c, err)
		return
	}
	if err := svc.UpdateType(ctx, in); err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, nil)
}

// TypeDelete DELETE /system/dict/type/{dict_ids}
func (h *Handler) TypeDelete(c *gin.Context) {
	ids, err := parseIDs(c.Param("dict_ids"))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	if err := svc.DeleteTypes(c.Request.Context(), ids); err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, nil)
}

// ---- 字典数据 ----

// DataList GET /system/dict/data/list
func (h *Handler) DataList(c *gin.Context) {
	q, err := BindDataQuery(c)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	rows, total, err := svc.DataPage(c.Request.Context(), q)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.Page(c, rows, total)
}

// DataExport POST /system/dict/data/export
//
// ⚠️ isDefault 按 sys_yes_no 转标签（Y→是 / N→否）后再导出，照抄 controller。
func (h *Handler) DataExport(c *gin.Context) {
	q, err := BindDataQuery(c)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	rows, err := svc.DataListAll(c.Request.Context(), q)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if err := excelx.Export(c, dataAnySlice(rows), dataExportColumns(), "字典数据"); err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgSystemError))
		return
	}
}

// DataByType GET /system/dict/data/type/{dict_type}（仅登录，无权限串）
func (h *Handler) DataByType(c *gin.Context) {
	dictType := c.Param("dict_type")
	svc, ok := h.service(c)
	if !ok {
		return
	}
	rows, err := svc.DataByType(c.Request.Context(), dictType)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if rows == nil {
		rows = []DataOut{}
	}
	envelope.OK(c, rows)
}

// DataGet GET /system/dict/data/{dict_code}
func (h *Handler) DataGet(c *gin.Context) {
	id, err := pathInt(c, "dict_code")
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.DataGet(c.Request.Context(), id)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, data)
}

// DataAdd POST /system/dict/data
func (h *Handler) DataAdd(c *gin.Context) {
	in, err := DecodeDataCreate(readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	if err := svc.CheckDataUnique(ctx, in.DictType, in.DictValue, nil); err != nil {
		if errors.Is(err, errDuplicateValue) {
			h.errs.Abort(c, errs.Newf("新增字典数据'%s'失败，字典键值已存在", deref(in.DictValue)))
			return
		}
		h.errs.Abort(c, err)
		return
	}
	if err := svc.InsertData(ctx, in); err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, nil)
}

// DataUpdate PUT /system/dict/data
func (h *Handler) DataUpdate(c *gin.Context) {
	in, err := DecodeDataUpdate(readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	if err := svc.CheckDataUnique(ctx, in.DictType, in.DictValue, in.ID); err != nil {
		if errors.Is(err, errDuplicateValue) {
			h.errs.Abort(c, errs.Newf("修改字典数据'%s'失败，字典键值已存在", deref(in.DictValue)))
			return
		}
		h.errs.Abort(c, err)
		return
	}
	if err := svc.UpdateData(ctx, in); err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, nil)
}

// DataDelete DELETE /system/dict/data/{dict_codes}
func (h *Handler) DataDelete(c *gin.Context) {
	ids, err := parseIDs(c.Param("dict_codes"))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	if err := svc.DeleteData(c.Request.Context(), ids); err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, nil)
}

// ---- 导出列（顺序即列序，键为驼峰出参键）----

func typeExportColumns() []excelx.Column {
	return []excelx.Column{
		{Header: "字典主键", Value: pick("id")},
		{Header: "字典名称", Value: pick("dictName")},
		{Header: "字典类型", Value: pick("dictType")},
		{Header: "备注", Value: pick("remark")},
		{Header: "创建时间", Value: pick("createTime")},
	}
}

func dataExportColumns() []excelx.Column {
	return []excelx.Column{
		{Header: "字典编码", Value: pick("id")},
		{Header: "字典排序", Value: pick("dictSort")},
		{Header: "字典标签", Value: pick("dictLabel")},
		{Header: "字典键值", Value: pick("dictValue")},
		{Header: "字典类型", Value: pick("dictType")},
		{Header: "是否默认", Value: pick("isDefault")},
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

// typeAnySlice 出参 → 驼峰键 map（等价 model_dump(by_alias=True, mode="json")）。
func typeAnySlice(rows []TypeOut) []any {
	out := make([]any, 0, len(rows))
	for i := range rows {
		out = append(out, typeCamelMap(rows[i]))
	}
	return out
}

func typeCamelMap(o TypeOut) map[string]any {
	m := map[string]any{
		"id":         nil,
		"dictName":   nil,
		"dictType":   nil,
		"remark":     nil,
		"createTime": nil,
	}
	if o.ID != nil {
		m["id"] = o.ID.Int64()
	}
	if o.DictName != nil {
		m["dictName"] = *o.DictName
	}
	if o.DictType != nil {
		m["dictType"] = *o.DictType
	}
	if o.Remark != nil {
		m["remark"] = *o.Remark
	}
	if o.CreateTime != nil {
		m["createTime"] = o.CreateTime.Time().Format("2006-01-02 15:04:05")
	}
	return m
}

// dataAnySlice 出参 → 驼峰键 map；isDefault 先按 sys_yes_no 转标签（导出契约）。
func dataAnySlice(rows []DataOut) []any {
	yesNo := map[string]string{"Y": "是", "N": "否"}
	out := make([]any, 0, len(rows))
	for i := range rows {
		m := dataCamelMap(rows[i])
		if v, ok := m["isDefault"].(string); ok {
			if label, hit := yesNo[v]; hit {
				m["isDefault"] = label
			}
		}
		out = append(out, m)
	}
	return out
}

func dataCamelMap(o DataOut) map[string]any {
	m := map[string]any{
		"id":         nil,
		"dictSort":   nil,
		"dictLabel":  nil,
		"dictValue":  nil,
		"dictType":   nil,
		"cssClass":   nil,
		"listClass":  nil,
		"isDefault":  nil,
		"remark":     nil,
		"createTime": nil,
	}
	if o.ID != nil {
		m["id"] = o.ID.Int64()
	}
	if o.DictSort != nil {
		m["dictSort"] = o.DictSort.Int64()
	}
	if o.DictLabel != nil {
		m["dictLabel"] = *o.DictLabel
	}
	if o.DictValue != nil {
		m["dictValue"] = *o.DictValue
	}
	if o.DictType != nil {
		m["dictType"] = *o.DictType
	}
	if o.CssClass != nil {
		m["cssClass"] = *o.CssClass
	}
	if o.ListClass != nil {
		m["listClass"] = *o.ListClass
	}
	if o.IsDefault != nil {
		m["isDefault"] = *o.IsDefault
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

// pathInt 解析整型路径参数，失败时给出 Pydantic 同款文案。
func pathInt(c *gin.Context, name string) (int64, error) {
	raw := strings.TrimSpace(c.Param(name))
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, errs.BadRequest(msgValidInt)
	}
	return n, nil
}

// parseIDs 对应 controller._parse_ids：逗号切分去空白，空串 → 空列表（不报错）；
// 任一非整数 → 400「参数格式错误」。
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
