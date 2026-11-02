package handler

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

// ListRoles GET /api/v1/roles
func (h *Handler) ListRoles(w http.ResponseWriter, r *http.Request) {
	data, rsp := h.svc.ListRoles(r.Context())
	errcode.GenFinalResponse(w, data, rsp)
}

// CreateRole POST /api/v1/roles
func (h *Handler) CreateRole(w http.ResponseWriter, r *http.Request) {
	var req modules.CreateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	data, rsp := h.svc.CreateRole(r.Context(), &req)
	errcode.GenFinalResponse(w, data, rsp)
}

// DeleteRole DELETE /api/v1/roles/{name}
func (h *Handler) DeleteRole(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if decoded, err := url.PathUnescape(name); err == nil {
		name = decoded
	}
	rsp := h.svc.DeleteRole(r.Context(), name)
	errcode.GenFinalResponse(w, nil, rsp)
}

// rolesRoutes 挂载角色管理路由（管理面保护组内调用）
func rolesRoutes(admin chi.Router, h *Handler) {
	admin.Get("/roles", h.ListRoles)
	admin.Post("/roles", h.CreateRole)
	admin.Delete("/roles/{name}", h.DeleteRole)
}
