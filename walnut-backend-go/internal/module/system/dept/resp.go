// 本文件定义部门域的出参 DTO，对应 Python 端 module_system/dept/schema.py。
//
// ⚠️ 契约约束（与 post/resp.go 同一套，改动前先看 docs/go/01-续作交接.md §5.2）：
//   - 出参键名一律**驼峰**，字段顺序照抄 Pydantic 声明序（键序是响应字节的一部分）
//   - 出参字段**恒定出现**，无值为 null，故可空字段用指针且不加 omitempty
//   - 整数出参走 idtype.JSInt（Python 端对所有 int 施加 _encode_int）

package dept

import (
	"walnut-seed-go/internal/pkg/idtype"
	"walnut-seed-go/internal/pkg/timex"
)

// ---- 出参 ----

// Out 部门出参，对应 DeptOutSchema。
//
// ⚠️ Children 恒为 []（Pydantic default_factory=list），**不能加 omitempty**：
// 列表/详情每一行都带 "children": []，与 treeselect 的手工字典形态不同。
type Out struct {
	ID           *idtype.JSInt   `json:"id"`
	ParentID     *idtype.JSInt   `json:"parentId"`
	ParentName   *string         `json:"parentName"`
	Ancestors    *string         `json:"ancestors"`
	DeptName     *string         `json:"deptName"`
	DeptCategory *string         `json:"deptCategory"`
	OrderNum     *idtype.JSInt   `json:"orderNum"`
	Leader       *idtype.JSInt   `json:"leader"`
	LeaderName   *string         `json:"leaderName"`
	Phone        *string         `json:"phone"`
	Email        *string         `json:"email"`
	Status       *string         `json:"status"`
	CreateTime   *timex.JSONTime `json:"createTime"`
	Children     []Out           `json:"children"`
}

// NewOut 由实体构造出参，对应 DeptOutSchema.model_validate(dept)。
// ParentName / LeaderName 留空，由 service 的 fillOut 回填。
func NewOut(d *SysDept) Out {
	out := Out{
		ID:           jsIntPtrOrNil(d.ID),
		ParentID:     jsIntPtrOrNilPtr(d.ParentID),
		Ancestors:    d.Ancestors,
		DeptName:     d.DeptName,
		DeptCategory: d.DeptCategory,
		OrderNum:     intPtrToJSInt(d.OrderNum),
		Leader:       jsIntPtrOrNilPtr(d.Leader),
		Phone:        d.Phone,
		Email:        d.Email,
		Status:       d.Status,
		Children:     []Out{},
	}
	if d.CreateTime != nil {
		jt := timex.FromTime(*d.CreateTime)
		out.CreateTime = &jt
	}
	return out
}

// TreeNode 部门下拉树节点，对应 build_dept_tree_select 里构造的字典。
//
// ⚠️ 键序 = 字典字面量顺序：id, parentId, label, weight, disabled，
// children 是**后加的**故排在最后，且**只在非空时出现**（omitempty）。
type TreeNode struct {
	ID       idtype.JSInt  `json:"id"`
	ParentID *idtype.JSInt `json:"parentId"`
	Label    *string       `json:"label"`
	Weight   *idtype.JSInt `json:"weight"`
	Disabled bool          `json:"disabled"`
	Children []*TreeNode   `json:"children,omitempty"`
}

// ---- 小工具（与其它模块的 resp.go 同构）----

func jsIntPtrOrNil(v int64) *idtype.JSInt {
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

func intPtrToJSInt(v *int) *idtype.JSInt {
	if v == nil {
		return nil
	}
	j := idtype.JSInt(int64(*v))
	return &j
}
