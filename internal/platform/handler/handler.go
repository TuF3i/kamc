// Package handler 平台 HTTP 参数绑定与错误映射：业务一律下沉 service，
// 本包不做任何业务判断；响应统一走 errcode.GenFinalResponse（模式 B 出口）。
package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/TuF3i/kamc/internal/platform/audit"
	mw "github.com/TuF3i/kamc/internal/platform/middleware"

	"github.com/TuF3i/kamc/internal/auth"
	"github.com/TuF3i/kamc/internal/platform/catalog"
	"github.com/TuF3i/kamc/internal/platform/directory"
	"github.com/TuF3i/kamc/internal/platform/errcode"
	ldapclient "github.com/TuF3i/kamc/internal/platform/ldapclient"
	"github.com/TuF3i/kamc/internal/platform/luaauth"
	"github.com/TuF3i/kamc/internal/platform/modules"
	"github.com/TuF3i/kamc/internal/platform/oidc"
	"github.com/TuF3i/kamc/internal/platform/proxy"
	"github.com/TuF3i/kamc/internal/platform/service"
	"github.com/TuF3i/kamc/internal/tunnel"
	"github.com/TuF3i/kamc/internal/version"
)

// Deps Handler 的可空依赖（nil 即功能降级：对应端点返回 503/501）
type Deps struct {
	Auth            *auth.Service      // JWT 签发/校验（nil = 认证未装配）
	OIDC            *oidc.Client       // OIDC 登录（nil = 未配置 → /auth/login 503）
	Tunnels         *tunnel.Service    // 隧道模块（nil = 未启用 → /tunnel 返回 503）
	Proxies         *proxy.Pool        // API 代理池（nil = 未装配 → /proxy 返回 501）
	CAData          []byte             // kubeconfig 嵌入的平台 CA（AM_KUBECONFIG_CA_FILE，可选）
	OIDCIssuer      string             // exec 插件参数（kubeconfig 生成用）
	OIDCClientID    string             // exec 插件参数
	OIDCRedirectURL string             // OIDC 回调地址（平台信息页展示）
	RateLimiter     mw.RateLimiter     // 数据面限流器（nil 或 RateLimit<=0 不启用）
	RateLimitN      int                // 窗口内最大请求数
	Audit           *audit.Writer      // 审计异步写出器（nil = 不审计）
	RobotTokenTTL   time.Duration      // Robot 长期 JWT 有效期（默认 365 天）
	Directory       *directory.Client  // Authentik 用户/组织目录（nil = 未配置）
	Catalog         *catalog.Service   // 集群 GVR 资源目录（nil = 未装配）
	AgentImageRepo  string             // Agent 镜像仓库完整地址（不含 tag，AM_AGENT_IMAGE_REPO）
	Settings        Settings           // 平台静态配置快照（平台信息页展示，敏感值已在装配层脱敏）
	LDAP            *ldapclient.Config // LDAP 登录（nil = 未配置 → /auth/ldap-login 503）
	LuaAuth         *luaauth.Engine    // 登录期映射脚本引擎（nil = 脚本功能关闭）
}

// Handler 平台 HTTP 处理器（业务下沉 Service；依赖可空，对应降级行为）
type Handler struct {
	svc     *service.Service
	auth    *auth.Service
	oidc    *oidc.Client
	tunnels *tunnel.Service
	proxies *proxy.Pool

	caData       []byte // kubeconfig 嵌入的平台 CA
	oidcIssuer   string // exec 插件参数
	oidcClientID string // exec 插件参数

	rateLimiter    mw.RateLimiter // 数据面限流器（nil 或 rateLimitN<=0 不启用）
	rateLimitN     int            // 窗口内最大请求数
	audit          *audit.Writer
	robotTokenTTL  time.Duration     // Robot 长期 JWT 有效期
	directory      *directory.Client // Authentik 用户/组织目录
	catalog        *catalog.Service  // 集群 GVR 资源目录
	agentImageRepo string            // Agent 镜像仓库完整地址（不含 tag）

	settings        Settings  // 平台静态配置快照（平台信息页）
	oidcRedirectURL string    // OIDC 回调地址（平台信息页展示）
	adminGroup      string    // 管理面用户组（AM_ADMIN_GROUP）
	devTokenEnabled bool      // 开发用直签 Token 开关（AM_DEV_ISSUE_TOKEN）
	startedAt       time.Time // Handler 装配时间（运行时长统计）

	ldap    *ldapclient.Config // LDAP 登录配置
	luaAuth *luaauth.Engine    // 登录期映射脚本引擎

	passwordLoginEnabled bool   // 内置账号密码登录开关（AM_PASSWORD_LOGIN）
	passwordUser         string // 内置账号（AM_PASSWORD_LOGIN_USER）
	passwordPass         string // 内置密码（AM_PASSWORD_LOGIN_PASSWORD）
}

