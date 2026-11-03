// 本文件实现部门域的业务逻辑，对应 Python 端 module_system/dept/service.py。
//
// # 数据权限：为什么不用 permx.Build
//
// Python 端 dept service 的 _dept_data_scope_condition 是**手写**条件，与
// core/permission.py（permx 的对标物）有三处语义差异，逐条镜像：
//
//  1. 过滤列是 sys_dept **自身 id**（不是 create_dept / dept_id）；
//  2. **角色集为空时不过滤**（return None）——与 permx 的 fail-closed「仅本人」
//     兜底相反。这是 Python 端既有事实，契约优先，不"修正"；
//  3. SELF 范围恒假（部门表无创建人账号语义），DEPT_AND_CHILD_OR_SELF 等价于
//     DEPT_AND_CHILD（不做「或本人」）。
//
// 其余分支（ALL/CUSTOM/DEPT/DEPT_AND_CHILD、非法取值文案）与 permx 一致。
// fail-closed 仍然成立：查询角色范围失败时 error 向上传递，handler 转 500。

package dept

import (
	"context"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/idtype"
	"walnut-seed-go/internal/pkg/modelx"
	"walnut-seed-go/internal/pkg/permx"
)

// nowFunc 提供「当前时间」，抽成变量便于单测钉住审计字段（同 post）。
var nowFunc = time.Now

// 契约文案（与 Python 端逐字一致）。
const (
	msgNoScope      = "没有权限访问部门数据！"
	msgScopeInvalid = "角色数据范围异常 => %s"
	// rootDeptAncestors 根部门的祖级列表，对应 SystemConstants.ROOT_DEPT_ANCESTORS。
	rootDeptAncestors = "0"
)

// Service 部门业务层。
type Service struct {
	repo *Repo
	auth authctx.Context
}

// NewService 构造业务层。
func NewService(db *gorm.DB, ac authctx.Context) *Service {
	return &Service{repo: NewRepo(db), auth: ac}
}

// ---- 数据权限 ----

// scopeCond 数据权限求解结果。noFilter=true 表示不追加条件。
type scopeCond struct {
	noFilter bool
	sql      string // 多条以 OR 连接，应用时外层加括号
	args     []any
}

// cond 转成 crudx.Cond；noFilter / 空 SQL 返回零值（crudx.Apply 会跳过空 SQL）。
func (s scopeCond) cond() crudx.Cond {
	if s.noFilter || s.sql == "" {
		return crudx.Cond{}
	}
	return crudx.Cond{SQL: "(" + s.sql + ")", Args: s.args}
}

// dataScope 对应 _dept_data_scope_condition。
func (s *Service) dataScope(ctx context.Context) (scopeCond, error) {
	user := s.auth.User
	// if not user or not user.id: return None
	if user.ID == 0 {
		return scopeCond{noFilter: true}, nil
	}
	// 超管不过滤（Python 端 is_superuser 或 id==1 两个条件都判）
	if user.IsSuperuser || user.IsSuperAdmin() {
		return scopeCond{noFilter: true}, nil
	}

	scopes, err := s.repo.RoleScopes(ctx, user.ID)
	if err != nil {
		// fail-closed：error 向上传递，绝不吞掉后放行
		return scopeCond{}, errs.Wrap(err, errs.MsgDBFailed)
	}
	// ⚠️ 与 permx 的关键差异：角色集为空 → **不过滤**（Python dept 既有语义）
	if len(scopes) == 0 {
		return scopeCond{noFilter: true}, nil
	}

	var conds []string
	var args []any
	for _, rs := range scopes {
		code := ""
		if rs.DataScope != nil {
			code = *rs.DataScope
		}
		switch code {
		case permx.ScopeAll:
			return scopeCond{noFilter: true}, nil
		case permx.ScopeCustom:
			conds = append(conds, "id IN (SELECT dept_id FROM sys_role_dept WHERE role_id = ?)")
			args = append(args, rs.RoleID)
		case permx.ScopeDept:
			deptID := int64(-1)
			if user.DeptID != nil {
				deptID = *user.DeptID
			}
			conds = append(conds, "id = ?")
			args = append(args, deptID)
		case permx.ScopeDeptAndChild, permx.ScopeDeptChildOrSelf:
			ids, err := s.DescendantAndSelfIDs(ctx, user.DeptID)
			if err != nil {
				return scopeCond{}, errs.Wrap(err, errs.MsgDBFailed)
			}
			c := crudx.In("id", ids)
			conds = append(conds, c.SQL)
			args = append(args, c.Args...)
		case permx.ScopeSelf:
			// 部门表无创建人账号语义 → 恒假（Python 端 sqlalchemy.false()）
			conds = append(conds, "1 = 0")
		default:
			// 非法取值（含 NULL）是面向用户的契约文案；NULL 格式化成字面量 "None"
			literal := code
			if rs.DataScope == nil {
				literal = "None"
			}
			return scopeCond{}, errs.Newf(msgScopeInvalid, literal)
		}
	}
	// or_(*conditions) if conditions else None
	if len(conds) == 0 {
		return scopeCond{noFilter: true}, nil
	}
	return scopeCond{sql: strings.Join(conds, " OR "), args: args}, nil
}

