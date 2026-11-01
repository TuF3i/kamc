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
