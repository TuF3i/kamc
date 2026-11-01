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
	"gorm.io/gorm"

	"github.com/TuF3i/kamc/infrastructure/postgres"
	"github.com/TuF3i/kamc/internal/platform/dao"
	"github.com/TuF3i/kamc/internal/platform/handler"
	"github.com/TuF3i/kamc/internal/platform/middleware"
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
	db     *gorm.DB // 依赖的数据库连接 (自建时 Release 负责关闭)
	ownDB  bool     // db 是否为 CreateRuntime 自建 (决定 Release 是否关闭)
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

	// 依赖注入链: Dao → Service → Handler
	d := dao.NewDao(db)
	if err := d.SetupCluster(context.Background()); err != nil {
		return nil, fmt.Errorf("platform -> setup cluster table: %w", err)
	}
	svc := service.NewService(d)
	h := handler.NewHandler(svc)

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
