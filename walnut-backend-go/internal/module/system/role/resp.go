package role

import (
	"walnut-seed-go/internal/module/system/dept"
	"walnut-seed-go/internal/pkg/idtype"
	"walnut-seed-go/internal/pkg/modelx"
	"walnut-seed-go/internal/pkg/timex"
)

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

func toOuts(rows []SysRole) []Out {
	outs := make([]Out, 0, len(rows))
	for i := range rows {
		outs = append(outs, NewOut(&rows[i]))
	}
	return outs
}

// ---- AuthUser 出参 ----

type AuthUserOut struct {
	ID          *idtype.JSInt   `json:"id"`
	DeptID      *idtype.JSInt   `json:"deptId"`
	UserName    *string         `json:"userName"`
	NickName    *string         `json:"nickName"`
	Email       *string         `json:"email"`
	PhoneNumber *string         `json:"phonenumber"`
	Status      *string         `json:"status"`
	CreateTime  *timex.JSONTime `json:"createTime"`
	DeptName    *string         `json:"deptName"`
}

type AuthUserPageOut struct {
	Rows  []AuthUserOut `json:"rows"`
	Total idtype.JSInt  `json:"total"`
}

// ---- DeptTree 出参 ----

type DeptTreeOut struct {
	CheckedKeys []idtype.JSInt   `json:"checkedKeys"`
	Depts       []*dept.TreeNode `json:"depts"`
}

// ---- 导出 ----

const sheetName = "角色数据"

var dataScopeLabels = map[string]string{
	"1": "全部数据权限",
	"2": "自定义数据权限",
	"3": "本部门数据权限",
	"4": "本部门及以下数据权限",
	"5": "仅本人数据权限",
	"6": "部门及以下或本人数据权限",
}

var statusLabels = map[string]string{
	"0": "正常",
	"1": "停用",
}

func ExportColumns() []string {
	return []string{
		"id", "role_name", "role_key", "role_sort", "data_scope",
		"menu_check_strictly", "dept_check_strictly", "status", "remark", "create_time",
	}
}

func ExportHeaders() []string {
	return []string{
		"角色序号", "角色名称", "角色权限", "角色排序", "数据范围",
		"菜单树选择项是否关联显示", "部门树选择项是否关联显示", "角色状态", "备注", "创建时间",
	}
}

func ExportRow(r SysRole) map[string]any {
	m := map[string]any{
		"id":                  r.ID,
		"role_name":           r.RoleName,
		"role_key":            r.RoleKey,
		"role_sort":           r.RoleSort,
		"data_scope":          "",
		"menu_check_strictly": boolOrNone(r.MenuCheckStrictly),
		"dept_check_strictly": boolOrNone(r.DeptCheckStrictly),
		"status":              "",
		"remark":              r.Remark,
		"create_time":         "",
	}
	// 字典未命中保留原值（Python dict.get(value, value)），None → ""
	if r.DataScope != nil {
		if label, ok := dataScopeLabels[*r.DataScope]; ok {
			m["data_scope"] = label
		} else {
			m["data_scope"] = *r.DataScope
		}
	}
	if label, ok := statusLabels[r.Status]; ok {
		m["status"] = label
	} else {
		m["status"] = r.Status
	}
	if r.CreateTime != nil {
		m["create_time"] = r.CreateTime.Format("2006-01-02 15:04:05")
	}
	return m
}

func boolOrNone(v *int) any {
	if v == nil {
		return nil
	}
	return *v != 0
}

// ---- 基础辅助 ----

func intToBool(v *int) *bool {
	if v == nil {
		return nil
	}
	b := *v != 0
	return &b
}

func boolToInt(v *bool) *int {
	if v == nil {
		return nil
	}
	n := 0
	if *v {
		n = 1
	}
	return &n
}

func jsIntPtr(v int64) *idtype.JSInt {
	j := idtype.JSInt(v)
	return &j
}

func strPtr(v string) *string {
	s := v
	return &s
}
