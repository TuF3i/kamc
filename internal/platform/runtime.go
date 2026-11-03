// Package platform 平台 HTTP 服务的组装层：认证/鉴权/隧道代理/集群管理等
// 全部平台业务经此模块暴露（当前为基础框架：健康检查、版本、集群注册 CRUD）。
//
// 分层 (与项目其他模块同构):
//
//	Runtime (本文件, 只负责组装与生命周期) ──▶ handler (参数绑定/错误映射) ──▶ service (业务) ──▶ dao (gorm)
//	                                          middleware (横切: 访问日志)      errcode (统一响应, 叶包)
//
// 使用: CreateRuntime(WithDB(db), WithAddr(":8080"), WithTunnels(trt)) → Engine() 挂载测试
// 或内部 ListenAndServe → ReleaseRuntime() 优雅关停（摘流 readyz → Shutdown → 关自建连接）。
package platform

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/TuF3i/kamc/infrastructure/postgres"
	"github.com/TuF3i/kamc/infrastructure/redis"
	"github.com/TuF3i/kamc/internal/auth"
	"github.com/TuF3i/kamc/internal/platform/audit"
	"github.com/TuF3i/kamc/internal/platform/authz"
	"github.com/TuF3i/kamc/internal/platform/catalog"
	"github.com/TuF3i/kamc/internal/platform/dao"
	"github.com/TuF3i/kamc/internal/platform/directory"
	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/geoip"
	"github.com/TuF3i/kamc/internal/platform/handler"
	ldapclient "github.com/TuF3i/kamc/internal/platform/ldapclient"
	"github.com/TuF3i/kamc/internal/platform/luaauth"
	mw "github.com/TuF3i/kamc/internal/platform/middleware"
	"github.com/TuF3i/kamc/internal/platform/modules"
	"github.com/TuF3i/kamc/internal/platform/oidc"
	"github.com/TuF3i/kamc/internal/platform/proxy"
	"github.com/TuF3i/kamc/internal/platform/service"
	"github.com/TuF3i/kamc/internal/tunnel"
)

// Config 平台运行配置（环境变量装载）
type Config struct {
	Addr string // HTTP 监听地址
}

// LoadConfig 从环境变量装载配置（缺失取默认值，必填项的校验在 CreateRuntime 的依赖构建中体现）
func LoadConfig() Config {
	return Config{
		Addr: envDefault("AM_LISTEN_ADDR", ":8080"),
	}
}

// Service 服务类型别名: 供其他模块经本包引用
type Service = service.Service

// Runtime 模块运行时: 只负责组装与创建/释放, 业务方法全部在 Service 上.
type Runtime struct {
	Service *service.Service
	Handler *handler.Handler

	Tunnels *tunnel.Runtime // 隧道模块（可选注入；代理转发落地后使用，§6.2）

	engine      *chi.Mux
	srv         *http.Server
	db          *gorm.DB                // 依赖的数据库连接 (自建时 Release 负责关闭)
	ownDB       bool                    // db 是否为 CreateRuntime 自建 (决定 Release 是否关闭)
	authzEn     *authz.Enforcer         // 鉴权组件（Release 释放 watcher）
	redisCli    goredis.UniversalClient // 共享 Redis 客户端 (自建时 Release 负责关闭)
	auditWriter *audit.Writer           // 审计异步写出器
}

// Option CreateRuntime 选项
type Option func(*options)

type options struct {
	db      *gorm.DB
	addr    string
	tunnels *tunnel.Runtime
}

// WithDB 注入已有数据库连接 (不注入则使用 infrastructure 默认连接并自持)
func WithDB(db *gorm.DB) Option {
	return func(o *options) { o.db = db }
}

// WithAddr 指定 HTTP 监听地址 (默认 :8080)
func WithAddr(addr string) Option {
	return func(o *options) { o.addr = addr }
}

// WithTunnels 注入隧道模块运行时（可选；未注入时仅无隧道依赖的功能可用）
func WithTunnels(rt *tunnel.Runtime) Option {
	return func(o *options) { o.tunnels = rt }
}

// defaultRuntime 包级默认运行时 (CreateRuntime 记录, ReleaseRuntime 消费)
var defaultRuntime *Runtime

