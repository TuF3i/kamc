package modules

import "time"

// PolicyRule Casbin 策略行（适配器与 DB 之间的存储形态）：
// ptype 固定为 "p"，v0..v3 对应 sub/dom/obj/act；v4/v5 保留。
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

// PolicyRuleInput 创建策略时的一条资源规则。
type PolicyRuleInput struct {
	Namespace   string   `json:"namespace"` // "*" 或具体 namespace；cluster-scoped 资源留空
	Resource    string   `json:"resource"`  // 资源名，或 "*"
	Subresource string   `json:"subresource,omitempty"`
	Namespaced  bool     `json:"namespaced"`
	Verbs       []string `json:"verbs"` // 标准 K8s verb，或 "*"
}

// CreatePolicyRequest POST /api/v1/policies
type CreatePolicyRequest struct {
	SubjectType string            `json:"subject_type"` // org / user / robot
	Subject     string            `json:"subject"`      // 组名、用户名或机器人 ID
	Cluster     string            `json:"cluster"`
	Rules       []PolicyRuleInput `json:"rules"`
}

// RemovePolicyRequest DELETE /api/v1/policies（原始四元组精确删除）
type RemovePolicyRequest struct {
	Sub string `json:"sub"`
	Dom string `json:"dom"`
	Obj string `json:"obj"`
	Act string `json:"act"`
}

// PolicyDTO 策略列表项：原始四元组 + 便于 UI 展示的解析字段。
type PolicyDTO struct {
	SubjectType string   `json:"subject_type"` // org / user / robot / legacy
	Subject     string   `json:"subject"`
	Cluster     string   `json:"cluster"`
	Namespace   string   `json:"namespace"`
	Resource    string   `json:"resource"`
	Subresource string   `json:"subresource,omitempty"`
	Verbs       []string `json:"verbs"`
	Sub         string   `json:"sub"`
	Dom         string   `json:"dom"`
	Obj         string   `json:"obj"`
	Act         string   `json:"act"`
}

// CreatePolicyResult POST /api/v1/policies 响应。
type CreatePolicyResult struct {
	Created  int         `json:"created"`
	Policies []PolicyDTO `json:"policies"`
}
