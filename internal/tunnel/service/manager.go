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
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/hashicorp/yamux"

	"github.com/TuF3i/kamc/internal/metrics"
	"github.com/TuF3i/kamc/internal/tunnel/registry"
)

// ErrClusterNotConnected 目标集群当前在本副本上没有任何在线隧道。
// 跨 Pod 场景下调用方应再走注册表路由（设计文档 §7.2.4），而非直接报错给用户。
var ErrClusterNotConnected = errors.New("cluster not connected")

// gracefulCloser 支持"带关闭码的优雅关闭"的连接（*wsconn.Conn 满足）。
// 平台优雅下线经此发送 1001 GoingAway，Agent 语义化感知并立即重连。
type gracefulCloser interface {
	CloseWithCode(code int, reason string) error
}

// Tunnel 一条在线的 Agent 反向隧道
type Tunnel struct {
	ClusterID string
	AgentID   string
	Session   *yamux.Session // Open() 得到 yamux.Stream（net.Conn），即数据面
	Since     time.Time

	closeWith gracefulCloser  // 底层 WS 连接（Attach 时类型断言；peer 连接为 nil）
	activity  activityTracker // 读活性追踪（Attach 时类型断言；无则看门狗跳过）
}

// Option Manager 选项
type Option func(*options)

type options struct {
	podID            string
	ttl              time.Duration // 注册表 TTL（失联判定阈值）
	interval         time.Duration // 心跳上报间隔
	detachHook       func(clusterID, agentID string)
	watchdogInterval time.Duration // 读活性检查周期（0 = 默认 25s）
	watchdogTimeout  time.Duration // 无入站帧判定死亡阈值（0 = 默认 75s）
}

// WithWatchdog 覆盖读活性看门狗参数（§6.3：默认检查 25s / 超时 75s = 3×Agent ping + 余量）
func WithWatchdog(interval, timeout time.Duration) Option {
	return func(o *options) { o.watchdogInterval, o.watchdogTimeout = interval, timeout }
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
	peer     *PeerRouter // 跨副本转发路由（NewPeerRouter 挂载；nil = 单副本形态）

	watchdogIntvl   time.Duration
	watchdogTimeout time.Duration

	detachMu   sync.Mutex
	detachHook func(clusterID, agentID string) // 隧道移除回调（平台据此标记集群 offline）

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
	wi, wt := o.watchdogInterval, o.watchdogTimeout
	if wi <= 0 {
		wi = 25 * time.Second
	}
	if wt <= 0 {
		wt = 75 * time.Second
	}
	return &Manager{
		clusters:        make(map[string][]*Tunnel),
		registry:        reg,
		podID:           o.podID,
		ttl:             o.ttl,
		interval:        o.interval,
		detachHook:      o.detachHook,
		watchdogIntvl:   wi,
		watchdogTimeout: wt,
		done:            make(chan struct{}),
	}
}

// WithDetachHook 隧道移除回调（每条隧道断开时调用一次）
func WithDetachHook(fn func(clusterID, agentID string)) Option {
	return func(o *options) { o.detachHook = fn }
}

// SetDetachHook 事后装配回调（模块间装配顺序晚于 NewManager 时使用；须在 Attach 之前）
func (m *Manager) SetDetachHook(fn func(clusterID, agentID string)) {
	m.detachMu.Lock()
	defer m.detachMu.Unlock()
	m.detachHook = fn
}

func (m *Manager) notifyDetach(clusterID, agentID string) {
	m.detachMu.Lock()
	fn := m.detachHook
	m.detachMu.Unlock()
	if fn != nil {
		fn(clusterID, agentID)
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
	m.wg.Add(2)
	go m.heartbeatLoop()
	go m.watchdogLoop()
}

// watchdogLoop 读活性看门狗（§6.3）：Agent 半开死亡（网络静默断）时入站帧
// 停止，超时即判定死亡并关闭——经既有 monitorSession→Remove→分离钩子链路
// 触发注销。仅检查实现 activityTracker 的连接（wsconn 满足），无追踪器跳过。
func (m *Manager) watchdogLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.watchdogIntvl)
	defer ticker.Stop()
	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.reapIdleTunnels()
		}
	}
}

func (m *Manager) reapIdleTunnels() {
	deadline := time.Now().Add(-m.watchdogTimeout)
	m.mu.RLock()
	var idle []*Tunnel
	for _, list := range m.clusters {
		for _, t := range list {
			if tr, ok := any(t.activity).(activityTracker); ok && tr.LastActivity().Before(deadline) {
				idle = append(idle, t)
			}
		}
	}
	m.mu.RUnlock()
	for _, t := range idle {
		log.Printf("[tunnel] 看门狗：隧道 %s/%s 超过 %s 无入站帧，判定死亡",
			t.ClusterID, t.AgentID, m.watchdogTimeout)
		m.closeTunnel(t)
	}
}

// activityTracker 读活性追踪（*wsconn.Conn 满足）
type activityTracker interface {
	LastActivity() time.Time
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
	m.register(clusterID, agentID, sess, &Tunnel{ClusterID: clusterID, AgentID: agentID, Session: sess, Since: time.Now()})
}

