package post

import (
	"context"
	"errors"
	"testing"
	"time"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/modelx"
)

// mockRepo 嵌入 postRepo（nil），仅以函数字段覆盖被测方法；
// 未覆盖的方法被调用即 panic，暴露测试外的意外依赖。
type mockRepo struct {
	postRepo
	page            func(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysPost, int64, error)
	listAll         func(ctx context.Context, conds ...crudx.Cond) ([]SysPost, error)
	get             func(ctx context.Context, id int64) (*SysPost, error)
	existsByName    func(ctx context.Context, name string, deptID int64, excludeID *int64) (bool, error)
	existsByCode    func(ctx context.Context, code string, excludeID *int64) (bool, error)
	countUserPost   func(ctx context.Context, postID int64) (int64, error)
	countByDeptID   func(ctx context.Context, deptID int64) (int64, error)
	listOptionByIDs func(ctx context.Context, ids []int64) ([]SysPost, error)
	listByUserID    func(ctx context.Context, userID int64) ([]SysPost, error)
	create          func(ctx context.Context, row *SysPost) error
	update          func(ctx context.Context, row *SysPost) error
	deleteBatch     func(ctx context.Context, ids []int64) (int64, error)
	deptAndChildIDs func(ctx context.Context, deptID int64) ([]int64, error)
	deptNameMap     func(ctx context.Context, deptIDs []int64) (map[int64]*string, error)
	deptTree        func(ctx context.Context, conds []crudx.Cond) ([]DeptNode, error)
}

func (m *mockRepo) Page(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysPost, int64, error) {
	return m.page(ctx, page, conds)
}

func (m *mockRepo) ListAll(ctx context.Context, conds ...crudx.Cond) ([]SysPost, error) {
	return m.listAll(ctx, conds...)
}

func (m *mockRepo) Get(ctx context.Context, id int64) (*SysPost, error) {
	return m.get(ctx, id)
}

func (m *mockRepo) ExistsByName(ctx context.Context, name string, deptID int64, excludeID *int64) (bool, error) {
	return m.existsByName(ctx, name, deptID, excludeID)
}

func (m *mockRepo) ExistsByCode(ctx context.Context, code string, excludeID *int64) (bool, error) {
	return m.existsByCode(ctx, code, excludeID)
}

func (m *mockRepo) CountUserPost(ctx context.Context, postID int64) (int64, error) {
	return m.countUserPost(ctx, postID)
}

func (m *mockRepo) CountByDeptID(ctx context.Context, deptID int64) (int64, error) {
	return m.countByDeptID(ctx, deptID)
}

func (m *mockRepo) ListOptionByIDs(ctx context.Context, ids []int64) ([]SysPost, error) {
	return m.listOptionByIDs(ctx, ids)
}

func (m *mockRepo) ListByUserID(ctx context.Context, userID int64) ([]SysPost, error) {
	return m.listByUserID(ctx, userID)
}

func (m *mockRepo) Create(ctx context.Context, row *SysPost) error {
	return m.create(ctx, row)
}

func (m *mockRepo) Update(ctx context.Context, row *SysPost) error {
	return m.update(ctx, row)
}

func (m *mockRepo) DeleteBatch(ctx context.Context, ids []int64) (int64, error) {
	return m.deleteBatch(ctx, ids)
}

func (m *mockRepo) DeptAndChildIDs(ctx context.Context, deptID int64) ([]int64, error) {
	return m.deptAndChildIDs(ctx, deptID)
}

func (m *mockRepo) DeptNameMap(ctx context.Context, deptIDs []int64) (map[int64]*string, error) {
	return m.deptNameMap(ctx, deptIDs)
}

func (m *mockRepo) DeptTree(ctx context.Context, conds []crudx.Cond) ([]DeptNode, error) {
	return m.deptTree(ctx, conds)
}

func testAuth(id int64) authctx.Context {
	return authctx.Context{User: authctx.User{ID: id, UserName: "tester"}}
}

