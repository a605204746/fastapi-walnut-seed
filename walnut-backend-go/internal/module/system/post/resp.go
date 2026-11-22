// 本文件定义岗位域的出参 DTO，对应 Python 端 module_system/post/schema.py。
//
// ⚠️ 契约约束（与 user/resp.go 同一套，改动前先看 docs/go/01-续作交接.md §5.2）：
//   - 出参键名一律**驼峰**，字段顺序照抄 Pydantic 声明序（键序是响应字节的一部分）
//   - 出参字段**恒定出现**，无值为 null，故可空字段用指针且不加 omitempty
//   - 整数出参走 idtype.JSInt（Python 端对所有 int 施加 _encode_int）

package post

import (
	"time"

	"walnut-seed-go/internal/pkg/idtype"
	"walnut-seed-go/internal/pkg/timex"
)

// ---- 出参 ----

// Out 岗位出参，对应 PostOutSchema。
type Out struct {
	ID           *idtype.JSInt   `json:"id"`
	DeptID       *idtype.JSInt   `json:"deptId"`
	PostCode     *string         `json:"postCode"`
	PostName     *string         `json:"postName"`
	PostCategory *string         `json:"postCategory"`
	PostSort     *idtype.JSInt   `json:"postSort"`
	Status       *string         `json:"status"`
	Remark       *string         `json:"remark"`
	CreateTime   *timex.JSONTime `json:"createTime"`
	// DeptName 部门名翻译，由 service 层批量回填（不是 sys_post 的列）
	DeptName *string `json:"deptName"`
}

// NewOut 由实体构造出参，对应 PostOutSchema.model_validate(post)。
// DeptName 留空，由 service 的 fillDeptName 回填。
func NewOut(p *SysPost) Out {
	out := Out{
		ID:           jsIntPtr(p.ID),
		DeptID:       jsIntPtr(p.DeptID),
		PostCode:     strPtr(p.PostCode),
		PostName:     strPtr(p.PostName),
		PostCategory: p.PostCategory,
		PostSort:     jsIntPtr(int64(p.PostSort)),
		Status:       strPtr(p.Status),
		Remark:       p.Remark,
	}
	out.CreateTime = jsonTimeOrNil(p.CreateTime)
	return out
}

// Info 岗位精简信息，对应 PostInfoSchema（供用户等模块关联使用）。
type Info struct {
	ID           *idtype.JSInt `json:"id"`
	DeptID       *idtype.JSInt `json:"deptId"`
	PostCode     *string       `json:"postCode"`
	PostName     *string       `json:"postName"`
	PostCategory *string       `json:"postCategory"`
}

// NewInfo 由实体构造精简出参。
func NewInfo(p *SysPost) Info {
	return Info{
		ID:           jsIntPtr(p.ID),
		DeptID:       jsIntPtr(p.DeptID),
		PostCode:     strPtr(p.PostCode),
		PostName:     strPtr(p.PostName),
		PostCategory: p.PostCategory,
	}
}

// ---- 小工具（与其它模块的 resp.go 同构，便于复制）----

func jsIntPtr(v int64) *idtype.JSInt {
	j := idtype.JSInt(v)
	return &j
}

func strPtr(v string) *string {
	s := v
	return &s
}

func jsonTimeOrNil(t *time.Time) *timex.JSONTime {
	if t == nil {
		return nil
	}
	jt := timex.FromTime(*t)
	return &jt
}
