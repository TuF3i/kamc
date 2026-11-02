package modules

import "time"

// AuthScript 登录期权限绑定脚本（Lua，§5 扩展）：
// 每次登录/Token 兑换时按 name 排序执行，脚本经 kamc 桥接计算该用户的
// Casbin 绑定/策略（如按 Authentik 组绑定角色，组不存在则新建）。
type AuthScript struct {
	Name      string `gorm:"primaryKey;size:128" json:"name"`
	Content   string `gorm:"type:text;not null" json:"content"`
	Enabled   bool   `gorm:"not null;default:true" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
