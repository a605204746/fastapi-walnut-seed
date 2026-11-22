package notice

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

// mockRepo 嵌入 noticeRepo（nil），仅以函数字段覆盖被测方法；
// 未覆盖的方法被调用即 panic，暴露测试外的意外依赖。
type mockRepo struct {
	noticeRepo
	page             func(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysNotice, int64, error)
	get              func(ctx context.Context, id int64) (*SysNotice, error)
	create           func(ctx context.Context, row *SysNotice) error
	update           func(ctx context.Context, row *SysNotice) error
	deleteBatch      func(ctx context.Context, ids []int64) (int64, error)
	findUserIDByName func(ctx context.Context, name string) (*int64, error)
	userNameMap      func(ctx context.Context, ids []int64) (map[int64]*string, error)
	dictLabelMap     func(ctx context.Context, dictType string, values []string) (map[string]string, error)
}

func (m *mockRepo) Page(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysNotice, int64, error) {
	return m.page(ctx, page, conds)
}

func (m *mockRepo) Get(ctx context.Context, id int64) (*SysNotice, error) {
	return m.get(ctx, id)
}

func (m *mockRepo) Create(ctx context.Context, row *SysNotice) error {
	return m.create(ctx, row)
}

func (m *mockRepo) Update(ctx context.Context, row *SysNotice) error {
	return m.update(ctx, row)
}

func (m *mockRepo) DeleteBatch(ctx context.Context, ids []int64) (int64, error) {
	return m.deleteBatch(ctx, ids)
}

func (m *mockRepo) FindUserIDByName(ctx context.Context, name string) (*int64, error) {
	return m.findUserIDByName(ctx, name)
}

func (m *mockRepo) UserNameMap(ctx context.Context, ids []int64) (map[int64]*string, error) {
	return m.userNameMap(ctx, ids)
}