// pinNow 钉住 nowFunc，便于断言审计时间字段。
func pinNow(t *testing.T, fixed time.Time) {
	t.Helper()
	old := nowFunc
	nowFunc = func() time.Time { return fixed }
	t.Cleanup(func() { nowFunc = old })
}

func ptrOf(s string) *string { return &s }

func ptrInt64(v int64) *int64 { return &v }

func ptrInt(v int) *int { return &v }

func TestCheckPostNameUniqueMissingFields(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	// 缺名称/部门 id → 编程错误兜底，不触达 repo（触达即 panic）
	ok, err := svc.CheckPostNameUnique(context.Background(), CreateIn{}, nil)
	if ok {
		t.Error("缺字段应返回 false")
	}
	if se := errs.As(err); se == nil || se.Msg != "岗位名称与部门id不能为空" {
		t.Errorf("err = %v, want 岗位名称与部门id不能为空", err)
	}
}

func TestDeletePostByIDsAssigned(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	svc.repo = &mockRepo{
		listAll: func(_ context.Context, _ ...crudx.Cond) ([]SysPost, error) {
			return []SysPost{{BaseModel: modelx.BaseModel{ID: 1}, PostName: "董事长"}}, nil
		},
		countUserPost: func(_ context.Context, postID int64) (int64, error) {
			if postID != 1 {
				t.Errorf("postID = %d, want 1", postID)
			}
			return 1, nil
		},
	}
	n, err := svc.DeletePostByIDs(context.Background(), []int64{1})
	if n != 0 {
		t.Errorf("n = %d, want 0（已分配不删除任何行）", n)
	}
	if se := errs.As(err); se == nil || se.Msg != "董事长已分配，不能删除!" {
		t.Errorf("err = %v, want 董事长已分配，不能删除!", err)
	}
}

