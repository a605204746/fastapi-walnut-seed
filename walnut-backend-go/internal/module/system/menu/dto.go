package menu

// 本文件定义 getRouters 的出参 DTO，对应 Python 端 menu/schema.py 的
// MetaResp / RouterResp 与 menu/service.py 的 router_to_dict。

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
