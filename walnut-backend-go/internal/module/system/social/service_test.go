package social

import (
	"context"
	"errors"
	"testing"
	"time"

	"walnut-seed-go/internal/pkg/authctx"
)

// mockRepo 嵌入 socialRepo（nil），仅以函数字段覆盖被测方法；
// 未覆盖的方法被调用即 panic，暴露测试外的意外依赖。
type mockRepo struct {
	socialRepo
	listByUserID func(ctx context.Context, userID int64) ([]SysSocial, error)
}

func (m *mockRepo) ListByUserID(ctx context.Context, userID int64) ([]SysSocial, error) {
	return m.listByUserID(ctx, userID)
}

func TestListByUserIDConversion(t *testing.T) {
	svc := NewService(nil, authctx.Context{})
	expire := 3600
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	full := SysSocial{
		UserID:      7,
		AuthID:      "auth-1",
		Source:      "github",
		AccessToken: "tok",
		UserName:    "tester",
		ExpireIn:    &expire,
	}
	full.ID = 11
	full.CreateTime = &now
	bare := SysSocial{UserID: 7, AuthID: "auth-2", Source: "gitee", AccessToken: "tok2", UserName: "tester"}
	bare.ID = 12
	svc.repo = &mockRepo{listByUserID: func(_ context.Context, userID int64) ([]SysSocial, error) {
		if userID != 7 {
			t.Errorf("userID = %d, want 7", userID)
		}
		return []SysSocial{full, bare}, nil
	}}
	outs, err := svc.ListByUserID(context.Background(), 7)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(outs) != 2 {
		t.Fatalf("len = %d, want 2", len(outs))
	}
	o := outs[0]
	if o.ID == nil || int64(*o.ID) != 11 || o.UserID == nil || int64(*o.UserID) != 7 {
		t.Errorf("ID/UserID 转换错误: %+v", o)
	}
	if o.AuthID == nil || *o.AuthID != "auth-1" || o.Source == nil || *o.Source != "github" {
		t.Errorf("AuthID/Source 转换错误: %+v", o)
	}
	if o.ExpireIn == nil || int64(*o.ExpireIn) != 3600 {
		t.Errorf("ExpireIn = %v, want 3600", o.ExpireIn)
	}
	if o.CreateTime == nil {
		t.Error("CreateTime 应非 nil")
	}
	if outs[1].ExpireIn != nil || outs[1].CreateTime != nil {
		t.Errorf("nil 列应保持 nil: %+v", outs[1])
	}
}

func TestListByUserIDEmpty(t *testing.T) {
	svc := NewService(nil, authctx.Context{})
	svc.repo = &mockRepo{listByUserID: func(_ context.Context, _ int64) ([]SysSocial, error) {
		return nil, nil
	}}
	outs, err := svc.ListByUserID(context.Background(), 7)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// 空结果序列化为 []（非 null）：切片必须非 nil
	if outs == nil || len(outs) != 0 {
		t.Errorf("outs = %v, want 非 nil 空切片", outs)
	}
}

// TestListByUserIDRepoError 查询出错原样上抛（handler 负责 Wrap），不吞错返回空列表。
func TestListByUserIDRepoError(t *testing.T) {
	svc := NewService(nil, authctx.Context{})
	boom := errors.New("boom")
	svc.repo = &mockRepo{listByUserID: func(_ context.Context, _ int64) ([]SysSocial, error) {
		return nil, boom
	}}
	outs, err := svc.ListByUserID(context.Background(), 7)
	if outs != nil || !errors.Is(err, boom) {
		t.Errorf("outs = %v, err = %v; want (nil, 原错误)", outs, err)
	}
}

// TestListByUserIDOrderPreserved 绑定列表保持查询返回序（无内存排序），
// 且 userID 原样透传（当前登录用户由 handler 从 authctx 取）。
func TestListByUserIDOrderPreserved(t *testing.T) {
	svc := NewService(nil, authctx.Context{User: authctx.User{ID: 7}})
	mk := func(id int64, source string) SysSocial {
		s := SysSocial{UserID: 7, AuthID: "a", Source: source, AccessToken: "t", UserName: "u"}
		s.ID = id
		return s
	}
	svc.repo = &mockRepo{listByUserID: func(_ context.Context, userID int64) ([]SysSocial, error) {
		if userID != 7 {
			t.Errorf("userID = %d, want 7", userID)
		}
		return []SysSocial{mk(2, "gitee"), mk(1, "github")}, nil
	}}
	outs, err := svc.ListByUserID(context.Background(), 7)
	if err != nil || len(outs) != 2 {
		t.Fatalf("len = %d, err = %v", len(outs), err)
	}
	if int64(*outs[0].ID) != 2 || *outs[0].Source != "gitee" || int64(*outs[1].ID) != 1 {
		t.Errorf("顺序被改动: %+v", outs)
	}
}
