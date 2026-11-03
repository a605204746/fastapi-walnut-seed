// 本文件实现社交授权关系的业务逻辑，对应 Python 端 module_system/social/service.py。
//
// 本模块仅 /list 一个对外端点（查当前登录用户的绑定列表）；
// 其余 service 方法供认证模块社交登录使用，P2 认证链路未启用社交登录，暂不迁移。

package social

import (
	"context"

	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
)

// Service 社交授权关系业务层。
type Service struct {
	repo *Repo
	auth authctx.Context
}

// NewService 构造业务层。
func NewService(db *gorm.DB, ac authctx.Context) *Service {
	return &Service{repo: NewRepo(db), auth: ac}
}

// ListByUserID 按用户 ID 查询，对应 query_list_by_user_id。
func (s *Service) ListByUserID(ctx context.Context, userID int64) ([]Out, error) {
	rows, err := s.repo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	outs := make([]Out, 0, len(rows))
	for i := range rows {
		outs = append(outs, NewOut(&rows[i]))
	}
	return outs, nil
}
