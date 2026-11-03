// 本文件实现菜单 → 前端路由的构建逻辑，对应 Python 端 menu/service.py。
//
// P3 窄切片只落地 GET /system/menu/getRouters —— 它是前端**动态路由的命脉**，
// 没有它登录后进不了主页；其余 8 个端点随 menu 模块整体补齐。

package menu

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
)

// 内链地址替换用到的字面量，对应 Python 端 Constants.HTTP / HTTPS / WWW。
const (
	schemeHTTP  = "http://"
	schemeHTTPS = "https://"
	prefixWWW   = "www."
)

// isCacheNo / visibleHidden 是菜单表里两个反直觉的 CHAR(1) 取值：
//
//	is_cache: '0' 缓存、'1' 不缓存   → noCache = (is_cache == '1')
//	visible : '0' 显示、'1' 隐藏     → hidden  = (visible  == '1')
const (
	isCacheNo     = "1"
	visibleHidden = "1"
)

// Service 菜单域业务层，对应 Python 端 MenuService。
type Service struct {
	repo *Repo
	auth authctx.Context
}

// NewService 构造业务层。
func NewService(db *gorm.DB, ac authctx.Context) *Service {
	return &Service{repo: NewRepo(db), auth: ac}
}

// GetRouters 对应 controller.get_routers：查菜单 → 构建前端路由树。
//
// 返回值恒为非 nil 切片：无授权菜单时输出 [] 而非 null，
// 前端 `routers.forEach` 才不会崩（Python 端列表推导的结果同样是 []）。
func (s *Service) GetRouters(ctx context.Context) ([]Router, error) {
	menus, err := s.repo.SelectMenuTreeByUserID(ctx, s.auth)
	if err != nil {
		return nil, fmt.Errorf("查询菜单失败: %w", err)
	}
	return s.BuildMenus(menus), nil
}

// BuildMenus 对应 MenuService.build_menus：扁平菜单 → 树 → 前端路由。
func (s *Service) BuildMenus(menus []SysMenu) []Router {
	return buildRouters(buildTree(menus))
}

// node 把菜单与其子节点配对，对应 Python 端的 MenuNode dataclass。
type node struct {
	menu     *SysMenu
	children []node
}

// buildTree 按 parent_id 分组后从顶级(0)开始递归，对应 _build_tree + _build_children。
//
// ⚠️ parent_id 为 **NULL** 的菜单必须跳过：Python 端把它归到字典的 None 键下，
// 而 _build_children 只用整数 parent_id 查找，None 键永远取不到 —— 等价于丢弃。
// 若在这里用 `ParentIDOrZero()` 之类的写法把 NULL 当 0，会凭空多出一批顶级路由。
func buildTree(menus []SysMenu) []node {
	grouped := make(map[int64][]*SysMenu, len(menus))
	for i := range menus {
		m := &menus[i]
		if m.ParentID == nil {
			continue
		}
		grouped[*m.ParentID] = append(grouped[*m.ParentID], m)
	}
	return buildChildren(grouped, TopParentID)
}

func buildChildren(grouped map[int64][]*SysMenu, parentID int64) []node {
	parents := grouped[parentID]
	nodes := make([]node, 0, len(parents))
	for _, m := range parents {
		nodes = append(nodes, node{menu: m, children: buildChildren(grouped, m.ID)})
	}
	return nodes
}

