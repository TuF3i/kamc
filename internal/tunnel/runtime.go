// Package tunnel 隧道模块的组装层：管理平台侧全部 Agent 反向隧道的
// 登记、择优与归属上报（数据面为 yamux stream，经 internal/wsconn 承载）。
//
// 分层 (与项目其他模块同构):
//
//	Runtime (本文件, 只负责组装与生命周期) ──▶ service (隧道管理器) ──▶ registry (Redis 注册表)
//
// 使用: CreateRuntime(WithRedis(cli), WithPodID(pod)) → rt.Service.Register/Get
// → ReleaseRuntime() 释放（停心跳 + 下线归属 + 关闭全部会话）。
//
// Agent 侧的 WebSocket 拨出与 yamux 会话建立见 internal/agent；
// 跨 Pod 转发（peer yamux 拼接）见设计文档 §7.2.4，属后续路线图。
package tunnel

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/TuF3i/kamc/infrastructure/redis"
	"github.com/TuF3i/kamc/internal/tunnel/registry"
	"github.com/TuF3i/kamc/internal/tunnel/service"
)

// Service 服务类型别名: 供其他模块 (platform 等) 经本包引用,
// 不深入 service 内部包 (跨模块只经对方 Runtime 暴露的 API 交互).
type Service = service.Manager

// Runtime 模块运行时: 只负责组装与创建/释放, 业务方法全部在 Service 上.
type Runtime struct {
	Service *service.Manager

	cli      goredis.UniversalClient // 依赖的 Redis 客户端 (自建时 Release 负责关闭)
	ownCLI   bool                    // cli 是否为 CreateRuntime 自建 (决定 Release 是否关闭)
	peerSrv  *http.Server            // 副本间 peer 内部服务 (PEER 配置存在时启动)
	peerAddr net.Addr                // peer 实际监听地址
}

// PeerAddr peer 服务实际监听地址（未启用返回 nil）
func (r *Runtime) PeerAddr() net.Addr {
	if r == nil {
		return nil
	}
	return r.peerAddr
}

// Option CreateRuntime 选项
type Option func(*options)

type options struct {
	cli      goredis.UniversalClient
	registry registry.Registry // 测试可注入 fake；默认 Redis 实现
	podID    string
	ttl      time.Duration
	interval time.Duration
	peers    service.PeerOptions // 副本间转发（WithPeers / 环境变量）
	peersSet bool
}

// WithPeers 显式注入副本间转发配置（默认经 PeerConfigFromEnv 读环境变量）
func WithPeers(cfg service.PeerOptions) Option {
	return func(o *options) { o.peers, o.peersSet = cfg, true }
}

// WithRedis 注入已有 Redis 客户端 (不注入则使用 infrastructure 默认连接并自持)
func WithRedis(cli goredis.UniversalClient) Option {
	return func(o *options) { o.cli = cli }
}

// WithRegistry 注入注册表实现 (仅测试使用)
func WithRegistry(reg registry.Registry) Option {
	return func(o *options) { o.registry = reg }
}

// WithPodID 本副本标识（默认 hostname；K8s 内即 Pod 名）
func WithPodID(pod string) Option {
	return func(o *options) { o.podID = pod }
}

// WithHeartbeat 覆盖注册表 TTL 与心跳间隔
func WithHeartbeat(ttl, interval time.Duration) Option {
	return func(o *options) { o.ttl, o.interval = ttl, interval }
}

// defaultRuntime 包级默认运行时 (CreateRuntime 记录, ReleaseRuntime 消费)
var defaultRuntime *Runtime

// CreateRuntime 装配模块:
// Redis 连接 (可注入) → 注册表 → 隧道管理器 → 启动心跳。
func CreateRuntime(opts ...Option) (*Runtime, error) {
	o := &options{ttl: 15 * time.Second, interval: 5 * time.Second}
	for _, opt := range opts {
		opt(o)
	}

	// Redis 连接: 注入优先, 否则自建 (自建的连接由 ReleaseRuntime 负责);
	// 已注入 Registry 实现（测试路径）时无需 Redis。
	cli := o.cli
	ownCLI := false
	if cli == nil && o.registry == nil {
		var err error
		cli, err = redis.NewRedisClient()
		if err != nil {
			return nil, fmt.Errorf("tunnel -> connect redis: %w", err)
		}
		ownCLI = true
	}

	// 依赖注入链: Registry → Manager
	reg := o.registry
	if reg == nil {
		reg = registry.NewRedisRegistry(cli, o.ttl)
	}
	if o.podID == "" {
		if host, err := os.Hostname(); err == nil {
			o.podID = host
		}
	}

	m := service.NewManager(reg,
		service.WithPodID(o.podID),
		service.WithHeartbeat(o.ttl, o.interval),
	)
	m.Start()

	rt := &Runtime{Service: m, cli: cli, ownCLI: ownCLI}

	// 副本间转发（§7.2.4）：配置存在时挂载 PeerRouter 并启动内部监听
	if o.peersSet {
		router := service.NewPeerRouter(m, reg, o.podID, o.peers)
		if o.peers.ListenAddr != "" {
			ln, err := net.Listen("tcp", o.peers.ListenAddr)
			if err != nil {
				router.Close()
				_ = rt.ReleaseRuntime()
				return nil, fmt.Errorf("tunnel -> peer listen %s: %w", o.peers.ListenAddr, err)
			}
			rt.peerSrv = &http.Server{Handler: router.Handler(), ReadHeaderTimeout: 5 * time.Second}
			rt.peerAddr = ln.Addr()
			go func() {
				if err := rt.peerSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
					log.Printf("[tunnel] peer server: %v", err)
				}
			}()
		}
	}

	defaultRuntime = rt
	log.Printf("[tunnel] runtime created (pod=%s own_redis=%v peers=%v)",
		o.podID, ownCLI, o.peersSet)
	return rt, nil
}

// ReleaseRuntime 释放运行时: 停止心跳、下线归属、关闭会话与自建的 Redis 连接 (幂等).
func (r *Runtime) ReleaseRuntime() error {
	if r == nil {
		return nil
	}
	r.Service.Release()

	if r.peerSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = r.peerSrv.Shutdown(ctx)
	}

	if r.ownCLI && r.cli != nil {
		_ = r.cli.Close()
	}

	if defaultRuntime == r {
		defaultRuntime = nil
	}
	log.Printf("[tunnel] runtime released")
	return nil
}

// ReleaseRuntime 释放包级默认运行时 (CreateRuntime 记录的那个), 幂等
func ReleaseRuntime() error {
	if defaultRuntime == nil {
		return nil
	}
	return defaultRuntime.ReleaseRuntime()
}

// PeerConfigFromEnv 从环境变量装载副本间转发配置：
//
//	PEER_ADDRESSES  逗号分隔的可拨副本（"podName=host:port" 或 "host:port"）
//	PEER_TOKEN      peer 认证令牌（Secret 下发）
//	AM_PEER_ADDR    本机 peer 监听地址（默认 :9090，仅集群内网可达）
//
// 未设置 PEER_ADDRESSES 返回 false（单副本形态，不启用跨副本转发）。
func PeerConfigFromEnv() (service.PeerOptions, bool) {
	addrs := os.Getenv("PEER_ADDRESSES")
	if addrs == "" {
		return service.PeerOptions{}, false
	}
	return service.PeerOptions{
		Token:      os.Getenv("PEER_TOKEN"),
		ListenAddr: envDefault("AM_PEER_ADDR", ":9090"),
		Addresses:  service.ParsePeerAddresses(strings.Split(addrs, ",")),
	}, true
}

// envDefault 读环境变量，空值取默认
func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
