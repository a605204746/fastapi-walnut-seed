// 本文件实现岗位域的业务逻辑，对应 Python 端 module_system/post/service.py。

package post

import (
	"context"
	"sort"
	"time"

	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/idtype"
	"walnut-seed-go/internal/pkg/modelx"
)

// nowFunc 提供「当前时间」。抽成变量是为了让单元测试能钉住时间，
// 从而断言 FillCreate / FillUpdate 写入的 create_time / update_time。
var nowFunc = time.Now

// Service 岗位业务层。与 Python 端一样持有**请求级**认证上下文（审计填充要用）。
type Service struct {
	repo *Repo
	auth authctx.Context
}

// NewService 构造业务层。
func NewService(db *gorm.DB, ac authctx.Context) *Service {
	return &Service{repo: NewRepo(db), auth: ac}
}

// PageOut 分页响应，对应 {"rows": [...], "total": N}。
//
// ⚠️ 键序是 rows 在前、total 在后（Python 端字典的字面顺序），
// 逐字节比对时顺序也是契约的一部分。
type PageOut struct {
	Rows  []Out        `json:"rows"`
	Total idtype.JSInt `json:"total"`
}

// TreeNode 部门树节点，对应 PostService._build_dept_tree 里构造的字典。
//
// ⚠️ 键序 = 字典字面量顺序：id, parentId, label, weight, disabled，
// 而 children 是**后加的**故排在最后。
// ⚠️ Children 用 omitempty：Python 端 `if children: node["children"] = children`，
// 即**没有子节点时该键根本不出现**，而不是出现一个空数组。
type TreeNode struct {
	ID       idtype.JSInt  `json:"id"`
	ParentID *idtype.JSInt `json:"parentId"`
	Label    *string       `json:"label"`
	Weight   *idtype.JSInt `json:"weight"`
	Disabled bool          `json:"disabled"`
	Children []*TreeNode   `json:"children,omitempty"`
}

// ---- 查询条件 ----

// buildConditions 对应 PostService._build_conditions。
//
// ⚠️ dept_id 与 belong_dept_id 是**互斥的 if/elif**：
// 前者是「单部门精确搜索」，后者是「部门树搜索（含子孙）」。
// 两者都传时只有 dept_id 生效——写成两个独立 if 会让部门树搜索失效。
func (s *Service) buildConditions(ctx context.Context, q QueryParam) ([]crudx.Cond, error) {
	conds := make([]crudx.Cond, 0, 6)
	if crudx.IsNotBlank(q.PostCode) {
		conds = append(conds, crudx.Like("post_code", q.PostCode))
	}
	if crudx.IsNotBlank(q.PostCategory) {
		conds = append(conds, crudx.Like("post_category", q.PostCategory))
	}
	if crudx.IsNotBlank(q.PostName) {
		conds = append(conds, crudx.Like("post_name", q.PostName))
	}
	if crudx.IsNotBlank(q.Status) {
		conds = append(conds, crudx.Eq("status", q.Status))
	}
	// ⚠️ 必须**两个都提供**才施加 BETWEEN（Python 端是 and 而非 or）
	if q.BeginTime != "" && q.EndTime != "" {
		conds = append(conds, crudx.Between("create_time", q.BeginTime, q.EndTime))
	}
	switch {
	case q.DeptID != nil:
		conds = append(conds, crudx.Eq("dept_id", *q.DeptID))
	case q.BelongDeptID != nil:
		ids, err := s.repo.DeptAndChildIDs(ctx, *q.BelongDeptID)
		if err != nil {
			return nil, err
		}
		conds = append(conds, crudx.In("dept_id", ids))
	}
	return conds, nil
}

// ---- 查询 ----

// PageList 分页查询，对应 PostService.page_list。
func (s *Service) PageList(ctx context.Context, q QueryParam) (*PageOut, error) {
	conds, err := s.buildConditions(ctx, q)
	if err != nil {
		return nil, err
	}
	rows, total, err := s.repo.Page(ctx, q.Page, conds)
	if err != nil {
		return nil, err
	}
	outs := s.toOuts(rows)
	if err := s.fillDeptName(ctx, outs); err != nil {
		return nil, err
	}
	return &PageOut{Rows: outs, Total: idtype.JSInt(total)}, nil
}

