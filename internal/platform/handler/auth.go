package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/middleware"
)

// AuthLogin GET /auth/login —— 发起 OIDC 登录（302 到 Provider）
func (h *Handler) AuthLogin(w http.ResponseWriter, r *http.Request) {
	if h.oidc == nil {
		http.Error(w, "oidc not configured", http.StatusServiceUnavailable)
		return
	}
	url, _, err := h.oidc.LoginURL(r.Context())
	if err != nil {
		http.Error(w, "oidc unavailable", http.StatusBadGateway)
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

// AuthCallback GET /auth/callback —— 授权码兑换，签发平台 JWT。
// 返回 JSON（CLI/调试友好）；浏览器直连场景的前端跳转属路线图。
func (h *Handler) AuthCallback(w http.ResponseWriter, r *http.Request) {
	if h.oidc == nil || h.auth == nil {
		http.Error(w, "oidc not configured", http.StatusServiceUnavailable)
		return
	}
	user, err := h.oidc.Exchange(r.Context(), r.URL.Query().Get("code"), r.URL.Query().Get("state"))
	if err != nil {
		log.Printf("[platform] oidc exchange: %v", err)
		http.Error(w, "login failed", http.StatusUnauthorized)
		return
	}
	h.issueAndRespond(w, user.Sub, user.Email, user.Groups)
}

// AuthMe GET /auth/me —— 回显当前 JWT claims（调试与客户端自检）
func (h *Handler) AuthMe(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromCtx(r.Context())
	if !ok {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	errcode.GenFinalResponse(w, claims, nil)
}

// AuthDevToken POST /auth/dev-token —— 开发用直签 Token（AM_DEV_ISSUE_TOKEN=1 才可用）。
// 仅限本地/测试环境：生产环境未设开关时返回 404，避免出现绕过 SSO 的通道。
func (h *Handler) AuthDevToken(w http.ResponseWriter, r *http.Request) {
	if !h.devTokenEnabled || h.auth == nil {
		http.NotFound(w, r)
		return
	}
	var req struct {
		Sub    string   `json:"sub"`
		Email  string   `json:"email"`
		Groups []string `json:"groups"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Sub == "" {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	h.issueAndRespond(w, req.Sub, req.Email, req.Groups)
}

// JWKS GET /.well-known/jwks.json —— 平台 JWT 公钥集（§4.3）
func (h *Handler) JWKS(w http.ResponseWriter, _ *http.Request) {
	if h.auth == nil {
		http.Error(w, "auth not configured", http.StatusServiceUnavailable)
		return
	}
	b, err := h.auth.JWKS()
	if err != nil {
		http.Error(w, "jwks error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

// issueAndRespond 签发平台 JWT 并输出（callback 与 dev-token 共用出口）
func (h *Handler) issueAndRespond(w http.ResponseWriter, sub, email string, groups []string) {
	token, exp, err := h.auth.Sign(sub, email, groups)
	if err != nil {
		http.Error(w, "sign error", http.StatusInternalServerError)
		return
	}
	errcode.GenFinalResponse(w, map[string]any{
		"token":      token,
		"expires_at": exp.UTC().Format(time.RFC3339),
		"token_type": "Bearer",
	}, nil)
}