func TestOptionSelectEmpty(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	// 无 deptId 也无 postIds → 空数组而非 null，不触达 repo
	outs, err := svc.OptionSelect(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if outs == nil || len(outs) != 0 {
		t.Errorf("outs = %#v, want 非 nil 空数组", outs)
	}
}

// ---- 新增：InsertPost 主流程 ----

// TestInsertPostSuccess 钉住新增成功路径：提供字段原样落库、
// 未提供字段不写（走列默认值）、审计填充、返回 row.ID != 0。
func TestInsertPostSuccess(t *testing.T) {
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	pinNow(t, fixed)
	svc := NewService(nil, testAuth(2))
	var got *SysPost
	svc.repo = &mockRepo{create: func(_ context.Context, row *SysPost) error {
		got = row
		return nil
	}}
	ok, err := svc.InsertPost(context.Background(), CreateIn{
		DeptID: ptrInt64(100), PostCode: ptrOf("ceo"), PostName: ptrOf("董事长"),
		PostCategory: ptrOf("management"), PostSort: ptrInt(1), Status: ptrOf("0"), Remark: ptrOf("r"),
	})
	if err != nil || !ok {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	if got.DeptID != 100 || got.PostCode != "ceo" || got.PostName != "董事长" ||
		got.PostCategory == nil || *got.PostCategory != "management" || got.PostSort != 1 || got.Status != "0" || *got.Remark != "r" {
		t.Errorf("字段落库错误: %+v", got)
	}
	if got.ID == 0 || got.CreateBy == nil || *got.CreateBy != 2 {
		t.Errorf("审计填充错误: id=%d createBy=%v", got.ID, got.CreateBy)
	}
	if got.CreateTime == nil || !got.CreateTime.Equal(fixed) || got.UpdateTime == nil || !got.UpdateTime.Equal(fixed) {
		t.Errorf("审计时间错误: %v / %v", got.CreateTime, got.UpdateTime)
	}
}

// TestInsertPostOmittedFields 未提供的可空字段保持零值（Python exclude_none 语义）。
func TestInsertPostOmittedFields(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	var got *SysPost
	svc.repo = &mockRepo{create: func(_ context.Context, row *SysPost) error {
		got = row
		return nil
	}}
	ok, err := svc.InsertPost(context.Background(), CreateIn{PostCode: ptrOf("c"), PostName: ptrOf("n")})
	if err != nil || !ok {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	if got.DeptID != 0 || got.PostSort != 0 || got.Status != "" || got.PostCategory != nil || got.Remark != nil {
		t.Errorf("未提供字段不应赋值: %+v", got)
	}
}

// TestInsertPostDBError 建库失败原样上抛（handler 负责 Wrap）。
func TestInsertPostDBError(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	boom := errors.New("boom")
	svc.repo = &mockRepo{create: func(_ context.Context, _ *SysPost) error { return boom }}
	ok, err := svc.InsertPost(context.Background(), CreateIn{PostName: ptrOf("n")})
	if ok || !errors.Is(err, boom) {
		t.Errorf("ok = %v, err = %v; want (false, 原错误)", ok, err)
	}
}

// ---- 新增：唯一性校验 ----

func TestCheckPostCodeUniqueBranches(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	// 缺编码 → 编程错误兜底，不触达 repo
	ok, err := svc.CheckPostCodeUnique(context.Background(), CreateIn{}, nil)
	if ok || errs.As(err) == nil || errs.As(err).Msg != "岗位编码不能为空" {
		t.Errorf("ok = %v, err = %v; want 岗位编码不能为空", ok, err)
	}

	var gotCode string
	var gotExclude *int64
	svc.repo = &mockRepo{existsByCode: func(_ context.Context, code string, excludeID *int64) (bool, error) {
		gotCode, gotExclude = code, excludeID
		return true, nil
	}}
	exclude := int64(5)
	ok, err = svc.CheckPostCodeUnique(context.Background(), CreateIn{PostCode: ptrOf("ceo")}, &exclude)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ok {
		t.Error("已存在应返回 false（不唯一）")
	}
	if gotCode != "ceo" || gotExclude == nil || *gotExclude != 5 {
		t.Errorf("入参透传错误: code=%q exclude=%v", gotCode, gotExclude)
	}

	svc.repo = &mockRepo{existsByCode: func(_ context.Context, _ string, _ *int64) (bool, error) {
		return false, nil
	}}
	ok, err = svc.CheckPostCodeUnique(context.Background(), CreateIn{PostCode: ptrOf("ceo")}, nil)
	if err != nil || !ok {
		t.Errorf("ok = %v, err = %v; want (true, nil)", ok, err)
	}
}

func TestCheckPostNameUniqueBranches(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	var gotName string
	var gotDept int64
	svc.repo = &mockRepo{existsByName: func(_ context.Context, name string, deptID int64, _ *int64) (bool, error) {
		gotName, gotDept = name, deptID
		return false, nil
	}}
	ok, err := svc.CheckPostNameUnique(context.Background(), CreateIn{PostName: ptrOf("董事长"), DeptID: ptrInt64(100)}, nil)
	if err != nil || !ok {
		t.Fatalf("ok = %v, err = %v; want (true, nil)", ok, err)
	}
	if gotName != "董事长" || gotDept != 100 {
		t.Errorf("入参透传错误: name=%q dept=%d", gotName, gotDept)
	}

	svc.repo = &mockRepo{existsByName: func(_ context.Context, _ string, _ int64, _ *int64) (bool, error) {
		return true, nil
	}}
	ok, err = svc.CheckPostNameUnique(context.Background(), CreateIn{PostName: ptrOf("董事长"), DeptID: ptrInt64(100)}, nil)
	if err != nil || ok {
		t.Errorf("ok = %v, err = %v; want (false, nil)（同部门重名）", ok, err)
	}
}

// ---- 新增：UpdatePost 主流程 ----

func TestUpdatePostPartial(t *testing.T) {
	fixed := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	pinNow(t, fixed)
	svc := NewService(nil, testAuth(3))
	created := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	old := &SysPost{DeptID: 1, PostCode: "old", PostName: "旧名", PostCategory: ptrOf("cat"), PostSort: 1, Status: "0", Remark: ptrOf("r")}
	old.ID = 5
	old.CreateTime = &created
	var updated *SysPost
	svc.repo = &mockRepo{
		get: func(_ context.Context, id int64) (*SysPost, error) {
			if id != 5 {
				t.Errorf("id = %d, want 5", id)
			}
			return old, nil
		},
		update: func(_ context.Context, row *SysPost) error {
			updated = row
			return nil
		},
	}
	id := int64(5)
	ok, err := svc.UpdatePost(context.Background(), UpdateIn{
		CreateIn: CreateIn{PostName: ptrOf("新名"), Status: ptrOf("1")},
		ID:       &id,
	})
	if err != nil || !ok {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	if updated.PostName != "新名" || updated.Status != "1" {
		t.Errorf("提供字段未更新: %+v", updated)
	}
	if updated.DeptID != 1 || updated.PostCode != "old" || updated.PostSort != 1 || *updated.PostCategory != "cat" || *updated.Remark != "r" {
		t.Errorf("未提供字段被误改: %+v", updated)
	}
	if updated.UpdateBy == nil || *updated.UpdateBy != 3 || !updated.UpdateTime.Equal(fixed) {
		t.Errorf("更新审计错误: %v / %v", updated.UpdateBy, updated.UpdateTime)
	}
	if !updated.CreateTime.Equal(created) {
		t.Error("create_time 不应被更新覆盖")
	}
}

func TestUpdatePostNotFound(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	svc.repo = &mockRepo{get: func(_ context.Context, _ int64) (*SysPost, error) { return nil, nil }}
	id := int64(404)
	ok, err := svc.UpdatePost(context.Background(), UpdateIn{ID: &id})
	if ok || err != nil {
		t.Errorf("ok = %v, err = %v; want (false, nil)", ok, err)
	}
}

// ---- 新增：DeletePostByIDs 主流程 ----

// TestDeletePostByIDsSuccess 全部未分配 → 删除的是 **ListAll 返回行的 ID**
// （不是入参 ids：不存在的 id 不进 DELETE）。
func TestDeletePostByIDsSuccess(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	var gotIDs []int64
	svc.repo = &mockRepo{
		listAll: func(_ context.Context, conds ...crudx.Cond) ([]SysPost, error) {
			if len(conds) != 1 || conds[0].SQL != "id IN ?" {
				t.Errorf("conds = %+v, want id IN ?", conds)
			}
			return []SysPost{
				{BaseModel: modelx.BaseModel{ID: 1}, PostName: "a"},
				{BaseModel: modelx.BaseModel{ID: 2}, PostName: "b"},
			}, nil
		},
		countUserPost: func(_ context.Context, _ int64) (int64, error) { return 0, nil },
		deleteBatch: func(_ context.Context, ids []int64) (int64, error) {
			gotIDs = ids
			return 2, nil
		},
	}
	n, err := svc.DeletePostByIDs(context.Background(), []int64{1, 2, 99})
	if err != nil || n != 2 {
		t.Fatalf("n = %d, err = %v", n, err)
	}
	if len(gotIDs) != 2 || gotIDs[0] != 1 || gotIDs[1] != 2 {
		t.Errorf("DeleteBatch ids = %v, want [1 2]（99 不存在不删）", gotIDs)
	}
}

// TestDeletePostByIDsSecondAssigned 第二个岗位已分配 → 报其名称且一行不删
// （DeleteBatch 未覆盖，触达即 panic）。
func TestDeletePostByIDsSecondAssigned(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	svc.repo = &mockRepo{
		listAll: func(_ context.Context, _ ...crudx.Cond) ([]SysPost, error) {
			return []SysPost{
				{BaseModel: modelx.BaseModel{ID: 1}, PostName: "自由岗"},
				{BaseModel: modelx.BaseModel{ID: 2}, PostName: "董事长"},
			}, nil
		},
		countUserPost: func(_ context.Context, postID int64) (int64, error) {
			if postID == 2 {
				return 3, nil
			}
			return 0, nil
		},
	}
	n, err := svc.DeletePostByIDs(context.Background(), []int64{1, 2})
	if n != 0 {
		t.Errorf("n = %d, want 0", n)
	}
	if se := errs.As(err); se == nil || se.Msg != "董事长已分配，不能删除!" {
		t.Errorf("err = %v, want 董事长已分配，不能删除!", err)
	}
}

// ---- 新增：导出/查询数据组装 ----

// TestSelectListExportAssembly 导出走 ListAll（无分页）+ 内存稳定排序
// （post_sort 相同保持查询返回序）+ 部门名批量回填（去重升序，缺失保持 nil）。
func TestSelectListExportAssembly(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	var gotIDs []int64
	mk := func(id int64, name string, sort int, dept int64) SysPost {
		p := SysPost{PostName: name, PostSort: sort, DeptID: dept, PostCode: "c", Status: "0"}
		p.ID = id
		return p
	}
	svc.repo = &mockRepo{
		listAll: func(_ context.Context, _ ...crudx.Cond) ([]SysPost, error) {
			return []SysPost{mk(1, "A", 2, 10), mk(2, "B", 1, 20), mk(3, "C", 2, 10)}, nil
		},
		deptNameMap: func(_ context.Context, ids []int64) (map[int64]*string, error) {
			gotIDs = ids
			return map[int64]*string{20: ptrOf("部门20")}, nil
		},
	}
	outs, err := svc.SelectList(context.Background(), QueryParam{})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(outs) != 3 || *outs[0].PostName != "B" || *outs[1].PostName != "A" || *outs[2].PostName != "C" {
		t.Fatalf("稳定排序错误: %v %v %v", *outs[0].PostName, *outs[1].PostName, *outs[2].PostName)
	}
	if len(gotIDs) != 2 || gotIDs[0] != 10 || gotIDs[1] != 20 {
		t.Errorf("DeptNameMap 入参 = %v, want 去重升序 [10 20]", gotIDs)
	}
	if outs[0].DeptName == nil || *outs[0].DeptName != "部门20" {
		t.Errorf("B 的部门名 = %v, want 部门20", outs[0].DeptName)
	}
	if outs[1].DeptName != nil || outs[2].DeptName != nil {
		t.Error("部门已删除（映射缺失）时 DeptName 应保持 nil")
	}
}

// TestPageListAssembly 分页：条件透传、total 透传、部门名回填。
func TestPageListAssembly(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	var gotConds []crudx.Cond
	var gotPage crudx.Page
	size := 10
	svc.repo = &mockRepo{
		page: func(_ context.Context, page crudx.Page, conds []crudx.Cond) ([]SysPost, int64, error) {
			gotPage, gotConds = page, conds
			p := SysPost{PostName: "n", PostCode: "c", Status: "0", DeptID: 10}
			p.ID = 1
			return []SysPost{p}, 37, nil
		},
		deptNameMap: func(_ context.Context, _ []int64) (map[int64]*string, error) {
			return map[int64]*string{10: ptrOf("研发部")}, nil
		},
	}
	q := QueryParam{Page: crudx.Page{PageNum: 2, PageSize: &size}, PostCode: "ce", Status: "0"}
	out, err := svc.PageList(context.Background(), q)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if int64(out.Total) != 37 || len(out.Rows) != 1 || out.Rows[0].DeptName == nil || *out.Rows[0].DeptName != "研发部" {
		t.Errorf("out = %+v", out)
	}
	if gotPage.PageNum != 2 || gotPage.PageSize == nil || *gotPage.PageSize != 10 {
		t.Errorf("page = %+v, want 原样透传", gotPage)
	}
	if len(gotConds) != 2 || gotConds[0].SQL != "post_code LIKE ?" || gotConds[0].Args[0] != "%ce%" || gotConds[1].SQL != "status = ?" {
		t.Errorf("conds = %+v", gotConds)
	}
}

// TestBuildConditionsDeptMutualExclusion dept_id 与 belong_dept_id 互斥：
// 两者都传时只有 dept_id 生效（DeptAndChildIDs 未覆盖，触达即 panic）。
func TestBuildConditionsDeptMutualExclusion(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	svc.repo = &mockRepo{}
	conds, err := svc.buildConditions(context.Background(), QueryParam{DeptID: ptrInt64(5), BelongDeptID: ptrInt64(9)})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(conds) != 1 || conds[0].SQL != "dept_id = ?" || conds[0].Args[0] != int64(5) {
		t.Errorf("conds = %+v, want 仅 dept_id = 5", conds)
	}

	// 只传 belong_dept_id → 部门树搜索（含子孙）
	svc.repo = &mockRepo{deptAndChildIDs: func(_ context.Context, deptID int64) ([]int64, error) {
		if deptID != 9 {
			t.Errorf("deptID = %d, want 9", deptID)
		}
		return []int64{9, 10, 11}, nil
	}}
	conds, err = svc.buildConditions(context.Background(), QueryParam{BelongDeptID: ptrInt64(9)})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(conds) != 1 || conds[0].SQL != "dept_id IN ?" {
		t.Errorf("conds = %+v, want dept_id IN ?", conds)
	}
}

// TestOptionSelectByDept deptId 优先于 postIds：走 SelectList（Eq dept_id）。
// 注意：Python select_list 同样不分页（crud.list_all 无 page 参数），两端一致返回全部。
func TestOptionSelectByDept(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	var gotConds []crudx.Cond
	svc.repo = &mockRepo{
		listAll: func(_ context.Context, conds ...crudx.Cond) ([]SysPost, error) {
			gotConds = conds
			p := SysPost{PostName: "n", PostCode: "c", Status: "0", DeptID: 5, PostSort: 1}
			p.ID = 1
			return []SysPost{p}, nil
		},
		deptNameMap: func(_ context.Context, _ []int64) (map[int64]*string, error) {
			return map[int64]*string{}, nil
		},
	}
	outs, err := svc.OptionSelect(context.Background(), []int64{99}, ptrInt64(5))
	if err != nil || len(outs) != 1 {
		t.Fatalf("outs = %+v, err = %v", outs, err)
	}
	if len(gotConds) != 1 || gotConds[0].SQL != "dept_id = ?" || gotConds[0].Args[0] != int64(5) {
		t.Errorf("conds = %+v, want dept_id = 5（postIds 被忽略）", gotConds)
	}
}

// TestOptionSelectByPostIDs 无 deptId 时按 postIds 查正常状态岗位并回填部门名。
func TestOptionSelectByPostIDs(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	var gotIDs []int64
	svc.repo = &mockRepo{
		listOptionByIDs: func(_ context.Context, ids []int64) ([]SysPost, error) {
			gotIDs = ids
			p := SysPost{PostName: "n", PostCode: "c", Status: "0", DeptID: 10}
			p.ID = 7
			return []SysPost{p}, nil
		},
		deptNameMap: func(_ context.Context, _ []int64) (map[int64]*string, error) {
			return map[int64]*string{10: ptrOf("研发部")}, nil
		},
	}
	outs, err := svc.OptionSelect(context.Background(), []int64{7, 8}, nil)
	if err != nil || len(outs) != 1 {
		t.Fatalf("outs = %+v, err = %v", outs, err)
	}
	if len(gotIDs) != 2 || gotIDs[0] != 7 || gotIDs[1] != 8 {
		t.Errorf("ids = %v, want [7 8]", gotIDs)
	}
	if outs[0].DeptName == nil || *outs[0].DeptName != "研发部" {
		t.Errorf("DeptName = %v, want 研发部", outs[0].DeptName)
	}
}

// TestSelectPostByUserID 用户所属岗位：Info 精简出参与 ID 列表两条路径。
func TestSelectPostByUserID(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	p := SysPost{DeptID: 10, PostCode: "ceo", PostName: "董事长", PostCategory: ptrOf("cat")}
	p.ID = 7
	svc.repo = &mockRepo{listByUserID: func(_ context.Context, userID int64) ([]SysPost, error) {
		if userID != 3 {
			t.Errorf("userID = %d, want 3", userID)
		}
		return []SysPost{p}, nil
	}}
	infos, err := svc.SelectPostByUserID(context.Background(), 3)
	if err != nil || len(infos) != 1 {
		t.Fatalf("infos = %+v, err = %v", infos, err)
	}
	if infos[0].ID == nil || int64(*infos[0].ID) != 7 || infos[0].PostCode == nil || *infos[0].PostCode != "ceo" {
		t.Errorf("Info 转换错误: %+v", infos[0])
	}
	ids, err := svc.SelectPostIDsByUserID(context.Background(), 3)
	if err != nil || len(ids) != 1 || ids[0] != 7 {
		t.Errorf("ids = %v, err = %v; want [7]", ids, err)
	}
}

// ---- 新增：部门树 ----

// TestDeptTreeListConditions 部门树查询：del_flag='0' 恒为首条件，
// belongDeptId 展开为 id IN (含子孙)。
func TestDeptTreeListConditions(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	var gotConds []crudx.Cond
	svc.repo = &mockRepo{
		deptAndChildIDs: func(_ context.Context, _ int64) ([]int64, error) { return []int64{9, 10}, nil },
		deptTree: func(_ context.Context, conds []crudx.Cond) ([]DeptNode, error) {
			gotConds = conds
			return []DeptNode{}, nil
		},
	}
	nodes, err := svc.DeptTreeList(context.Background(), DeptTreeQuery{BelongDeptID: ptrInt64(9), DeptName: "研"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if nodes == nil || len(nodes) != 0 {
		t.Errorf("空结果应为非 nil 空切片: %#v", nodes)
	}
	if len(gotConds) != 3 || gotConds[0].SQL != "del_flag = ?" || gotConds[0].Args[0] != modelx.Normal {
		t.Fatalf("conds = %+v, want del_flag 恒首", gotConds)
	}
	if gotConds[1].SQL != "dept_name LIKE ?" || gotConds[2].SQL != "id IN ?" {
		t.Errorf("conds = %+v", gotConds)
	}
}

// TestBuildDeptTree 纯函数建树：多根（父不在结果集）、children 仅非空才挂、
// status='1' → disabled、NULL parent 必为根。
func TestBuildDeptTree(t *testing.T) {
	if got := BuildDeptTree(nil); got == nil || len(got) != 0 {
		t.Errorf("空输入应返回非 nil 空切片: %#v", got)
	}
	rows := []DeptNode{
		{ID: 1, ParentID: nil, DeptName: ptrOf("根"), OrderNum: ptrInt(1), Status: ptrOf("0")},
		{ID: 2, ParentID: ptrInt64(1), DeptName: ptrOf("子"), Status: ptrOf("1")},
		{ID: 3, ParentID: ptrInt64(99), DeptName: ptrOf("孤儿")}, // 父不在结果集 → 也是根
	}
	roots := BuildDeptTree(rows)
	if len(roots) != 2 || int64(roots[0].ID) != 1 || int64(roots[1].ID) != 3 {
		t.Fatalf("roots = %+v, want [1 3]（多根）", roots)
	}
	if len(roots[0].Children) != 1 || int64(roots[0].Children[0].ID) != 2 {
		t.Errorf("根 1 应挂子节点 2: %+v", roots[0].Children)
	}
	if !roots[0].Children[0].Disabled {
		t.Error("status='1' 的子节点应 disabled")
	}
	if roots[0].Disabled || roots[1].Children != nil {
		t.Errorf("无子节点不应挂 children: %+v", roots[1])
	}
	if roots[0].Label == nil || *roots[0].Label != "根" || roots[0].Weight == nil || int64(*roots[0].Weight) != 1 {
		t.Errorf("label/weight 转换错误: %+v", roots[0])
	}
	if roots[1].ParentID == nil || int64(*roots[1].ParentID) != 99 {
		t.Errorf("孤儿节点 parentId 应保留原值: %+v", roots[1].ParentID)
	}
}
