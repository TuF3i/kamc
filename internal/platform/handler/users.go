package handler

// 用户与组管理 API（管理面）：平台自维护用户目录的查询/编辑/删除
// 与组聚合视图。用户的创建不由本 API 负责——登录链路（SSO/LDAP/
// 密码）自动 upsert。

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/middleware"
	"github.com/TuF3i/kamc/internal/platform/service"
)

// revokeUserTokens 权限相关变更后吊销用户已签发 JWT（旧 claims 立即失效）。
// 测试装配可能没有 auth 服务：此时跳过。
func (h *Handler) revokeUserTokens(r *http.Request, username string) error {
	if h.auth == nil || username == "" {
		return nil
	}
	return h.auth.RevokeUser(r.Context(), username)
}

// groupMembers 查询组成员；组不存在返回 ErrGroupNotFound。
func (h *Handler) groupMembers(r *http.Request, name string) ([]string, *errcode.Response) {
	groups, rsp := h.svc.ListGroups(r.Context())
	if rsp != nil {
		return nil, rsp
	}
	for _, g := range groups {
		if g.Name == name {
			return g.Members, nil
		}
	}
	return nil, errcode.ErrGroupNotFound
}

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
	username := chi.URLParam(r, "username")
	// groups 字段出现即视为权限变更：先吊销旧 Token 再改库（fail-closed）。
	groupsChanged := req.Groups != nil
	if groupsChanged {
		if err := h.revokeUserTokens(r, username); err != nil {
			errcode.GenFinalResponse(w, nil, errcode.ServerInternal(err))
			return
		}
	}
	data, rsp := h.svc.UpdateUser(r.Context(), username, &req)
	reason := "update"
	if groupsChanged {
		reason = "update+revoke"
	}
	h.auditPlatform(r, "user-update", username, "", "/api/v1/users/"+username, errcode.IsSuccess(rsp), reason)
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
	// 改名或管理员标记变化都会影响 JWT 内的组声明：成功后吊销全部成员。
	needsRevoke := (req.Name != nil && *req.Name != "" && *req.Name != name) || req.IsAdmin != nil
	var members []string
	if needsRevoke {
		list, rsp := h.groupMembers(r, name)
		if rsp != nil {
			errcode.GenFinalResponse(w, nil, rsp)
			return
		}
		members = list
	}
	data, rsp := h.svc.UpdateGroup(r.Context(), name, &req)
	if errcode.IsSuccess(rsp) && needsRevoke {
		for _, username := range members {
			if err := h.revokeUserTokens(r, username); err != nil {
				log.Printf("[platform] revoke after group update %s/%s: %v", name, username, err)
			}
		}
	}
	reason := "update"
	if needsRevoke {
		reason = "update+revoke"
	}
	h.auditPlatform(r, "group-update", "", "", "/api/v1/groups/"+name, errcode.IsSuccess(rsp), reason)
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
	// 加组后吊销：新组声明（尤其 is_admin）下次登录立即生效。
	if errcode.IsSuccess(rsp) {
		if err := h.revokeUserTokens(r, req.Username); err != nil {
			log.Printf("[platform] revoke after group add %s/%s: %v", name, req.Username, err)
		}
	}
	h.auditPlatform(r, "group-member-add", req.Username, "", "/api/v1/groups/"+name+"/members", errcode.IsSuccess(rsp), req.Username)
	errcode.GenFinalResponse(w, nil, rsp)
}

// RemoveGroupMember DELETE /api/v1/groups/{name}/members/{username}
func (h *Handler) RemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	username := chi.URLParam(r, "username")
	// 先吊销再改库：失败则不改，避免"已移出组但 Token 仍是旧权限"。
	if err := h.revokeUserTokens(r, username); err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ServerInternal(err))
		return
	}
	rsp := h.svc.RemoveGroupMember(r.Context(), name, username)
	h.auditPlatform(r, "group-member-remove", username, "", "/api/v1/groups/"+name+"/members/"+username, errcode.IsSuccess(rsp), username+"+revoke")
	errcode.GenFinalResponse(w, nil, rsp)
}

// DeleteGroup DELETE /api/v1/groups/{name}
func (h *Handler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	members, rsp := h.groupMembers(r, name)
	if rsp != nil {
		errcode.GenFinalResponse(w, nil, rsp)
		return
	}
	// 删组会移除成员组归属与 org 策略：先吊销全部成员 Token。
	for _, username := range members {
		if err := h.revokeUserTokens(r, username); err != nil {
			errcode.GenFinalResponse(w, nil, errcode.ServerInternal(err))
			return
		}
	}
	rsp = h.svc.DeleteGroup(r.Context(), name)
	reason := "delete+revoke"
	h.auditPlatform(r, "group-delete", "", "", "/api/v1/groups/"+name, errcode.IsSuccess(rsp), reason)
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
