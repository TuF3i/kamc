package service

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/yamux"

	"github.com/TuF3i/kamc/internal/tunnel/registry"
)

// fakeRegistry 内存注册表（接口注入，验证 Manager 的上报/注销行为）
type fakeRegistry struct {
	mu     sync.Mutex
	report map[string]int // "cluster/agent" → 上报次数
	dereg  []string
}

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{report: make(map[string]int)}
}

func (f *fakeRegistry) Report(_ context.Context, clusterID, agentID, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.report[clusterID+"/"+agentID]++
	return nil
}

func (f *fakeRegistry) Deregister(_ context.Context, clusterID, agentID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dereg = append(f.dereg, clusterID+"/"+agentID)
	return nil
}

func (f *fakeRegistry) Lookup(_ context.Context, _ string) ([]registry.Owner, error) {
	return nil, nil
}

func (f *fakeRegistry) reportCount(clusterID, agentID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.report[clusterID+"/"+agentID]
}

// newYamuxPair 经 net.Pipe 建一对真实 yamux 会话（客户端/客户端即可满足测试需要）
func newYamuxPair(t *testing.T) (*yamux.Session, *yamux.Session) {
	t.Helper()
	p1, p2 := net.Pipe()
	s1, err := yamux.Client(p1, nil)
	if err != nil {
		t.Fatalf("yamux client 1: %v", err)
	}
	s2, err := yamux.Client(p2, nil)
	if err != nil {
		t.Fatalf("yamux client 2: %v", err)
	}
	t.Cleanup(func() { _ = s1.Close(); _ = s2.Close() })
	return s1, s2
}

// waitFor 带超时的条件等待（约定：一切等待必须带超时）
func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting: %s", msg)
}

func newTestManager(reg registry.Registry) *Manager {
	return NewManager(reg,
		WithPodID("pod-test"),
		WithHeartbeat(50*time.Millisecond, 20*time.Millisecond),
	)
}

func TestRegisterAndGet(t *testing.T) {
	reg := newFakeRegistry()
	m := newTestManager(reg)
	m.Start()
	defer m.Release()

	s1, _ := newYamuxPair(t)
	m.Register("cluster-a", "agent-1", s1)

	tun, err := m.Get("cluster-a")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if tun.Session != s1 || tun.AgentID != "agent-1" {
		t.Fatalf("unexpected tunnel: %+v", tun)
	}
}

func TestGetNotConnected(t *testing.T) {
	m := newTestManager(newFakeRegistry())
	_, err := m.Get("no-such-cluster")
	if !errors.Is(err, ErrClusterNotConnected) {
		t.Fatalf("want ErrClusterNotConnected, got %v", err)
	}
}

// TestReRegisterReplacesOld 同一 Agent 重连：旧会话必须关闭，Get 返回新会话。
func TestReRegisterReplacesOld(t *testing.T) {
	reg := newFakeRegistry()
	m := newTestManager(reg)
	m.Start()
	defer m.Release()

	s1, s2 := newYamuxPair(t)
	m.Register("cluster-a", "agent-1", s1)
	m.Register("cluster-a", "agent-1", s2)

	if !s1.IsClosed() {
		t.Fatal("old session should be closed on re-register")
	}
	tun, err := m.Get("cluster-a")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if tun.Session != s2 {
		t.Fatal("get should return the new session")
	}
	if got := len(m.clusters["cluster-a"]); got != 1 {
		t.Fatalf("tunnel list should hold 1 entry, got %d", got)
	}
}

// TestRemove 注销后 Get 报未连接，且注册表收到 Deregister。
func TestRemove(t *testing.T) {
	reg := newFakeRegistry()
	m := newTestManager(reg)
	m.Start()
	defer m.Release()

	s1, _ := newYamuxPair(t)
	m.Register("cluster-a", "agent-1", s1)
	m.Remove("cluster-a", "agent-1")

	if !s1.IsClosed() {
		t.Fatal("removed session should be closed")
	}
	if _, err := m.Get("cluster-a"); !errors.Is(err, ErrClusterNotConnected) {
		t.Fatalf("want ErrClusterNotConnected, got %v", err)
	}
	waitFor(t, time.Second, func() bool {
		reg.mu.Lock()
		defer reg.mu.Unlock()
		return len(reg.dereg) == 1
	}, "registry should receive one deregister")
}

