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
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/idtype"
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

// ==================== 管理端业务（menu 模块 8 端点） ====================

// errDupName / errDupRoute 哨兵：handler 据此拼带菜单名的文案。
var (
	errDupName  = errs.New("菜单名称已存在")
	errDupRoute = errs.New("路由名称或地址已存在")
)

// nowFunc 提供「当前时间」，便于单测钉住审计字段。
var nowFunc = time.Now

// ListOut 菜单列表出参，对应 list_menu。
func (s *Service) ListOut(ctx context.Context, q Query) ([]Out, error) {
	rows, err := s.repo.SelectMenuList(ctx, s.auth, q)
	if err != nil {
		return nil, err
	}
	outs := make([]Out, 0, len(rows))
	for i := range rows {
		outs = append(outs, NewOut(&rows[i]))
	}
	return outs, nil
}

// GetOut 菜单详情，对应 get_menu（不存在返回 nil）。
func (s *Service) GetOut(ctx context.Context, id int64) (*Out, error) {
	row, err := s.repo.SelectMenuByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	out := NewOut(row)
	return &out, nil
}

// BuildTreeSelect 构建下拉树，对应 build_menu_tree_select。
//
// ⚠️ 根 parent 取 **第一行的 parent_id**（可能是 NULL），不是常量 0；
// 分组键含 NULL（Python 字典的 None 键），照抄。
func BuildTreeSelect(rows []SysMenu) []*TreeNode {
	if len(rows) == 0 {
		return []*TreeNode{}
	}
	type key struct {
		nilKey bool
		id     int64
	}
	grouped := make(map[key][]*SysMenu, len(rows))
	for i := range rows {
		k := key{nilKey: rows[i].ParentID == nil}
		if rows[i].ParentID != nil {
			k.id = *rows[i].ParentID
		}
		grouped[k] = append(grouped[k], &rows[i])
	}
	root := key{nilKey: rows[0].ParentID == nil}
	if rows[0].ParentID != nil {
		root.id = *rows[0].ParentID
	}
	var build func(parent key) []*TreeNode
	build = func(parent key) []*TreeNode {
		list := grouped[parent]
		nodes := make([]*TreeNode, 0, len(list))
		for _, m := range list {
			n := &TreeNode{
				ID:       jsIntPtr(m.ID),
				ParentID: jsIntPtrOrNil(m.ParentID),
				Label:    strPtr(m.MenuName),
				Weight:   intPtrToJSInt(m.OrderNum),
				MenuType: m.MenuType,
				Icon:     m.Icon,
				Visible:  m.Visible,
				Status:   m.Status,
			}
			if kids := build(key{id: m.ID}); len(kids) > 0 {
				n.Children = kids
			}
			nodes = append(nodes, n)
		}
		return nodes
	}
	return build(root)
}

// TreeSelectOut 菜单下拉树，对应 treeselect 端点。
func (s *Service) TreeSelectOut(ctx context.Context, q Query) ([]*TreeNode, error) {
	rows, err := s.repo.SelectMenuList(ctx, s.auth, q)
	if err != nil {
		return nil, err
	}
	return BuildTreeSelect(rows), nil
}

// RoleMenuTreeSelect 角色菜单树 + 勾选键，对应 roleMenuTreeselect 端点。
func (s *Service) RoleMenuTreeSelect(ctx context.Context, roleID int64) (*TreeSelectVo, error) {
	checked, err := s.repo.MenuIDsByRoleID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.SelectMenuList(ctx, s.auth, Query{})
	if err != nil {
		return nil, err
	}
	keys := make([]idtype.JSInt, 0, len(checked))
	for _, id := range checked {
		keys = append(keys, idtype.JSInt(id))
	}
	return &TreeSelectVo{CheckedKeys: keys, Menus: BuildTreeSelect(rows)}, nil
}

// CheckNameUnique 同级菜单名唯一性，对应 check_menu_name_unique。
func (s *Service) CheckNameUnique(ctx context.Context, in CreateIn) error {
	exists, err := s.repo.ExistsMenuName(ctx, deref(in.MenuName), in.ParentID, in.ID)
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	if exists {
		return errDupName
	}
	return nil
}

// routeNameOfIn 对应 _route_name_of：get_route_name 为空时回退 path。
func routeNameOfIn(in CreateIn) string {
	parentTop := in.ParentID != nil && *in.ParentID == TopParentID
	menuFrame := parentTop && deref(in.MenuType) == TypeMenu && deref(in.IsFrame) == NoFrame
	if menuFrame {
		return deref(in.Path)
	}
	name := capitalize(deref(in.Path))
	if name == "" {
		return deref(in.Path)
	}
	return name
}

// equalsIgnoreCase 对应 _equals_any_ignore_case 的双值形态。
func equalsIgnoreCase(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return strings.EqualFold(*a, *b)
}

