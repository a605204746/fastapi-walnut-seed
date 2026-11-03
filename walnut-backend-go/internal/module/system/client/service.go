// 本文件实现客户端域的业务逻辑，对应 Python 端 module_system/client/service.py。
//
// ⚠️ 新增时 client_id = **md5(clientKey + clientSecret)** 的 hex；
// grant_type 落库为 grantTypeList 的逗号连接（空列表 → 空串）。
// ⚠️ 缺省字段走列默认值：active_timeout=1800、timeout=604800、status='0'，其余 NULL。

package client

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"strings"
	"time"

	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/modelx"
)

// nowFunc 提供「当前时间」，便于单测钉住审计字段。
var nowFunc = time.Now

// 列默认值，对应 ClientModel 的 mapped_column(default=...)。
const (
	defaultActiveTimeout = 1800
	defaultTimeout       = 604800
)

// Service 客户端业务层。
type Service struct {
	repo *Repo
	auth authctx.Context
}

// NewService 构造业务层。
func NewService(db *gorm.DB, ac authctx.Context) *Service {
	return &Service{repo: NewRepo(db), auth: ac}
}

// errDupKey 哨兵：客户端 key 已存在（handler 据此拼带 key 的 500 文案）。
var errDupKey = errs.New("客户端key已存在")

// ---- 查询 ----

// buildConditions 对应 _build_conditions（全等值过滤）。
func buildConditions(q Query) []crudx.Cond {
	conds := make([]crudx.Cond, 0, 4)
	if crudx.IsNotBlank(q.ClientID) {
		conds = append(conds, crudx.Eq("client_id", q.ClientID))
	}
	if crudx.IsNotBlank(q.ClientKey) {
		conds = append(conds, crudx.Eq("client_key", q.ClientKey))
	}
	if crudx.IsNotBlank(q.ClientSecret) {
		conds = append(conds, crudx.Eq("client_secret", q.ClientSecret))
	}
	if crudx.IsNotBlank(q.Status) {
		conds = append(conds, crudx.Eq("status", q.Status))
	}
	return conds
}

// Page 分页查询，对应 query_page_list。
func (s *Service) Page(ctx context.Context, q Query) ([]Out, int64, error) {
	rows, total, err := s.repo.Page(ctx, q.Page, buildConditions(q))
	if err != nil {
		return nil, 0, err
	}
	return outs(rows), total, nil
}

// ListAll 不分页查询（导出用），对应 query_list。
func (s *Service) ListAll(ctx context.Context, q Query) ([]Out, error) {
	rows, err := s.repo.ListAll(ctx, buildConditions(q)...)
	if err != nil {
		return nil, err
	}
	return outs(rows), nil
}

// GetByID 按主键查询，对应 query_by_id（不存在返回 nil）。
func (s *Service) GetByID(ctx context.Context, id int64) (*Out, error) {
	row, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	out := NewOut(row)
	return &out, nil
}

// ---- 校验 ----

// CheckKeyUnique 客户端 key 唯一性，对应 check_client_key_unique。
func (s *Service) CheckKeyUnique(ctx context.Context, key *string, excludeID *int64) error {
	if key == nil {
		// Python 端是 assert（schema validate_default 已保证非空），防御性镜像
		return errs.New("客户端key不能为空")
	}
	exists, err := s.repo.ExistsByKey(ctx, *key, excludeID)
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	if exists {
		return errDupKey
	}
	return nil
}

// ---- 写操作 ----

// Insert 新增客户端，对应 insert_by_bo。
func (s *Service) Insert(ctx context.Context, in CreateIn) (bool, error) {
	row := SysClient{DelFlag: modelx.Normal}
	row.ClientKey = in.ClientKey
	row.ClientSecret = in.ClientSecret
	// grant_type = join_comma(grant_type_list)；空列表 → 空串
	joined := strings.Join(in.GrantTypeList, ",")
	row.GrantType = &joined
	// client_id = md5(clientKey + clientSecret) 的 hex
	sum := md5.Sum([]byte(deref(in.ClientKey) + deref(in.ClientSecret)))
	clientID := hex.EncodeToString(sum[:])
	row.ClientID = &clientID
	row.DeviceType = in.DeviceType
	if in.ActiveTimeout != nil {
		row.ActiveTimeout = in.ActiveTimeout
	} else {
		v := defaultActiveTimeout
		row.ActiveTimeout = &v
	}
	if in.Timeout != nil {
		row.Timeout = in.Timeout
	} else {
		v := defaultTimeout
		row.Timeout = &v
	}
	if in.Status != nil {
		row.Status = in.Status
	} else {
		v := modelx.Normal
		row.Status = &v
	}

	crudx.FillCreate(&row.BaseModel, s.auth, nowFunc())
	if err := s.repo.Create(ctx, &row); err != nil {
		return false, errs.Wrap(err, errs.MsgDBFailed)
	}
	return row.ID != 0, nil
}

// Update 修改客户端（仅更新非空字段），对应 update_by_bo。
func (s *Service) Update(ctx context.Context, in UpdateIn) (bool, error) {
	row, err := s.repo.Get(ctx, *in.ID)
	if err != nil {
		return false, err
	}
	if row == nil {
		return false, nil
	}
	if in.ClientKey != nil {
		row.ClientKey = in.ClientKey
	}
	if in.ClientSecret != nil {
		row.ClientSecret = in.ClientSecret
	}
	if in.GrantTypeList != nil {
		joined := strings.Join(in.GrantTypeList, ",")
		row.GrantType = &joined
	}
	if in.DeviceType != nil {
		row.DeviceType = in.DeviceType
	}
	if in.ActiveTimeout != nil {
		row.ActiveTimeout = in.ActiveTimeout
	}
	if in.Timeout != nil {
		row.Timeout = in.Timeout
	}
	if in.Status != nil {
		row.Status = in.Status
	}
	if in.ClientID != nil {
		row.ClientID = in.ClientID
	}
	crudx.FillUpdate(&row.BaseModel, s.auth, nowFunc())
	if err := s.repo.Update(ctx, row); err != nil {
		return false, err
	}
	return true, nil
}

// ChangeStatus 按客户端 id 改状态，对应 update_client_status。
func (s *Service) ChangeStatus(ctx context.Context, clientID, status string) (int64, error) {
	uid := s.auth.User.ID
	if uid == 0 {
		uid = crudx.DefaultUserID
	}
	return s.repo.UpdateStatusByClientID(ctx, clientID, status, uid, nowFunc())
}

// DeleteByIDs 批量逻辑删除，对应 delete_with_valid_by_ids。
func (s *Service) DeleteByIDs(ctx context.Context, ids []int64) (bool, error) {
	uid := s.auth.User.ID
	if uid == 0 {
		uid = crudx.DefaultUserID
	}
	n, err := s.repo.LogicalDeleteBatch(ctx, ids, uid, nowFunc())
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// ---- 内部工具 ----

func outs(rows []SysClient) []Out {
	out := make([]Out, 0, len(rows))
	for i := range rows {
		out = append(out, NewOut(&rows[i]))
	}
	return out
}