// TestHeartbeatReports 心跳循环周期性续报归属。
func TestHeartbeatReports(t *testing.T) {
	reg := newFakeRegistry()
	m := newTestManager(reg)
	m.Start()
	defer m.Release()

	s1, _ := newYamuxPair(t)
	m.Register("cluster-a", "agent-1", s1)

	// 立即上报 1 次 + 心跳至少 1 轮 → ≥ 2
	waitFor(t, 2*time.Second, func() bool {
		return reg.reportCount("cluster-a", "agent-1") >= 2
	}, "heartbeat should re-report tunnel ownership")
}

// TestReleaseClosesSessions Release 后全部会话关闭且幂等。
func TestReleaseClosesSessions(t *testing.T) {
	reg := newFakeRegistry()
	m := newTestManager(reg)
	m.Start()

	s1, _ := newYamuxPair(t)
	m.Register("cluster-a", "agent-1", s1)

	m.Release()
	m.Release() // 幂等

	if !s1.IsClosed() {
		t.Fatal("release should close all sessions")
	}
	waitFor(t, time.Second, func() bool {
		reg.mu.Lock()
		defer reg.mu.Unlock()
		return len(reg.dereg) == 1
	}, "release should deregister all tunnels")
}

// TestAttachOpenAndDetach Attach 端到端：服务端会话建立 → Open 可与 Agent 侧
// 互传字节 → 会话断开后自动注销并触发分离回调。
func TestAttachOpenAndDetach(t *testing.T) {
	reg := newFakeRegistry()
	detached := make(chan string, 1)
	m := NewManager(reg,
		WithPodID("pod-test"),
		WithHeartbeat(50*time.Millisecond, 20*time.Millisecond),
		WithDetachHook(func(clusterID, agentID string) {
			detached <- clusterID + "/" + agentID
		}),
	)
	m.Start()
	defer m.Release()

	serverConn, clientConn := net.Pipe()
	if err := m.Attach("cluster-a", "agent-1", serverConn); err != nil {
		t.Fatalf("attach: %v", err)
	}
	client, err := yamux.Client(clientConn, nil)
	if err != nil {
		t.Fatalf("yamux client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	// 平台侧 Open → Agent 侧 Accept，双向可写
	open, err := m.Open("cluster-a")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	accepted, err := client.Accept()
	if err != nil {
		t.Fatalf("agent accept: %v", err)
	}
	if _, err := open.Write([]byte("ping")); err != nil {
		t.Fatalf("write over tunnel: %v", err)
	}
	buf := make([]byte, 4)
	_ = accepted.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := accepted.Read(buf); err != nil || string(buf) != "ping" {
		t.Fatalf("read over tunnel: %q err=%v", buf, err)
	}

	// 数据流关闭不影响会话；Agent 断开后才注销
	_ = open.Close()
	_ = accepted.Close()

	// Agent 侧断开 → monitor 注销 → 分离回调
	_ = client.Close()
	select {
	case got := <-detached:
		if got != "cluster-a/agent-1" {
			t.Fatalf("unexpected detach: %s", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("detach hook not fired after agent disconnect")
	}
	if _, err := m.Get("cluster-a"); !errors.Is(err, ErrClusterNotConnected) {
		t.Fatalf("cluster should be removed after detach, got %v", err)
	}
}

// TestAttachRegistersOwnership Attach 后注册表应收到归属上报。
func TestAttachRegistersOwnership(t *testing.T) {
	reg := newFakeRegistry()
	m := newTestManager(reg)
	m.Start()
	defer m.Release()

	serverConn, clientConn := net.Pipe()
	if err := m.Attach("cluster-b", "agent-2", serverConn); err != nil {
		t.Fatalf("attach: %v", err)
	}
	client, err := yamux.Client(clientConn, nil)
	if err != nil {
		t.Fatalf("yamux client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	waitFor(t, 2*time.Second, func() bool {
		return reg.reportCount("cluster-b", "agent-2") >= 1
	}, "attach should report ownership to registry")
}

// gracefulConn Attach 用的连接桩：记录优雅关闭码
type gracefulConn struct {
	net.Conn
	mu   sync.Mutex
	code int
}

func (g *gracefulConn) CloseWithCode(code int, _ string) error {
	g.mu.Lock()
	g.code = code
	g.mu.Unlock()
	return g.Conn.Close()
}

func (g *gracefulConn) closeCode() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.code
}

// TestReleaseSendsGoingAway 优雅下线（§7.5）：Release 对每条 Agent 隧道
// 发送 1001 GoingAway（语义化通知，与异常断链区分）。
func TestReleaseSendsGoingAway(t *testing.T) {
	reg := newFakeRegistry()
	m := newTestManager(reg)
	m.Start()

	p1, p2 := net.Pipe()
	gc := &gracefulConn{Conn: p1}
	if err := m.Attach("cluster-a", "agent-1", gc); err != nil {
		t.Fatalf("attach: %v", err)
	}
	client, err := yamux.Client(p2, nil)
	if err != nil {
		t.Fatalf("yamux client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	m.Release()
	if code := gc.closeCode(); code != 1001 {
		t.Fatalf("release close code: got %d, want 1001", code)
	}
}

// activityConn 带（可注入的）活性追踪的连接桩（原子保护，watchdog 并发读）
type activityConn struct {
	*gracefulConn
	mu       sync.Mutex
	activity func() time.Time
}

func (a *activityConn) setActivity(fn func() time.Time) {
	a.mu.Lock()
	a.activity = fn
	a.mu.Unlock()
}

func (a *activityConn) LastActivity() time.Time {
	a.mu.Lock()
	fn := a.activity
	a.mu.Unlock()
	if fn == nil {
		return time.Now()
	}
	return fn()
}

// TestWatchdogReapsIdleTunnel 看门狗：无入站帧超时 → 隧道被回收。
func TestWatchdogReapsIdleTunnel(t *testing.T) {
	reg := newFakeRegistry()
	m := NewManager(reg,
		WithPodID("pod-test"),
		WithHeartbeat(time.Hour, time.Hour),
		WithWatchdog(10*time.Millisecond, 50*time.Millisecond),
	)
	m.Start()
	defer m.Release()

	p1, p2 := net.Pipe()
	stale := time.Now().Add(-time.Hour)
	ac := &activityConn{gracefulConn: &gracefulConn{Conn: p1}, activity: func() time.Time { return stale }}
	if err := m.Attach("cluster-a", "agent-idle", ac); err != nil {
		t.Fatalf("attach: %v", err)
	}
	client, err := yamux.Client(p2, nil)
	if err != nil {
		t.Fatalf("yamux client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	waitFor(t, 2*time.Second, func() bool {
		_, err := m.Get("cluster-a")
		return err != nil
	}, "idle tunnel should be reaped by watchdog")
}

// TestWatchdogSparesActiveTunnel 活跃连接（活性实时刷新）不被误杀。
func TestWatchdogSparesActiveTunnel(t *testing.T) {
	reg := newFakeRegistry()
	m := NewManager(reg,
		WithPodID("pod-test"),
		WithHeartbeat(time.Hour, time.Hour),
		WithWatchdog(10*time.Millisecond, 50*time.Millisecond),
	)
	m.Start()
	defer m.Release()

	p1, p2 := net.Pipe()
	ac := &activityConn{gracefulConn: &gracefulConn{Conn: p1}, activity: time.Now}
	if err := m.Attach("cluster-a", "agent-active", ac); err != nil {
		t.Fatalf("attach: %v", err)
	}
	client, err := yamux.Client(p2, nil)
	if err != nil {
		t.Fatalf("yamux client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	// 远超超时阈值的观察窗口：活性持续刷新 → 存活
	time.Sleep(300 * time.Millisecond)
	if _, err := m.Get("cluster-a"); err != nil {
		t.Fatalf("active tunnel must not be reaped: %v", err)
	}

	// 活性停止 → 被回收
	ac.setActivity(func() time.Time { return time.Now().Add(-time.Hour) })
	waitFor(t, 2*time.Second, func() bool {
		_, err := m.Get("cluster-a")
		return err != nil
	}, "tunnel should be reaped after activity stops")
}
