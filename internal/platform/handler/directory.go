package handler

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/TuF3i/kamc/internal/platform/errcode"
)

// DirectoryOrgs GET /api/v1/directory/orgs?q= —— Authentik groups（组织）。
func (h *Handler) DirectoryOrgs(w http.ResponseWriter, r *http.Request) {
	if h.directory == nil {
		http.Error(w, "authentik directory not configured", http.StatusServiceUnavailable)
		return
	}
	options, err := h.directory.ListOrgs(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		log.Printf("[platform] directory orgs: %v", err)
		http.Error(w, "authentik unavailable", http.StatusBadGateway)
		return
	}
	errcode.GenFinalResponse(w, options, nil)
}

// DirectoryUsers GET /api/v1/directory/users?q= —— Authentik users（用户）。
func (h *Handler) DirectoryUsers(w http.ResponseWriter, r *http.Request) {
	if h.directory == nil {
		http.Error(w, "authentik directory not configured", http.StatusServiceUnavailable)
		return
	}
	options, err := h.directory.ListUsers(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		log.Printf("[platform] directory users: %v", err)
		http.Error(w, "authentik unavailable", http.StatusBadGateway)
		return
	}
	errcode.GenFinalResponse(w, options, nil)
}

// directoryRoutes 挂载目录查询路由（管理面保护组内调用）。
func directoryRoutes(admin chi.Router, h *Handler) {
	admin.Get("/directory/orgs", h.DirectoryOrgs)
	admin.Get("/directory/users", h.DirectoryUsers)
}
