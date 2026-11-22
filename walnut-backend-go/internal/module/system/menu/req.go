// 本文件定义菜单域的入参 DTO（查询绑定与新增/修改解码），对应 Python 端 menu/schema.py。
//
// 写入入参走阶段 4 惯用绑定（validate tag + bindx.BindJSON + i18n 文案）；
// 查询 cluster（BindQuery）仍保留 Pydantic 兼容原文。

package menu

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"

	"walnut-seed-go/internal/pkg/bindx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
)

// BindQuery 从查询串解析。parentId 非法整数 → Pydantic 400 原文。
// Query 类型定义在 repo.go（SelectMenuList 的入参）。
func BindQuery(q crudx.Querier) (Query, error) {
	out := Query{
		MenuName: q.Query("menuName"),
		Visible:  q.Query("visible"),
		Status:   q.Query("status"),
		MenuType: q.Query("menuType"),
	}
	if raw, ok := q.GetQuery("parentId"); ok && strings.TrimSpace(raw) != "" {
		n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			return Query{}, errs.BadRequest(msgValidInt)
		}
		out.ParentID = &n
	}
	return out, nil
}

// CreateIn 新增/修改菜单入参，对应 MenuCreateSchema。
//
// ⚠️ 模型级 validate_default=True：**缺省字段也跑校验器**。
type CreateIn struct {
	ID         *int64
	ParentID   *int64
	MenuName   *string
	OrderNum   *int
	Path       *string
	Component  *string
	QueryParam *string
	IsFrame    *string
	IsCache    *string
	MenuType   *string
	Visible    *string
	Status     *string
	Perms      *string
	Icon       *string
	Remark     *string
}

// UpdateIn 修改菜单入参（id 必填）。
type UpdateIn struct {
	CreateIn
}

// Pydantic v2 类型错误原文（BindQuery 与 handler 的 ID 串解析仍在用）。
const msgValidInt = bindx.MsgValidInt

// permsRe 对应 RegexConstants.PERMISSION_STRING（re.match + $ → 补 NL 分支）。
var (
	permsRe   = regexp.MustCompile(`^$|^[a-zA-Z0-9_]+:[a-zA-Z0-9_*]+:[a-zA-Z0-9_*]+$`)
	permsReNL = regexp.MustCompile(`^$|^[a-zA-Z0-9_]+:[a-zA-Z0-9_*]+:[a-zA-Z0-9_*]+\n$`)
)

func init() {
	// pyperms：逐字复刻 RegexConstants.PERMISSION_STRING（空串合法、末尾换行合法）。
	bindx.RegisterTag("pyperms", func(fl validator.FieldLevel) bool {
		s := fl.Field().String()
		return permsRe.MatchString(s) || permsReNL.MatchString(s)
	})
	// jsonobj：queryParam 非空白时必须是 JSON 对象。
	// ⚠️ "null" 能被 Unmarshal 进 map（得到 nil map），但 Python json.loads→None
	// 非 dict → 400，故必须额外判 obj == nil。
	bindx.RegisterTag("jsonobj", func(fl validator.FieldLevel) bool {
		s := fl.Field().String()
		if strings.TrimSpace(s) == "" {
			return true
		}
		var obj map[string]any
		return json.Unmarshal([]byte(s), &obj) == nil && obj != nil
	})
}

// ---- 阶段 4 惯用绑定：validate tag + bindx.BindJSON + i18n 文案 ----
//
// 相对手写 Decode* 的语义演进（刻意，见 docs/go/04-接口契约）：
//   - 严格字符串回退退役（"5"/"3.0" 不再接受），严格 JSON 类型
//   - 类型错误文案走 i18n（不再是 Pydantic 英文原文）
//   - update 的 id 缺省或 null 统一报必填（旧实现缺省报 Field required、null 报 int_type）
//   - update 不再重复解析 id（旧实现基类+必填各报一次的怪癖退役）
//   - JSON 类型错误短路（encoding/json 限制），校验级错误仍全量按声明序收集

// CreateReq 新增菜单请求体，对应 MenuCreateSchema（模型级 validate_default）。
// 字段声明序镜像旧 Decode 的校验序。
type CreateReq struct {
	ID         *int64  `json:"id" label:"菜单ID"`
	ParentID   *int64  `json:"parentId" label:"父菜单ID"`
	MenuName   string  `json:"menuName" validate:"required,notblank,max=50" label:"菜单名称"`
	OrderNum   *int    `json:"orderNum" validate:"required" label:"显示顺序"`
	Path       *string `json:"path" validate:"omitnil,max=200" label:"路由地址"`
	Component  *string `json:"component" validate:"omitnil,max=200" label:"组件路径"`
	QueryParam *string `json:"queryParam" validate:"omitnil,jsonobj" label:"路由参数"`
	IsFrame    *string `json:"isFrame" label:"是否为外链"`
	IsCache    *string `json:"isCache" label:"是否缓存"`
	Visible    *string `json:"visible" label:"显示状态"`
	Status     *string `json:"status" label:"菜单状态"`
	Icon       *string `json:"icon" label:"菜单图标"`
	Remark     *string `json:"remark" label:"备注"`
	MenuType   string  `json:"menuType" validate:"required,notblank" label:"菜单类型"`
	Perms      *string `json:"perms" validate:"omitnil,max=100,pyperms" label:"权限标识"`
}

// UpdateReq 修改菜单请求体（id 必填，追加在父类字段之后；
// 外层 id 遮蔽嵌入 CreateReq 的可选 id，JSON 与 validator 均以外层为准）。
type UpdateReq struct {
	CreateReq
	ID *int64 `json:"id" validate:"required" label:"菜单ID"`
}

func (r CreateReq) toIn() CreateIn {
	return CreateIn{
		ID:         r.ID,
		ParentID:   r.ParentID,
		MenuName:   &r.MenuName,
		OrderNum:   r.OrderNum,
		Path:       r.Path,
		Component:  r.Component,
		QueryParam: r.QueryParam,
		IsFrame:    r.IsFrame,
		IsCache:    r.IsCache,
		MenuType:   &r.MenuType,
		Visible:    r.Visible,
		Status:     r.Status,
		Perms:      r.Perms,
		Icon:       r.Icon,
		Remark:     r.Remark,
	}
}

// DecodeCreate 解析并校验新增入参（service 消费 CreateIn，零改动）。
func DecodeCreate(ctx context.Context, body []byte) (CreateIn, error) {
	var req CreateReq
	if err := bindx.BindJSON(ctx, body, &req); err != nil {
		return CreateIn{}, err
	}
	return req.toIn(), nil
}

// DecodeUpdate 解析修改入参。
func DecodeUpdate(ctx context.Context, body []byte) (UpdateIn, error) {
	var req UpdateReq
	if err := bindx.BindJSON(ctx, body, &req); err != nil {
		return UpdateIn{}, err
	}
	in := req.CreateReq.toIn()
	in.ID = req.ID
	return UpdateIn{CreateIn: in}, nil
}
