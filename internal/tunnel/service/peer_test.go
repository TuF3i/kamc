package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/yamux"

	"github.com/TuF3i/kamc/internal/tunnel/registry"
)

// sharedRegistry 多副本共享的内存注册表（peer e2e/单测用）
type sharedRegistry struct {
	mu     sync.Mutex
	owners map[string][]registry.Owner // clusterID → owners
}

func newSharedRegistry() *sharedRegistry {
	return &sharedRegistry{owners: make(map[string][]registry.Owner)}
}

func (r *sharedRegistry) Report(_ context.Context, clusterID, agentID, pod string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.owners[clusterID]
	for i, o := range list {
		if o.AgentID == agentID {
			list[i].Pod, list[i].Since = pod, time.Now()
			return nil
		}
	}
	r.owners[clusterID] = append(list, registry.Owner{ClusterID: clusterID, AgentID: agentID, Pod: pod, Since: time.Now()})
	return nil
}

func (r *sharedRegistry) Deregister(_ context.Context, clusterID, agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.owners[clusterID]
	for i, o := range list {
		if o.AgentID == agentID {
			r.owners[clusterID] = append(list[:i], list[i+1:]...)
			return nil
		}
	}
	return nil
}

func (r *sharedRegistry) Lookup(_ context.Context, clusterID string) ([]registry.Owner, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]registry.Owner(nil), r.owners[clusterID]...), nil
}

// fakeAgentEcho 模拟 Agent 隧道上的回环服务：yamux 会话每条流原样回显
func fakeAgentEcho(t *testing.T) *yamux.Session {
	t.Helper()
	p1, p2 := net.Pipe()
	client, err := yamux.Client(p1, nil)
	if err != nil {
		t.Fatalf("yamux client: %v", err)
	}
	server, err := yamux.Server(p2, nil)
	if err != nil {
		t.Fatalf("yamux server: %v", err)
	}
	go func() {
		for {
			stream, err := server.Accept()
			if err != nil {
				return
			}
			go func(s net.Conn) {
				defer s.Close()
				_, _ = io.Copy(s, s)
			}(stream)
		}
	}()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
	return client
}

// setupPeerPair 构造两个副本：A 持有隧道（echo agent），B 通过 peer 转发访问。
// 返回 B 的 PeerRouter 与 A 的监听地址（供 addresses 映射）。
func setupPeerPair(t *testing.T, reg *sharedRegistry, token string) (podA, podB *PeerRouter, addrA string) {
	t.Helper()

	// 副本 A：本地持有 cluster-a 隧道
	mgrA := NewManager(reg, WithPodID("pod-a"), WithHeartbeat(time.Hour, time.Hour))
	mgrA.Start()
	mgrA.Register("cluster-a", "agent-1", fakeAgentEcho(t))

	podA = NewPeerRouter(mgrA, reg, "pod-a", PeerOptions{Token: token, ListenAddr: "127.0.0.1:0"})
	srv := httptest.NewServer(podA.Handler())
	t.Cleanup(srv.Close)
	t.Cleanup(func() { mgrA.Release() })

	// 副本 B：无本地隧道，addresses 指向 A
	mgrB := NewManager(reg, WithPodID("pod-b"), WithHeartbeat(time.Hour, time.Hour))
	mgrB.Start()
	podB = NewPeerRouter(mgrB, reg, "pod-b", PeerOptions{
		Token:     token,
		Addresses: map[string]string{"pod-a": strings.TrimPrefix(srv.URL, "http://")},
	})
	t.Cleanup(func() { mgrB.Release() })
	return podA, podB, srv.URL
}

// TestPeerRelayRoundtrip 跨副本中继：B 经 RELAY 拿到 A 的隧道流并完成字节回环。
func TestPeerRelayRoundtrip(t *testing.T) {
	reg := newSharedRegistry()
	reg.Report(context.Background(), "cluster-a", "agent-1", "pod-a")
	_, podB, _ := setupPeerPair(t, reg, "peer-secret")

	conn, err := podB.DialCluster(context.Background(), "cluster-a")
	if err != nil {
		t.Fatalf("dial via peer: %v", err)
	}
	defer conn.Close()

	msg := "hello-across-pods"
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, len(msg))
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != msg {
		t.Fatalf("roundtrip: %q err=%v", buf, err)
	}
}

