// Package metrics 平台 Prometheus 指标（§12.3）。
// 全部注册到 prometheus.DefaultRegisterer（/metrics 由 promhttp 暴露）。
// 位置在 internal/metrics（platform 与 tunnel 两模块共用，叶包语义）。
//
// 指标分组：
//   - HTTP：请求总数/耗时/并发/响应字节（middleware 统一采集，route 为 chi 模式）
//   - 认证：登录尝试（按方式与结果）、JWT 校验失败原因
//   - 鉴权：Casbin allow/deny/error 与判定耗时
//   - 数据面：代理请求、耗时、拒绝、限流
//   - 隧道：在线数、attach/detach、peer 转发
//   - 审计：写入/落盘/丢弃/回放、队列深度、写入耗时
//   - 头像：上传/读取/删除（按后端与结果）
//   - 构建与就绪：BuildInfo、Ready
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// RequestsTotal 数据面请求计数
	RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "kamc_requests_total", Help: "数据面代理请求数"},
		[]string{"cluster", "verb", "code"},
	)
	// CasbinDenialsTotal 鉴权拒绝计数
	CasbinDenialsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "kamc_casbin_denials_total", Help: "Casbin 鉴权拒绝数"},
		[]string{"cluster"},
	)
	// RateLimitedTotal 限流计数
	RateLimitedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{Name: "kamc_rate_limited_total", Help: "限流触发次数"},
	)
	// ProxyDurationSeconds 数据面请求耗时分布
	ProxyDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kamc_proxy_duration_seconds",
			Help:    "数据面代理耗时",
			Buckets: prometheus.ExponentialBuckets(0.005, 2, 12), // 5ms ~ 10s
		},
		[]string{"cluster"},
	)
	// TunnelConnectedClusters 在线集群数（持有至少一条本机隧道的集群）
	TunnelConnectedClusters = prometheus.NewGauge(
		prometheus.GaugeOpts{Name: "kamc_tunnel_connected_clusters", Help: "本副本持有隧道的集群数"},
	)
	// TunnelAttachedAgents 本副本 Agent 隧道连接数
	TunnelAttachedAgents = prometheus.NewGauge(
		prometheus.GaugeOpts{Name: "kamc_tunnel_attached_agents", Help: "本副本 Agent 隧道连接数"},
	)
	// PeerRelayTotal 跨副本转发计数
	PeerRelayTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "kamc_peer_relay_total", Help: "跨副本 RELAY 转发次数"},
		[]string{"owner"},
	)
	// AuditDroppedTotal 审计丢弃计数
	AuditDroppedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{Name: "kamc_audit_dropped_total", Help: "审计事件丢弃数（缓冲满/spill 失败）"},
	)
	// RevocationsTotal 吊销操作计数
	RevocationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "kamc_revocations_total", Help: "Token 吊销操作数"},
		[]string{"kind"},
	)

	// HTTPRequestsTotal HTTP 请求总数（route 为 chi 路由模式，避免原始路径高基数）
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "kamc_http_requests_total", Help: "平台 HTTP 请求数"},
		[]string{"method", "route", "status"},
	)
	// HTTPRequestDurationSeconds HTTP 请求耗时
	HTTPRequestDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kamc_http_request_duration_seconds",
			Help:    "平台 HTTP 请求耗时",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 14),
		},
		[]string{"method", "route"},
	)
	// HTTPRequestsInFlight 当前处理中的 HTTP 请求数
	HTTPRequestsInFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{Name: "kamc_http_requests_in_flight", Help: "当前处理中的 HTTP 请求数"},
	)
	// HTTPResponseBytesTotal HTTP 响应字节数
	HTTPResponseBytesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "kamc_http_response_bytes_total", Help: "平台 HTTP 响应字节数"},
		[]string{"method", "route"},
	)
	// AuthAttemptsTotal 登录尝试（method: password/oidc/ldap/token-exchange/dev-token）
	AuthAttemptsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "kamc_auth_attempts_total", Help: "登录尝试数"},
		[]string{"method", "result"},
	)
	// JWTAuthFailuresTotal JWT 中间件校验失败（reason: missing/invalid）
	JWTAuthFailuresTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "kamc_jwt_auth_failures_total", Help: "JWT 校验失败数"},
		[]string{"reason"},
	)
	// CasbinDecisionsTotal Casbin 判定结果（result: allow/deny/error）
	CasbinDecisionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "kamc_casbin_decisions_total", Help: "Casbin 判定次数"},
		[]string{"cluster", "result"},
	)
	// CasbinDecisionDurationSeconds Casbin 判定耗时
	CasbinDecisionDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kamc_casbin_decision_duration_seconds",
			Help:    "Casbin 判定耗时",
			Buckets: prometheus.ExponentialBuckets(0.0005, 2, 12),
		},
		[]string{"cluster"},
	)
	// TunnelAttachTotal Agent 隧道 attach 次数
	TunnelAttachTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "kamc_tunnel_attach_total", Help: "Agent 隧道 attach 次数"},
		[]string{"cluster"},
	)
	// TunnelDetachTotal Agent 隧道 detach 次数
	TunnelDetachTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "kamc_tunnel_detach_total", Help: "Agent 隧道 detach 次数"},
		[]string{"cluster"},
	)
	// AuditEventsTotal 审计事件处理结果（kind: platform/cluster/replay，result: written/spill/dropped/replayed）
	AuditEventsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "kamc_audit_events_total", Help: "审计事件处理数"},
		[]string{"kind", "result"},
	)
	// AuditQueueDepth 审计内存队列深度
	AuditQueueDepth = prometheus.NewGauge(
		prometheus.GaugeOpts{Name: "kamc_audit_queue_depth", Help: "审计内存队列深度"},
	)
	// AuditWriteDurationSeconds 审计批量写 PG 耗时
	AuditWriteDurationSeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "kamc_audit_write_duration_seconds",
			Help:    "审计批量写入耗时",
			Buckets: prometheus.ExponentialBuckets(0.005, 2, 10),
		},
	)
	// AvatarOperationsTotal 头像操作（operation: upload/get/delete，result: ok/not_found/error）
	AvatarOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "kamc_avatar_operations_total", Help: "头像存储操作数"},
		[]string{"backend", "operation", "result"},
	)
	// BuildInfo 构建信息（恒为 1）
	BuildInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "kamc_build_info", Help: "构建版本信息"},
		[]string{"version", "commit"},
	)
	// Ready 平台就绪状态（数据库可连通为 1）
	Ready = prometheus.NewGauge(
		prometheus.GaugeOpts{Name: "kamc_ready", Help: "平台就绪状态（数据库连通）"},
	)
)

func init() {
	prometheus.MustRegister(
		RequestsTotal,
		CasbinDenialsTotal,
		RateLimitedTotal,
		ProxyDurationSeconds,
		TunnelConnectedClusters,
		TunnelAttachedAgents,
		PeerRelayTotal,
		AuditDroppedTotal,
		RevocationsTotal,
		HTTPRequestsTotal,
		HTTPRequestDurationSeconds,
		HTTPRequestsInFlight,
		HTTPResponseBytesTotal,
		AuthAttemptsTotal,
		JWTAuthFailuresTotal,
		CasbinDecisionsTotal,
		CasbinDecisionDurationSeconds,
		TunnelAttachTotal,
		TunnelDetachTotal,
		AuditEventsTotal,
		AuditQueueDepth,
		AuditWriteDurationSeconds,
		AvatarOperationsTotal,
		BuildInfo,
		Ready,
	)
}
