package user

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"walnut-seed-go/internal/module/system/role"
	"walnut-seed-go/internal/pkg/authctx"
)

// Service 用户域业务层，对应 Python 端 module_system/user/service.py。
//
// 与 Python 端一样持有**请求级**的认证上下文：数据权限、getInfo 的
// permissions/roles 都从它取，而不是每处重新查库。
type Service struct {
	repo *Repo
	auth authctx.Context
}

// NewService 构造业务层。ac 由认证中间件写入请求后经 handler 传入。
func NewService(db *gorm.DB, ac authctx.Context) *Service {
	return &Service{repo: NewRepo(db), auth: ac}
}

// GetInfo 对应 UserService.get_info：GET /system/user/getInfo。
//
// 语义要点（照抄 Python 端）：
//   - **忽略数据权限**：查的是当前登录用户自己，不需要行级权限过滤
//   - permissions / roles 直接取自会话快照，不查库
//   - 用户不存在时返回 (nil, nil)，由 handler 转成"没有权限访问用户数据!"
func (s *Service) GetInfo(ctx context.Context) (*GetInfoOut, error) {
	u, err := s.repo.FindByID(ctx, s.auth.User.ID)
	if err != nil {
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}
	if u == nil {
		return nil, nil
	}

	out := NewOut(u)

	roles, err := s.repo.FindRoles(ctx, u.ID)
	if err != nil {
		return nil, fmt.Errorf("查询用户角色失败: %w", err)
	}
	// 恒为非 nil 切片：无角色时输出 [] 而非 null（Python 端列表推导的结果就是 []）
	roleOuts := make([]role.Out, 0, len(roles))
	for i := range roles {
		roleOuts = append(roleOuts, role.NewOut(&roles[i]))
	}
	out.Roles = roleOuts

	if u.DeptID != nil {
		names, err := s.repo.MapDeptNames(ctx, []int64{*u.DeptID})
		if err != nil {
			return nil, fmt.Errorf("查询部门名失败: %w", err)
		}
		// 键缺失（部门已删除）与值为 nil（部门名为 NULL）都落到 DeptName=nil → null
		out.DeptName = names[*u.DeptID]
	}

	return &GetInfoOut{
		User:        &out,
		Permissions: s.auth.Permissions,
		Roles:       s.auth.Roles,
	}, nil
}
