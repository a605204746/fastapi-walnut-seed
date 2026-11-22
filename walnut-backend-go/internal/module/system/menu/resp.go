// 本文件定义菜单域的出参 DTO：getRouters 对应 Python 端 menu/schema.py 的
// MetaResp / RouterResp 与 menu/service.py 的 router_to_dict；管理端出参对应 MenuOutSchema。

package menu

import (
	"walnut-seed-go/internal/pkg/idtype"
	"walnut-seed-go/internal/pkg/timex"
)

// Meta 路由元信息，对应 MetaResp。
//
// ⚠️ 与 Router 不同：meta 是**全量输出**——Python 端
// `router.meta.model_dump(by_alias=True)` 不做空值过滤，5 个键恒定出现、
// 无值为 null。故这里一律不加 omitempty。
type Meta struct {
	Title   *string `json:"title"`
	Icon    *string `json:"icon"`
	NoCache *bool   `json:"noCache"`
	// Link 内链地址（http(s):// 开头），非内链时为 null
	Link *string `json:"link"`
	// ActiveMenu 高亮菜单（备注以 / 开头时取备注），否则 null
	ActiveMenu *string `json:"activeMenu"`
}

// Router 前端路由节点，对应 RouterResp + router_to_dict 的空值过滤。
//
// ⚠️ omitempty 在这里是**契约**而不是优化。Python 端 router_to_dict 的规则是
// 「None 与空字符串跳过，布尔值（含 false）保留，空集合跳过」，逐条对应：
//   - 字符串字段用 *string，构造时把 "" 归一成 nil（见 strOrNil）
//   - hidden / alwaysShow 用 *bool —— 指向 false 的指针不是零值，omitempty 不会丢它
//   - children 用切片，nil 与空切片都被 omitempty 丢掉，正好对应 `if router.children:`
//   - meta 用 *Meta，nil 时整个键消失（is_menu_frame 分支就会把它置空）
//
// 字段顺序照抄 router_to_dict 的写入顺序。
type Router struct {
	Name       *string  `json:"name,omitempty"`
	Path       *string  `json:"path,omitempty"`
	Hidden     *bool    `json:"hidden,omitempty"`
	Redirect   *string  `json:"redirect,omitempty"`
	Component  *string  `json:"component,omitempty"`
	Query      *string  `json:"query,omitempty"`
	AlwaysShow *bool    `json:"alwaysShow,omitempty"`
	Meta       *Meta    `json:"meta,omitempty"`
	Children   []Router `json:"children,omitempty"`
}

// ==================== 管理端 DTO（menu 模块 8 端点） ====================

// Out 菜单出参，对应 MenuOutSchema（键序 = 声明序）。
//
// ⚠️ Children 恒为 []（default_factory=list），不能 omitempty。
type Out struct {
	ID         *idtype.JSInt   `json:"id"`
	MenuName   *string         `json:"menuName"`
	ParentID   *idtype.JSInt   `json:"parentId"`
	OrderNum   *idtype.JSInt   `json:"orderNum"`
	Path       *string         `json:"path"`
	Component  *string         `json:"component"`
	QueryParam *string         `json:"queryParam"`
	IsFrame    *string         `json:"isFrame"`
	IsCache    *string         `json:"isCache"`
	MenuType   *string         `json:"menuType"`
	Visible    *string         `json:"visible"`
	Status     *string         `json:"status"`
	Perms      *string         `json:"perms"`
	Icon       *string         `json:"icon"`
	CreateDept *idtype.JSInt   `json:"createDept"`
	Remark     *string         `json:"remark"`
	CreateTime *timex.JSONTime `json:"createTime"`
	Children   []Out           `json:"children"`
}

// NewOut 由实体构造出参。
func NewOut(m *SysMenu) Out {
	out := Out{
		ID:         jsIntPtr(m.ID),
		MenuName:   strPtr(m.MenuName),
		ParentID:   jsIntPtrOrNil(m.ParentID),
		OrderNum:   intPtrToJSInt(m.OrderNum),
		Path:       m.Path,
		Component:  m.Component,
		QueryParam: m.QueryParam,
		IsFrame:    m.IsFrame,
		IsCache:    m.IsCache,
		MenuType:   m.MenuType,
		Visible:    m.Visible,
		Status:     m.Status,
		Perms:      m.Perms,
		Icon:       m.Icon,
		CreateDept: jsIntPtrOrNil(m.CreateDept),
		Remark:     m.Remark,
		Children:   []Out{},
	}
	if m.CreateTime != nil {
		jt := timex.FromTime(*m.CreateTime)
		out.CreateTime = &jt
	}
	return out
}

// TreeNode 菜单下拉树节点，对应 build_menu_tree_select 的手工字典。
//
// ⚠️ 键序 = 字典字面量顺序；children 后加且仅非空时出现。
type TreeNode struct {
	ID       *idtype.JSInt `json:"id"`
	ParentID *idtype.JSInt `json:"parentId"`
	Label    *string       `json:"label"`
	Weight   *idtype.JSInt `json:"weight"`
	MenuType *string       `json:"menuType"`
	Icon     *string       `json:"icon"`
	Visible  *string       `json:"visible"`
	Status   *string       `json:"status"`
	Children []*TreeNode   `json:"children,omitempty"`
}

// TreeSelectVo 角色菜单树响应，对应 MenuTreeSelectVoSchema。
type TreeSelectVo struct {
	CheckedKeys []idtype.JSInt `json:"checkedKeys"`
	Menus       []*TreeNode    `json:"menus"`
}

// ---- 小工具（与其它模块的 resp.go 同构）----

func jsIntPtr(v int64) *idtype.JSInt {
	j := idtype.JSInt(v)
	return &j
}

func jsIntPtrOrNil(v *int64) *idtype.JSInt {
	if v == nil {
		return nil
	}
	j := idtype.JSInt(*v)
	return &j
}

func intPtrToJSInt(v *int) *idtype.JSInt {
	if v == nil {
		return nil
	}
	j := idtype.JSInt(int64(*v))
	return &j
}