func (m *mockRepo) DictLabelMap(ctx context.Context, dictType string, values []string) (map[string]string, error) {
	return m.dictLabelMap(ctx, dictType, values)
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

func TestInsertMissingTypeConflict(t *testing.T) {
	svc := NewService(nil, testAuth(1))
	title := "t"
	ok, err := svc.Insert(context.Background(), CreateIn{NoticeTitle: &title})
	if ok {
		t.Error("缺 noticeType 应新增失败")
	}
	if se := errs.As(err); se == nil || se.Code != errs.CodeConflict || se.Msg != errs.MsgDBIntegrity {
		t.Errorf("err = %v, want 409 完整性约束", err)
	}
}

func TestGetNoticeTypeLabelFallback(t *testing.T) {
	svc := NewService(nil, testAuth(1))
	svc.repo = &mockRepo{dictLabelMap: func(_ context.Context, dictType string, _ []string) (map[string]string, error) {
		if dictType != "sys_notice_type" {
			t.Errorf("dictType = %s, want sys_notice_type", dictType)
		}
		return map[string]string{"1": "通知"}, nil
	}}
	cases := []struct {
		name string
		in   *string
		want string
	}{
		{"nil返回空串", nil, ""},
		{"空串返回空串", ptrOf(""), ""},
		{"命中返回标签", ptrOf("1"), "通知"},
		{"未命中回退原值", ptrOf("3"), "3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := svc.GetNoticeTypeLabel(context.Background(), tc.in); got != tc.want {
				t.Errorf("label = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildConditionsUnknownCreator(t *testing.T) {
	svc := NewService(nil, testAuth(1))
	svc.repo = &mockRepo{findUserIDByName: func(_ context.Context, _ string) (*int64, error) {
		return nil, nil
	}}
	conds, err := svc.buildConditions(context.Background(), Query{CreateByName: "ghost"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// 查不到用户时条件退化为 create_by IS NULL（不是忽略该条件）
	if len(conds) != 1 || conds[0].SQL != "create_by IS NULL" {
		t.Errorf("conds = %+v, want create_by IS NULL", conds)
	}
}

func ptrOf(s string) *string { return &s }

// ---- 新增：Insert 主流程 ----

// TestInsertSuccessDefaults 钉住新增成功路径：content 字符串→BLOB 字节、
// status 缺省走列默认值 '0'、remark 缺省 NULL、审计填充、返回 row.ID != 0。
func TestInsertSuccessDefaults(t *testing.T) {
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	pinNow(t, fixed)
	svc := NewService(nil, testAuth(9))
	var got *SysNotice
	svc.repo = &mockRepo{create: func(_ context.Context, row *SysNotice) error {
		got = row
		return nil
	}}
	ok, err := svc.Insert(context.Background(), CreateIn{
		NoticeTitle: ptrOf("标题"), NoticeType: ptrOf("1"), NoticeContent: ptrOf("正文<b>x</b>"),
	})
	if err != nil || !ok {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	if got.NoticeTitle != "标题" || got.NoticeType != "1" || string(got.NoticeContent) != "正文<b>x</b>" {
		t.Errorf("字段落库错误: %+v", got)
	}
	if got.Status == nil || *got.Status != modelx.Normal {
		t.Errorf("status = %v, want 列默认值 0", got.Status)
	}
	if got.Remark != nil {
		t.Errorf("remark 缺省应为 NULL, got %v", *got.Remark)
	}
	if got.ID == 0 || got.CreateBy == nil || *got.CreateBy != 9 {
		t.Errorf("审计填充错误: id=%d createBy=%v", got.ID, got.CreateBy)
	}
	if got.CreateTime == nil || !got.CreateTime.Equal(fixed) || got.UpdateTime == nil || !got.UpdateTime.Equal(fixed) {
		t.Errorf("审计时间错误: %v / %v", got.CreateTime, got.UpdateTime)
	}
}

// TestInsertExplicitStatus 显式 status/remark 原样落库（停用公告）。
func TestInsertExplicitStatus(t *testing.T) {
	svc := NewService(nil, testAuth(1))
	var got *SysNotice
	svc.repo = &mockRepo{create: func(_ context.Context, row *SysNotice) error {
		got = row
		return nil
	}}
	ok, err := svc.Insert(context.Background(), CreateIn{
		NoticeTitle: ptrOf("t"), NoticeType: ptrOf("2"), Status: ptrOf("1"), Remark: ptrOf("r"),
	})
	if err != nil || !ok {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	if *got.Status != "1" || *got.Remark != "r" || got.NoticeContent != nil {
		t.Errorf("显式字段错误: %+v", got)
	}
}

// TestInsertDBError 建库失败 → (false, 500 数据库操作失败)。
func TestInsertDBError(t *testing.T) {
	svc := NewService(nil, testAuth(1))
	svc.repo = &mockRepo{create: func(_ context.Context, _ *SysNotice) error {
		return errors.New("boom")
	}}
	ok, err := svc.Insert(context.Background(), CreateIn{NoticeTitle: ptrOf("t"), NoticeType: ptrOf("1")})
	if ok {
		t.Error("失败时 ok 应为 false")
	}
	if se := errs.As(err); se == nil || se.Code != errs.CodeError || se.Msg != errs.MsgDBFailed {
		t.Errorf("err = %v, want 数据库操作失败", err)
	}
}

// ---- 新增：Update 主流程 ----

// TestUpdateSuccessPartial 仅更新非空字段；content 字符串→字节；
// FillUpdate 覆盖 update_by/update_time 而不动 create_time。
func TestUpdateSuccessPartial(t *testing.T) {
	fixed := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	pinNow(t, fixed)
	svc := NewService(nil, testAuth(8))
	created := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	old := &SysNotice{NoticeTitle: "旧标题", NoticeType: "1", NoticeContent: []byte("旧正文"), Status: ptrOf("0"), Remark: ptrOf("旧备注")}
	old.ID = 5
	old.CreateTime = &created
	var updated *SysNotice
	svc.repo = &mockRepo{
		get: func(_ context.Context, id int64) (*SysNotice, error) {
			if id != 5 {
				t.Errorf("id = %d, want 5", id)
			}
			return old, nil
		},
		update: func(_ context.Context, row *SysNotice) error {
			updated = row
			return nil
		},
	}
	id := int64(5)
	ok, err := svc.Update(context.Background(), UpdateIn{
		CreateIn: CreateIn{NoticeTitle: ptrOf("新标题"), NoticeContent: ptrOf("新正文")},
		ID:       &id,
	})
	if err != nil || !ok {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	if updated.NoticeTitle != "新标题" || string(updated.NoticeContent) != "新正文" {
		t.Errorf("提供字段未更新: %+v", updated)
	}
	if updated.NoticeType != "1" || *updated.Status != "0" || *updated.Remark != "旧备注" {
		t.Errorf("未提供字段被误改: %+v", updated)
	}
	if updated.UpdateBy == nil || *updated.UpdateBy != 8 || !updated.UpdateTime.Equal(fixed) {
		t.Errorf("更新审计错误: %v / %v", updated.UpdateBy, updated.UpdateTime)
	}
	if !updated.CreateTime.Equal(created) {
		t.Error("create_time 不应被更新覆盖")
	}
}

// TestUpdateNotFound 记录不存在 → (false, nil)，由 handler 转 ErrorResponse。
func TestUpdateNotFound(t *testing.T) {
	svc := NewService(nil, testAuth(1))
	svc.repo = &mockRepo{get: func(_ context.Context, _ int64) (*SysNotice, error) {
		return nil, nil
	}}
	id := int64(404)
	ok, err := svc.Update(context.Background(), UpdateIn{ID: &id})
	if ok || err != nil {
		t.Errorf("ok = %v, err = %v; want (false, nil)", ok, err)
	}
}

// TestUpdateGetError 查询出错原样上抛（service 不包装，handler 负责 Wrap）。
func TestUpdateGetError(t *testing.T) {
	svc := NewService(nil, testAuth(1))
	boom := errors.New("boom")
	svc.repo = &mockRepo{get: func(_ context.Context, _ int64) (*SysNotice, error) {
		return nil, boom
	}}
	id := int64(1)
	ok, err := svc.Update(context.Background(), UpdateIn{ID: &id})
	if ok || !errors.Is(err, boom) {
		t.Errorf("ok = %v, err = %v; want (false, 原错误)", ok, err)
	}
}

// ---- 新增：Delete / 查询回填 ----

func TestDeleteByIDsPassthrough(t *testing.T) {
	svc := NewService(nil, testAuth(1))
	var gotIDs []int64
	svc.repo = &mockRepo{deleteBatch: func(_ context.Context, ids []int64) (int64, error) {
		gotIDs = ids
		return 3, nil
	}}
	n, err := svc.DeleteByIDs(context.Background(), []int64{1, 2, 3})
	if err != nil || n != 3 || len(gotIDs) != 3 {
		t.Errorf("n = %d, err = %v, ids = %v", n, err, gotIDs)
	}
}

// TestPageFillCreateByName 分页：createByName 反查用户 ID 施加等值条件；
// 创建人账号批量回填（入参去重升序），映射缺失保持 nil。
func TestPageFillCreateByName(t *testing.T) {
	svc := NewService(nil, testAuth(1))
	uid := int64(3)
	var gotConds []crudx.Cond
	var gotIDs []int64
	mkRow := func(id int64, createBy *int64) SysNotice {
		r := SysNotice{NoticeTitle: "t", NoticeType: "1"}
		r.ID = id
		r.CreateBy = createBy
		return r
	}
	svc.repo = &mockRepo{
		findUserIDByName: func(_ context.Context, name string) (*int64, error) {
			if name != "tester" {
				t.Errorf("name = %s, want tester", name)
			}
			return &uid, nil
		},
		page: func(_ context.Context, _ crudx.Page, conds []crudx.Cond) ([]SysNotice, int64, error) {
			gotConds = conds
			return []SysNotice{mkRow(1, &uid), mkRow(2, ptrInt64(1)), mkRow(3, &uid), mkRow(4, nil)}, 4, nil
		},
		userNameMap: func(_ context.Context, ids []int64) (map[int64]*string, error) {
			gotIDs = ids
			return map[int64]*string{1: ptrOf("alice"), 3: ptrOf("tester")}, nil
		},
	}
	outs, total, err := svc.Page(context.Background(), Query{CreateByName: "tester"})
	if err != nil || total != 4 || len(outs) != 4 {
		t.Fatalf("total = %d, len = %d, err = %v", total, len(outs), err)
	}
	if len(gotConds) != 1 || gotConds[0].SQL != "create_by = ?" || gotConds[0].Args[0] != uid {
		t.Errorf("conds = %+v, want create_by = 3", gotConds)
	}
	if len(gotIDs) != 2 || gotIDs[0] != 1 || gotIDs[1] != 3 {
		t.Errorf("UserNameMap 入参 = %v, want 去重升序 [1 3]", gotIDs)
	}
	if outs[0].CreateByName == nil || *outs[0].CreateByName != "tester" {
		t.Errorf("outs[0].CreateByName = %v, want tester", outs[0].CreateByName)
	}
	if outs[1].CreateByName == nil || *outs[1].CreateByName != "alice" {
		t.Errorf("outs[1].CreateByName = %v, want alice", outs[1].CreateByName)
	}
	if outs[3].CreateByName != nil {
		t.Errorf("createBy=NULL 的行账号应保持 nil, got %v", *outs[3].CreateByName)
	}
}

func ptrInt64(v int64) *int64 { return &v }

// TestGetByIDFoundAndMissing 详情：命中回填创建人账号；不存在返回 (nil, nil)。
func TestGetByIDFoundAndMissing(t *testing.T) {
	svc := NewService(nil, testAuth(1))
	row := &SysNotice{NoticeTitle: "t", NoticeType: "1", NoticeContent: []byte("c")}
	row.ID = 7
	row.CreateBy = ptrInt64(3)
	svc.repo = &mockRepo{
		get: func(_ context.Context, _ int64) (*SysNotice, error) { return row, nil },
		userNameMap: func(_ context.Context, ids []int64) (map[int64]*string, error) {
			if len(ids) != 1 || ids[0] != 3 {
				t.Errorf("ids = %v, want [3]", ids)
			}
			return map[int64]*string{3: ptrOf("tester")}, nil
		},
	}
	out, err := svc.GetByID(context.Background(), 7)
	if err != nil || out == nil {
		t.Fatalf("out = %v, err = %v", out, err)
	}
	if out.NoticeContent == nil || *out.NoticeContent != "c" {
		t.Errorf("BLOB 应转回字符串: %v", out.NoticeContent)
	}
	if out.CreateByName == nil || *out.CreateByName != "tester" {
		t.Errorf("CreateByName = %v, want tester", out.CreateByName)
	}

	svc.repo = &mockRepo{get: func(_ context.Context, _ int64) (*SysNotice, error) { return nil, nil }}
	out, err = svc.GetByID(context.Background(), 404)
	if out != nil || err != nil {
		t.Errorf("out = %v, err = %v; want (nil, nil)", out, err)
	}
}

// TestGetNoticeTypeLabelDictError 字典查询出错 → 回退原值（SSE 载荷不为空标签）。
func TestGetNoticeTypeLabelDictError(t *testing.T) {
	svc := NewService(nil, testAuth(1))
	svc.repo = &mockRepo{dictLabelMap: func(_ context.Context, _ string, _ []string) (map[string]string, error) {
		return nil, errors.New("boom")
	}}
	if got := svc.GetNoticeTypeLabel(context.Background(), ptrOf("2")); got != "2" {
		t.Errorf("label = %q, want 回退原值 2", got)
	}
}