// CreateRuntime 装配模块:
// 数据库连接 (可注入) → Dao → 建表 → Service → Handler → 路由 → HTTP 服务启动。
func CreateRuntime(opts ...Option) (*Runtime, error) {
	o := &options{addr: ":8080"}
	for _, opt := range opts {
		opt(o)
	}

	// 数据库连接: 注入优先, 否则自建 (自建的连接由 ReleaseRuntime 负责)
	db := o.db
	ownDB := false
	if db == nil {
		var err error
		db, err = postgres.NewPostgresClient()
		if err != nil {
			return nil, fmt.Errorf("platform -> connect postgres: %w", err)
		}
		ownDB = true
	}

	// 依赖注入链: Dao → authz 组件 → Service → Handler
	d := dao.NewDao(db)
	if err := d.SetupCluster(context.Background()); err != nil {
		return nil, fmt.Errorf("platform -> setup cluster table: %w", err)
	}
	if err := d.SetupToken(context.Background()); err != nil {
		return nil, fmt.Errorf("platform -> setup token table: %w", err)
	}
	if err := d.SetupPolicy(context.Background()); err != nil {
		return nil, fmt.Errorf("platform -> setup policy table: %w", err)
	}
	if err := d.SetupAudit(context.Background()); err != nil {
		return nil, fmt.Errorf("platform -> setup audit table: %w", err)
	}
	if err := d.SetupRobot(context.Background()); err != nil {
		return nil, fmt.Errorf("platform -> setup robot table: %w", err)
	}
	if err := d.SetupUser(context.Background()); err != nil {
		return nil, fmt.Errorf("platform -> setup user table: %w", err)
	}
	if err := d.SetupGroup(context.Background()); err != nil {
		return nil, fmt.Errorf("platform -> setup group table: %w", err)
	}
	if err := d.SetupAuthScript(context.Background()); err != nil {
		return nil, fmt.Errorf("platform -> setup auth script table: %w", err)
	}
	// Redis 客户端（配置存在时）：策略同步 watcher 与 Token 吊销共用；
	// 归 Runtime 所有（Release 关闭）。未配置 = 单副本语义（无跨副本同步）。
	var sharedRedisCli goredis.UniversalClient
	if os.Getenv("REDIS_ADDRESSES") != "" || os.Getenv("REDIS_ADDR") != "" {
		cli, err := redis.NewRedisClient()
		if err != nil {
			return nil, fmt.Errorf("platform -> connect redis: %w", err)
		}
		sharedRedisCli = cli
	}

	authzEn, err := authz.New(d, authz.WithSyncChannel(sharedRedisCli, authz.DefaultSyncChannel))
	if err != nil {
		if sharedRedisCli != nil {
			_ = sharedRedisCli.Close()
		}
		return nil, fmt.Errorf("platform -> build authz: %w", err)
	}
	svc := service.NewService(d, authzEn)

	// 认证服务：密钥环（Secret 挂载）或开发环境临时密钥（重启失效，打警告）；
	// Token 吊销：Redis 可用时跨副本同步（惰性周期拉取，鉴权热路径只查本地
	// 缓存——§7.1 鉴权零外部依赖不变量），否则进程内语义（单副本）
	var revStore auth.RevocationStore
	if sharedRedisCli != nil {
		revStore = auth.NewRedisRevocations(sharedRedisCli)
	}
	authSvc, err := buildAuthService(revStore)
	if err != nil {
		if sharedRedisCli != nil {
			_ = sharedRedisCli.Close()
		}
		return nil, fmt.Errorf("platform -> build auth: %w", err)
	}

	// OIDC 登录客户端：未配置时 /auth/login 降级 503（本地开发走 dev-token）
	var oidcOpts []oidc.Option
	if sharedRedisCli != nil {
		// state 外置 Redis（多副本登录发起与回调可落在不同副本，§4.1）
		oidcOpts = append(oidcOpts, oidc.WithStateStore(oidc.NewRedisStateStore(sharedRedisCli)))
	}
	oidcClient, err := oidc.New(oidc.Config{
		Issuer:       os.Getenv("OIDC_ISSUER"),
		ClientID:     os.Getenv("OIDC_CLIENT_ID"),
		ClientSecret: os.Getenv("OIDC_CLIENT_SECRET"),
		RedirectURL:  os.Getenv("OIDC_REDIRECT_URL"),
	}, oidcOpts...)
	if err != nil && !errors.Is(err, oidc.ErrNotConfigured) {
		return nil, fmt.Errorf("platform -> build oidc: %w", err)
	}

	// API 代理池：依赖隧道模块；鉴权已挂载（JWT），无开关依赖
	var proxies *proxy.Pool
	if o.tunnels != nil {
		proxies = proxy.NewPool(o.tunnels.Service.Open)
	}
	catalogSvc := catalog.New(svc, proxies)

	// LDAP 登录（LDAP_URL 未配置 = /auth/ldap-login 503）
	var ldapCfg *ldapclient.Config
	if os.Getenv("LDAP_URL") != "" {
		ldapCfg = ldapclient.FromEnv()
		if ldapCfg == nil {
			log.Printf("[platform] LDAP_URL 已设置但缺少 LDAP_BASE_DN，LDAP 登录未启用")
		} else {
			log.Printf("[platform] LDAP login enabled (%s)", ldapCfg.URL)
		}
	}

	// 登录期映射脚本引擎（gopher-lua 沙箱；fail-closed）
	luaEngine := luaauth.New(d, svc, authzEn)

	// kubeconfig 生成配置：平台 CA（可选嵌入）与 exec 插件参数
	var caData []byte
	if caFile := os.Getenv("AM_KUBECONFIG_CA_FILE"); caFile != "" {
		caData, err = os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("platform -> read kubeconfig ca: %w", err)
		}
	}

	// 数据面限流（AM_RATE_LIMIT：每用户每分钟最大请求数，0=不限）。
	// Redis 可用时跨副本共享窗口；否则进程内（单副本语义）。
	var limiter mw.RateLimiter
	rateLimitN := 0
	if v, err := strconv.Atoi(os.Getenv("AM_RATE_LIMIT")); err == nil && v > 0 {
		rateLimitN = v
		if sharedRedisCli != nil {
			limiter = mw.NewRedisRateLimiter(sharedRedisCli, time.Minute, rateLimitN)
		} else {
			limiter = mw.NewMemoryRateLimiter(time.Minute, rateLimitN)
			log.Printf("[platform] 未配置 Redis，限流窗口为进程内（多副本各算各的）")
		}
	}

	// 审计异步写出（§12.2）：缓冲投递 → 批量写 PG；PG 故障落本地文件回放。
	// 出口分流：平台事件 → stdio；集群事件 → 可选的 syslog 远程日志服务器。
	auditWriter := audit.NewWriter(d, envDefault("AM_AUDIT_SPILL_FILE", "/tmp/kamc-audit-spill.jsonl"))
	syslogSink := audit.NewSyslogFromEnv()
	auditWriter.WithSyslog(syslogSink)
	auditWriter.Start()
	log.Printf("[platform] audit syslog export: %s", syslogSink.Summary())

	// GeoIP（可选）：AM_GEOIP_DB 指向 MaxMind .mmdb；未配置/加载失败则审计不含地理信息
	geoResolver := geoip.FromEnv()

	// Authentik 用户/组织目录（策略编辑器下拉；未配置时端点返回 503）
	directoryClient := directory.NewFromEnv()
	if directoryClient == nil {
		log.Printf("[platform] Authentik directory not configured (AUTHENTIK_API_URL/AUTHENTIK_API_TOKEN)")
	}

	// Agent 镜像仓库：完整地址（不含 tag，tag 随平台版本）；默认走 Docker Hub
	agentImageRepo := envDefault("AM_AGENT_IMAGE_REPO", "tuf3i/k8s-am-agent")

	// 平台信息页配置快照：敏感值（数据库密码）在此脱敏后注入
	redisAddrs := os.Getenv("REDIS_ADDRESSES")
	if redisAddrs == "" {
		redisAddrs = os.Getenv("REDIS_ADDR")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = postgres.DefaultDSN
	}
	platSettings := handler.Settings{
		ListenAddr:        o.addr,
		ExternalURL:       os.Getenv("AM_EXTERNAL_URL"),
		DSN:               redactDSN(dsn),
		RedisEnabled:      sharedRedisCli != nil,
		RedisAddrs:        redisAddrs,
		AgentImageRepo:    agentImageRepo,
		AuthentikAPIURL:   os.Getenv("AUTHENTIK_API_URL"),
		AuthentikSubField: envDefault("AUTHENTIK_USER_SUB_FIELD", "uid"),
	}
	if ttl, err := time.ParseDuration(os.Getenv("AUTHENTIK_CACHE_TTL")); err == nil && ttl > 0 {
		platSettings.AuthentikCacheTTL = ttl
	} else {
		platSettings.AuthentikCacheTTL = 60 * time.Second
	}

	h := handler.NewHandler(svc, handler.Deps{
		Auth:            authSvc,
		OIDC:            oidcClient,
		Tunnels:         tunnelsService(o.tunnels),
		Proxies:         proxies,
		CAData:          caData,
		OIDCIssuer:      os.Getenv("OIDC_ISSUER"),
		OIDCClientID:    os.Getenv("OIDC_CLIENT_ID"),
		OIDCRedirectURL: os.Getenv("OIDC_REDIRECT_URL"),
		RateLimiter:     limiter,
		RateLimitN:      rateLimitN,
		Audit:           auditWriter,
		RobotTokenTTL:   envDuration("AM_ROBOT_TOKEN_TTL", 365*24*time.Hour),
		Directory:       directoryClient,
		Catalog:         catalogSvc,
		AgentImageRepo:  agentImageRepo,
		Settings:        platSettings,
		LDAP:            ldapCfg,
		LuaAuth:         luaEngine,
		Geo:             geoResolver,
	})
	h.SetPolicy(envDefault("AM_ADMIN_GROUP", "am:admin"), os.Getenv("AM_DEV_ISSUE_TOKEN") == "1")
	passwordUser := envDefault("AM_PASSWORD_LOGIN_USER", "sysadmin")
	passwordPass := envDefault("AM_PASSWORD_LOGIN_PASSWORD", "sysadmin123")
	passwordEnabled := os.Getenv("AM_PASSWORD_LOGIN") != "0"
	h.SetPasswordLogin(passwordUser, passwordPass, passwordEnabled)
	if passwordEnabled && passwordUser == "sysadmin" && passwordPass == "sysadmin123" {
		log.Printf("[platform] WARNING: default password login enabled (sysadmin/sysadmin123); set AM_PASSWORD_LOGIN_USER/AM_PASSWORD_LOGIN_PASSWORD")
	}

	// 隧道分离回调：该集群在本副本已无隧道时标记 offline。
	// 跨副本收敛由注册表驱动（路线图），当前保守取本地视角。
	if o.tunnels != nil {
		o.tunnels.Service.SetDetachHook(func(clusterID, agentID string) {
			if _, err := o.tunnels.Service.Get(clusterID); err != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				if _, rsp := svc.SetClusterStatus(ctx, clusterID, modules.ClusterStatusOffline); !errcode.IsSuccess(rsp) {
					log.Printf("[platform] mark cluster %s offline: %s", clusterID, rsp)
				}
			}
		})
	}

	// 路由与 HTTP 服务
	engine := newEngine(h)
	srv := &http.Server{
		Addr:              o.addr,
		Handler:           engine,
		ReadHeaderTimeout: 5 * time.Second,
	}

	rt := &Runtime{
		Service:     svc,
		Handler:     h,
		Tunnels:     o.tunnels,
		engine:      engine,
		srv:         srv,
		db:          db,
		ownDB:       ownDB,
		authzEn:     authzEn,
		redisCli:    sharedRedisCli,
		auditWriter: auditWriter,
	}

	// ErrServerClosed 不算错误：ReleaseRuntime 主动 Shutdown 的正常路径
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[platform] http server: %v", err)
		}
	}()

	defaultRuntime = rt
	log.Printf("[platform] runtime created (addr=%s own_db=%v tunnels=%v)",
		o.addr, ownDB, o.tunnels != nil)
	return rt, nil
}

