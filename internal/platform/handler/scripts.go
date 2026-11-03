package handler

// 映射脚本管理 API（管理面）：登录期 Lua 脚本的 CRUD 与 dry-run 执行。
// 引擎未装配（LuaAuth nil）时端点返回 503。

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/luaauth"
	"github.com/TuF3i/kamc/internal/platform/middleware"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

// ListAuthScripts GET /api/v1/scripts?all=1
func (h *Handler) ListAuthScripts(w http.ResponseWriter, r *http.Request) {
	if h.luaAuth == nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrScriptsDisabled)
		return
	}
	all := r.URL.Query().Get("all") == "1"
	scripts, err := h.luaAuth.ListScripts(r.Context(), !all)
	if err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ServerInternal(err))
		return
	}
	if scripts == nil {
		scripts = []modules.AuthScript{}
	}
	errcode.GenFinalResponse(w, scripts, nil)
}

// GetAuthScript GET /api/v1/scripts/{name}
func (h *Handler) GetAuthScript(w http.ResponseWriter, r *http.Request) {
	if h.luaAuth == nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrScriptsDisabled)
		return
	}
	sc, err := h.luaAuth.GetScript(r.Context(), chi.URLParam(r, "name"))
	if err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrAuthScriptNotFound)
		return
	}
	errcode.GenFinalResponse(w, sc, nil)
}

// PutAuthScript PUT /api/v1/scripts/{name} —— 保存（Lua 语法校验失败不落库）
func (h *Handler) PutAuthScript(w http.ResponseWriter, r *http.Request) {
	if h.luaAuth == nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrScriptsDisabled)
		return
	}
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
		// 语法错误以系统错误呈现（管理员排错需要看到编译信息）
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

// DeleteAuthScript DELETE /api/v1/scripts/{name}
func (h *Handler) DeleteAuthScript(w http.ResponseWriter, r *http.Request) {
	if h.luaAuth == nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrScriptsDisabled)
		return
	}
	deleted, err := h.luaAuth.DeleteScript(r.Context(), chi.URLParam(r, "name"))
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

// ExecuteAuthScript POST /api/v1/scripts/{name}/execute?dry_run=1
// body 可选注入 mock claims（缺省用调用者身份）——改脚本的验证闭环。
func (h *Handler) ExecuteAuthScript(w http.ResponseWriter, r *http.Request) {
	if h.luaAuth == nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrScriptsDisabled)
		return
	}
	name := chi.URLParam(r, "name")
	var req struct {
		Sub    string            `json:"sub"`
		Email  string            `json:"email"`
		Groups []string          `json:"groups"`
		Source string            `json:"source"`
		Extra  map[string]string `json:"extra"`
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
	actions, err := h.luaAuth.ExecuteScript(r.Context(), name, luaauth.Claims{
		Sub: req.Sub, Email: req.Email, Groups: req.Groups, Source: req.Source, Extra: req.Extra,
	}, dryRun)
	if err != nil {
		h.auditSimple(r, req.Sub, "", "/api/v1/scripts/"+name+"/execute", "script-failed", false)
		errcode.GenFinalResponse(w, map[string]any{"error": err.Error()}, errcode.ServerInternal(err))
		return
	}
	if actions == nil {
		actions = []luaauth.Action{}
	}
	errcode.GenFinalResponse(w, map[string]any{"dry_run": dryRun, "actions": actions}, nil)
}

// scriptsRoutes 管理组内挂载
func scriptsRoutes(admin chi.Router, h *Handler) {
	admin.Get("/scripts", h.ListAuthScripts)
	admin.Get("/scripts/{name}", h.GetAuthScript)
	admin.Put("/scripts/{name}", h.PutAuthScript)
	admin.Delete("/scripts/{name}", h.DeleteAuthScript)
	admin.Post("/scripts/{name}/execute", h.ExecuteAuthScript)
}
