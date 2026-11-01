// Package metrics 平台 Prometheus 指标（§12.3）。
// 全部注册到 prometheus.DefaultRegisterer（/metrics 由 promhttp 暴露）。
// 位置在 internal/metrics（platform 与 tunnel 两模块共用，叶包语义）。
//
// 范围说明：仅平台侧指标；Agent 无 HTTP 端口，其侧指标（重连次数等）
// 属后续（可用 pushgateway 或 agent 本地端点）。
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
	)
}