// DescendantAndSelfIDs 部门及其全部子孙 ID（含自身），对应 _dept_and_child_ids。
//
// ⚠️ dept_id 为 None 时返回 [-1]（Python 端同款兜底，保证查不到任何行）。
// ⚠️ 自身 ID 在查询结果之后 append，排在最后（顺序无契约意义，仅镜像）。
func (s *Service) DescendantAndSelfIDs(ctx context.Context, deptID *int64) ([]int64, error) {
	if deptID == nil {
		return []int64{-1}, nil
	}
	ids, err := s.repo.DescendantIDs(ctx, *deptID)
	if err != nil {
		return nil, err
	}
	return append(ids, *deptID), nil
}

// CheckDataScope 校验是否有该部门的数据权限，对应 check_dept_data_scope。
func (s *Service) CheckDataScope(ctx context.Context, deptID *int64) error {
	if deptID == nil {
		return nil
	}
	user := s.auth.User
	if user.IsSuperuser || user.IsSuperAdmin() {
		return nil
	}
	conds := []crudx.Cond{crudx.Eq("id", *deptID)}
	sc, err := s.dataScope(ctx)
	if err != nil {
		return err
	}
	if c := sc.cond(); c.SQL != "" {
		conds = append(conds, c)
	}
	n, err := s.repo.Count(ctx, conds...)
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	if n == 0 {
		return errs.New(msgNoScope)
	}
	return nil
}

// ---- 查询 ----

// buildConditions 对应 _build_query_conditions。
//
// ⚠️ begin/end **同时存在**才施加范围（Python 端是 and 不是 or）；
// 且上界是 end 日期的 **00:00**（datetime.combine(end, time.min)），
// 即 end 当天创建的部门**不在**结果内——照抄不"修正"。
func (s *Service) buildConditions(ctx context.Context, q QueryParam) ([]crudx.Cond, error) {
	conds := make([]crudx.Cond, 0, 7)
	if q.ID != nil {
		conds = append(conds, crudx.Eq("id", *q.ID))
	}
	if q.ParentID != nil {
		conds = append(conds, crudx.Eq("parent_id", *q.ParentID))
	}
	if crudx.IsNotBlank(q.DeptName) {
		conds = append(conds, crudx.Like("dept_name", q.DeptName))
	}
	if crudx.IsNotBlank(q.DeptCategory) {
		conds = append(conds, crudx.Like("dept_category", q.DeptCategory))
	}
	if crudx.IsNotBlank(q.Status) {
		conds = append(conds, crudx.Eq("status", q.Status))
	}
	if q.BeginTime != "" && q.EndTime != "" {
		conds = append(conds, crudx.Between("create_time", q.BeginTime, q.EndTime))
	}
	if q.BelongDeptID != nil {
		ids, err := s.DescendantAndSelfIDs(ctx, q.BelongDeptID)
		if err != nil {
			return nil, err
		}
		conds = append(conds, crudx.In("id", ids))
	}
	return conds, nil
}

