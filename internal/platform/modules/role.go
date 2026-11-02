package modules

import "time"

// Role 角色定义（Casbin 中的角色名是策略/绑定的字符串主体；
// 本表用于让管理员显式创建和管理角色，便于"先建角色、再分配用户与策略"）。
type Role struct {
	Name      string    `gorm:"primaryKey;size:256" json:"name"`
	Desc      string    `gorm:"size:512" json:"desc"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RoleDTO 角色列表项：附带策略数/绑定数，managed 表示是否在 roles 表显式创建。
type RoleDTO struct {
	Name         string     `json:"name"`
	Desc         string     `json:"desc"`
	PolicyCount  int        `json:"policy_count"`
	BindingCount int        `json:"binding_count"`
	Managed      bool       `json:"managed"`
	CreatedAt    *time.Time `json:"created_at,omitempty"`
}

// CreateRoleRequest POST /api/v1/roles
type CreateRoleRequest struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
}