// register 登记的具体实现（Attach 与 Register 共用）
func (m *Manager) register(clusterID, agentID string, sess *yamux.Session, t *Tunnel) {
	m.mu.Lock()
	list := m.clusters[clusterID]
	for i, old := range list {
		if old.AgentID == agentID {
			m.closeTunnel(old)
			list[i] = t
			m.mu.Unlock()
			m.reportOne(clusterID, agentID)
			return
		}
	}
	m.clusters[clusterID] = append(list, t)
	m.mu.Unlock()

	metrics.TunnelAttachedAgents.Set(float64(len(m.clustersAll())))
	metrics.TunnelConnectedClusters.Set(float64(len(m.clusters)))
	m.reportOne(clusterID, agentID)
}

// closeTunnel 关闭单条隧道：优雅下线路径先发 1001（§7.5），
// 语义化告知 Agent"服务端计划下线，立即重连其他副本"。
func (m *Manager) closeTunnel(t *Tunnel) {
	if t.closeWith != nil {
		_ = t.closeWith.CloseWithCode(1001, "going away")
	}
	_ = t.Session.Close()
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
	metrics.TunnelAttachedAgents.Set(float64(m.countTunnels()))
	metrics.TunnelConnectedClusters.Set(float64(len(m.clusters)))
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := m.registry.Deregister(ctx, clusterID, agentID); err != nil {
		log.Printf("[tunnel] deregister %s/%s: %v", clusterID, agentID, err)
	}
	m.notifyDetach(clusterID, agentID)
}

// Attach 接管一条已升级的 Agent 连接：建立服务端 yamux 会话 → 登记 → 启动监控。
// rwc 的所有权随之转移（由会话持有；错误路径下由本方法负责关闭）。
// 数据面方向：平台经 Session.Open() 向 Agent 开流（Agent 侧为 Accept 方）。
func (m *Manager) Attach(clusterID, agentID string, rwc io.ReadWriteCloser) error {
	session, err := yamux.Server(rwc, nil)
	if err != nil {
		_ = rwc.Close()
		return fmt.Errorf("tunnel -> yamux server: %w", err)
	}

	t := &Tunnel{ClusterID: clusterID, AgentID: agentID, Session: session, Since: time.Now()}
	if gc, ok := rwc.(gracefulCloser); ok {
		t.closeWith = gc
	}
	if at, ok := rwc.(activityTracker); ok {
		t.activity = at
	}
	m.register(clusterID, agentID, session, t)
	m.wg.Add(1)
	go m.monitorSession(clusterID, agentID, session)
	return nil
}

// monitorSession 监控会话存活：Agent 不应向平台开流（数据面由平台发起），
// 收到入流属异常，防御性关闭；Accept 出错即会话死亡 → 注销并触发分离回调。
func (m *Manager) monitorSession(clusterID, agentID string, session *yamux.Session) {
	defer m.wg.Done()
	for {
		stream, err := session.Accept()
		if err != nil {
			log.Printf("[tunnel] 会话关闭 %s/%s: %v", clusterID, agentID, err)
			m.Remove(clusterID, agentID)
			return
		}
		_ = stream.Close()
	}
}

// Open 取集群的数据流连接（设计文档 §6.2 Dialer 模型的拨号入口）：
// 本地有隧道直接开流；否则经 PeerRouter 跨副本 RELAY（§7.2.4）。
func (m *Manager) Open(clusterID string) (net.Conn, error) {
	if conn, err := m.openLocal(clusterID); err == nil {
		return conn, nil
	}
	if m.peer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return m.peer.DialCluster(ctx, clusterID)
	}
	return nil, fmt.Errorf("cluster %s: %w", clusterID, ErrClusterNotConnected)
}

// openLocal 仅本机隧道开流（PeerRouter 的中继出口必须用它，
// 避免递归触发跨副本转发）
func (m *Manager) openLocal(clusterID string) (net.Conn, error) {
	t, err := m.Get(clusterID)
	if err != nil {
		return nil, err
	}
	conn, err := t.Session.Open()
	if err != nil {
		return nil, fmt.Errorf("tunnel -> open stream %s: %w", clusterID, err)
	}
	return conn, nil
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
// 顺序：close(done) 停心跳 → 等循环退出 → 关 peer 路由 → 清理会话与注册。
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
	if m.peer != nil {
		m.peer.Close()
		m.peer.waitExit()
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
			m.closeTunnel(t)
			if err := m.registry.Deregister(ctx, cid, t.AgentID); err != nil {
				log.Printf("[tunnel] release deregister %s/%s: %v", cid, t.AgentID, err)
			}
		}
		delete(m.clusters, cid)
	}
	metrics.TunnelAttachedAgents.Set(0)
	metrics.TunnelConnectedClusters.Set(0)
}

// clustersAll 汇总全部隧道（指标观测用；调用方须已持有 mu）
func (m *Manager) clustersAll() []*Tunnel {
	var all []*Tunnel
	for _, list := range m.clusters {
		all = append(all, list...)
	}
	return all
}

// countTunnels 隧道总数（调用方须已持有 mu）
func (m *Manager) countTunnels() int {
	n := 0
	for _, list := range m.clusters {
		n += len(list)
	}
	return n
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