// selectList 带数据权限的部门列表（返回实体供树构建复用），对应 select_dept_list。
func (s *Service) selectList(ctx context.Context, q *QueryParam) ([]SysDept, error) {
	var conds []crudx.Cond
	if q != nil {
		built, err := s.buildConditions(ctx, *q)
		if err != nil {
			return nil, err
		}
		conds = built
	}
	sc, err := s.dataScope(ctx)
	if err != nil {
		return nil, err
	}
	if c := sc.cond(); c.SQL != "" {
		conds = append(conds, c)
	}
	return s.repo.List(ctx, conds...)
}

// ListOut 部门列表出参（回填 parentName/leaderName），对应 list_dept_out。
func (s *Service) ListOut(ctx context.Context, q QueryParam) ([]Out, error) {
	rows, err := s.selectList(ctx, &q)
	if err != nil {
		return nil, err
	}
	return s.fillOut(ctx, rows)
}

// ListOutExclude 排除指定节点及其子孙，对应 list_dept_out_exclude。
//
// ⚠️ 排除判据是**元素级**匹配：str(dept_id) not in str2list(ancestors)，
// 不是子串匹配——dept_id=1 不能误排除 ancestors 含 "100" 的行。
func (s *Service) ListOutExclude(ctx context.Context, deptID int64) ([]Out, error) {
	rows, err := s.selectList(ctx, nil)
	if err != nil {
		return nil, err
	}
	target := strconv.FormatInt(deptID, 10)
	kept := make([]SysDept, 0, len(rows))
	for i := range rows {
		if rows[i].ID == deptID {
			continue
		}
		if rows[i].Ancestors != nil && containsStr(crudx.Str2List(*rows[i].Ancestors), target) {
			continue
		}
		kept = append(kept, rows[i])
	}
	return s.fillOut(ctx, kept)
}

// fillOut 批量回填父部门名称与负责人账号，对应 _fill_dept_out。
func (s *Service) fillOut(ctx context.Context, rows []SysDept) ([]Out, error) {
	outs := make([]Out, 0, len(rows))
	if len(rows) == 0 {
		return outs, nil
	}
	parentSeen := make(map[int64]struct{})
	leaderSeen := make(map[int64]struct{})
	var parentIDs, leaderIDs []int64
	for i := range rows {
		if rows[i].ParentID != nil {
			if _, ok := parentSeen[*rows[i].ParentID]; !ok {
				parentSeen[*rows[i].ParentID] = struct{}{}
				parentIDs = append(parentIDs, *rows[i].ParentID)
			}
		}
		if rows[i].Leader != nil {
			if _, ok := leaderSeen[*rows[i].Leader]; !ok {
				leaderSeen[*rows[i].Leader] = struct{}{}
				leaderIDs = append(leaderIDs, *rows[i].Leader)
			}
		}
	}
	parentNames, err := s.repo.DeptNameMap(ctx, parentIDs)
	if err != nil {
		return nil, err
	}
	leaderNames, err := s.repo.LeaderNameMap(ctx, leaderIDs)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		out := NewOut(&rows[i])
		if rows[i].ParentID != nil {
			out.ParentName = parentNames[*rows[i].ParentID]
		}
		if rows[i].Leader != nil {
			out.LeaderName = leaderNames[*rows[i].Leader]
		}
		outs = append(outs, out)
	}
	return outs, nil
}