// SelectList 查询全部（导出用），对应 PostService.select_list。
//
// ⚠️ 导出走的是 repo.ListAll（**没有** page 里的 post_sort 默认排序），
// 之后在内存里按 (post_sort is None, post_sort) 稳定排序。
// post_sort 列是 NOT NULL，故 None 分支实际不可达，但排序必须**稳定**——
// Python 的 list.sort 是稳定的，post_sort 相同的行保持查询返回的相对顺序。
func (s *Service) SelectList(ctx context.Context, q QueryParam) ([]Out, error) {
	conds, err := s.buildConditions(ctx, q)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListAll(ctx, conds...)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].PostSort < rows[j].PostSort })
	outs := s.toOuts(rows)
	if err := s.fillDeptName(ctx, outs); err != nil {
		return nil, err
	}
	return outs, nil
}

// GetByID 按主键查询，对应 PostService.get_by_id。
//
// 记录不存在时返回 (nil, nil)，由 handler 决定响应形态。
func (s *Service) GetByID(ctx context.Context, id int64) (*Out, error) {
	row, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	outs := s.toOuts([]SysPost{*row})
	if err := s.fillDeptName(ctx, outs); err != nil {
		return nil, err
	}
	return &outs[0], nil
}

// OptionSelect 岗位选择框列表，对应 PostService.option_select。
//
// 三分支的**优先级**是契约：deptId 优先于 postIds，两者都没有时返回空数组。
func (s *Service) OptionSelect(ctx context.Context, postIDs []int64, deptID *int64) ([]Out, error) {
	if deptID != nil {
		q := QueryParam{DeptID: deptID}
		// Python 端构造 PostQueryParam(dept_id=dept_id)，其余字段走默认值。
		// ⚠️ PageQueryParam 的 page_size 默认是 10，故这里也必须带上默认分页，
		// 否则 Go 端会返回全部而 Python 端只返回 10 条。
		q.Page = defaultListPage()
		return s.SelectList(ctx, q)
	}
	if len(postIDs) > 0 {
		rows, err := s.repo.ListOptionByIDs(ctx, postIDs)
		if err != nil {
			return nil, err
		}
		outs := s.toOuts(rows)
		if err := s.fillDeptName(ctx, outs); err != nil {
			return nil, err
		}
		return outs, nil
	}
	return []Out{}, nil
}

// SelectPostByUserID 用户所属岗位（供用户模块使用），对应 select_posts_by_user_id。
func (s *Service) SelectPostByUserID(ctx context.Context, userID int64) ([]Info, error) {
	rows, err := s.repo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	outs := make([]Info, 0, len(rows))
	for i := range rows {
		outs = append(outs, NewInfo(&rows[i]))
	}
	return outs, nil
}

// SelectPostIDsByUserID 用户所属岗位 ID 列表，对应 select_post_list_by_user_id。
func (s *Service) SelectPostIDsByUserID(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := s.repo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ID)
	}
	return ids, nil
}

// CountByDeptID 部门下岗位数量（供部门模块删除前校验），对应 count_post_by_dept_id。
func (s *Service) CountByDeptID(ctx context.Context, deptID int64) (int64, error) {
	return s.repo.CountByDeptID(ctx, deptID)
}

// defaultListPage 复刻 PageQueryParam 的缺省分页（pageNum=1, pageSize=10）。
func defaultListPage() crudx.Page {
	size := 10
	return crudx.Page{PageNum: 1, PageSize: &size}
}

// ---- 部门树 ----

// DeptTreeList 对应 PostService.dept_tree_list。
func (s *Service) DeptTreeList(ctx context.Context, q DeptTreeQuery) ([]*TreeNode, error) {
	conds := make([]crudx.Cond, 0, 7)
	conds = append(conds, crudx.Eq("del_flag", modelx.Normal))
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
		ids, err := s.repo.DeptAndChildIDs(ctx, *q.BelongDeptID)
		if err != nil {
			return nil, err
		}
		conds = append(conds, crudx.In("id", ids))
	}

	rows, err := s.repo.DeptTree(ctx, conds)
	if err != nil {
		return nil, err
	}
	return BuildDeptTree(rows), nil
}

