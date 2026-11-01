package modules

import "time"

// PolicyRule Casbin 策略行（适配器与 DB 之间的存储形态）：
// ptype 为 "p"（策略）或 "g"（角色绑定），v0..v5 对应规则字段。
type PolicyRule struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Ptype     string    `gorm:"size:8;not null;index" json:"ptype"`
	V0        string    `gorm:"size:128;not null" json:"v0"`
	V1        string    `gorm:"size:128;not null;default:''" json:"v1"`
	V2        string    `gorm:"size:256;not null;default:''" json:"v2"`
	V3        string    `gorm:"size:256;not null;default:''" json:"v3"`
	V4        string    `gorm:"size:256;not null;default:''" json:"v4"`
	V5        string    `gorm:"size:256;not null;default:''" json:"v5"`
	CreatedAt time.Time `json:"created_at"`
}

// AddPolicyRequest POST /api/v1/policies 请求体（p, sub, dom, obj, act）
type AddPolicyRequest struct {
	Sub string `json:"sub"`
	Dom string `json:"dom"`
	Obj string `json:"obj"`
	Act string `json:"act"` // 正则片段，如 get|list|watch（匹配时锚定，FIX-8）
}

// AddBindingRequest POST /api/v1/bindings 请求体（g, user, role, dom）
type AddBindingRequest struct {
	User string `json:"user"`
	Role string `json:"role"` // 如 role:developer
	Dom  string `json:"dom"`  // 集群 ID
}
