package client

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

// mockRepo 嵌入 clientRepo（nil），仅以函数字段覆盖被测方法；
// 未覆盖的方法被调用即 panic，暴露测试外的意外依赖。
type mockRepo struct {
	clientRepo
	get                    func(ctx context.Context, id int64) (*SysClient, error)
	existsByKey            func(ctx context.Context, key string, excludeID *int64) (bool, error)
	create                 func(ctx context.Context, row *SysClient) error
	update                 func(ctx context.Context, row *SysClient) error
	updateStatusByClientID func(ctx context.Context, clientID, status string, uid int64, now time.Time) (int64, error)
	logicalDeleteBatch     func(ctx context.Context, ids []int64, uid int64, now time.Time) (int64, error)
}

func (m *mockRepo) Get(ctx context.Context, id int64) (*SysClient, error) {
	return m.get(ctx, id)
}

func (m *mockRepo) ExistsByKey(ctx context.Context, key string, excludeID *int64) (bool, error) {
	return m.existsByKey(ctx, key, excludeID)
}

func (m *mockRepo) Create(ctx context.Context, row *SysClient) error {
	return m.create(ctx, row)
}

func (m *mockRepo) Update(ctx context.Context, row *SysClient) error {
	return m.update(ctx, row)
}

func (m *mockRepo) UpdateStatusByClientID(ctx context.Context, clientID, status string, uid int64, now time.Time) (int64, error) {
	return m.updateStatusByClientID(ctx, clientID, status, uid, now)
}

