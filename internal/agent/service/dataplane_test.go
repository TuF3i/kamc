package service

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
)

// echoAPIServer 假 kube-apiserver：回显请求特征（路径/收到的头），供断言转发语义。
func echoAPIServer(t *testing.T, hit func(r *http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hit != nil {
			hit(r)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"PodList","path":"` + r.URL.Path + `"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newTunnelPair 建立"平台开流 → Agent 服务"的完整数据面：
// 返回平台侧拨号函数（模拟 TunnelManager.Open）。
func newTunnelPair(t *testing.T, upstream *url.URL, tr http.RoundTripper) func() net.Conn {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	agentSession, err := yamux.Client(clientConn, nil) // Agent 侧为拨出方（Client 角色）
	if err != nil {
		t.Fatalf("yamux client: %v", err)
	}
	t.Cleanup(func() { _ = agentSession.Close() })

	platformSession, err := yamux.Server(serverConn, nil)
	if err != nil {
		t.Fatalf("yamux server: %v", err)
	}
	t.Cleanup(func() { _ = platformSession.Close() })

	// Agent 侧数据面：yamux 流 → HTTP 服务 → 反代上游
	srv := &http.Server{Handler: newUpstreamProxy(upstream, tr), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(&yamuxListener{session: agentSession}) }()

	// 平台侧：每次"拨号"即开一条流
	return func() net.Conn {
		conn, err := platformSession.Open()
		if err != nil {
			t.Fatalf("platform open stream: %v", err)
		}
		return conn
	}
}

// TestUpstreamProxyRoundtrip 普通请求：路径透传、用户 Authorization 不透传（FIX-7）。
func TestUpstreamProxyRoundtrip(t *testing.T) {
	var gotAuth, gotPath string
	upstream := echoAPIServer(t, func(r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
	})
	u, _ := url.Parse(upstream.URL)
	dial := newTunnelPair(t, u, http.DefaultTransport)

	// 模拟平台侧 http.Transport 经 DialContext 走隧道
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) { return dial(), nil },
	}}
	req, _ := http.NewRequest(http.MethodGet, "http://tunnel/api/v1/namespaces/dev/pods", nil)
	req.SetBasicAuth("platform-user", "platform-jwt") // 平台未剥离时的用户凭证

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request over tunnel: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if gotPath != "/api/v1/namespaces/dev/pods" {
		t.Fatalf("path: got %q", gotPath)
	}
	if gotAuth != "" {
		t.Fatalf("user Authorization leaked to upstream: %q", gotAuth)
	}
	if !strings.Contains(string(body), `"path":"/api/v1/namespaces/dev/pods"`) {
		t.Fatalf("upstream body: %s", body)
	}
}

// TestUpstreamProxyUpgrade 101 升级（exec/port-forward 的数据面语义）：
// 客户端发 Upgrade → 反代劫持两侧 → 升级后字节双向通流。
func TestUpstreamProxyUpgrade(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Connection") != "Upgrade" {
			http.Error(w, "expect upgrade", http.StatusBadRequest)
			return
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "no hijack", http.StatusInternalServerError)
			return
		}
		// 101 响应必须回带匹配的 Upgrade 头（真实 SPDY 升级同样如此），
		// 否则客户端 Transport 以 "switch protocol mismatch" 拒收
		w.Header().Set("Connection", "Upgrade")
		w.Header().Set("Upgrade", "tunnel-proto")
		w.WriteHeader(http.StatusSwitchingProtocols)
		conn, rw, _ := hj.Hijack()
		defer conn.Close()
		// 升级后回显 3 条字节块，验证双向通道
		for i := 0; i < 3; i++ {
			line, err := rw.Reader.ReadString('\n')
			if err != nil {
				return
			}
			_, _ = rw.WriteString("echo:" + line)
			_ = rw.Flush()
		}
	}))
	defer upstream.Close()
	u, _ := url.Parse(upstream.URL)
	dial := newTunnelPair(t, u, http.DefaultTransport)

	// 升级请求经隧道：手写请求更贴近 kubectl 的 SPDY/WS 升级形态
	conn := dial()
	defer conn.Close()
	req := "GET /api/v1/namespaces/dev/pods/exec HTTP/1.1\r\n" +
		"Host: tunnel\r\nConnection: Upgrade\r\nUpgrade: tunnel-proto\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("write upgrade req: %v", err)
	}

	// http.ReadResponse 的 bufio 可能预读升级后字节，必须把缓冲拼回流
	// （BUG-3 的教训在测试侧的镜像：生产路径由 http.Transport/ReverseProxy 自行处理）
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("want 101, got %d", resp.StatusCode)
	}

	rwc := upgradedConn{br: br, conn: conn}
	for i := 0; i < 3; i++ {
		if _, err := rwc.Write([]byte("ping\n")); err != nil {
			t.Fatalf("write after upgrade: %v", err)
		}
		buf := make([]byte, 32)
		_ = rwc.SetReadDeadline(time.Now().Add(3 * time.Second))
		n, err := rwc.Read(buf)
		if err != nil || string(buf[:n]) != "echo:ping\n" {
			t.Fatalf("upgrade echo: %q err=%v", buf[:n], err)
		}
	}
}

// upgradedConn 把 bufio.Reader 的预读残余与底层 conn 拼成连续读写流
type upgradedConn struct {
	br   *bufio.Reader
	conn net.Conn
}

func (u upgradedConn) Read(p []byte) (int, error)  { return u.br.Read(p) }
func (u upgradedConn) Write(p []byte) (int, error) { return u.conn.Write(p) }
func (u upgradedConn) Close() error                { return u.conn.Close() }
func (u upgradedConn) LocalAddr() net.Addr         { return u.conn.LocalAddr() }
func (u upgradedConn) RemoteAddr() net.Addr        { return u.conn.RemoteAddr() }
func (u upgradedConn) SetDeadline(t time.Time) error {
	return u.conn.SetDeadline(t)
}
func (u upgradedConn) SetReadDeadline(t time.Time) error {
	return u.conn.SetReadDeadline(t)
}
func (u upgradedConn) SetWriteDeadline(t time.Time) error {
	return u.conn.SetWriteDeadline(t)
}