// BuildDeptTree 对应 PostService._build_dept_tree（纯函数，可离线测试）。
//
// 语义要点：
//   - 节点顺序 = 查询返回顺序（SQL 已按 ancestors/parent_id/order_num/id 排好）
//   - children **只在非空时**才挂上（否则 JSON 里不出现该键）
//   - 根判定是「parentId 不在本次节点 ID 集合里」，故支持**多根**
//     （按条件过滤后父节点可能不在结果集内）
//   - parentId 为 NULL 的节点，NULL 不会等于任何 ID，故必为根
func BuildDeptTree(rows []DeptNode) []*TreeNode {
	if len(rows) == 0 {
		return []*TreeNode{}
	}
	nodes := make([]*TreeNode, 0, len(rows))
	idSet := make(map[int64]struct{}, len(rows))
	for i := range rows {
		r := &rows[i]
		n := &TreeNode{
			ID:       idtype.JSInt(r.ID),
			ParentID: jsIntPtrOrNil(r.ParentID),
			Label:    r.DeptName,
			Weight:   intPtrOrNil(r.OrderNum),
			// status == '1' 即停用
			Disabled: r.Status != nil && *r.Status == modelx.Disabled,
		}
		nodes = append(nodes, n)
		idSet[r.ID] = struct{}{}
	}

	// 按 parentId 分组。parentId 为 NULL 的节点不会成为任何节点的孩子
	childrenOf := make(map[int64][]*TreeNode, len(rows))
	for _, n := range nodes {
		if n.ParentID == nil {
			continue
		}
		pid := n.ParentID.Int64()
		childrenOf[pid] = append(childrenOf[pid], n)
	}
	for _, n := range nodes {
		if kids := childrenOf[n.ID.Int64()]; len(kids) > 0 {
			n.Children = kids
		}
	}

	// 多根：parentId 不在节点 ID 集合中的即为根（含 parentId 为 NULL 的）
	roots := make([]*TreeNode, 0, len(nodes))
	for _, n := range nodes {
		if n.ParentID == nil {
			roots = append(roots, n)
			continue
		}
		if _, ok := idSet[n.ParentID.Int64()]; !ok {
			roots = append(roots, n)
		}
	}
	return roots
}

// ---- 校验 ----

// CheckPostNameUnique 校验同部门下岗位名称唯一，对应 check_post_name_unique。
func (s *Service) CheckPostNameUnique(ctx context.Context, in CreateIn, excludeID *int64) (bool, error) {
	if in.PostName == nil || in.DeptID == nil {
		// Python 端用 assert 兜底（schema 校验已保证非空），这里同样视为编程错误
		return false, errs.New("岗位名称与部门id不能为空")
	}
	ok, err := s.repo.ExistsByName(ctx, *in.PostName, *in.DeptID, excludeID)
	return !ok, err
}

// CheckPostCodeUnique 校验岗位编码唯一，对应 check_post_code_unique。
func (s *Service) CheckPostCodeUnique(ctx context.Context, in CreateIn, excludeID *int64) (bool, error) {
	if in.PostCode == nil {
		return false, errs.New("岗位编码不能为空")
	}
	ok, err := s.repo.ExistsByCode(ctx, *in.PostCode, excludeID)
	return !ok, err
}

// CountUserPostByID 岗位已分配用户数，对应 count_user_post_by_id。
func (s *Service) CountUserPostByID(ctx context.Context, postID int64) (int64, error) {
	return s.repo.CountUserPost(ctx, postID)
}

// ---- 写入 ----

// InsertPost 新增岗位，对应 insert_post。
//
// ⚠️ Python 端是 `req.model_dump(exclude_none=True)` 后构造实体，
// 故**未提供的字段不写库**（走列默认值）。Go 侧等价做法是只在指针非 nil 时赋值。
func (s *Service) InsertPost(ctx context.Context, in CreateIn) (bool, error) {
	row := SysPost{}
	if in.DeptID != nil {
		row.DeptID = *in.DeptID
	}
	if in.PostCode != nil {
		row.PostCode = *in.PostCode
	}
	if in.PostName != nil {
		row.PostName = *in.PostName
	}
	row.PostCategory = in.PostCategory
	if in.PostSort != nil {
		row.PostSort = *in.PostSort
	}
	if in.Status != nil {
		row.Status = *in.Status
	}
	row.Remark = in.Remark

	crudx.FillCreate(&row.BaseModel, s.auth, nowFunc())
	if err := s.repo.Create(ctx, &row); err != nil {
		return false, err
	}
	return row.ID != 0, nil
}

