// 本文件定义通知公告域的出参 DTO 与映射辅助，对应 Python 端 module_system/notice/schema.py。
//
// ⚠️ 出参键序 = 声明序；create_by_name 是回填字段（创建人账号翻译）。

package notice

import (
	"walnut-seed-go/internal/pkg/idtype"
	"walnut-seed-go/internal/pkg/timex"
)

// ---- 出参 ----

// Out 通知公告出参，对应 NoticeOutSchema（alias_generator=to_camel）。
type Out struct {
	ID            *idtype.JSInt   `json:"id"`
	NoticeTitle   *string         `json:"noticeTitle"`
	NoticeType    *string         `json:"noticeType"`
	NoticeContent *string         `json:"noticeContent"`
	Status        *string         `json:"status"`
	Remark        *string         `json:"remark"`
	CreateBy      *idtype.JSInt   `json:"createBy"`
	CreateByName  *string         `json:"createByName"`
	CreateTime    *timex.JSONTime `json:"createTime"`
}

// NewOut 由实体构造出参；CreateByName 留空由 service 回填。
//
// notice_content 是 BLOB：Python 端 decode("utf-8", errors="replace")，
// 种子与业务写入均为合法 UTF-8，Go 直接 string(b) 等价。
func NewOut(r *SysNotice) Out {
	title := r.NoticeTitle
	ntype := r.NoticeType
	out := Out{
		ID:          jsIntPtr(r.ID),
		NoticeTitle: &title,
		NoticeType:  &ntype,
		Status:      r.Status,
		Remark:      r.Remark,
		CreateBy:    jsIntPtrOrNilPtr(r.CreateBy),
	}
	if r.NoticeContent != nil {
		s := string(r.NoticeContent)
		out.NoticeContent = &s
	}
	if r.CreateTime != nil {
		jt := timex.FromTime(*r.CreateTime)
		out.CreateTime = &jt
	}
	return out
}

// ---- 映射辅助 ----

func jsIntPtr(v int64) *idtype.JSInt {
	j := idtype.JSInt(v)
	return &j
}

func jsIntPtrOrNilPtr(v *int64) *idtype.JSInt {
	if v == nil {
		return nil
	}
	j := idtype.JSInt(*v)
	return &j
}
