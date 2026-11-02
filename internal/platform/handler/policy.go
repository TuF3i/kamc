package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

// CreatePolicies POST /api/v1/policies —— 按主体 + 集群 + 多条资源规则批量创建。
func (h *Handler) CreatePolicies(w http.ResponseWriter, r *http.Request) {
	var req modules.CreatePolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	data, rsp := h.svc.CreatePolicies(r.Context(), &req)
	errcode.GenFinalResponse(w, data, rsp)
}

// ListPolicies GET /api/v1/policies
func (h *Handler) ListPolicies(w http.ResponseWriter, _ *http.Request) {
	data, rsp := h.svc.ListPolicies()
	errcode.GenFinalResponse(w, data, rsp)
}

// RemovePolicy DELETE /api/v1/policies（原始四元组经请求体）
func (h *Handler) RemovePolicy(w http.ResponseWriter, r *http.Request) {
	var req modules.RemovePolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	rsp := h.svc.RemovePolicy(r.Context(), req)
	errcode.GenFinalResponse(w, nil, rsp)
}

// policiesRoutes 挂载策略管理路由（管理面保护组内调用）
func policiesRoutes(api chi.Router, h *Handler) {
	api.Post("/policies", h.CreatePolicies)
	api.Get("/policies", h.ListPolicies)
	api.Delete("/policies", h.RemovePolicy)
}
