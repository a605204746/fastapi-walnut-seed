package user

// 本文件定义用户域的出参 DTO，对应 Python 端 module_system/user/schema.py 的出参部分。
//
// ⚠️ 三条契约约束（改之前先看 docs/go/01-续作交接.md §5.2）：
//   - 键名一律**驼峰**（Python 端 model_dump(by_alias=True)）
//   - **所有字段恒定出现**，无值时为 null（Pydantic 不做 exclude_none），
//     故可空字段一律用指针且不加 omitempty
//   - 整数字段走 idtype.JSInt：Python 端 jsonable_response_content 对**所有** int
//     施加 _encode_int（|v| >= 2^53-1 才转字符串）。id/deptId 这类雪花 ID 会变字符串，
//     而 sex/status 是 CHAR(1) 字符串、total 是小整数——都不受影响
//   - 出参**不含 password**

import (
	"time"

	"walnut-seed-go/internal/module/system/role"
	"walnut-seed-go/internal/pkg/idtype"
	"walnut-seed-go/internal/pkg/timex"
)

// Out 用户出参，对应 UserOutSchema。
//
// 字段顺序照抄 Pydantic 模型的声明顺序：Python 端 model_dump 保留声明序、
// JSONResponse 按插入序输出，故键序是响应字节的一部分。
type Out struct {
	ID          *idtype.JSInt `json:"id"`
	DeptID      *idtype.JSInt `json:"deptId"`
	UserName    *string       `json:"userName"`
	NickName    *string       `json:"nickName"`
	UserType    *string       `json:"userType"`
	Email       *string       `json:"email"`
	Phonenumber *string       `json:"phonenumber"`
	// Sex 用户性别（0男 1女 2未知），CHAR(1) 字符串契约——**不是**整数
	Sex    *string `json:"sex"`
	Avatar *string `json:"avatar"`
	// Status 帐号状态（0正常 1停用），同样是字符串契约
	Status    *string         `json:"status"`
	LoginIP   *string         `json:"loginIp"`
	LoginDate *timex.JSONTime `json:"loginDate"`
	Remark    *string         `json:"remark"`

	CreateTime *timex.JSONTime `json:"createTime"`
	// DeptName 部门名，由 MapDeptNames 回填（不是 sys_user 的列）
	DeptName *string `json:"deptName"`
	// Roles 角色对象列表；列表类接口会显式置 nil（输出 null）以省流量
	Roles []role.Out `json:"roles"`
	// RoleIDs / PostIDs / RoleID 仅 GET /user 与 /user/{userId} 填充，
	// getInfo 恒为 null——但字段必须存在
	RoleIDs []idtype.JSInt `json:"roleIds"`
	PostIDs []idtype.JSInt `json:"postIds"`
	RoleID  *idtype.JSInt  `json:"roleId"`
}

// NewOut 由实体构造出参，对应 UserOutSchema.model_validate(user)。
//
// ⚠️ 只填实体自带的列。DeptName / Roles / RoleIDs / PostIDs / RoleID
// 是 service 层另行回填的派生字段，此处一律留零值（nil → 输出 null）。
func NewOut(u *SysUser) Out {
	out := Out{
		ID:          jsIntPtr(u.ID),
		DeptID:      jsIntPtrOrNil(u.DeptID),
		UserName:    strPtr(u.UserName),
		NickName:    strPtr(u.NickName),
		UserType:    strPtr(u.UserType),
		Email:       u.Email,
		Phonenumber: u.Phonenumber,
		Sex:         u.Sex,
		Avatar:      u.Avatar,
		Status:      strPtr(u.Status),
		LoginIP:     u.LoginIP,
		Remark:      u.Remark,
	}
	out.LoginDate = jsonTimeOrNil(u.LoginDate)
	out.CreateTime = jsonTimeOrNil(u.CreateTime)
	return out
}

// GetInfoOut 是 GET /system/user/getInfo 的 data，对应 GetInfoOutSchema。
type GetInfoOut struct {
	User *Out `json:"user"`
	// Permissions 菜单权限标识串（超管为 ["*:*:*"]），取自会话快照
	Permissions []string `json:"permissions"`
	// Roles 角色权限字符串（超管为 ["superadmin"]）。
	// ⚠️ 与 User.Roles（角色**对象**列表）同名不同义，这个不一致照抄 Python 端
	Roles []string `json:"roles"`
}

func jsIntPtr(v int64) *idtype.JSInt {
	j := idtype.JSInt(v)
	return &j
}

func jsIntPtrOrNil(v *int64) *idtype.JSInt {
	if v == nil {
		return nil
	}
	return jsIntPtr(*v)
}

// strPtr 用于 NOT NULL 列：值恒存在，故恒返回非 nil 指针。
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
