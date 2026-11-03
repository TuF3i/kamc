package modules

import "time"

// AuditKind 审计事件类别：平台事件与集群操作分开存储/导出。
type AuditKind string

const (
	AuditKindPlatform AuditKind = "platform" // 平台事件：登录/Token/下载/管理动作
	AuditKindCluster  AuditKind = "cluster"  // 集群操作：数据面 API 请求（审计）
)

// AuditEvent 审计事件（§12.2）：一行一次关键动作。
// 平台事件（kind=platform）直接输出到 stdio；集群事件（kind=cluster）不落
// stdio，经 DB 供管理面查询，并可按配置经 syslog 导出到远程日志服务器。
type AuditEvent struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	Kind      AuditKind `gorm:"size:16;not null;default:cluster;index" json:"kind"`
	Event     string    `gorm:"size:64" json:"event"` // 事件名（login-success/kubeconfig-download/k8s-api…）
	Timestamp time.Time `gorm:"index" json:"timestamp"`
	User      string    `gorm:"size:128;index" json:"user"`
	Groups    string    `gorm:"size:1024" json:"groups"` // JSON 数组字符串
	Cluster   string    `gorm:"size:64;index" json:"cluster"`
	Method    string    `gorm:"size:8" json:"method"`
	Path      string    `gorm:"size:1024" json:"path"`
	Verb      string    `gorm:"size:32" json:"verb"`
	Allowed   bool      `json:"allowed"`
	Reason    string    `gorm:"size:128" json:"reason"` // allowed / denied-casbin / login-failed…

	// 对端与请求上下文
	SourceIP     string `gorm:"size:64;index" json:"source_ip"` // 解析后的客户端 IP
	ForwardedFor string `gorm:"size:512" json:"forwarded_for"`  // 原始 X-Forwarded-For 链
	UserAgent    string `gorm:"size:512" json:"user_agent"`
	GeoCountry   string `gorm:"size:8" json:"geo_country"`
	GeoRegion    string `gorm:"size:128" json:"geo_region"`
	GeoCity      string `gorm:"size:128" json:"geo_city"`

	// 集群操作细节（原始请求/响应，truncated）
	StatusCode   int    `json:"status_code"`
	RequestBody  string `gorm:"type:text" json:"request_body"`
	ResponseBody string `gorm:"type:text" json:"response_body"`
	LatencyMS    int64  `json:"latency_ms"`
}