// UpdatePost 修改岗位（仅更新非空字段），对应 update_post。
//
// 记录不存在时返回 (false, nil)，由 handler 转成 ErrorResponse()。
func (s *Service) UpdatePost(ctx context.Context, in UpdateIn) (bool, error) {
	row, err := s.repo.Get(ctx, *in.ID)
	if err != nil {
		return false, err
	}
	if row == nil {
		return false, nil
	}
	// 对应 Python 的 `for field, value in model_dump(exclude_none=True, exclude={"id"}).items()`
	if in.DeptID != nil {
		row.DeptID = *in.DeptID
	}
	if in.PostCode != nil {
		row.PostCode = *in.PostCode
	}
	if in.PostName != nil {
		row.PostName = *in.PostName
	}
	if in.PostCategory != nil {
		row.PostCategory = in.PostCategory
	}
	if in.PostSort != nil {
		row.PostSort = *in.PostSort
	}
	if in.Status != nil {
		row.Status = *in.Status
	}
	if in.Remark != nil {
		row.Remark = in.Remark
	}

	crudx.FillUpdate(&row.BaseModel, s.auth, nowFunc())
	if err := s.repo.Update(ctx, row); err != nil {
		return false, err
	}
	return true, nil
}

// DeletePostByIDs 批量删除岗位，对应 delete_post_by_ids。
//
// ⚠️ 删除前逐个校验「是否已分配用户」，任一被分配就抛错且**不删除任何行**
// （Python 端在循环里 raise，事务尚未提交）。文案是 `{post_name}已分配，不能删除!`。
func (s *Service) DeletePostByIDs(ctx context.Context, ids []int64) (int64, error) {
	rows, err := s.repo.ListAll(ctx, crudx.In("id", ids))
	if err != nil {
		return 0, err
	}
	for i := range rows {
		n, err := s.repo.CountUserPost(ctx, rows[i].ID)
		if err != nil {
			return 0, err
		}
		if n > 0 {
			return 0, errs.Newf("%s已分配，不能删除!", rows[i].PostName)
		}
	}
	delIDs := make([]int64, 0, len(rows))
	for i := range rows {
		delIDs = append(delIDs, rows[i].ID)
	}
	return s.repo.DeleteBatch(ctx, delIDs)
}

// ---- 内部工具 ----

// toOuts 把实体切片转出参切片，恒返回非 nil。
func (s *Service) toOuts(rows []SysPost) []Out {
	outs := make([]Out, 0, len(rows))
	for i := range rows {
		outs = append(outs, NewOut(&rows[i]))
	}
	return outs
}

// fillDeptName 批量回填部门名，对应 PostService._fill_dept_name。
//
// 批量查一次而不是逐行查（N+1）。部门已删除时映射里没有该键 → DeptName 保持 nil。
func (s *Service) fillDeptName(ctx context.Context, outs []Out) error {
	if len(outs) == 0 {
		return nil
	}
	// 去重 + 排序（Python 端 sorted({...})）；排序让 SQL 参数顺序稳定，便于比对
	seen := make(map[int64]struct{}, len(outs))
	ids := make([]int64, 0, len(outs))
	for i := range outs {
		if outs[i].DeptID == nil {
			continue
		}
		id := outs[i].DeptID.Int64()
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	names, err := s.repo.DeptNameMap(ctx, ids)
	if err != nil {
		return err
	}
	for i := range outs {
		if outs[i].DeptID == nil {
			continue
		}
		outs[i].DeptName = names[outs[i].DeptID.Int64()]
	}
	return nil
}

func jsIntPtrOrNil(v *int64) *idtype.JSInt {
	if v == nil {
		return nil
	}
	j := idtype.JSInt(*v)
	return &j
}

func intPtrOrNil(v *int) *idtype.JSInt {
	if v == nil {
		return nil
	}
	j := idtype.JSInt(int64(*v))
	return &j
}
