package modules

import "time"

// TokenKind Agent Token 类型（设计文档 §6.1）
type TokenKind string

const (
	// TokenKindRegister 注册 Token：短期一次性，用于 Agent 首次接入换取长期 Token
	// （换取流程属路线图；当前阶段两类 Token 均可直接建立隧道）
	TokenKindRegister TokenKind = "register"
	// TokenKindLongterm 长期 Token：日常隧道认证，可轮换带宽限期
	TokenKindLongterm TokenKind = "longterm"
)

// TokenKindValid 校验 Token 类型合法性
func TokenKindValid(k TokenKind) bool {
	return k == TokenKindRegister || k == TokenKindLongterm
}

// Token 明文前缀：类型可从 Token 字面量直接辨识，便于日志与人工排查
const (
	TokenPrefixRegister = "am-reg-"
	TokenPrefixLongterm = "am-clu-"
)

// ClusterToken 集群 Agent Token：只存 SHA256 哈希，明文仅在签发响应中出现一次。
// 同一集群每类 Token 一行（复合主键）；轮换时 Current → Previous 进入宽限期。
type ClusterToken struct {
	ClusterID string `gorm:"primaryKey;size:64"` // 所属集群
	Kind      string `gorm:"primaryKey;size:16"` // register / longterm

	CurrentHash  string    `gorm:"size:64;not null"` // 当前生效 Token 的 SHA256
	PreviousHash string    `gorm:"size:64"`          // 轮换前的 Token，宽限期内仍可用
	GraceEndsAt  time.Time `gorm:""`                 // 宽限期截止；零值表示无宽限
	ExpiresAt    time.Time `gorm:""`                 // 过期时间；零值表示长期有效（注册 Token 为签发+1h）

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Status 对外状态：rotating = 旧 Token 仍在宽限期内
func (t *ClusterToken) Status() string {
	if t.PreviousHash != "" && time.Now().Before(t.GraceEndsAt) {
		return "rotating"
	}
	return "active"
}

// IssueTokenRequest POST /api/v1/clusters/{id}/tokens 请求体
type IssueTokenRequest struct {
	Kind TokenKind `json:"kind"`
}

// IssuedToken 签发响应：明文 Token 仅出现在这里一次，平台侧不落盘
type IssuedToken struct {
	Token       string     `json:"token"`
	Kind        TokenKind  `json:"kind"`
	GraceEndsAt *time.Time `json:"grace_ends_at,omitempty"` // 轮换时旧 Token 的宽限截止
}

// TokenDTO 对外输出的 Token 元数据（永不包含哈希与明文）
type TokenDTO struct {
	ClusterID string     `json:"cluster_id"`
	Kind      TokenKind  `json:"kind"`
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// ToDTO 转对外 DTO
func (t *ClusterToken) ToDTO() *TokenDTO {
	var expires *time.Time
	if !t.ExpiresAt.IsZero() {
		e := t.ExpiresAt
		expires = &e
	}
	return &TokenDTO{
		ClusterID: t.ClusterID,
		Kind:      TokenKind(t.Kind),
		Status:    t.Status(),
		ExpiresAt: expires,
		CreatedAt: t.CreatedAt,
	}
}
