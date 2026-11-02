package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/TuF3i/kamc/internal/platform/errcode"
)

// ListClusterResources GET /api/v1/clusters/{id}/resources —— 读取目标集群
// 的 Kubernetes discovery（GVR + 可用动词）与 namespace 列表。
func (h *Handler) ListClusterResources(w http.ResponseWriter, r *http.Request) {
	if h.catalog == nil {
		http.Error(w, "resource catalog disabled", http.StatusServiceUnavailable)
		return
	}
	refresh := r.URL.Query().Get("refresh") == "1"
	data, rsp := h.catalog.List(r.Context(), chi.URLParam(r, "id"), refresh)
	errcode.GenFinalResponse(w, data, rsp)
}

// clusterResourcesRoutes 挂载集群资源目录路由（管理面保护组内调用）。
func clusterResourcesRoutes(admin chi.Router, h *Handler) {
	admin.Get("/clusters/{id}/resources", h.ListClusterResources)
}
