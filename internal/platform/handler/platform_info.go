package handler

import (
	"net/http"
	"runtime"
	"time"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/version"
)

// Settings 平台静态配置快照（装配层注入，平台信息页展示用）；
// 敏感值（数据库密码等）必须由装配层先脱敏再注入。
type Settings struct {
	ListenAddr     string // HTTP 监听地址
	ExternalURL    string // AM_EXTERNAL_URL（空 = 运行时按请求推导）
	DSN            string // 数据库连接串（密码已脱敏）
	RedisEnabled   bool   // Redis 是否启用
	RedisAddrs     string // Redis 地址列表
	RedisDB        int    // Redis 逻辑库
	AgentImageRepo string // Agent 镜像仓库完整地址（不含 tag，tag 取平台版本）
}

// PlatformInfo GET /api/v1/platform/info —— 平台信息页（管理面）：
// 平台设置 / SSO 接入 / 数据库 / 缓存 / Authentik API / Go 运行时。
// 只展示配置快照与聚合指标；密钥、明文密码一律不出现在响应中。
func (h *Handler) PlatformInfo(w http.ResponseWriter, _ *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	dbStats := h.svc.DBStats()
	uptime := int64(time.Since(h.startedAt).Seconds())
	if uptime < 0 {
		uptime = 0
	}

	s := h.settings
	externalURL := s.ExternalURL
	if externalURL == "" {
		externalURL = "（未设置，按请求推导）"
	}

	errcode.GenFinalResponse(w, map[string]any{
		"version": map[string]any{
			"version":   version.Version,
			"commit":    version.Commit,
			"component": "platform",
		},
		"settings": map[string]any{
			"listen_addr":     s.ListenAddr,
			"external_url":    externalURL,
			"admin_group":     h.adminGroup,
			"password_login":  h.passwordLoginEnabled,
			"password_user":   h.passwordUser,
			"robot_token_ttl": int64(h.robotTokenTTL.Seconds()),
			"agent_image":     h.agentImageRepo + ":" + version.Version,
		},
		"sso": map[string]any{
			"enabled":      h.oidc != nil,
			"issuer":       h.oidcIssuer,
			"client_id":    h.oidcClientID,
			"redirect_url": h.oidcRedirectURL,
		},
		"database": map[string]any{
			"dsn":                  s.DSN,
			"open_connections":     dbStats.OpenConnections,
			"in_use":               dbStats.InUse,
			"idle":                 dbStats.Idle,
			"max_open_connections": dbStats.MaxOpenConnections,
			"wait_count":           dbStats.WaitCount,
		},
		"cache": map[string]any{
			"enabled": s.RedisEnabled,
			"addrs":   s.RedisAddrs,
			"db":      s.RedisDB,
		},
		"runtime": map[string]any{
			"go_version":        runtime.Version(),
			"goroutines":        runtime.NumGoroutine(),
			"num_cpu":           runtime.NumCPU(),
			"heap_alloc_bytes":  m.HeapAlloc,
			"sys_bytes":         m.Sys,
			"total_alloc_bytes": m.TotalAlloc,
			"num_gc":            m.NumGC,
			"uptime_seconds":    uptime,
		},
	}, nil)
}
