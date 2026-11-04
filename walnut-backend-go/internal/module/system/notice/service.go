// 本文件实现通知公告域的业务逻辑，对应 Python 端 module_system/notice/service.py。

package notice

import (
	"context"
	"time"

	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/modelx"
)

// nowFunc 提供「当前时间」，便于单测钉住审计字段。
var nowFunc = time.Now

// Service 通知公告业务层。
type Service struct {
	repo *Repo
	auth authctx.Context
}

// NewService 构造业务层。
func NewService(db *gorm.DB, ac authctx.Context) *Service {
	return &Service{repo: NewRepo(db), auth: ac}
}

// ---- 查询 ----

// buildConditions 对应 _build_conditions。
//
// ⚠️ createByName 先反查用户 ID：查不到用户时条件退化为 create_by IS NULL
// （即查不到任何公告），照抄不要改成「忽略该条件」。
func (s *Service) buildConditions(ctx context.Context, q Query) ([]crudx.Cond, error) {
	conds := make([]crudx.Cond, 0, 3)
	if crudx.IsNotBlank(q.NoticeTitle) {
		conds = append(conds, crudx.Like("notice_title", q.NoticeTitle))
	}
	if crudx.IsNotBlank(q.NoticeType) {
		conds = append(conds, crudx.Eq("notice_type", q.NoticeType))
	}
	if crudx.IsNotBlank(q.CreateByName) {
		uid, err := s.repo.FindUserIDByName(ctx, q.CreateByName)
		if err != nil {
			return nil, err
		}
		if uid == nil {
			conds = append(conds, crudx.Cond{SQL: "create_by IS NULL"})
		} else {
			conds = append(conds, crudx.Eq("create_by", *uid))
		}
	}
	return conds, nil
}

// Page 分页查询，对应 page_list（含创建人账号回填）。
func (s *Service) Page(ctx context.Context, q Query) ([]Out, int64, error) {
	conds, err := s.buildConditions(ctx, q)
	if err != nil {
		return nil, 0, errs.Wrap(err, errs.MsgDBFailed)
	}
	rows, total, err := s.repo.Page(ctx, q.Page, conds)
	if err != nil {
		return nil, 0, err
	}
	outs := s.toOuts(rows)
	if err := s.fillCreateByName(ctx, outs); err != nil {
		return nil, 0, err
	}
	return outs, total, nil
}

// GetByID 按 ID 查询，对应 get_by_id（不存在返回 nil）。
func (s *Service) GetByID(ctx context.Context, id int64) (*Out, error) {
	row, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	outs := s.toOuts([]SysNotice{*row})
	if err := s.fillCreateByName(ctx, outs); err != nil {
		return nil, err
	}
	return &outs[0], nil
}

// ---- 写入 ----

// Insert 新增公告，对应 insert_notice。
//
// ⚠️ notice_type 缺省时 Python 端撞 NOT NULL 约束抛 IntegrityError，
// 全局异常处理映射成 **409「数据已存在或违反完整性约束」**（不是 500），照抄。
// ⚠️ status 缺省走列默认值 '0'；remark/content 缺省为 NULL。
func (s *Service) Insert(ctx context.Context, in CreateIn) (bool, error) {
	if in.NoticeType == nil {
		return false, errs.Conflict(errs.MsgDBIntegrity)
	}
	row := SysNotice{
		NoticeTitle: *in.NoticeTitle,
		NoticeType:  *in.NoticeType,
	}
	if in.NoticeContent != nil {
		row.NoticeContent = []byte(*in.NoticeContent)
	}
	if in.Status != nil {
		row.Status = in.Status
	} else {
		status := modelx.Normal
		row.Status = &status
	}
	row.Remark = in.Remark

	crudx.FillCreate(&row.BaseModel, s.auth, nowFunc())
	if err := s.repo.Create(ctx, &row); err != nil {
		return false, errs.Wrap(err, errs.MsgDBFailed)
	}
	return row.ID != 0, nil
}

// Update 修改公告（仅更新非空字段），对应 update_notice。
func (s *Service) Update(ctx context.Context, in UpdateIn) (bool, error) {
	row, err := s.repo.Get(ctx, *in.ID)
	if err != nil {
		return false, err
	}
	if row == nil {
		return false, nil
	}
	if in.NoticeTitle != nil {
		row.NoticeTitle = *in.NoticeTitle
	}
	if in.NoticeType != nil {
		row.NoticeType = *in.NoticeType
	}
	if in.NoticeContent != nil {
		row.NoticeContent = []byte(*in.NoticeContent)
	}
	if in.Status != nil {
		row.Status = in.Status
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

// DeleteByIDs 批量物理删除，对应 delete_notice_by_ids。
func (s *Service) DeleteByIDs(ctx context.Context, ids []int64) (int64, error) {
	return s.repo.DeleteBatch(ctx, ids)
}

// ---- 内部工具 ----

// GetNoticeTypeLabel 查 sys_notice_type 字典标签，对应 get_notice_type_label。
func (s *Service) GetNoticeTypeLabel(ctx context.Context, noticeType *string) string {
	if noticeType == nil {
		return ""
	}
	m, err := s.repo.DictLabelMap(ctx, "sys_notice_type", []string{*noticeType})
	if err != nil || m == nil {
		return ""
	}
	return m[*noticeType]
}

func (s *Service) toOuts(rows []SysNotice) []Out {
	outs := make([]Out, 0, len(rows))
	for i := range rows {
		outs = append(outs, NewOut(&rows[i]))
	}
	return outs
}

// fillCreateByName 批量回填创建人账号，对应 _fill_create_by_name。
func (s *Service) fillCreateByName(ctx context.Context, outs []Out) error {
	if len(outs) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(outs))
	for i := range outs {
		if outs[i].CreateBy != nil {
			ids = append(ids, outs[i].CreateBy.Int64())
		}
	}
	if len(ids) == 0 {
		return nil
	}
	names, err := s.repo.UserNameMap(ctx, sortedIDs(ids))
	if err != nil {
		return err
	}
	for i := range outs {
		if outs[i].CreateBy != nil {
			outs[i].CreateByName = names[outs[i].CreateBy.Int64()]
		}
	}
	return nil
}
