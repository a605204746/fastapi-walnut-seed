package role

// 本文件定义角色域的出参 DTO，对应 Python 端 module_system/role/schema.py 的出参部分。
//
// ⚠️ 契约要点：
//   - 键名一律**驼峰**（Python 端 model_dump(by_alias=True)）
//   - **所有字段恒定出现**，无值时为 null（Pydantic 不做 exclude_none），
//     故可空字段一律用指针且不加 omitempty
//   - 整数字段走 idtype.JSInt：Python 端 jsonable_response_content 对**所有** int
//     施加 _encode_int（|v| >= 2^53-1 才转字符串），roleId 这类雪花 ID 会变成字符串，
//     而 roleSort 这类小整数必须保持数字

import (
	"walnut-seed-go/internal/pkg/idtype"
	"walnut-seed-go/internal/pkg/modelx"
	"walnut-seed-go/internal/pkg/timex"
)

// Out 角色出参，对应 RoleOutSchema。
//
// 字段顺序照抄 Pydantic 模型的声明顺序：Python 端 model_dump 保留声明序，
// JSONResponse 又按插入序输出，故键序是响应字节的一部分。
type Out struct {
	ID       *idtype.JSInt `json:"id"`
	RoleName *string       `json:"roleName"`
	RoleKey  *string       `json:"roleKey"`
	RoleSort *idtype.JSInt `json:"roleSort"`
	// DataScope 数据范围（1-6），CHAR(1) 字符串契约
	DataScope *string `json:"dataScope"`
	// MenuCheckStrictly / DeptCheckStrictly 实体列是整型 1/0，
	// 出参经 _int_to_bool 转成布尔（None 保持 None）
	MenuCheckStrictly *bool           `json:"menuCheckStrictly"`
	DeptCheckStrictly *bool           `json:"deptCheckStrictly"`
	Status            *string         `json:"status"`
	Remark            *string         `json:"remark"`
	CreateTime        *timex.JSONTime `json:"createTime"`
	// Flag 用户是否持有此角色，默认 false（authRole 接口才会置 true）
	Flag bool `json:"flag"`
	// SuperAdmin 由 ID == 1 派生，不是数据库列
	SuperAdmin bool `json:"superAdmin"`
}

// NewOut 由实体构造出参，对应 RoleOutSchema.model_validate(role)。
func NewOut(r *SysRole) Out {
	out := Out{
		ID:                jsIntPtr(r.ID),
		RoleName:          strPtr(r.RoleName),
		RoleKey:           strPtr(r.RoleKey),
		RoleSort:          jsIntPtr(int64(r.RoleSort)),
		DataScope:         r.DataScope,
		MenuCheckStrictly: intToBool(r.MenuCheckStrictly),
		DeptCheckStrictly: intToBool(r.DeptCheckStrictly),
		Status:            strPtr(r.Status),
		Remark:            r.Remark,
		Flag:              false,
		SuperAdmin:        r.ID == modelx.SuperAdminID,
	}
	if r.CreateTime != nil {
		t := timex.FromTime(*r.CreateTime)
		out.CreateTime = &t
	}
	return out
}

// intToBool 复刻 Python 端 _int_to_bool：None → None，其余 → bool(value)。
func intToBool(v *int) *bool {
	if v == nil {
		return nil
	}
	b := *v != 0
	return &b
}

func jsIntPtr(v int64) *idtype.JSInt {
	j := idtype.JSInt(v)
	return &j
}

func strPtr(v string) *string {
	s := v
	return &s
}