// TestPeerRejectWithoutToken 错误 peer token → 401 → 拨号失败。
func TestPeerRejectWithoutToken(t *testing.T) {
	reg := newSharedRegistry()
	reg.Report(context.Background(), "cluster-a", "agent-1", "pod-a")
	podA, _, addrA := setupPeerPair(t, reg, "peer-secret")
	_ = podA

	mgrB := NewManager(reg, WithPodID("pod-b"), WithHeartbeat(time.Hour, time.Hour))
	mgrB.Start()
	defer mgrB.Release()
	podB := NewPeerRouter(mgrB, reg, "pod-b", PeerOptions{
		Token:     "wrong-token",
		Addresses: map[string]string{"pod-a": strings.TrimPrefix(addrA, "http://")},
	})

	if _, err := podB.DialCluster(context.Background(), "cluster-a"); err == nil {
		t.Fatal("wrong token must fail")
	}
}

// TestPeerFailoverOnReject owner 无该集群隧道（回执 ok:false）→ 剔除后报不可达；
// 注册表中的过期归属被主动清理。
func TestPeerFailoverOnReject(t *testing.T) {
	reg := newSharedRegistry()
	// owner 已登记，但 A 本地没有该集群隧道（Register 未调用）
	_, podB, _ := setupPeerPair(t, reg, "peer-secret")
	reg.Report(context.Background(), "cluster-gone", "agent-x", "pod-a")

	if _, err := podB.DialCluster(context.Background(), "cluster-gone"); err == nil {
		t.Fatal("unreachable cluster must fail")
	}

	owners, _ := reg.Lookup(context.Background(), "cluster-gone")
	if len(owners) != 0 {
		t.Fatalf("stale owner should be evicted, got %v", owners)
	}
}

// TestPeerUnknownPeerPod 注册表归属的副本不在 addresses 中 → 报错（不盲拨）。
func TestPeerUnknownPeerPod(t *testing.T) {
	reg := newSharedRegistry()
	_, podB, _ := setupPeerPair(t, reg, "peer-secret")
	// 独立集群 + 未知副本（避免 setup 登记的 pod-a 干扰）
	reg.Report(context.Background(), "cluster-mystery", "agent-9", "pod-unknown")

	if _, err := podB.DialCluster(context.Background(), "cluster-mystery"); err == nil {
		t.Fatal("unknown peer pod must fail")
	}
}

// TestParsePeerAddresses 地址解析："name=addr" 与 "host:port"（首段标签为 pod 名）。
func TestParsePeerAddresses(t *testing.T) {
	got := ParsePeerAddresses([]string{
		"am-platform-0.k8s-am-platform-headless.k8s-am-system.svc:9090",
		"pod-x=10.0.0.5:9090",
		"",
	})
	if got["am-platform-0"] != "am-platform-0.k8s-am-platform-headless.k8s-am-system.svc:9090" {
		t.Fatalf("dns form: %v", got)
	}
	if got["pod-x"] != "10.0.0.5:9090" {
		t.Fatalf("named form: %v", got)
	}
	if len(got) != 2 {
		t.Fatalf("unexpected entries: %v", got)
	}
}

// TestResidualConnBufio 首行读取后的残余字节并入流（防丢，BUG-3 教训镜像）。
// 服务端把回执与载荷写在同一包里；客户端按行读回执时 bufio 预读了载荷，
// residualConn 必须无损交付。
func TestResidualConnBufio(t *testing.T) {
	p1, p2 := net.Pipe()
	go func() {
		defer p1.Close()
		// 模拟持有方：读 RELAY 头 → 回执 + 载荷一次性写回
		br := bufio.NewReader(p1)
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		var hdr relayHeader
		if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &hdr); err != nil || hdr.Cluster != "c1" {
			return
		}
		fmt.Fprintf(p1, "{\"ok\":true}\nRESIDUAL-PAYLOAD")
	}()

	// 客户端侧：写 RELAY 头，随后按行读回执，残余经 residualConn 续读
	br := bufio.NewReader(p2)
	if _, err := fmt.Fprintf(p2, "{\"cluster\":\"c1\"}\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	ackLine, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("ack: %v", err)
	}
	var ack relayAck
	if err := json.Unmarshal([]byte(strings.TrimSpace(ackLine)), &ack); err != nil || !ack.OK {
		t.Fatalf("ack: %s", ackLine)
	}
	rc := &residualConn{br: br, conn: p2}
	buf := make([]byte, 32)
	_ = rc.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, err := rc.Read(buf)
	if err != nil || string(buf[:n]) != "RESIDUAL-PAYLOAD" {
		t.Fatalf("residual: %q err=%v", buf[:n], err)
	}
}