func (m *mockRepo) LogicalDeleteBatch(ctx context.Context, ids []int64, uid int64, now time.Time) (int64, error) {
	return m.logicalDeleteBatch(ctx, ids, uid, now)
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

func ptrInt(v int) *int { return &v }

func TestCheckKeyUniqueNilGuard(t *testing.T) {
	svc := NewService(nil, testAuth(1))
	err := svc.CheckKeyUnique(context.Background(), nil, nil)
	if se := errs.As(err); se == nil || se.Msg != "客户端key不能为空" {
		t.Errorf("err = %v, want 客户端key不能为空", err)
	}
}

func TestCheckKeyUniqueDuplicate(t *testing.T) {
	svc := NewService(nil, testAuth(1))
	svc.repo = &mockRepo{existsByKey: func(_ context.Context, key string, _ *int64) (bool, error) {
		if key != "k" {
			t.Errorf("key = %s, want k", key)
		}
		return true, nil
	}}
	key := "k"
	err := svc.CheckKeyUnique(context.Background(), &key, nil)
	if se := errs.As(err); se == nil || se.Msg != "客户端key已存在" {
		t.Errorf("err = %v, want 客户端key已存在", err)
	}
}

func TestInsertMD5AndColumnDefaults(t *testing.T) {
	svc := NewService(nil, testAuth(1))
	var got *SysClient
	svc.repo = &mockRepo{create: func(_ context.Context, row *SysClient) error {
		row.ID = 1
		got = row
		return nil
	}}
	key, secret := "k", "s"
	ok, err := svc.Insert(context.Background(), CreateIn{
		ClientKey: &key, ClientSecret: &secret, GrantTypeList: []string{"password", "sms"},
	})
	if err != nil || !ok {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	if got.ClientID == nil || *got.ClientID != "05f39d8aef6a4f54dcc0ce5ab4385742" {
		t.Errorf("clientId = %v, want md5(key+secret)", got.ClientID)
	}
	if got.GrantType == nil || *got.GrantType != "password,sms" {
		t.Errorf("grantType = %v, want 逗号连接", got.GrantType)
	}
	if got.ActiveTimeout == nil || *got.ActiveTimeout != 1800 || got.Timeout == nil || *got.Timeout != 604800 {
		t.Errorf("缺省超时错误: %v / %v", got.ActiveTimeout, got.Timeout)
	}
	if got.Status == nil || *got.Status != "0" {
		t.Errorf("status = %v, want 列默认值 0", got.Status)
	}
}

// ---- 新增：Insert 补充路径 ----

// TestInsertEmptyGrantListAndExplicit 空授权列表 → 空串（不是 NULL）；
// 显式超时/状态/设备类型原样落库；del_flag 恒 '0'；审计填充。
func TestInsertEmptyGrantListAndExplicit(t *testing.T) {
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	pinNow(t, fixed)
	svc := NewService(nil, testAuth(7))
	var got *SysClient
	svc.repo = &mockRepo{create: func(_ context.Context, row *SysClient) error {
		got = row
		return nil
	}}
	ok, err := svc.Insert(context.Background(), CreateIn{
		ClientKey: ptrOf("app"), ClientSecret: ptrOf("secret"),
		GrantTypeList: []string{}, DeviceType: ptrOf("android"),
		ActiveTimeout: ptrInt(60), Timeout: ptrInt(3600), Status: ptrOf("1"),
	})
	if err != nil || !ok {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	if got.GrantType == nil || *got.GrantType != "" {
		t.Errorf("grantType = %v, want 空串", got.GrantType)
	}
	if got.ClientID == nil || *got.ClientID != "ec78223ce3cc5bbfc549672f6e4ede34" {
		// md5("appsecret") 的 hex，钉住拼接顺序 key+secret
		t.Errorf("clientId = %v, want md5(app+secret)", got.ClientID)
	}
	if *got.ActiveTimeout != 60 || *got.Timeout != 3600 || *got.Status != "1" || *got.DeviceType != "android" {
		t.Errorf("显式字段错误: %+v", got)
	}
	if got.DelFlag != modelx.Normal {
		t.Errorf("delFlag = %q, want 0", got.DelFlag)
	}
	if got.ID == 0 || got.CreateBy == nil || *got.CreateBy != 7 {
		t.Errorf("审计填充错误: id=%d createBy=%v", got.ID, got.CreateBy)
	}
	if got.CreateTime == nil || !got.CreateTime.Equal(fixed) {
		t.Errorf("createTime = %v, want %v", got.CreateTime, fixed)
	}
}

// TestInsertDBError 建库失败 → (false, 500 数据库操作失败)。
func TestInsertDBError(t *testing.T) {
	svc := NewService(nil, testAuth(1))
	svc.repo = &mockRepo{create: func(_ context.Context, _ *SysClient) error {
		return errors.New("boom")
	}}
	ok, err := svc.Insert(context.Background(), CreateIn{ClientKey: ptrOf("k"), ClientSecret: ptrOf("s")})
	if ok {
		t.Error("失败时 ok 应为 false")
	}
	if se := errs.As(err); se == nil || se.Code != errs.CodeError || se.Msg != errs.MsgDBFailed {
		t.Errorf("err = %v, want 数据库操作失败", err)
	}
}

// ---- 新增：唯一性校验补充 ----

func TestCheckKeyUniquePassAndDBError(t *testing.T) {
	svc := NewService(nil, testAuth(1))
	var gotExclude *int64
	svc.repo = &mockRepo{existsByKey: func(_ context.Context, _ string, excludeID *int64) (bool, error) {
		gotExclude = excludeID
		return false, nil
	}}
	exclude := int64(9)
	if err := svc.CheckKeyUnique(context.Background(), ptrOf("k"), &exclude); err != nil {
		t.Fatalf("err = %v", err)
	}
	if gotExclude == nil || *gotExclude != 9 {
		t.Errorf("excludeID = %v, want 9（修改时排除自身）", gotExclude)
	}

	svc.repo = &mockRepo{existsByKey: func(_ context.Context, _ string, _ *int64) (bool, error) {
		return false, errors.New("boom")
	}}
	err := svc.CheckKeyUnique(context.Background(), ptrOf("k"), nil)
	if se := errs.As(err); se == nil || se.Msg != errs.MsgDBFailed {
		t.Errorf("err = %v, want 数据库操作失败", err)
	}
}

// ---- 新增：Update 主流程 ----

// TestUpdatePartial 仅更新非空字段；GrantTypeList 重新逗号连接；
// ClientID 可显式覆盖；FillUpdate 覆盖更新审计。
func TestUpdatePartial(t *testing.T) {
	fixed := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	pinNow(t, fixed)
	svc := NewService(nil, testAuth(6))
	created := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	old := &SysClient{
		ClientID: ptrOf("old-id"), ClientKey: ptrOf("pc"), ClientSecret: ptrOf("s"),
		GrantType: ptrOf("password"), DeviceType: ptrOf("pc"),
		ActiveTimeout: ptrInt(1800), Timeout: ptrInt(604800), Status: ptrOf("0"), DelFlag: "0",
	}
	old.ID = 4
	old.CreateTime = &created
	var updated *SysClient
	svc.repo = &mockRepo{
		get: func(_ context.Context, id int64) (*SysClient, error) {
			if id != 4 {
				t.Errorf("id = %d, want 4", id)
			}
			return old, nil
		},
		update: func(_ context.Context, row *SysClient) error {
			updated = row
			return nil
		},
	}
	id := int64(4)
	ok, err := svc.Update(context.Background(), UpdateIn{
		CreateIn: CreateIn{GrantTypeList: []string{"password", "social"}, Status: ptrOf("1")},
		ID:       &id,
		ClientID: ptrOf("new-id"),
	})
	if err != nil || !ok {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	if *updated.GrantType != "password,social" || *updated.Status != "1" || *updated.ClientID != "new-id" {
		t.Errorf("提供字段未更新: %+v", updated)
	}
	if *updated.ClientKey != "pc" || *updated.ClientSecret != "s" || *updated.DeviceType != "pc" ||
		*updated.ActiveTimeout != 1800 || *updated.Timeout != 604800 {
		t.Errorf("未提供字段被误改: %+v", updated)
	}
	if updated.UpdateBy == nil || *updated.UpdateBy != 6 || !updated.UpdateTime.Equal(fixed) {
		t.Errorf("更新审计错误: %v / %v", updated.UpdateBy, updated.UpdateTime)
	}
	if !updated.CreateTime.Equal(created) {
		t.Error("create_time 不应被更新覆盖")
	}
}

// TestUpdateGrantListNilKeepsOld GrantTypeList=nil 时不重算 grant_type；
// 空列表（非 nil）则置空串。
func TestUpdateGrantListNilKeepsOld(t *testing.T) {
	svc := NewService(nil, testAuth(1))
	old := &SysClient{GrantType: ptrOf("password,sms")}
	old.ID = 1
	var updated *SysClient
	svc.repo = &mockRepo{
		get:    func(_ context.Context, _ int64) (*SysClient, error) { return old, nil },
		update: func(_ context.Context, row *SysClient) error { updated = row; return nil },
	}
	id := int64(1)
	if _, err := svc.Update(context.Background(), UpdateIn{CreateIn: CreateIn{Status: ptrOf("1")}, ID: &id}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if *updated.GrantType != "password,sms" {
		t.Errorf("nil 列表不应改动 grantType: %v", *updated.GrantType)
	}
	if _, err := svc.Update(context.Background(), UpdateIn{CreateIn: CreateIn{GrantTypeList: []string{}}, ID: &id}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if updated.GrantType == nil || *updated.GrantType != "" {
		t.Errorf("空列表应置空串: %v", updated.GrantType)
	}
}

func TestUpdateNotFound(t *testing.T) {
	svc := NewService(nil, testAuth(1))
	svc.repo = &mockRepo{get: func(_ context.Context, _ int64) (*SysClient, error) { return nil, nil }}
	id := int64(404)
	ok, err := svc.Update(context.Background(), UpdateIn{ID: &id})
	if ok || err != nil {
		t.Errorf("ok = %v, err = %v; want (false, nil)", ok, err)
	}
}

// ---- 新增：ChangeStatus（按 client_id） ----

// TestChangeStatus 状态修改走 client_id（不是主键），uid/now 透传。
func TestChangeStatus(t *testing.T) {
	fixed := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	pinNow(t, fixed)
	svc := NewService(nil, testAuth(5))
	var gotClientID, gotStatus string
	var gotUID int64
	var gotNow time.Time
	svc.repo = &mockRepo{updateStatusByClientID: func(_ context.Context, clientID, status string, uid int64, now time.Time) (int64, error) {
		gotClientID, gotStatus, gotUID, gotNow = clientID, status, uid, now
		return 1, nil
	}}
	n, err := svc.ChangeStatus(context.Background(), "cid-1", "1")
	if err != nil || n != 1 {
		t.Fatalf("n = %d, err = %v", n, err)
	}
	if gotClientID != "cid-1" || gotStatus != "1" || gotUID != 5 || !gotNow.Equal(fixed) {
		t.Errorf("入参错误: clientID=%q status=%q uid=%d now=%v", gotClientID, gotStatus, gotUID, gotNow)
	}
}

// TestChangeStatusDefaultUID 未登录（uid=0）→ DefaultUserID(-1)。
func TestChangeStatusDefaultUID(t *testing.T) {
	svc := NewService(nil, authctx.Context{})
	var gotUID int64
	svc.repo = &mockRepo{updateStatusByClientID: func(_ context.Context, _, _ string, uid int64, _ time.Time) (int64, error) {
		gotUID = uid
		return 1, nil
	}}
	if _, err := svc.ChangeStatus(context.Background(), "cid", "0"); err != nil {
		t.Fatalf("err = %v", err)
	}
	if gotUID != crudx.DefaultUserID {
		t.Errorf("uid = %d, want %d", gotUID, crudx.DefaultUserID)
	}
}

// ---- 新增：DeleteByIDs（逻辑删） ----

// TestDeleteByIDsLogical 逻辑删除：uid/now 透传；n>0 → true、n=0 → false。
func TestDeleteByIDsLogical(t *testing.T) {
	fixed := time.Date(2026, 4, 5, 6, 7, 8, 0, time.UTC)
	pinNow(t, fixed)
	svc := NewService(nil, testAuth(4))
	var gotIDs []int64
	var gotUID int64
	var gotNow time.Time
	svc.repo = &mockRepo{logicalDeleteBatch: func(_ context.Context, ids []int64, uid int64, now time.Time) (int64, error) {
		gotIDs, gotUID, gotNow = ids, uid, now
		return 2, nil
	}}
	ok, err := svc.DeleteByIDs(context.Background(), []int64{1, 2})
	if err != nil || !ok {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	if len(gotIDs) != 2 || gotUID != 4 || !gotNow.Equal(fixed) {
		t.Errorf("入参错误: ids=%v uid=%d now=%v", gotIDs, gotUID, gotNow)
	}

	svc.repo = &mockRepo{logicalDeleteBatch: func(_ context.Context, _ []int64, _ int64, _ time.Time) (int64, error) {
		return 0, nil
	}}
	ok, err = svc.DeleteByIDs(context.Background(), []int64{404})
	if err != nil || ok {
		t.Errorf("ok = %v, err = %v; want (false, nil)（未命中）", ok, err)
	}
}

// TestDeleteByIDsError 删除出错原样上抛（handler 负责 Wrap）。
func TestDeleteByIDsError(t *testing.T) {
	svc := NewService(nil, testAuth(1))
	boom := errors.New("boom")
	svc.repo = &mockRepo{logicalDeleteBatch: func(_ context.Context, _ []int64, _ int64, _ time.Time) (int64, error) {
		return 0, boom
	}}
	ok, err := svc.DeleteByIDs(context.Background(), []int64{1})
	if ok || !errors.Is(err, boom) {
		t.Errorf("ok = %v, err = %v; want (false, 原错误)", ok, err)
	}
}

// ---- 新增：GetByID ----

func TestGetByIDFoundAndMissing(t *testing.T) {
	svc := NewService(nil, testAuth(1))
	row := &SysClient{ClientID: ptrOf("cid"), ClientKey: ptrOf("pc"), GrantType: ptrOf("password,sms")}
	row.ID = 3
	svc.repo = &mockRepo{get: func(_ context.Context, _ int64) (*SysClient, error) { return row, nil }}
	out, err := svc.GetByID(context.Background(), 3)
	if err != nil || out == nil {
		t.Fatalf("out = %v, err = %v", out, err)
	}
	if out.ClientID == nil || *out.ClientID != "cid" {
		t.Errorf("clientId = %v, want cid", out.ClientID)
	}
	// grantTypeList 由逗号串拆分（出参契约）
	if len(out.GrantTypeList) != 2 || out.GrantTypeList[0] != "password" || out.GrantTypeList[1] != "sms" {
		t.Errorf("grantTypeList = %v, want [password sms]", out.GrantTypeList)
	}

	svc.repo = &mockRepo{get: func(_ context.Context, _ int64) (*SysClient, error) { return nil, nil }}
	out, err = svc.GetByID(context.Background(), 404)
	if out != nil || err != nil {
		t.Errorf("out = %v, err = %v; want (nil, nil)", out, err)
	}
}
