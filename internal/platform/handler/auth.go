package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/TuF3i/kamc/internal/metrics"
	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/luaauth"
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
		h.auditSimple(r, "", "", "/auth/callback", "login-failed", false)
		http.Error(w, "login failed", http.StatusUnauthorized)
		return
	}
	h.issueAfterBinding(w, r, user.Sub, user.Email, user.Groups, "/auth/callback")
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
	h.issueAfterBinding(w, r, req.Sub, req.Email, req.Groups, "/auth/dev-token")
}

// AuthTokenExchange POST /auth/token-exchange —— OIDC ID Token 兑换平台 JWT
// （k8s-am-login exec plugin 用，§4.4）：ID Token 经 Provider JWKS 验签后，
// 按 sub/groups 签发平台 JWT。失败一律 401 不区分原因（防枚举）。
func (h *Handler) AuthTokenExchange(w http.ResponseWriter, r *http.Request) {
	if h.oidc == nil || h.auth == nil {
		http.Error(w, "oidc not configured", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		IDToken string `json:"id_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IDToken == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	user, err := h.oidc.VerifyIDToken(r.Context(), req.IDToken)
	if err != nil {
		log.Printf("[platform] token-exchange verify: %v", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	h.issueAfterBinding(w, r, user.Sub, user.Email, user.Groups, "/auth/token-exchange")
}

// AuthRevokeUser POST /api/v1/auth/revoke —— 踢出用户（管理面）：
// 该时刻之前签发的该用户全部 Token 立即失效（本地即时 + Redis 跨副本同步）。
func (h *Handler) AuthRevokeUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Sub string `json:"sub"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Sub) == "" {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	if err := h.auth.RevokeUser(r.Context(), strings.TrimSpace(req.Sub)); err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ServerInternal(err))
		return
	}
	h.auditSimple(r, req.Sub, "", "/api/v1/auth/revoke", "user-revoked", true)
	metrics.RevocationsTotal.WithLabelValues("user").Inc()
	errcode.GenFinalResponse(w, map[string]string{"sub": req.Sub, "revoked": "before-now"}, nil)
}

// AuthLogout POST /auth/logout —— 当前 Token 立即失效（自服务登出，jti 黑名单）
func (h *Handler) AuthLogout(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromCtx(r.Context())
	if !ok {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	ttl := time.Until(claims.ExpiresAt.Time)
	if ttl < 0 {
		ttl = 0
	}
	if err := h.auth.RevokeToken(r.Context(), claims.ID, ttl); err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ServerInternal(err))
		return
	}
	metrics.RevocationsTotal.WithLabelValues("token").Inc()
	errcode.GenFinalResponse(w, map[string]bool{"logout": true}, nil)
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

// issueAfterBinding 登录挂载点（§5 扩展，fail-closed）：
// 执行启用的权限绑定脚本（Authentik claims → Casbin 绑定/策略），
// 任何脚本错误拒绝签发（403 + 审计告警）——权限没算出来不放行；
// 管理员禁用脚本即可恢复登录。脚本未启用（nil 引擎/无脚本）直接签发。
func (h *Handler) issueAfterBinding(w http.ResponseWriter, r *http.Request, sub, email string, groups []string, source string) {
	if h.luaAuth != nil {
		if err := h.luaAuth.Execute(r.Context(), luaauth.ClaimsOf(sub, email, groups)); err != nil {
			log.Printf("[platform] binding script failed for %s: %v", sub, err)
			h.auditSimple(r, sub, "", source, "binding-script-failed", false)
			http.Error(w, "binding script failed", http.StatusForbidden)
			return
		}
	}
	h.auditSimple(r, sub, "", source, "login-success", true)
	h.issueAndRespond(w, sub, email, groups)
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
