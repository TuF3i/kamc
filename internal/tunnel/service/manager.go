// Package service 平台侧隧道管理器：登记每条 Agent 反向隧道，
// 按 clusterID 提供本地数据面入口，并周期性向注册表上报归属（供跨 Pod 路由）。
//
// 职责边界：本包只管"隧道在不在、归谁"，不做任何转发与权限判断；
// 数据面经 Tunnel.Session.Open() 取 yamux stream（net.Conn），由上层组装
// http.Transport（设计文档 §6.2 Dialer 模型）。
package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/hashicorp/yamux"

	"github.com/TuF3i/kamc/internal/tunnel/registry"
)

// ErrClusterNotConnected 目标集群当前在本副本上没有任何在线隧道。
// 跨 Pod 场景下调用方应再走注册表路由（设计文档 §7.2.4），而非直接报错给用户。
var ErrClusterNotConnected = errors.New("cluster not connected")

// Tunnel 一条在线的 Agent 反向隧道
type Tunnel struct {
	ClusterID string
	AgentID   string
	Session   *yamux.Session // Open() 得到 yamux.Stream（net.Conn），即数据面
	Since     time.Time
}

// Option Manager 选项
type Option func(*options)

type options struct {
	podID    string
	ttl      time.Duration // 注册表 TTL（失联判定阈值）
	interval time.Duration // 心跳上报间隔
}

// WithPodID 本副本标识（默认 hostname；K8s 内即 Pod 名）
func WithPodID(pod string) Option {
	return func(o *options) { o.podID = pod }
}

// WithHeartbeat 覆盖心跳参数（默认 ttl=15s / interval=5s，见设计文档 §7.2.3）
func WithHeartbeat(ttl, interval time.Duration) Option {
	return func(o *options) { o.ttl, o.interval = ttl, interval }
}

// Manager 隧道管理器
type Manager struct {
	mu       sync.RWMutex
	clusters map[string][]*Tunnel // clusterID → 按注册顺序的隧道列表（Get 取第一条）

	registry registry.Registry
	podID    string
	ttl      time.Duration
	interval time.Duration

	done    chan struct{}
	wg      sync.WaitGroup
	startMu sync.Mutex
	started bool
}

// NewManager 构造管理器（Start 由调用方显式触发，便于测试）
func NewManager(reg registry.Registry, opts ...Option) *Manager {
	o := &options{ttl: 15 * time.Second, interval: 5 * time.Second}
	for _, opt := range opts {
		opt(o)
	}
	if o.podID == "" {
		o.podID = "unknown-pod"
	}
	return &Manager{
		clusters: make(map[string][]*Tunnel),
		registry: reg,
		podID:    o.podID,
		ttl:      o.ttl,
		interval: o.interval,
		done:     make(chan struct{}),
	}
}

// Start 启动心跳上报循环（幂等）
func (m *Manager) Start() {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if m.started {
		return
	}
	m.started = true
	m.wg.Add(1)
	go m.heartbeatLoop()
}

// heartbeatLoop 周期性续报全部隧道归属；单循环串行上报，
// 失败只记一行日志（下一轮会再试，注册表键过期属自愈语义而非故障）。
func (m *Manager) heartbeatLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.reportAll()
		}
	}
}

// reportAll 一轮全量续报
func (m *Manager) reportAll() {
	m.mu.RLock()
	type pair struct{ clusterID, agentID string }
	var snapshot []pair
	for cid, list := range m.clusters {
		for _, t := range list {
			snapshot = append(snapshot, pair{cid, t.AgentID})
		}
	}
	m.mu.RUnlock()
	if len(snapshot) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), m.interval)
	defer cancel()
	for _, p := range snapshot {
		if err := m.registry.Report(ctx, p.clusterID, p.agentID, m.podID); err != nil {
			log.Printf("[tunnel] report %s/%s: %v", p.clusterID, p.agentID, err)
		}
	}
}

// Register 登记隧道：同一 Agent 的旧会话立即关闭并被替换
// （Agent 重连前必然已断开旧连接，这里兜底防泄漏）。
// 登记即上报一次归属；失败不回滚——心跳会在下一轮补报（"先落库再动内存"不适用此场景，
// 注册表是可再生的派生状态）。
func (m *Manager) Register(clusterID, agentID string, sess *yamux.Session) {
	m.mu.Lock()
	list := m.clusters[clusterID]
	for i, t := range list {
		if t.AgentID == agentID {
			_ = t.Session.Close()
			list[i] = &Tunnel{ClusterID: clusterID, AgentID: agentID, Session: sess, Since: time.Now()}
			m.mu.Unlock()
			m.reportOne(clusterID, agentID)
			return
		}
	}
	m.clusters[clusterID] = append(list, &Tunnel{ClusterID: clusterID, AgentID: agentID, Session: sess, Since: time.Now()})
	m.mu.Unlock()

	m.reportOne(clusterID, agentID)
}

// Get 取集群的可用隧道：返回最先注册的一条。
// Agent 双副本场景下另一条隧道由跨 Pod 路由按注册表择优，本副本不感知（§7.4）。
func (m *Manager) Get(clusterID string) (*Tunnel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := m.clusters[clusterID]
	if len(list) == 0 {
		return nil, fmt.Errorf("cluster %s: %w", clusterID, ErrClusterNotConnected)
	}
	return list[0], nil
}

// Remove 注销隧道（Agent 连接关闭时的回调），并从注册表摘除。
func (m *Manager) Remove(clusterID, agentID string) {
	m.mu.Lock()
	list := m.clusters[clusterID]
	for i, t := range list {
		if t.AgentID != agentID {
			continue
		}
		_ = t.Session.Close()
		list = append(list[:i], list[i+1:]...)
		break
	}
	if len(list) == 0 {
		delete(m.clusters, clusterID)
	} else {
		m.clusters[clusterID] = list
	}
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := m.registry.Deregister(ctx, clusterID, agentID); err != nil {
		log.Printf("[tunnel] deregister %s/%s: %v", clusterID, agentID, err)
	}
}

// reportOne 单条归属上报（2s 超时）
func (m *Manager) reportOne(clusterID, agentID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := m.registry.Report(ctx, clusterID, agentID, m.podID); err != nil {
		log.Printf("[tunnel] report %s/%s: %v", clusterID, agentID, err)
	}
}

// Release 停止心跳、下线全部归属并关闭全部会话（幂等）。
// 顺序：close(done) 停心跳 → 等循环退出 → 清理会话与注册。
func (m *Manager) Release() {
	m.startMu.Lock()
	select {
	case <-m.done:
		// 已释放
	default:
		close(m.done)
	}
	started := m.started
	m.startMu.Unlock()

	if started {
		waitTimeout(&m.wg, 3*time.Second)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.clusters) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for cid, list := range m.clusters {
		for _, t := range list {
			_ = t.Session.Close()
			if err := m.registry.Deregister(ctx, cid, t.AgentID); err != nil {
				log.Printf("[tunnel] release deregister %s/%s: %v", cid, t.AgentID, err)
			}
		}
		delete(m.clusters, cid)
	}
}

// waitTimeout 带超时等待 wg（约定：一切等待必须带超时）
func waitTimeout(wg *sync.WaitGroup, timeout time.Duration) {
	ch := make(chan struct{})
	go func() {
		wg.Wait()
		close(ch)
	}()
	select {
	case <-ch:
	case <-time.After(timeout):
	}
}
