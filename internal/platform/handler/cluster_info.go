package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
	"github.com/TuF3i/kamc/internal/platform/service"
	"github.com/TuF3i/kamc/internal/version"
)

// ClusterInfo GET /api/v1/clusters/{id}/info —— 经 Agent 隧道读取目标集群
// kube-apiserver /version（集群详情「集群详细信息」卡片）。
func (h *Handler) ClusterInfo(w http.ResponseWriter, r *http.Request) {
	if h.catalog == nil {
		http.Error(w, "resource catalog disabled", http.StatusServiceUnavailable)
		return
	}
	data, rsp := h.catalog.ServerVersion(r.Context(), chi.URLParam(r, "id"))
	errcode.GenFinalResponse(w, data, rsp)
}

// PreviewAgentInstall POST /api/v1/clusters/{id}/agent-install —— 用刚签发的
// 长期 Agent Token 生成一键安装预览（kubectl 命令 + YAML）。明文 Token 只在
// 签发响应中出现且平台只存哈希，因此预览必须由管理员在签发窗口内发起。
func (h *Handler) PreviewAgentInstall(w http.ResponseWriter, r *http.Request) {
	var req modules.AgentInstallPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	data, rsp := h.svc.AgentInstallYAML(r.Context(), &service.AgentInstallParams{
		Token:       req.Token,
		PlatformURL: externalURL(r),
		ImageRepo:   h.agentImageRepo,
		ImageTag:    version.Version,
	})
	errcode.GenFinalResponse(w, data, rsp)
}

// AgentInstallYAMLForFetch GET /agent-install.yaml?token= —— kubectl apply 的
// 拉取端点：Agent 长期 Token 即拉取凭证（与清单内嵌 Token 同源，持有即授权），
// 故不入 JWT 保护组。错误按纯文本返回（kubectl 不解析 JSON 信封）。
func (h *Handler) AgentInstallYAMLForFetch(w http.ResponseWriter, r *http.Request) {
	data, rsp := h.svc.AgentInstallYAML(r.Context(), &service.AgentInstallParams{
		Token:       r.URL.Query().Get("token"),
		PlatformURL: externalURL(r),
		ImageRepo:   h.agentImageRepo,
		ImageTag:    version.Version,
	})
	if rsp != nil {
		http.Error(w, rsp.Message, http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	_, _ = w.Write([]byte(data.YAML))
}

// clusterInfoRoutes 挂载集群信息与 Agent 安装预览路由（管理面保护组内调用）。
func clusterInfoRoutes(admin chi.Router, h *Handler) {
	admin.Get("/clusters/{id}/info", h.ClusterInfo)
	admin.Post("/clusters/{id}/agent-install", h.PreviewAgentInstall)
}
