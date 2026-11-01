// Package agent Agent 模块的组装层：驻留于目标集群，向平台拨出反向隧道
// （WebSocket 承载 + yamux 多路复用），并把隧道暴露为本机 HTTP 服务
// 转发至 kube-apiserver（数据面见设计文档 §6.2，属路线图）。
//
// 分层 (与项目其他模块同构):
//
//	Runtime (本文件, 只负责组装与生命周期) ──▶ service (连接循环/Token 轮询/流处理)
//
// 使用: CreateRuntime(WithConfig(cfg)) 自动开始连接（配置缺省经环境变量装载）
// → ReleaseRuntime() 优雅断开。
package agent

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/TuF3i/kamc/internal/agent/service"
)

// Config Agent 运行配置
type Config = service.Config

// Service 服务类型别名: 供其他模块经本包引用
type Service = service.Service

// Runtime 模块运行时: 只负责组装与创建/释放, 业务方法全部在 Service 上.
type Runtime struct {
	Service *service.Service
}

// Option CreateRuntime 选项
type Option func(*options)

type options struct {
	cfg           Config
	upstreamURL   *url.URL
	upstreamTrans http.RoundTripper
}

// WithConfig 显式注入配置（不注入则 LoadConfig 读环境变量）
func WithConfig(cfg Config) Option {
	return func(o *options) { o.cfg = cfg }
}

// WithUpstream 显式注入上游 kube-apiserver（测试用；默认经 BuildUpstream 解析）
func WithUpstream(u *url.URL, tr http.RoundTripper) Option {
	return func(o *options) { o.upstreamURL, o.upstreamTrans = u, tr }
}

// defaultRuntime 包级默认运行时 (CreateRuntime 记录, ReleaseRuntime 消费)
var defaultRuntime *Runtime

// CreateRuntime 装配模块：配置 → 上游解析 → 校验 → 连接服务启动。
func CreateRuntime(opts ...Option) (*Runtime, error) {
	o := &options{cfg: LoadConfig()}
	for _, opt := range opts {
		opt(o)
	}

	if err := o.cfg.Validate(); err != nil {
		return nil, fmt.Errorf("agent -> validate config: %w", err)
	}

	svc := service.NewService(o.cfg)
	if o.upstreamURL != nil && o.upstreamTrans != nil {
		svc.SetUpstream(o.upstreamURL, o.upstreamTrans)
	} else {
		// 上游解析失败不阻塞隧道保活（仅数据面不可用）
		u, tr, err := service.BuildUpstream(o.cfg)
		switch {
		case err != nil:
			log.Printf("[agent] 上游解析失败，数据面未启用: %v", err)
		case u != nil:
			svc.SetUpstream(u, tr)
		}
	}

	if err := svc.Start(); err != nil {
		return nil, fmt.Errorf("agent -> start service: %w", err)
	}

	rt := &Runtime{Service: svc}
	defaultRuntime = rt
	return rt, nil
}

// ReleaseRuntime 优雅断开（幂等）
func (r *Runtime) ReleaseRuntime() error {
	if r == nil {
		return nil
	}
	r.Service.Release()
	if defaultRuntime == r {
		defaultRuntime = nil
	}
	return nil
}

// ReleaseRuntime 释放包级默认运行时 (CreateRuntime 记录的那个), 幂等
func ReleaseRuntime() error {
	if defaultRuntime == nil {
		return nil
	}
	return defaultRuntime.ReleaseRuntime()
}

// LoadConfig 从环境变量装载配置（必填项由 Config.Validate 校验）
func LoadConfig() Config {
	return Config{
		PlatformURL:  os.Getenv("PLATFORM_URL"),
		ClusterID:    os.Getenv("CLUSTER_ID"),
		AgentID:      envDefault("AGENT_ID", hostname()),
		TokenFile:    envDefault("TOKEN_FILE", "/etc/k8s-am/token/token"),
		PingInterval: envDuration("HEARTBEAT_PING_INTERVAL", 20*time.Second),
		BackoffBase:  envDuration("BACKOFF_BASE", 1*time.Second),
		BackoffMax:   envDuration("BACKOFF_MAX", 30*time.Second),
		APIServerURL: os.Getenv("APISERVER_URL"), // 显式覆盖；空则探测 in-cluster SA
		SACAFile:     os.Getenv("SA_CA_FILE"),
		SATokenFile:  os.Getenv("SA_TOKEN_FILE"),
	}
}

func hostname() string {
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return "unknown-agent"
}

func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		log.Printf("[agent] 无效的 %s=%q，取默认 %s", key, v, def)
		return def
	}
	return d
}