// newEngine 组装路由树与中间件（不含业务）
func newEngine(h *handler.Handler) *chi.Mux {
	engine := chi.NewRouter()
	engine.Use(chimw.RequestID)
	engine.Use(mw.AccessLog)
	engine.Use(chimw.Recoverer)
	h.RegisterRoutes(engine)
	return engine
}

// Engine 返回路由引擎（测试经 httptest 挂载，不走真实端口）
func (r *Runtime) Engine() *chi.Mux {
	return r.engine
}

// ReleaseRuntime 优雅关停: 摘流（依赖方读 readyz）→ 排空在途请求 → 关自建连接 (幂等).
func (r *Runtime) ReleaseRuntime() error {
	if r == nil {
		return nil
	}

	if r.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := r.srv.Shutdown(ctx); err != nil {
			log.Printf("[platform] http shutdown: %v", err)
		}
	}

	r.Service.Release()

	if r.ownDB && r.db != nil {
		if sqlDB, err := r.db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}

	r.auditWriter.Release()

	if r.authzEn != nil {
		r.authzEn.Release()
	}
	if r.redisCli != nil {
		_ = r.redisCli.Close()
	}

	if defaultRuntime == r {
		defaultRuntime = nil
	}
	log.Printf("[platform] runtime released")
	return nil
}