// buildRouters 把菜单树转成前端路由，对应 _build_routers + router_to_dict。
//
// 三个互斥分支（顺序即优先级，照抄 Python 端 if/elif 链）：
//  1. 有子节点且自身是目录 → alwaysShow + redirect=noRedirect + 递归子路由
//  2. 是「菜单内部跳转」   → 自身 meta 置空，把真实页面塞进唯一的子路由
//  3. 顶级内链             → path 收敛成 "/"，子路由用 InnerLink 组件承载外链
func buildRouters(nodes []node) []Router {
	routers := make([]Router, 0, len(nodes))
	for _, n := range nodes {
		m := n.menu
		idSuffix := strconv.FormatInt(m.ID, 10)

		r := Router{
			Hidden:    boolPtr(deref(m.Visible) == visibleHidden),
			Name:      strOrNil(routeName(m) + idSuffix),
			Path:      strOrNil(routerPath(m)),
			Component: strOrNil(componentInfo(m)),
			Query:     strOrNilPtr(m.QueryParam),
			Meta:      buildMeta(m),
		}

		switch {
		case len(n.children) > 0 && deref(m.MenuType) == TypeDir:
			r.AlwaysShow = boolPtr(true)
			r.Redirect = strOrNil("noRedirect")
			r.Children = buildRouters(n.children)

		case isMenuFrame(m):
			// 菜单内部跳转：外层只当容器（无 meta），真实页面在子路由里
			r.Meta = nil
			r.Children = []Router{{
				Path:      strOrNilPtr(m.Path),
				Component: strOrNilPtr(m.Component),
				Name:      strOrNil(capitalize(deref(m.Path)) + idSuffix),
				Meta:      buildMeta(m),
				Query:     strOrNilPtr(m.QueryParam),
			}}

		case isTopLevel(m) && isInnerLink(m):
			r.Meta = &Meta{Title: strPtr(m.MenuName), Icon: m.Icon}
			r.Path = strOrNil("/")
			inner := innerLinkReplaceEach(deref(m.Path))
			r.Children = []Router{{
				Path:      strOrNil(inner),
				Component: strOrNil(InnerLink),
				Name:      strOrNil(capitalize(inner) + idSuffix),
				Meta: &Meta{
					Title: strPtr(m.MenuName),
					Icon:  m.Icon,
					Link:  strOrNilPtr(m.Path),
				},
			}}
		}

		routers = append(routers, r)
	}
	return routers
}

// buildMeta 构建完整 meta，对应 _build_meta。
//
// ⚠️ 五个键**全量输出**（含 null），与 Router 的空值过滤规则不同。
func buildMeta(m *SysMenu) *Meta {
	meta := &Meta{
		Title:   strPtr(m.MenuName),
		Icon:    m.Icon,
		NoCache: boolPtr(deref(m.IsCache) == isCacheNo),
	}
	if isHTTP(deref(m.Path)) {
		meta.Link = m.Path
	}
	// 备注以 "/" 开头时用作高亮菜单（RuoYi 的约定：把激活态挂在别的菜单上）
	if startsWithSlash(deref(m.Remark)) {
		meta.ActiveMenu = m.Remark
	}
	return meta
}

// ==================== 路由辅助（对应 menu/service.py 的模块级函数） ====================

// isTopLevel 判断 parent_id 是否为顶级(0)。
//
// ⚠️ NULL **不等于** 0：Python 端 `menu.parent_id == Constants.TOP_PARENT_ID`
// 在 parent_id 为 None 时是 False。用「解引用后比较」会把 NULL 误判成顶级，
// 进而让本该是内链的菜单走进一级目录分支。
func isTopLevel(m *SysMenu) bool {
	return m.ParentID != nil && *m.ParentID == TopParentID
}

// isMenuFrame 是否为「菜单内部跳转」，对应 is_menu_frame。
func isMenuFrame(m *SysMenu) bool {
	return isTopLevel(m) && deref(m.MenuType) == TypeMenu && deref(m.IsFrame) == NoFrame
}

// isInnerLink 是否为内链组件，对应 is_inner_link。
func isInnerLink(m *SysMenu) bool {
	return deref(m.IsFrame) == NoFrame && isHTTP(deref(m.Path))
}

// isParentView 是否为 parent_view 组件，对应 is_parent_view。
func isParentView(m *SysMenu) bool {
	return !isTopLevel(m) && deref(m.MenuType) == TypeDir
}

