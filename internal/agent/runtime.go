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
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/TuF3i/kamc/internal/agent/service"
	"github.com/TuF3i/kamc/internal/config"
	"go.uber.org/zap"
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
	cfgSet        bool
	upstreamURL   *url.URL
	upstreamTrans http.RoundTripper
}

// WithConfig 显式注入配置（不注入则 LoadConfig 读环境变量）
func WithConfig(cfg Config) Option {
	return func(o *options) { o.cfg, o.cfgSet = cfg, true }
}

// WithUpstream 显式注入上游 kube-apiserver（测试用；默认经 BuildUpstream 解析）
func WithUpstream(u *url.URL, tr http.RoundTripper) Option {
	return func(o *options) { o.upstreamURL, o.upstreamTrans = u, tr }
}

// defaultRuntime 包级默认运行时 (CreateRuntime 记录, ReleaseRuntime 消费)
var defaultRuntime *Runtime

// CreateRuntime 装配模块：配置 → 上游解析 → 校验 → 连接服务启动。
func CreateRuntime(opts ...Option) (*Runtime, error) {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}
	if !o.cfgSet {
		cfg, err := LoadConfig()
		if err != nil {
			return nil, fmt.Errorf("agent -> load config: %w", err)
		}
		o.cfg = cfg
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
			zap.S().Infof("[agent] 上游解析失败，数据面未启用: %v", err)
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

// LoadConfig 装载 Agent 配置（默认值 → 配置文件 → 环境变量）。
// path 为空时读 AM_CONFIG_FILE，再按 kamc-agent 搜索默认路径。
func LoadConfig(paths ...string) (Config, error) {
	path := ""
	if len(paths) > 0 {
		path = paths[0]
	}
	l := config.New()
	l.SetDefault("agent.id", hostname())
	l.SetDefault("agent.token_file", "/etc/k8s-am/token/token")
	l.SetDefault("agent.heartbeat_ping_interval", 20*time.Second)
	l.SetDefault("agent.backoff_base", time.Second)
	l.SetDefault("agent.backoff_max", 30*time.Second)

	l.Bind("agent.platform_url", "PLATFORM_URL")
	l.Bind("agent.cluster_id", "CLUSTER_ID")
	l.Bind("agent.id", "AGENT_ID")
	l.Bind("agent.token_file", "TOKEN_FILE")
	l.Bind("agent.heartbeat_ping_interval", "HEARTBEAT_PING_INTERVAL")
	l.Bind("agent.backoff_base", "BACKOFF_BASE")
	l.Bind("agent.backoff_max", "BACKOFF_MAX")
	l.Bind("agent.platform_insecure", "PLATFORM_INSECURE")
	l.Bind("agent.apiserver_url", "APISERVER_URL")
	l.Bind("agent.sa_ca_file", "SA_CA_FILE")
	l.Bind("agent.sa_token_file", "SA_TOKEN_FILE")

	if err := l.LoadFile(path, "kamc-agent"); err != nil {
		return Config{}, err
	}
	return Config{
		PlatformURL:  l.String("agent.platform_url"),
		ClusterID:    l.String("agent.cluster_id"),
		AgentID:      l.String("agent.id"),
		TokenFile:    l.String("agent.token_file"),
		PingInterval: l.Duration("agent.heartbeat_ping_interval", 20*time.Second),
		BackoffBase:  l.Duration("agent.backoff_base", time.Second),
		BackoffMax:   l.Duration("agent.backoff_max", 30*time.Second),
		APIServerURL: l.String("agent.apiserver_url"),
		SACAFile:     l.String("agent.sa_ca_file"),
		SATokenFile:  l.String("agent.sa_token_file"),
		InsecureTLS:  l.Bool("agent.platform_insecure", false),
	}, nil
}

func hostname() string {
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return "unknown-agent"
}
