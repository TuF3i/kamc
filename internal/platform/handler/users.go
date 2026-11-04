package handler

// 用户与组管理 API（管理面）：平台自维护用户目录的查询/编辑/删除
// 与组聚合视图。用户的创建不由本 API 负责——登录链路（SSO/LDAP/
// 密码）自动 upsert。

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/middleware"
	"github.com/TuF3i/kamc/internal/platform/service"
)

// GetProfile GET /api/v1/profile —— 当前登录用户的平台资料（自服务，仅需 JWT）。
// JWT 只承载身份与组，头像/昵称等展示字段由本接口读取，避免改资料要重新登录。
func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromCtx(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	data, rsp := h.svc.GetUser(r.Context(), claims.Subject)
	errcode.GenFinalResponse(w, data, rsp)
}

// CreateUser POST /api/v1/users —— 手动创建本地用户（SSO/LDAP 之外的来源）
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req service.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	data, rsp := h.svc.CreateUser(r.Context(), &req)
	h.auditPlatform(r, "user-create", req.Username, "", "/api/v1/users", errcode.IsSuccess(rsp), req.Username)
	errcode.GenFinalResponse(w, data, rsp)
}

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

// DeleteUser DELETE /api/v1/users/{username} —— 删除账号并立即吊销其已签发 Token。
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	user, rsp := h.svc.GetUser(r.Context(), username)
	if rsp != nil {
		errcode.GenFinalResponse(w, nil, rsp)
		return
	}
	// 先吊销再删除：吊销失败则不删（避免账号已删但 Token 仍可用）。
	if h.auth != nil {
		if err := h.auth.RevokeUser(r.Context(), username); err != nil {
			errcode.GenFinalResponse(w, nil, errcode.ServerInternal(err))
			return
		}
	}
	rsp = h.svc.DeleteUser(r.Context(), username)
	if errcode.IsSuccess(rsp) && user != nil && h.avatarDir != "" {
		_ = os.Remove(filepath.Join(h.avatarDir, user.UID))
	}
	h.auditPlatform(r, "user-delete", username, "", "/api/v1/users/"+username, errcode.IsSuccess(rsp), "delete+revoke")
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

// UpdateGroup PATCH /api/v1/groups/{name} —— 编辑名称/描述（重命名同步用户组字段）
func (h *Handler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	var req service.UpdateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	name := chi.URLParam(r, "name")
	data, rsp := h.svc.UpdateGroup(r.Context(), name, &req)
	h.auditPlatform(r, "group-update", "", "", "/api/v1/groups/"+name, errcode.IsSuccess(rsp), "update")
	errcode.GenFinalResponse(w, data, rsp)
}

// AddGroupMember POST /api/v1/groups/{name}/members {username}
func (h *Handler) AddGroupMember(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	name := chi.URLParam(r, "name")
	rsp := h.svc.AddGroupMember(r.Context(), name, req.Username)
	h.auditPlatform(r, "group-member-add", req.Username, "", "/api/v1/groups/"+name+"/members", errcode.IsSuccess(rsp), req.Username)
	errcode.GenFinalResponse(w, nil, rsp)
}

// RemoveGroupMember DELETE /api/v1/groups/{name}/members/{username}
func (h *Handler) RemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	username := chi.URLParam(r, "username")
	rsp := h.svc.RemoveGroupMember(r.Context(), name, username)
	h.auditPlatform(r, "group-member-remove", username, "", "/api/v1/groups/"+name+"/members/"+username, errcode.IsSuccess(rsp), username)
	errcode.GenFinalResponse(w, nil, rsp)
}

// DeleteGroup DELETE /api/v1/groups/{name}
func (h *Handler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	rsp := h.svc.DeleteGroup(r.Context(), chi.URLParam(r, "name"))
	h.auditPlatform(r, "group-delete", "", "", "/api/v1/groups/"+chi.URLParam(r, "name"), errcode.IsSuccess(rsp), "delete")
	errcode.GenFinalResponse(w, nil, rsp)
}

// usersRoutes / groupsRoutes 管理组内挂载
func usersRoutes(admin chi.Router, h *Handler) {
	admin.Get("/users", h.ListUsers)
	admin.Post("/users", h.CreateUser)
	admin.Patch("/users/{username}", h.UpdateUser)
	admin.Delete("/users/{username}", h.DeleteUser)
}

func groupsRoutes(admin chi.Router, h *Handler) {
	admin.Get("/groups", h.ListGroups)
	admin.Post("/groups", h.CreateGroup)
	admin.Patch("/groups/{name}", h.UpdateGroup)
	admin.Post("/groups/{name}/members", h.AddGroupMember)
	admin.Delete("/groups/{name}/members/{username}", h.RemoveGroupMember)
	admin.Delete("/groups/{name}", h.DeleteGroup)
}
