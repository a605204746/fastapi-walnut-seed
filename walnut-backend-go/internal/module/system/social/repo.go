// 本文件实现社交授权关系的数据访问，对应 Python 端 module_system/social/crud.py。
//
// 查询一律追加 del_flag='0'；list_by_user_id **不排序**（base list_all 语义）。

package social

import (
	"context"

	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/modelx"
)

// Repo 社交授权关系数据访问层。
type Repo struct{ db *gorm.DB }

// NewRepo 构造。
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// ListByUserID 按用户 ID 查询授权列表（del_flag 过滤、不排序）。
func (r *Repo) ListByUserID(ctx context.Context, userID int64) ([]SysSocial, error) {
	return crudx.ListAll[SysSocial](ctx, r.db,
		crudx.Eq("user_id", userID),
		crudx.Eq("del_flag", modelx.Normal))
}