// ReleaseRuntime 释放包级默认运行时 (CreateRuntime 记录的那个), 幂等
func ReleaseRuntime() error {
	if defaultRuntime == nil {
		return nil
	}
	return defaultRuntime.ReleaseRuntime()
}

// envDefault 读环境变量，空值取默认
func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envDuration 读时间间隔环境变量，空值/非法值取默认（非法值显式告警）。
func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		log.Printf("[platform] invalid %s=%q, using default %s", key, v, def)
		return def
	}
	return d
}

// redactDSN 数据库连接串脱敏：kw 形式（password=xxx）与 URL 形式
// （postgres://user:pass@host/db）统一把密码段替换为 ***。
func redactDSN(dsn string) string {
	if dsn == "" {
		return ""
	}
	if strings.Contains(dsn, "://") {
		slash := strings.Index(dsn, "//")
		at := strings.Index(dsn, "@")
		if slash >= 0 && at > slash+2 {
			auth := dsn[slash+2 : at]
			if colon := strings.Index(auth, ":"); colon >= 0 {
				return dsn[:slash+2+colon+1] + "***" + dsn[at:]
			}
		}
		return dsn
	}
	return regexp.MustCompile(`(password=)[^\s]*`).ReplaceAllString(dsn, "${1}***")
}

// buildAuthService 构造 JWT 服务：JWT_KEYRING_DIR（Secret 挂载，多 kid）优先，
// 未配置则生成临时密钥（开发语义：重启后存量 Token 失效，显式告警）。
func buildAuthService(revStore auth.RevocationStore) (*auth.Service, error) {
	var opts []auth.Option
	opts = append(opts,
		auth.WithIssuer(envDefault("AM_JWT_ISSUER", "https://kamc.local")),
		auth.WithAudience(envDefault("AM_JWT_AUD", "kamc")),
	)
	if dir := os.Getenv("JWT_KEYRING_DIR"); dir != "" {
		opts = append(opts, auth.WithKeyDir(dir))
	} else {
		pem, err := auth.GenerateKeyPEM()
		if err != nil {
			return nil, fmt.Errorf("generate ephemeral key: %w", err)
		}
		opts = append(opts, auth.WithKey("dev-ephemeral", pem))
		log.Printf("[platform] 未配置 JWT_KEYRING_DIR，使用临时签名密钥（重启后存量 Token 失效，仅开发用）")
	}
	if revStore != nil {
		opts = append(opts, auth.WithRevocations(revStore))
	}
	return auth.NewService(opts...)
}

// tunnelsService 取隧道模块的 Service（未启用返回 nil，handler 据此降级）
func tunnelsService(rt *tunnel.Runtime) *tunnel.Service {
	if rt == nil {
		return nil
	}
	return rt.Service
}
