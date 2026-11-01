package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

// AddPolicy POST /api/v1/policies
func (h *Handler) AddPolicy(w http.ResponseWriter, r *http.Request) {
	var req modules.AddPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	rsp := h.svc.AddPolicy(r.Context(), req.Sub, req.Dom, req.Obj, req.Act)
	errcode.GenFinalResponse(w, nil, rsp)
}

// ListPolicies GET /api/v1/policies
func (h *Handler) ListPolicies(w http.ResponseWriter, _ *http.Request) {
	data, rsp := h.svc.ListPolicies()
	errcode.GenFinalResponse(w, data, rsp)
}

// RemovePolicy DELETE /api/v1/policies（字段经请求体）
func (h *Handler) RemovePolicy(w http.ResponseWriter, r *http.Request) {
	var req modules.AddPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	rsp := h.svc.RemovePolicy(r.Context(), req.Sub, req.Dom, req.Obj, req.Act)
	errcode.GenFinalResponse(w, nil, rsp)
}

// AddBinding POST /api/v1/bindings
func (h *Handler) AddBinding(w http.ResponseWriter, r *http.Request) {
	var req modules.AddBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	rsp := h.svc.AddBinding(r.Context(), req.User, req.Role, req.Dom)
	errcode.GenFinalResponse(w, nil, rsp)
}

// ListBindings GET /api/v1/bindings
func (h *Handler) ListBindings(w http.ResponseWriter, _ *http.Request) {
	data, rsp := h.svc.ListBindings()
	errcode.GenFinalResponse(w, data, rsp)
}

// RemoveBinding DELETE /api/v1/bindings（字段经请求体）
func (h *Handler) RemoveBinding(w http.ResponseWriter, r *http.Request) {
	var req modules.AddBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	rsp := h.svc.RemoveBinding(r.Context(), req.User, req.Role, req.Dom)
	errcode.GenFinalResponse(w, nil, rsp)
}

// policiesRoutes 挂载策略/绑定管理路由（管理面保护组内调用）
func policiesRoutes(api chi.Router, h *Handler) {
	api.Post("/policies", h.AddPolicy)
	api.Get("/policies", h.ListPolicies)
	api.Delete("/policies", h.RemovePolicy)
	api.Post("/bindings", h.AddBinding)
	api.Get("/bindings", h.ListBindings)
	api.Delete("/bindings", h.RemoveBinding)
}