// GetOut 部门详情，对应 select_dept_by_id（不存在返回 nil）。
func (s *Service) GetOut(ctx context.Context, deptID int64) (*Out, error) {
	row, err := s.repo.Get(ctx, deptID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	outs, err := s.fillOut(ctx, []SysDept{*row})
	if err != nil {
		return nil, err
	}
	return &outs[0], nil
}

// OptionSelectOut 部门选择框列表，对应 select_dept_by_ids。
//
// ⚠️ 只回填 id/deptName/leader 三个字段，其余出参字段为 null——
// Python 端是 DeptOutSchema(id=..., dept_name=..., leader=...) 手工构造，
// 不是 model_validate，故 parentName 等不会被填。children 仍是 []。
func (s *Service) OptionSelectOut(ctx context.Context, ids []int64) ([]Out, error) {
	conds := []crudx.Cond{crudx.Eq("status", modelx.Normal)}
	if len(ids) > 0 {
		conds = append(conds, crudx.In("id", ids))
	}
	sc, err := s.dataScope(ctx)
	if err != nil {
		return nil, err
	}
	if c := sc.cond(); c.SQL != "" {
		conds = append(conds, c)
	}
	rows, err := s.repo.List(ctx, conds...)
	if err != nil {
		return nil, err
	}
	outs := make([]Out, 0, len(rows))
	for i := range rows {
		outs = append(outs, Out{
			ID:       jsIntPtrOrNil(rows[i].ID),
			DeptName: rows[i].DeptName,
			Leader:   jsIntPtrOrNilPtr(rows[i].Leader),
			Children: []Out{},
		})
	}
	return outs, nil
}

// ---- 树构建 ----

// treeSortKey 对应 _tree_sort_key：(order_num or 0, id or 0)。
func treeSortKey(n *TreeNode) (int64, int64) {
	var w int64
	if n.Weight != nil {
		w = n.Weight.Int64()
	}
	return w, n.ID.Int64()
}

// BuildTreeSelect 构建下拉树，对应 build_dept_tree_select（纯函数，可离线测试）。
//
// 与 post.BuildDeptTree 的差异（**不要统一**）：
//   - 根与每层 children 都按 (orderNum, id) **排序**（post 那棵保持 SQL 顺序）
//   - 根判定同为「parentId 不在节点 ID 集合里」（支持多根、NULL 必为根）
func BuildTreeSelect(rows []SysDept) []*TreeNode {
	if len(rows) == 0 {
		return []*TreeNode{}
	}
	idSet := make(map[int64]struct{}, len(rows))
	nodes := make(map[int64]*TreeNode, len(rows))
	childrenOf := make(map[int64][]*TreeNode, len(rows))
	for i := range rows {
		r := &rows[i]
		n := &TreeNode{
			ID:       idtype.JSInt(r.ID),
			ParentID: jsIntPtrOrNilPtr(r.ParentID),
			Label:    r.DeptName,
			Weight:   intPtrToJSInt(r.OrderNum),
			// status == '1' 即停用
			Disabled: r.Status != nil && *r.Status == modelx.Disabled,
		}
		nodes[r.ID] = n
		idSet[r.ID] = struct{}{}
		if r.ParentID != nil {
			childrenOf[*r.ParentID] = append(childrenOf[*r.ParentID], n)
		}
	}

	sortNodes := func(list []*TreeNode) {
		for i := 1; i < len(list); i++ {
			for j := i; j > 0; j-- {
				wi, ii := treeSortKey(list[j-1])
				wj, ij := treeSortKey(list[j])
				if wi < wj || (wi == wj && ii < ij) {
					break
				}
				list[j-1], list[j] = list[j], list[j-1]
			}
		}
	}

	var assign func(n *TreeNode)
	assign = func(n *TreeNode) {
		kids := childrenOf[n.ID.Int64()]
		if len(kids) == 0 {
			return
		}
		sortNodes(kids)
		for _, k := range kids {
			assign(k)
		}
		n.Children = kids
	}

	roots := make([]*TreeNode, 0, len(rows))
	for i := range rows {
		if rows[i].ParentID == nil {
			roots = append(roots, nodes[rows[i].ID])
			continue
		}
		if _, ok := idSet[*rows[i].ParentID]; !ok {
			roots = append(roots, nodes[rows[i].ID])
		}
	}
	sortNodes(roots)
	for _, r := range roots {
		assign(r)
	}
	return roots
}

// TreeSelectOut 部门下拉树，对应 select_dept_tree_list。
func (s *Service) TreeSelectOut(ctx context.Context, q QueryParam) ([]*TreeNode, error) {
	rows, err := s.selectList(ctx, &q)
	if err != nil {
		return nil, err
	}
	return BuildTreeSelect(rows), nil
}

// ---- 校验 ----

// CheckNameUnique 同父级下部门名称唯一性，对应 check_dept_name_unique。
//
// ⚠️ parent_id 为 None 时 Python 端 `parent_id == None` 生成 IS NULL，照抄。
func (s *Service) CheckNameUnique(ctx context.Context, name string, parentID, excludeID *int64) (bool, error) {
	conds := []crudx.Cond{crudx.Eq("dept_name", name)}
	if parentID == nil {
		conds = append(conds, crudx.Cond{SQL: "parent_id IS NULL"})
	} else {
		conds = append(conds, crudx.Eq("parent_id", *parentID))
	}
	if excludeID != nil {
		conds = append(conds, crudx.Cond{SQL: "id <> ?", Args: []any{*excludeID}})
	}
	n, err := s.repo.Count(ctx, conds...)
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

// HasChild 是否存在子部门，对应 has_child_by_dept_id。
func (s *Service) HasChild(ctx context.Context, deptID int64) (bool, error) {
	n, err := s.repo.Count(ctx, crudx.Eq("parent_id", deptID))
	return n > 0, err
}

// ExistsUser 部门下是否存在用户，对应 check_dept_exist_user。
func (s *Service) ExistsUser(ctx context.Context, deptID int64) (bool, error) {
	n, err := s.repo.CountUserInDept(ctx, deptID)
	return n > 0, err
}

// CountPost 部门下岗位数，对应 count_post_by_dept_id。
func (s *Service) CountPost(ctx context.Context, deptID int64) (int64, error) {
	return s.repo.CountPostInDept(ctx, deptID)
}

// CountNormalChildren 正常状态的子孙部门数，对应 select_normal_children_dept_by_id。
func (s *Service) CountNormalChildren(ctx context.Context, deptID int64) (int64, error) {
	return s.repo.CountNormalChildren(ctx, deptID)
}

// ---- 写操作 ----

// Insert 新增部门，对应 insert_dept。
func (s *Service) Insert(ctx context.Context, in CreateIn) error {
	parent, err := s.repo.Get(ctx, *in.ParentID)
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	if parent == nil {
		return errs.New("上级部门不存在，无法新增")
	}
	// 父节点非正常状态不允许新增子节点
	if parent.Status == nil || *parent.Status != modelx.Normal {
		return errs.New("部门停用，不允许新增")
	}

	row := SysDept{DelFlag: modelx.Normal}
	row.ParentID = in.ParentID
	row.DeptName = in.DeptName
	row.DeptCategory = in.DeptCategory
	if in.OrderNum != nil {
		row.OrderNum = in.OrderNum
	}
	row.Leader = in.Leader
	row.Phone = in.Phone
	row.Email = in.Email
	row.Status = in.Status
	// ⚠️ status 未提供时走**模型默认值 '0'**：Python 端 DeptModel.status 是
	// mapped_column(default="0")，model_dump(exclude_none=True) 不含 status 时
	// 由 SQLAlchemy 客户端默认值补 '0'。Go 侧不镜像就会插入 NULL（契约比对实测抓到）。
	if row.Status == nil {
		status := modelx.Normal
		row.Status = &status
	}

	// ancestors = 父.ancestors + "," + 父id。
	// ⚠️ 父.ancestors 为 NULL 时 Python f-string 会拼出字面量 "None"，照抄不修正。
	anc := "None"
	if parent.Ancestors != nil {
		anc = *parent.Ancestors
	}
	anc += crudx.Separator + strconv.FormatInt(*in.ParentID, 10)
	row.Ancestors = &anc

	crudx.FillCreate(&row.BaseModel, s.auth, nowFunc())
	if err := s.repo.Create(ctx, &row); err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	return nil
}

// Update 修改部门（含子孙 ancestors 维护），对应 update_dept。
func (s *Service) Update(ctx context.Context, in UpdateIn) error {
	old, err := s.repo.Get(ctx, *in.ID)
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	if old == nil {
		return errs.New("部门不存在，无法修改")
	}

	newAncestors := ""
	ancestorsChanged := false
	if in.ParentID != nil && (old.ParentID == nil || *old.ParentID != *in.ParentID) {
		// 换父部门：先校验对新父部门的数据权限，避免越权挂载
		if err := s.CheckDataScope(ctx, in.ParentID); err != nil {
			return err
		}
		newParent, err := s.repo.Get(ctx, *in.ParentID)
		if err != nil {
			return errs.Wrap(err, errs.MsgDBFailed)
		}
		if newParent != nil {
			// 父.ancestors 为 NULL 时 Python f-string 拼出字面量 "None"，照抄
			anc := "None"
			if newParent.Ancestors != nil {
				anc = *newParent.Ancestors
			}
			newAncestors = anc + crudx.Separator + strconv.FormatInt(newParent.ID, 10)
			ancestorsChanged = true
			if err := s.updateChildrenAncestors(ctx, *in.ID, newAncestors, old.Ancestors); err != nil {
				return err
			}
		}
	}

	// 对应 model_dump(exclude_none=True) 去掉 id 后逐字段 setattr
	if in.ParentID != nil {
		old.ParentID = in.ParentID
	}
	if in.DeptName != nil {
		old.DeptName = in.DeptName
	}
	if in.DeptCategory != nil {
		old.DeptCategory = in.DeptCategory
	}
	if in.OrderNum != nil {
		old.OrderNum = in.OrderNum
	}
	if in.Leader != nil {
		old.Leader = in.Leader
	}
	if in.Phone != nil {
		old.Phone = in.Phone
	}
	if in.Email != nil {
		old.Email = in.Email
	}
	if in.Status != nil {
		old.Status = in.Status
	}
	if ancestorsChanged {
		old.Ancestors = &newAncestors
	}

	crudx.FillUpdate(&old.BaseModel, s.auth, nowFunc())
	if err := s.repo.Update(ctx, old); err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}

	// 启用部门时连带启用其全部上级（根 "0" 与空串除外）
	if old.Status != nil && *old.Status == modelx.Normal &&
		old.Ancestors != nil && *old.Ancestors != "" && *old.Ancestors != rootDeptAncestors {
		var ids []int64
		for _, item := range crudx.Str2List(*old.Ancestors) {
			n, err := strconv.ParseInt(item, 10, 64)
			if err != nil {
				// Python 端 int(item) 同样会抛错 → 500；此处等价转业务异常
				return errs.Wrap(err, errs.MsgDBFailed)
			}
			ids = append(ids, n)
		}
		if _, err := s.repo.UpdateStatusNormal(ctx, ids); err != nil {
			return errs.Wrap(err, errs.MsgDBFailed)
		}
	}
	return nil
}

// updateChildrenAncestors 移动部门时同步更新所有子孙的 ancestors，
// 对应 _update_dept_children。
//
// ⚠️ 只替换**首次出现**的旧祖级串（Python str.replace(old, new, 1)）。
// old 为 None 时 Python 用空串，replace("", new, 1) 等价于前缀插入，照抄。
func (s *Service) updateChildrenAncestors(ctx context.Context, deptID int64, newAncestors string, oldAncestors *string) error {
	children, err := s.repo.ListDescendants(ctx, deptID)
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	old := ""
	if oldAncestors != nil {
		old = *oldAncestors
	}
	for i := range children {
		childAnc := ""
		if children[i].Ancestors != nil {
			childAnc = *children[i].Ancestors
		}
		replaced := strings.Replace(childAnc, old, newAncestors, 1)
		children[i].Ancestors = &replaced
		crudx.FillUpdate(&children[i].BaseModel, s.auth, nowFunc())
		if err := s.repo.Update(ctx, &children[i]); err != nil {
			return errs.Wrap(err, errs.MsgDBFailed)
		}
	}
	return nil
}

// Delete 逻辑删除，对应 delete_dept_by_id。返回是否删除成功。
func (s *Service) Delete(ctx context.Context, deptID int64) (bool, error) {
	n, err := s.repo.LogicalDelete(ctx, deptID)
	if err != nil {
		return false, errs.Wrap(err, errs.MsgDBFailed)
	}
	return n > 0, nil
}

// ---- 内部工具 ----

func containsStr(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}
