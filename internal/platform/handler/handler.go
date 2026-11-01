// Package handler 平台 HTTP 参数绑定与错误映射：业务一律下沉 service，
// 本包不做任何业务判断；响应统一走 errcode.GenFinalResponse（模式 B 出口）。
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
	"github.com/TuF3i/kamc/internal/platform/proxy"
	"github.com/TuF3i/kamc/internal/platform/service"
	"github.com/TuF3i/kamc/internal/tunnel"
	"github.com/TuF3i/kamc/internal/version"
)

// Handler 平台 HTTP 处理器（业务下沉 Service；隧道/代理依赖可空，对应降级行为）
type Handler struct {
	svc     *service.Service
	tunnels *tunnel.Service // 隧道模块（nil = 未启用 → /tunnel 返回 503）
	proxies *proxy.Pool     // API 代理池（nil = 未装配 → /proxy 返回 501）
}

// NewHandler 构造 Handler
func NewHandler(svc *service.Service, tunnels *tunnel.Service, proxies *proxy.Pool) *Handler {
	return &Handler{svc: svc, tunnels: tunnels, proxies: proxies}
}

// RegisterRoutes 挂载全部路由（健康检查 + /api/v1 业务 API + /tunnel + /proxy）。
// 认证/鉴权中间件（JWT、Casbin）按设计文档 §4/§5 属后续路线图，届时在
// /api/v1 与 /proxy 分组上挂载；健康检查与 /tunnel（自带 Token 认证）保持现状。
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/healthz", h.Healthz) // 存活：不碰任何依赖
	r.Get("/readyz", h.Readyz)   // 就绪：数据库连通（下线摘流依据）
	r.Get("/version", h.Version)
	r.Get("/tunnel", h.TunnelWS) // Agent 隧道入口（WS 升级 + Token 认证）

	r.Route("/api/v1", func(api chi.Router) {
		// Agent 注册交换：注册 Token 即凭证，不入 JWT 保护组（后续里程碑）
		api.Post("/agent/register", h.AgentRegister)

		api.Post("/clusters", h.CreateCluster)
		api.Get("/clusters", h.ListClusters)
		api.Get("/clusters/{id}", h.GetCluster)
		api.Delete("/clusters/{id}", h.DeleteCluster)
		api.Post("/clusters/{id}/tokens", h.IssueToken)
		api.Get("/clusters/{id}/tokens", h.ListTokens)
		api.Delete("/clusters/{id}/tokens/{kind}", h.RevokeToken)
	})

	// /proxy/{clusterID}/... —— kubeconfig 的 server 路径前缀（FIX-6）
	r.HandleFunc("/proxy/{clusterID}", h.Proxy)
	r.HandleFunc("/proxy/{clusterID}/*", h.Proxy)
}

// Healthz 存活探针：进程活着即返回 200，不做依赖检查
func (h *Handler) Healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// Readyz 就绪探针：数据库不可达返回 503（LB/编排据此摘流，§7.5 优雅下线第一步）
func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Ping(r.Context()); err != nil {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

// Version 构建版本信息
func (h *Handler) Version(w http.ResponseWriter, _ *http.Request) {
	errcode.GenFinalResponse(w, map[string]string{
		"version":   version.Version,
		"commit":    version.Commit,
		"component": "platform",
	}, nil)
}

// CreateCluster POST /api/v1/clusters
func (h *Handler) CreateCluster(w http.ResponseWriter, r *http.Request) {
	var req modules.CreateClusterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	data, rsp := h.svc.CreateCluster(r.Context(), &req)
	errcode.GenFinalResponse(w, data, rsp)
}

// ListClusters GET /api/v1/clusters
func (h *Handler) ListClusters(w http.ResponseWriter, r *http.Request) {
	data, rsp := h.svc.ListClusters(r.Context())
	errcode.GenFinalResponse(w, data, rsp)
}

// GetCluster GET /api/v1/clusters/{id}
func (h *Handler) GetCluster(w http.ResponseWriter, r *http.Request) {
	data, rsp := h.svc.GetCluster(r.Context(), chi.URLParam(r, "id"))
	errcode.GenFinalResponse(w, data, rsp)
}

// DeleteCluster DELETE /api/v1/clusters/{id}
func (h *Handler) DeleteCluster(w http.ResponseWriter, r *http.Request) {
	rsp := h.svc.DeleteCluster(r.Context(), chi.URLParam(r, "id"))
	errcode.GenFinalResponse(w, nil, rsp)
}