// routeName 获取路由名称（path 首字母大写；菜单内部跳转时为空），对应 get_route_name。
func routeName(m *SysMenu) string {
	if isMenuFrame(m) {
		return ""
	}
	return capitalize(deref(m.Path))
}

// routerPath 获取路由地址，对应 get_router_path。
func routerPath(m *SysMenu) string {
	p := deref(m.Path)
	if !isTopLevel(m) && isInnerLink(m) {
		p = innerLinkReplaceEach(p)
	}
	// 下面两个分支在 Python 端读的是**原始** menu.path（不是上面改写后的值）；
	// 由于「非顶级内链」与「顶级」互斥，走到这里时 p 仍等于原始值，可直接用
	if isTopLevel(m) && deref(m.MenuType) == TypeDir && deref(m.IsFrame) == NoFrame {
		p = "/" + p
	} else if isMenuFrame(m) {
		p = "/"
	}
	return p
}

// componentInfo 获取组件信息，对应 get_component_info。
func componentInfo(m *SysMenu) string {
	empty := isEmpty(m.Component)
	switch {
	case !empty && !isMenuFrame(m):
		return *m.Component
	case empty && !isTopLevel(m) && isInnerLink(m):
		return InnerLink
	case empty && isParentView(m):
		return ParentView
	default:
		return Layout
	}
}

// innerLinkReplaceEach 内链域名特殊字符替换，对应 inner_link_replace_each。
//
// ⚠️ **替换顺序就是契约**：先剥协议与 www. 前缀，再把 '.' 与 ':' 换成 '/'。
// 颠倒顺序会得到完全不同的路径（例如先替换 '.' 会破坏 "http://" 里的结构）。
func innerLinkReplaceEach(path string) string {
	if path == "" {
		return path
	}
	for _, r := range [...]struct{ old, new string }{
		{schemeHTTP, ""},
		{schemeHTTPS, ""},
		{prefixWWW, ""},
		{".", "/"},
		{":", "/"},
	} {
		path = strings.ReplaceAll(path, r.old, r.new)
	}
	return path
}

// capitalize 首字母大写（仅首字符，其余不变），对应 _capitalize。
//
// 按 **rune** 而非 byte 切：path 理论上可以含非 ASCII 字符，
// 按字节切会把多字节字符劈成非法 UTF-8。
func capitalize(v string) string {
	if v == "" {
		return ""
	}
	r, size := utf8.DecodeRuneInString(v)
	return strings.ToUpper(string(r)) + v[size:]
}

// ==================== 字符串工具（对应 utils/string_util.py） ====================

// isHTTP 对应 is_http：非空且以 http:// 或 https:// 开头。
func isHTTP(url string) bool {
	return url != "" && (strings.HasPrefix(url, schemeHTTP) || strings.HasPrefix(url, schemeHTTPS))
}

// isEmpty 对应 is_empty：None 或长度为 0（按**字符数**，不是字节数）。
func isEmpty(v *string) bool {
	return v == nil || utf8.RuneCountInString(*v) == 0
}

// startsWithSlash 对应 start_with_any_ignore_case(value, "/")。
func startsWithSlash(v string) bool {
	return v != "" && strings.HasPrefix(strings.ToLower(v), "/")
}

// ==================== 序列化辅助 ====================

// strOrNil 把空串归一成 nil，对应 router_to_dict 的 `if router.X not in (None, "")`。
func strOrNil(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// strOrNilPtr 是 strOrNil 的指针版（入参本身就是可空列）。
func strOrNilPtr(v *string) *string {
	if v == nil {
		return nil
	}
	return strOrNil(*v)
}

// strPtr 用于 NOT NULL 列：值恒存在，故恒返回非 nil 指针。
func strPtr(v string) *string {
	s := v
	return &s
}

func boolPtr(v bool) *bool {
	b := v
	return &b
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
