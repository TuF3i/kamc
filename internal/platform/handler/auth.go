package handler

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/TuF3i/kamc/internal/metrics"
	"github.com/TuF3i/kamc/internal/platform/errcode"
	ldapclient "github.com/TuF3i/kamc/internal/platform/ldapclient"
	"github.com/TuF3i/kamc/internal/platform/luaauth"
	"github.com/TuF3i/kamc/internal/platform/middleware"
	"github.com/TuF3i/kamc/internal/platform/modules"
	"github.com/TuF3i/kamc/internal/platform/service"
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
		h.auditPlatform(r, "login-failed", "", "", "/auth/callback", false, "oidc-exchange-failed")
		http.Error(w, "login failed", http.StatusUnauthorized)
		return
	}
	h.issueJWTWithSync(w, r, &service.Identity{
		Username: user.Sub, Email: user.Email, Nickname: user.Sub, Avatar: user.Picture,
		Groups: user.Groups, Source: modules.UserSourceSSO,
	}, "/auth/callback", wantsHTML(r))
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
	h.issueJWTWithSync(w, r, &service.Identity{
		Username: req.Sub, Email: req.Email, Groups: req.Groups, Source: modules.UserSourceLocal,
	}, "/auth/dev-token", false)
}

// AuthLdapLogin POST /auth/ldap-login —— 独立 LDAP 直登端点（保留给 CLI/exec
// 场景；Web 登录走 /auth/password-login 的统一 fallback 链路）。
// 认证通过后与 SSO 一致：同步平台用户/组 → 跑映射脚本 → 签发 JWT。
func (h *Handler) AuthLdapLogin(w http.ResponseWriter, r *http.Request) {
	if h.ldap == nil || h.auth == nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrLDAPDisabled)
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
	user, err := h.ldap.Authenticate(strings.TrimSpace(req.Username), req.Password)
	if err != nil {
		h.auditPlatform(r, "login-failed", req.Username, "", "/auth/ldap-login", false, "ldap-auth-failed")
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	h.issueJWTWithSync(w, r, &service.Identity{
		Username: user.Username, Email: user.Email, Nickname: user.Nickname,
		Groups: user.Groups, Source: modules.UserSourceLDAP,
	}, "/auth/ldap-login", false)
}

// AuthPasswordLogin POST /auth/password-login —— 统一账号密码登录：
// 1) 先匹配内置管理员（AM_PASSWORD_LOGIN_USER/PASSWORD）；
// 2) 未命中且已配置 LDAP 时走 LDAP 认证（成功即同步数据写入平台用户库）；
// 3) 都未命中统一 401（不泄露内置账号是否启用/LDAP 是否配置，防枚举）。
func (h *Handler) AuthPasswordLogin(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil {
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

	// 1) 内置管理员（恒定时间比较；AM_PASSWORD_LOGIN=0 关闭）
	if h.passwordLoginEnabled {
		userOK := subtle.ConstantTimeCompare([]byte(username), []byte(h.passwordUser)) == 1
		passOK := subtle.ConstantTimeCompare([]byte(req.Password), []byte(h.passwordPass)) == 1
		if userOK && passOK {
			h.issueJWTWithSync(w, r, &service.Identity{
				Username: username, Groups: []string{h.adminGroup}, Source: modules.UserSourceLocal,
			}, "/auth/password-login", false)
			return
		}
	}

	// 2) LDAP 认证（未配置 = 跳过；目录级系统错误按登录失败处理防枚举）
	if h.ldap != nil {
		user, err := h.ldap.Authenticate(username, req.Password)
		if err == nil {
			h.issueJWTWithSync(w, r, &service.Identity{
				Username: user.Username, Email: user.Email, Nickname: user.Nickname,
				Groups: user.Groups, Source: modules.UserSourceLDAP,
			}, "/auth/ldap-login", false)
			return
		}
		if !errors.Is(err, ldapclient.ErrAuth) {
			log.Printf("[platform] ldap auth %s: %v", username, err)
		}
	}

	h.auditPlatform(r, "login-failed", username, "", "/auth/password-login", false, "bad-credentials")
	http.Error(w, "unauthorized", http.StatusUnauthorized)
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
	h.issueJWTWithSync(w, r, &service.Identity{
		Username: user.Sub, Email: user.Email, Nickname: user.Sub, Avatar: user.Picture,
		Groups: user.Groups, Source: modules.UserSourceSSO,
	}, "/auth/token-exchange", false)
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
	h.auditPlatform(r, "user-revoked", req.Sub, "", "/api/v1/auth/revoke", true, "revoked")
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
	h.auditPlatform(r, "logout", claims.Subject, "", "/auth/logout", true, "ok")
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

// issueJWT 登录签发挂载点（不做用户同步——仅测试/特殊场景使用）；
// browser=true 时以桥接 HTML 输出（仅 /auth/callback 浏览器导航使用）。
func (h *Handler) issueJWT(w http.ResponseWriter, r *http.Request, sub, email string, groups []string, source string, browser bool) {
	h.auditPlatform(r, "login-success", sub, "", source, true, "ok")
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

// issueJWTWithSync 登录签发完整链路（SSO/LDAP/dev-token/密码登录统一入口）：
// 平台用户/组同步（失败仅记录，不阻断登录）→ Lua 映射脚本（fail-closed，
// 失败拒绝签发 + 审计 script-failed）→ 签发 JWT。
func (h *Handler) issueJWTWithSync(w http.ResponseWriter, r *http.Request, id *service.Identity, source string, browser bool) {
	if id == nil || h.auth == nil {
		http.Error(w, "auth not configured", http.StatusServiceUnavailable)
		return
	}
	// 1) 平台用户目录同步（失败不阻断登录——目录是增强信息）
	if rsp := h.svc.SyncUserOnLogin(r.Context(), id); rsp != nil {
		log.Printf("[platform] user sync %s: %s", id.Username, rsp)
	}
	// 2) 映射脚本（fail-closed）
	if h.luaAuth != nil {
		extra := map[string]string{}
		if id.Source == modules.UserSourceLDAP {
			extra["login_method"] = "ldap"
		}
		if err := h.luaAuth.Execute(r.Context(), luaauth.Claims{
			Sub: id.Username, Email: id.Email, Groups: id.Groups,
			Source: string(id.Source), Extra: extra,
		}); err != nil {
			log.Printf("[platform] lua script %s: %v", id.Username, err)
			h.auditPlatform(r, "script-failed", id.Username, "", source, false, "mapping-script-rejected")
			errcode.GenFinalResponse(w, map[string]any{
				"status": 10003, "message": "login rejected by mapping script",
			}, &errcode.Response{Status: 10003, Message: "login rejected by mapping script: " + err.Error()})
			return
		}
	}
	h.issueJWT(w, r, id.Username, id.Email, id.Groups, source, browser)
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
