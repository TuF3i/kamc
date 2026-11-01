package modules

import "time"

// AuditEvent 审计事件（§12.2）：一次数据面/管理面关键动作一行。
type AuditEvent struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	Timestamp time.Time `gorm:"index" json:"timestamp"`
	User      string    `gorm:"size:128;index" json:"user"`
	Groups    string    `gorm:"size:512" json:"groups"` // JSON 数组字符串
	Cluster   string    `gorm:"size:64;index" json:"cluster"`
	Method    string    `gorm:"size:8" json:"method"`
	Path      string    `gorm:"size:512" json:"path"`
	Verb      string    `gorm:"size:32" json:"verb"`
	Allowed   bool      `json:"allowed"`
	Reason    string    `gorm:"size:64" json:"reason"` // allowed / denied-casbin / attached / revoked / login...
	SourceIP  string    `gorm:"size:64" json:"source_ip"`
	LatencyMS int64     `json:"latency_ms"`
}
