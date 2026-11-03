// Package notice 对应 sys_notice（通知公告表）。
package notice

import "walnut-seed-go/internal/pkg/modelx"

// SysNotice 通知公告表 sys_notice。
//
// ⚠️ 无 del_flag：删除是物理删除。
// ⚠️ notice_content 是 BLOB（UTF-8 字节），入参字符串 / 出参转回字符串。
type SysNotice struct {
	modelx.BaseModel

	NoticeTitle   string  `gorm:"column:notice_title;not null"`
	NoticeType    string  `gorm:"column:notice_type;not null"`
	NoticeContent []byte  `gorm:"column:notice_content"`
	Status        *string `gorm:"column:status"`
	Remark        *string `gorm:"column:remark"`
}

func (SysNotice) TableName() string { return "sys_notice" }