// CheckRouteConfigUnique 路由名称/地址组合唯一性，对应 check_route_config_unique。
//
// ⚠️ 按钮(F)直接放行；三类冲突判定的顺序与忽略大小写语义照抄。
func (s *Service) CheckRouteConfigUnique(ctx context.Context, in CreateIn) error {
	if deref(in.MenuType) == TypeButton {
		return nil
	}
	menuID := int64(-1)
	if in.ID != nil {
		menuID = *in.ID
	}
	routeName := routeNameOfIn(in)
	candidates, err := s.repo.RouteConflictCandidates(ctx, deref(in.Path), routeName)
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	for i := range candidates {
		m := &candidates[i]
		if m.ID == menuID {
			continue
		}
		dbRouteName := routeNameOfMenu(m)
		if equalsIgnoreCase(in.Path, m.Path) && ptrEq(in.ParentID, m.ParentID) {
			return errDupRoute
		}
		if equalsIgnoreCase(in.Path, m.Path) &&
			in.ParentID != nil && *in.ParentID == TopParentID &&
			m.ParentID != nil && *m.ParentID == TopParentID {
			return errDupRoute
		}
		if equalsIgnoreCase(&routeName, &dbRouteName) && deref(m.MenuType) == deref(in.MenuType) {
			return errDupRoute
		}
	}
	return nil
}

// routeNameOfMenu 是 routeNameOfIn 的实体版（候选菜单侧）。
func routeNameOfMenu(m *SysMenu) string {
	if isMenuFrame(m) {
		return deref(m.Path)
	}
	name := capitalize(deref(m.Path))
	if name == "" {
		return deref(m.Path)
	}
	return name
}

func ptrEq(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// Insert 新增菜单，对应 insert_menu。
//
// ⚠️ 缺省字段走列默认值：parent_id=0、order_num=0、path=""、is_frame="1"、
// is_cache="0"、menu_type=""、visible="0"、status="0"、icon="#"、remark=""；
// component/query_param/perms 缺省为 NULL。
func (s *Service) Insert(ctx context.Context, in CreateIn) (int64, error) {
	row := SysMenu{MenuName: deref(in.MenuName)}
	if in.ParentID != nil {
		row.ParentID = in.ParentID
	} else {
		v := int64(0)
		row.ParentID = &v
	}
	if in.OrderNum != nil {
		row.OrderNum = in.OrderNum
	} else {
		v := 0
		row.OrderNum = &v
	}
	if in.Path != nil {
		row.Path = in.Path
	} else {
		row.Path = strPtr("")
	}
	row.Component = in.Component
	row.QueryParam = in.QueryParam
	if in.IsFrame != nil {
		row.IsFrame = in.IsFrame
	} else {
		row.IsFrame = strPtr(NoFrame)
	}
	if in.IsCache != nil {
		row.IsCache = in.IsCache
	} else {
		row.IsCache = strPtr("0")
	}
	if in.MenuType != nil {
		row.MenuType = in.MenuType
	} else {
		row.MenuType = strPtr("")
	}
	if in.Visible != nil {
		row.Visible = in.Visible
	} else {
		row.Visible = strPtr("0")
	}
	if in.Status != nil {
		row.Status = in.Status
	} else {
		row.Status = strPtr("0")
	}
	row.Perms = in.Perms
	if in.Icon != nil {
		row.Icon = in.Icon
	} else {
		row.Icon = strPtr("#")
	}
	if in.Remark != nil {
		row.Remark = in.Remark
	} else {
		row.Remark = strPtr("")
	}

	crudx.FillCreate(&row.BaseModel, s.auth, nowFunc())
	if err := s.repo.Create(ctx, &row); err != nil {
		return 0, errs.Wrap(err, errs.MsgDBFailed)
	}
	return 1, nil
}

// Update 修改菜单（仅更新非空字段），对应 update_menu。
func (s *Service) Update(ctx context.Context, in UpdateIn) (int64, error) {
	row, err := s.repo.SelectMenuByID(ctx, *in.ID)
	if err != nil {
		return 0, err
	}
	if row == nil {
		return 0, nil
	}
	if in.ParentID != nil {
		row.ParentID = in.ParentID
	}
	if in.MenuName != nil {
		row.MenuName = *in.MenuName
	}
	if in.OrderNum != nil {
		row.OrderNum = in.OrderNum
	}
	if in.Path != nil {
		row.Path = in.Path
	}
	if in.Component != nil {
		row.Component = in.Component
	}
	if in.QueryParam != nil {
		row.QueryParam = in.QueryParam
	}
	if in.IsFrame != nil {
		row.IsFrame = in.IsFrame
	}
	if in.IsCache != nil {
		row.IsCache = in.IsCache
	}
	if in.MenuType != nil {
		row.MenuType = in.MenuType
	}
	if in.Visible != nil {
		row.Visible = in.Visible
	}
	if in.Status != nil {
		row.Status = in.Status
	}
	if in.Perms != nil {
		row.Perms = in.Perms
	}
	if in.Icon != nil {
		row.Icon = in.Icon
	}
	if in.Remark != nil {
		row.Remark = in.Remark
	}
	crudx.FillUpdate(&row.BaseModel, s.auth, nowFunc())
	if err := s.repo.Update(ctx, row); err != nil {
		return 0, errs.Wrap(err, errs.MsgDBFailed)
	}
	return 1, nil
}

// DeleteByID 物理删除单条，对应 delete_menu_by_id。
func (s *Service) DeleteByID(ctx context.Context, id int64) (int64, error) {
	done, err := s.repo.Delete(ctx, id)
	if err != nil {
		return 0, errs.Wrap(err, errs.MsgDBFailed)
	}
	if done {
		return 1, nil
	}
	return 0, nil
}

// DeleteCascade 级联删除菜单及角色关联，对应 delete_menu_by_ids。
func (s *Service) DeleteCascade(ctx context.Context, ids []int64) error {
	if _, err := s.repo.DeleteBatch(ctx, ids); err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	if _, err := s.repo.DeleteRoleMenuByMenuIDs(ctx, ids); err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	return nil
}
