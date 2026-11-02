package handler

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/TuF3i/kamc/internal/metrics"
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
// 非浏览器客户端（CLI/exec plugin/e2e）返回 JSON 信封；浏览器导航
// （Accept: text/html）返回桥接页：JWT 写入同源 localStorage 后跳回 SPA 根
// 路径（web/ 前端约定，见 web/README.md）。
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
	h.issueJWT(w, r, user.Sub, user.Email, user.Groups, "/auth/callback", wantsHTML(r))
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
	h.issueJWT(w, r, req.Sub, req.Email, req.Groups, "/auth/dev-token", false)
}

// AuthPasswordLogin POST /auth/password-login —— 内置管理员账号密码登录。
// 默认账号 sysadmin/sysadmin123（可用 AM_PASSWORD_LOGIN_USER/PASSWORD 覆盖，
// AM_PASSWORD_LOGIN=0 关闭）。成功后的签发/审计流程与 OIDC 一致。
func (h *Handler) AuthPasswordLogin(w http.ResponseWriter, r *http.Request) {
	if !h.passwordLoginEnabled || h.auth == nil {
		http.NotFound(w, r)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" || req.Password == "" {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(h.passwordUser)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(req.Password), []byte(h.passwordPass)) == 1
	if !userOK || !passOK {
		h.auditSimple(r, username, "", "/auth/password-login", "login-failed", false)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	h.issueJWT(w, r, username, "", []string{h.adminGroup}, "/auth/password-login", false)
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
	h.issueJWT(w, r, user.Sub, user.Email, user.Groups, "/auth/token-exchange", false)
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

// wantsHTML 浏览器导航检测：顶层导航的 Accept 首选 text/html；
// fetch/XHR 默认 Accept: */*，CLI 客户端一般不携带——均走 JSON 分支
func wantsHTML(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// issueJWT 登录签发挂载点：审计登录成功并签发平台 JWT；
// browser=true 时以桥接 HTML 输出（仅 /auth/callback 浏览器导航使用）。
func (h *Handler) issueJWT(w http.ResponseWriter, r *http.Request, sub, email string, groups []string, source string, browser bool) {
	h.auditSimple(r, sub, "", source, "login-success", true)
	token, exp, err := h.auth.Sign(sub, email, groups)
	if err != nil {
		http.Error(w, "sign error", http.StatusInternalServerError)
		return
	}
	if browser {
		writeLoginBridge(w, token)
		return
	}
	errcode.GenFinalResponse(w, map[string]any{
		"token":      token,
		"expires_at": exp.UTC().Format(time.RFC3339),
		"token_type": "Bearer",
	}, nil)
}

// writeLoginBridge 浏览器登录桥接页：OIDC 回调落在后端路径上，SPA 需要拿到
// JWT 才能继续——桥接页写入同源 localStorage 后跳回根路径。token 经
// json.Marshal 转义为字符串字面量嵌入脚本，避免注入。
func writeLoginBridge(w http.ResponseWriter, token string) {
	literal, err := json.Marshal(token)
	if err != nil {
		http.Error(w, "sign error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = fmt.Fprintf(w, `<!doctype html><html><body><script>
try{localStorage.setItem('kamc_token',%s);}catch(e){}
location.replace('/');
</script></body></html>`, literal)
}