// NewHandler 构造 Handler
func NewHandler(svc *service.Service, deps Deps) *Handler {
	return &Handler{
		svc: svc, auth: deps.Auth, oidc: deps.OIDC, tunnels: deps.Tunnels, proxies: deps.Proxies,
		caData: deps.CAData, oidcIssuer: deps.OIDCIssuer, oidcClientID: deps.OIDCClientID,
		oidcRedirectURL: deps.OIDCRedirectURL,
		rateLimiter:     deps.RateLimiter, rateLimitN: deps.RateLimitN, audit: deps.Audit,
		robotTokenTTL: deps.RobotTokenTTL, directory: deps.Directory, catalog: deps.Catalog,
		agentImageRepo: deps.AgentImageRepo, settings: deps.Settings, startedAt: time.Now(),
		ldap: deps.LDAP, luaAuth: deps.LuaAuth,
	}
}

// SetPolicy 设置运行策略（装配层调用）
func (h *Handler) SetPolicy(adminGroup string, devToken bool) {
	h.adminGroup = adminGroup
	h.devTokenEnabled = devToken
}

// SetPasswordLogin 设置内置账号密码登录（装配层调用）。
func (h *Handler) SetPasswordLogin(user, pass string, enabled bool) {
	h.passwordUser = user
	h.passwordPass = pass
	h.passwordLoginEnabled = enabled
}

