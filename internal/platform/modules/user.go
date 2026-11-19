// Package modules 用户与组：平台自维护的用户目录（SSO/LDAP 登录自动同步）。
package modules

import (
	"encoding/json"
	"strings"
	"time"
)

// jsonUnmarshal 供 ToDTO 使用（避免每个调用点重复 import encoding/json）
func jsonUnmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }

// splitGroups 逗号分隔组串 → 组名列表（去空白去空项）
func splitGroups(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// joinGroups 组名列表 → 逗号分隔串（去空白去重）
func joinGroups(groups []string) string {
	seen := make(map[string]struct{}, len(groups))
	out := make([]string, 0, len(groups))
	for _, g := range groups {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		if _, dup := seen[g]; dup {
			continue
		}
		seen[g] = struct{}{}
		out = append(out, g)
	}
	return strings.Join(out, ",")
}

// UserSource 用户来源
type UserSource string

const (
	UserSourceSSO   UserSource = "sso"   // OIDC 登录（Authentik）
	UserSourceLDAP  UserSource = "ldap"  // LDAP 直登
	UserSourceLocal UserSource = "local" // 内置管理员
)

// PlatformUser 平台用户：登录时从 SSO/LDAP 自动 upsert，metadata 供扩展。
// uid 为平台内唯一标识（uuid），username 与身份源对齐（OIDC sub / LDAP uid）。
type PlatformUser struct {
	UID       string     `gorm:"primaryKey;size:36" json:"uid"`
	Username  string     `gorm:"uniqueIndex;size:128;not null" json:"username"`
	Nickname  string     `gorm:"size:128" json:"nickname"`
	AvatarURL string     `gorm:"size:512" json:"avatar_url"`
	Groups    string     `gorm:"size:1024" json:"groups"` // 逗号分隔组名
	Source    UserSource `gorm:"size:16;not null;default:local" json:"source"`
	Email     string     `gorm:"size:256" json:"email"`
	Metadata  string     `gorm:"type:jsonb;not null;default:'{}'" json:"metadata"`
	// PasswordHash 本地密码的 bcrypt 哈希（个人中心修改密码后写入；
	// 空表示未设置——内置管理员回退到环境变量密码）。绝不外发。
	// 登录同步的 UpsertUser 冲突更新列不包含本列，避免登录冲掉密码。
	PasswordHash string     `gorm:"size:128" json:"-"`
	LastLogin    *time.Time `json:"last_login"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// UserDTO 用户对外形态（metadata 已解析为对象；不含密码哈希）
type UserDTO struct {
	UID       string         `json:"uid"`
	Username  string         `json:"username"`
	Nickname  string         `json:"nickname"`
	AvatarURL string         `json:"avatar_url"`
	Groups    []string       `json:"groups"`
	Source    UserSource     `json:"source"`
	Email     string         `json:"email"`
	Metadata  map[string]any `json:"metadata"`
	LastLogin *time.Time     `json:"last_login"`
	CreatedAt time.Time      `json:"created_at"`
	// PasswordSet 是否已设置本地密码（个人中心据此展示修改密码入口；
	// 内置管理员的“环境变量密码”由 handler 层补充标记）
	PasswordSet bool `json:"password_set"`
}

// ToDTO 转对外 DTO（groups 拆分、metadata 解析失败按空对象）
func (u *PlatformUser) ToDTO() *UserDTO {
	groups := splitGroups(u.Groups)
	meta := map[string]any{}
	if u.Metadata != "" {
		if err := jsonUnmarshal([]byte(u.Metadata), &meta); err != nil {
			meta = map[string]any{}
		}
	}
	return &UserDTO{
		UID:         u.UID,
		Username:    u.Username,
		Nickname:    u.Nickname,
		AvatarURL:   u.AvatarURL,
		Groups:      groups,
		Source:      u.Source,
		Email:       u.Email,
		Metadata:    meta,
		LastLogin:   u.LastLogin,
		CreatedAt:   u.CreatedAt,
		PasswordSet: u.PasswordHash != "",
	}
}

// PlatformGroup 平台组：成员关系存于用户表的 groups 字段，本表维护
// 组的元信息（描述/来源/是否平台管理员组），成员数查询时聚合。
type PlatformGroup struct {
	Name      string    `gorm:"primaryKey;size:128" json:"name"`
	Desc      string    `gorm:"size:512" json:"desc"`
	Source    string    `gorm:"size:16;not null;default:sso" json:"source"` // sso/ldap/local
	IsAdmin   bool      `gorm:"not null;default:false" json:"is_admin"`     // 成员登录后获得管理面权限
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GroupDTO 组对外形态
type GroupDTO struct {
	Name      string    `json:"name"`
	Desc      string    `json:"desc"`
	Source    string    `json:"source"`
	IsAdmin   bool      `json:"is_admin"`
	Members   []string  `json:"members"` // 用户名列表（聚合自用户表）
	CreatedAt time.Time `json:"created_at"`
}
