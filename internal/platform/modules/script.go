package modules

import "time"

// AuthScript 登录期映射脚本（Lua，§5 扩展）：每次 SSO/LDAP 登录、
// Token 兑换时按 name 排序执行，脚本经 kamc 桥接 API 操作平台用户
// 目录与策略（自定义身份源数据到平台数据的映射）。
type AuthScript struct {
	Name      string    `gorm:"primaryKey;size:128" json:"name"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	Enabled   bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
