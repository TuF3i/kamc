package handler

// 脚本管理 API（管理面，admin 组）：登录期权限绑定脚本的
// CRUD 与手动执行（dry-run 验证闭环）。

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/luaauth"
	"github.com/TuF3i/kamc/internal/platform/middleware"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

// ListAuthScripts GET /api/v1/auth-scripts?all=1
func (h *Handler) ListAuthScripts(w http.ResponseWriter, r *http.Request) {
	all := r.URL.Query().Get("all") == "1"
	scripts, err := h.luaAuth.ListScripts(r.Context(), all)
	if err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ServerInternal(err))
		return
	}
	if scripts == nil {
		scripts = []modules.AuthScript{}
	}
	errcode.GenFinalResponse(w, scripts, nil)
}

// GetAuthScript GET /api/v1/auth-scripts/{name}
func (h *Handler) GetAuthScript(w http.ResponseWriter, r *http.Request) {
	sc, err := h.luaAuth.GetScript(r.Context(), chi.URLParam(r, "name"))
	if err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrAuthScriptNotFound)
		return
	}
	errcode.GenFinalResponse(w, sc, nil)
}

// PutAuthScript PUT /api/v1/auth-scripts/{name} —— 保存（Lua 语法校验失败不落库）
func (h *Handler) PutAuthScript(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var req struct {
		Content string `json:"content"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	if err := h.luaAuth.UpsertScript(r.Context(), name, req.Content, req.Enabled); err != nil {
		// 语法错误对外以 10001 + message 呈现（管理员排错需要看到编译信息）
		errcode.GenFinalResponse(w, nil, errcode.ServerInternal(err))
		return
	}
	sc, err := h.luaAuth.GetScript(r.Context(), name)
	if err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ServerInternal(err))
		return
	}
	errcode.GenFinalResponse(w, sc, nil)
}

// DeleteAuthScript DELETE /api/v1/auth-scripts/{name}
func (h *Handler) DeleteAuthScript(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	deleted, err := h.luaAuth.DeleteScript(r.Context(), name)
	if err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ServerInternal(err))
		return
	}
	if !deleted {
		errcode.GenFinalResponse(w, nil, errcode.ErrAuthScriptNotFound)
		return
	}
	errcode.GenFinalResponse(w, nil, nil)
}

// ExecuteAuthScript POST /api/v1/auth-scripts/{name}/execute?dry_run=1
// body 可选注入 mock claims（缺省用调用者自己的身份）——改脚本的验证闭环。
func (h *Handler) ExecuteAuthScript(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var req struct {
		Sub    string   `json:"sub"`
		Email  string   `json:"email"`
		Groups []string `json:"groups"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req) // body 可选

	if req.Sub == "" {
		if claims, ok := middleware.ClaimsFromCtx(r.Context()); ok {
			req.Sub = claims.Subject
			req.Email = claims.Email
			req.Groups = claims.Groups
		}
	}

	dryRun := r.URL.Query().Get("dry_run") == "1"
	actions, err := h.luaAuth.ExecuteScript(r.Context(), name,
		luaauth.ClaimsOf(req.Sub, req.Email, req.Groups), dryRun)
	if err != nil {
		h.auditSimple(r, req.Sub, "", "/api/v1/auth-scripts/"+name+"/execute", "script-failed", false)
		errcode.GenFinalResponse(w, map[string]any{"error": err.Error()}, errcode.ServerInternal(err))
		return
	}
	if actions == nil {
		actions = []luaauth.Action{}
	}
	errcode.GenFinalResponse(w, map[string]any{"dry_run": dryRun, "actions": actions}, nil)
}

// authScriptsRoutes 管理组内挂载
func authScriptsRoutes(admin chi.Router, h *Handler) {
	admin.Get("/auth-scripts", h.ListAuthScripts)
	admin.Get("/auth-scripts/{name}", h.GetAuthScript)
	admin.Put("/auth-scripts/{name}", h.PutAuthScript)
	admin.Delete("/auth-scripts/{name}", h.DeleteAuthScript)
	admin.Post("/auth-scripts/{name}/execute", h.ExecuteAuthScript)
}
