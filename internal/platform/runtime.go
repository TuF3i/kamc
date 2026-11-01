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
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/TuF3i/kamc/infrastructure/postgres"
	"github.com/TuF3i/kamc/infrastructure/redis"
	"github.com/TuF3i/kamc/internal/auth"
	"github.com/TuF3i/kamc/internal/platform/authz"
	"github.com/TuF3i/kamc/internal/platform/dao"
	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/handler"
	"github.com/TuF3i/kamc/internal/platform/middleware"
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

	engine *chi.Mux
	srv    *http.Server
	db     *gorm.DB                // 依赖的数据库连接 (自建时 Release 负责关闭)
	ownDB  bool                    // db 是否为 CreateRuntime 自建 (决定 Release 是否关闭)
	revCli goredis.UniversalClient // 吊销存储的 Redis 客户端 (自建时 Release 负责关闭)
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
	authzEn, err := authz.New(d)
	if err != nil {
		return nil, fmt.Errorf("platform -> build authz: %w", err)
	}
	svc := service.NewService(d, authzEn)

	// 认证服务：密钥环（Secret 挂载）或开发环境临时密钥（重启失效，打警告）；
	// Token 吊销：Redis 可用时跨副本同步（惰性周期拉取，鉴权热路径只查本地
	// 缓存——§7.1 鉴权零外部依赖不变量），否则进程内语义（单副本）
	var revStore auth.RevocationStore
	var revCli goredis.UniversalClient
	if os.Getenv("REDIS_ADDRESSES") != "" || os.Getenv("REDIS_ADDR") != "" {
		revCli, err = redis.NewRedisClient()
		if err != nil {
			return nil, fmt.Errorf("platform -> connect redis (revocations): %w", err)
		}
		revStore = auth.NewRedisRevocations(revCli)
	}
	authSvc, err := buildAuthService(revStore)
	if err != nil {
		if revCli != nil {
			_ = revCli.Close()
		}
		return nil, fmt.Errorf("platform -> build auth: %w", err)
	}

	// OIDC 登录客户端：未配置时 /auth/login 降级 503（本地开发走 dev-token）
	oidcClient, err := oidc.New(oidc.Config{
		Issuer:       os.Getenv("OIDC_ISSUER"),
		ClientID:     os.Getenv("OIDC_CLIENT_ID"),
		ClientSecret: os.Getenv("OIDC_CLIENT_SECRET"),
		RedirectURL:  os.Getenv("OIDC_REDIRECT_URL"),
	})
	if err != nil && !errors.Is(err, oidc.ErrNotConfigured) {
		return nil, fmt.Errorf("platform -> build oidc: %w", err)
	}

	// API 代理池：依赖隧道模块；鉴权已挂载（JWT），无开关依赖
	var proxies *proxy.Pool
	if o.tunnels != nil {
		proxies = proxy.NewPool(o.tunnels.Service.Open)
	}

	// kubeconfig 生成配置：平台 CA（可选嵌入）与 exec 插件参数
	var caData []byte
	if caFile := os.Getenv("AM_KUBECONFIG_CA_FILE"); caFile != "" {
		caData, err = os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("platform -> read kubeconfig ca: %w", err)
		}
	}

	h := handler.NewHandler(svc, handler.Deps{
		Auth:         authSvc,
		OIDC:         oidcClient,
		Tunnels:      tunnelsService(o.tunnels),
		Proxies:      proxies,
		CAData:       caData,
		OIDCIssuer:   os.Getenv("OIDC_ISSUER"),
		OIDCClientID: os.Getenv("OIDC_CLIENT_ID"),
	})
	h.SetPolicy(envDefault("AM_ADMIN_GROUP", "am:admin"), os.Getenv("AM_DEV_ISSUE_TOKEN") == "1")

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
		Service: svc,
		Handler: h,
		Tunnels: o.tunnels,
		engine:  engine,
		srv:     srv,
		db:      db,
		ownDB:   ownDB,
		revCli:  revCli,
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
	engine.Use(middleware.AccessLog)
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

	if r.ownDB && r.db != nil {
		if sqlDB, err := r.db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}

	if r.revCli != nil {
		_ = r.revCli.Close()
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
