package handler

// 用户与组管理 API（管理面）：平台自维护用户目录的查询/编辑/删除
// 与组聚合视图。用户的创建不由本 API 负责——登录链路（SSO/LDAP/
// 密码）自动 upsert。

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/service"
)

// ListUsers GET /api/v1/users
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	data, rsp := h.svc.ListUsers(r.Context())
	errcode.GenFinalResponse(w, data, rsp)
}

// UpdateUser PATCH /api/v1/users/{username}
func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	var req service.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	data, rsp := h.svc.UpdateUser(r.Context(), chi.URLParam(r, "username"), &req)
	h.auditPlatform(r, "user-update", chi.URLParam(r, "username"), "", "/api/v1/users/"+chi.URLParam(r, "username"), errcode.IsSuccess(rsp), "update")
	errcode.GenFinalResponse(w, data, rsp)
}

// DeleteUser DELETE /api/v1/users/{username}
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	rsp := h.svc.DeleteUser(r.Context(), chi.URLParam(r, "username"))
	h.auditPlatform(r, "user-delete", chi.URLParam(r, "username"), "", "/api/v1/users/"+chi.URLParam(r, "username"), errcode.IsSuccess(rsp), "delete")
	errcode.GenFinalResponse(w, nil, rsp)
}

// ListGroups GET /api/v1/groups —— 聚合视图（含成员用户名列表）
func (h *Handler) ListGroups(w http.ResponseWriter, r *http.Request) {
	data, rsp := h.svc.ListGroups(r.Context())
	errcode.GenFinalResponse(w, data, rsp)
}

// CreateGroup POST /api/v1/groups
func (h *Handler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var req service.CreateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	data, rsp := h.svc.CreateGroup(r.Context(), &req)
	errcode.GenFinalResponse(w, data, rsp)
}

// DeleteGroup DELETE /api/v1/groups/{name}
func (h *Handler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	rsp := h.svc.DeleteGroup(r.Context(), chi.URLParam(r, "name"))
	errcode.GenFinalResponse(w, nil, rsp)
}

// usersRoutes / groupsRoutes 管理组内挂载
func usersRoutes(admin chi.Router, h *Handler) {
	admin.Get("/users", h.ListUsers)
	admin.Patch("/users/{username}", h.UpdateUser)
	admin.Delete("/users/{username}", h.DeleteUser)
}

func groupsRoutes(admin chi.Router, h *Handler) {
	admin.Get("/groups", h.ListGroups)
	admin.Post("/groups", h.CreateGroup)
	admin.Delete("/groups/{name}", h.DeleteGroup)
}