// RegisterRoutes 挂载全部路由（健康检查 + /api/v1 业务 API + /tunnel + /proxy）。
// 认证/鉴权中间件（JWT、Casbin）按设计文档 §4/§5 属后续路线图，届时在
// /api/v1 与 /proxy 分组上挂载；健康检查与 /tunnel（自带 Token 认证）保持现状。
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/healthz", h.Healthz) // 存活：不碰任何依赖
	r.Get("/readyz", h.Readyz)   // 就绪：数据库连通（下线摘流依据）
	r.Get("/version", h.Version)
	r.Get("/metrics", promhttp.Handler().ServeHTTP)          // 平台指标（集群内网约定，鉴权属部署层）
	r.Get("/tunnel", h.TunnelWS)                             // Agent 隧道入口（WS 升级 + Token 认证）
	r.Get("/agent-install.yaml", h.AgentInstallYAMLForFetch) // Agent 一键安装清单（Token 即凭证）
	r.Get("/.well-known/jwks.json", h.JWKS)

	// 认证流程：login/callback 匿名（OIDC 流程自身防 CSRF）；me 需 JWT；
	// token-exchange 匿名（凭证即 ID Token 本身）
	r.Get("/auth/login", h.AuthLogin)
	r.Get("/auth/callback", h.AuthCallback)
	r.Post("/auth/token-exchange", h.AuthTokenExchange)
	r.Post("/auth/dev-token", h.AuthDevToken)
	r.Post("/auth/password-login", h.AuthPasswordLogin)
	r.Post("/auth/ldap-login", h.AuthLdapLogin)
	r.With(mw.RequireJWT(h.auth)).Get("/auth/me", h.AuthMe)
	r.With(mw.RequireJWT(h.auth)).Post("/auth/logout", h.AuthLogout)

	// 数据面代理：JWT 必选 + 按用户滑动窗口限流（AM_RATE_LIMIT，0=不限）
	r.With(mw.RequireJWT(h.auth), mw.RateLimit(h.rateLimiter, h.rateLimitN, time.Minute)).
		HandleFunc("/proxy/{clusterID}", h.Proxy)
	r.With(mw.RequireJWT(h.auth), mw.RateLimit(h.rateLimiter, h.rateLimitN, time.Minute)).
		HandleFunc("/proxy/{clusterID}/*", h.Proxy)

	r.Route("/api/v1", func(api chi.Router) {
		// Agent 注册交换：注册 Token 即凭证，不入 JWT 保护组
		api.Post("/agent/register", h.AgentRegister)

		// 自服务：仅需 JWT（产物只含调用者自己的身份）
		api.Group(func(user chi.Router) {
			user.Use(mw.RequireJWT(h.auth))
			user.Get("/clusters/{id}/kubeconfig", h.GetKubeconfig)
		})

		// 管理面：JWT + 管理员组（最小授权；数据面资源级鉴权由 Casbin 承担）
		api.Group(func(admin chi.Router) {
			admin.Use(mw.RequireJWT(h.auth))
			admin.Use(mw.RequireAdmin(h.adminGroup))
			admin.Post("/clusters", h.CreateCluster)
			admin.Get("/clusters", h.ListClusters)
			admin.Get("/clusters/{id}", h.GetCluster)
			admin.Delete("/clusters/{id}", h.DeleteCluster)
			admin.Post("/clusters/{id}/tokens", h.IssueToken)
			admin.Get("/clusters/{id}/tokens", h.ListTokens)
			admin.Delete("/clusters/{id}/tokens/{kind}", h.RevokeToken)
			admin.Post("/auth/revoke", h.AuthRevokeUser)
			admin.Get("/audit", h.ListAudit)
			policiesRoutes(admin, h)
			directoryRoutes(admin, h)
			clusterResourcesRoutes(admin, h)
			clusterInfoRoutes(admin, h)
			robotsRoutes(admin, h)
			usersRoutes(admin, h)
			groupsRoutes(admin, h)
			scriptsRoutes(admin, h)
			admin.Get("/system", h.SystemInfo)
			admin.Get("/platform/info", h.PlatformInfo)
		})
	})
}

// Healthz 存活探针：进程活着即返回 200，不做依赖检查
func (h *Handler) Healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// Readyz 就绪探针：数据库不可达返回 503（LB/编排据此摘流，§7.5 优雅下线第一步）
func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Ping(r.Context()); err != nil {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

// Version 构建版本信息
func (h *Handler) Version(w http.ResponseWriter, _ *http.Request) {
	errcode.GenFinalResponse(w, map[string]string{
		"version":   version.Version,
		"commit":    version.Commit,
		"component": "platform",
	}, nil)
}

// CreateCluster POST /api/v1/clusters
func (h *Handler) CreateCluster(w http.ResponseWriter, r *http.Request) {
	var req modules.CreateClusterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	data, rsp := h.svc.CreateCluster(r.Context(), &req)
	errcode.GenFinalResponse(w, data, rsp)
}

// ListClusters GET /api/v1/clusters
func (h *Handler) ListClusters(w http.ResponseWriter, r *http.Request) {
	data, rsp := h.svc.ListClusters(r.Context())
	errcode.GenFinalResponse(w, data, rsp)
}

// GetCluster GET /api/v1/clusters/{id}
func (h *Handler) GetCluster(w http.ResponseWriter, r *http.Request) {
	data, rsp := h.svc.GetCluster(r.Context(), chi.URLParam(r, "id"))
	errcode.GenFinalResponse(w, data, rsp)
}

// DeleteCluster DELETE /api/v1/clusters/{id}
func (h *Handler) DeleteCluster(w http.ResponseWriter, r *http.Request) {
	rsp := h.svc.DeleteCluster(r.Context(), chi.URLParam(r, "id"))
	errcode.GenFinalResponse(w, nil, rsp)
}
