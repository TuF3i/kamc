package service

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"

	"github.com/TuF3i/kamc/internal/metrics"
	"github.com/TuF3i/kamc/internal/tunnel/registry"
	"github.com/TuF3i/kamc/internal/wsconn"
)

// PeerRouter 副本间转发路由（§7.2.4）：请求落在本副本但隧道在别的副本时，
// 查注册表找 owner → 经 peer yamux 会话开 RELAY 流 → owner 把该流与本机
// Agent 隧道流做字节拼接。平台内部全程字节透明（exec/port-forward 的
// 101 升级无感知通行），协议与 Agent 隧道完全同构（WS + yamux）。
//
// RELAY 协议（每条流）：
//
//	拨号方 → 首行 JSON {"cluster":"<id>"}\n
//	持有方 → 回执首行 JSON {"ok":true}\n（本地无隧道则 {"ok":false} 并关闭）
//	此后纯字节双向拼接。
type PeerRouter struct {
	self     string // 本副本 podID
	local    *Manager
	registry registry.Registry

	cfg PeerOptions

	mu       sync.Mutex
	sessions map[string]*yamux.Session // podName → 懒建立的 peer 会话

	done chan struct{}
	wg   sync.WaitGroup
}

// PeerOptions 副本间转发配置
type PeerOptions struct {
	Token      string            // peer 认证令牌（Secret 下发）
	ListenAddr string            // 本机 peer 监听地址（空 = 不启动服务端）
	Addresses  map[string]string // podName → host:port（可拨的副本地址）
}

// relayHeader RELAY 流首行
type relayHeader struct {
	Cluster string `json:"cluster"`
}

type relayAck struct {
	OK bool `json:"ok"`
}

// NewPeerRouter 构造路由并挂到 Manager（Open 的跨副本 fallback 由其接管）
func NewPeerRouter(m *Manager, reg registry.Registry, self string, cfg PeerOptions) *PeerRouter {
	p := &PeerRouter{
		self:     self,
		local:    m,
		registry: reg,
		cfg:      cfg,
		sessions: make(map[string]*yamux.Session),
		done:     make(chan struct{}),
	}
	m.peer = p
	return p
}

// Handler 内部 peer 服务端（独立监听，仅集群内网可达；NetworkPolicy 限平台 Pod）
func (p *PeerRouter) Handler() http.Handler {
	return http.HandlerFunc(p.handlePeer)
}

// Close 关闭全部 peer 会话（幂等；Manager.Release 调用）
func (p *PeerRouter) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	select {
	case <-p.done:
	default:
		close(p.done)
	}
	for pod, sess := range p.sessions {
		_ = sess.Close()
		delete(p.sessions, pod)
	}
}

// waitExit 带超时等待全部 relay goroutine
func (p *PeerRouter) waitExit() {
	ch := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(ch)
	}()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
	}
}

// ---- 服务端 ----

var peerUpgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

func (p *PeerRouter) handlePeer(w http.ResponseWriter, r *http.Request) {
	if !p.authPeer(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := peerUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[tunnel] peer upgrade: %v", err)
		return
	}
	sess, err := yamux.Server(wsconn.New(conn), nil)
	if err != nil {
		_ = conn.Close()
		log.Printf("[tunnel] peer yamux server: %v", err)
		return
	}
	p.wg.Add(1)
	go p.acceptRelays(sess)
}

func (p *PeerRouter) authPeer(r *http.Request) bool {
	token := p.cfg.Token
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if token == "" || len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(h[len(prefix):]), []byte(token)) == 1
}

func (p *PeerRouter) acceptRelays(sess *yamux.Session) {
	defer p.wg.Done()
	for {
		stream, err := sess.Accept()
		if err != nil {
			return // 会话关闭（对端下线/Release）
		}
		p.wg.Add(1)
		go p.relayStream(stream)
	}
}

// relayStream 解析 RELAY 头 → 打开本机 Agent 隧道流 → 回执 → 字节拼接
func (p *PeerRouter) relayStream(stream net.Conn) {
	defer p.wg.Done()
	defer stream.Close()

	br := bufio.NewReader(stream)
	line, err := br.ReadString('\n')
	if err != nil {
		return
	}
	var hdr relayHeader
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &hdr); err != nil || hdr.Cluster == "" {
		return
	}

	agent, err := p.local.openLocal(hdr.Cluster)
	if err != nil {
		ack, _ := json.Marshal(relayAck{OK: false})
		fmt.Fprintf(stream, "%s\n", ack) // 拨号方据此尝试下一个 owner
		return
	}
	defer agent.Close()

	ack, _ := json.Marshal(relayAck{OK: true})
	if _, err := fmt.Fprintf(stream, "%s\n", ack); err != nil {
		return
	}

	peerSide := &residualConn{br: br, conn: stream} // bufio 预读残余必须并入流
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(agent, peerSide)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(peerSide, agent)
		done <- struct{}{}
	}()
	<-done
}

// ---- 拨号端 ----

// DialCluster 跨副本取数据流：注册表择优 → 逐 owner RELAY → 失败剔除重试
func (p *PeerRouter) DialCluster(ctx context.Context, clusterID string) (net.Conn, error) {
	owners, err := p.registry.Lookup(ctx, clusterID)
	if err != nil {
		return nil, fmt.Errorf("peer -> lookup %s: %w", clusterID, err)
	}
	for _, o := range owners {
		if o.Pod == p.self || o.Pod == "" {
			continue
		}
		conn, rerr := p.openRelay(ctx, o.Pod, clusterID)
		if rerr == nil {
			metrics.PeerRelayTotal.WithLabelValues(o.Pod).Inc()
			return conn, nil
		}
		log.Printf("[tunnel] relay %s via %s: %v", clusterID, o.Pod, rerr)
		// 归属大概率已过期：提前剔除（TTL 兜底仍在），尝试下一个 owner
		rmCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_ = p.registry.Deregister(rmCtx, clusterID, o.AgentID)
		cancel()
	}
	return nil, fmt.Errorf("peer -> cluster %s unreachable: %w", clusterID, ErrClusterNotConnected)
}

// openRelay 在 owner 副本上开一条到 clusterID 的中继流（含握手回执）
func (p *PeerRouter) openRelay(ctx context.Context, pod, clusterID string) (net.Conn, error) {
	sess, err := p.peerSession(ctx, pod)
	if err != nil {
		return nil, err
	}
	stream, err := sess.Open()
	if err != nil {
		p.dropSession(pod) // 会话死亡，下次重拨
		return nil, fmt.Errorf("peer -> open stream: %w", err)
	}
	hdr, _ := json.Marshal(relayHeader{Cluster: clusterID})
	if _, err := fmt.Fprintf(stream, "%s\n", hdr); err != nil {
		_ = stream.Close()
		return nil, fmt.Errorf("peer -> relay header: %w", err)
	}

	br := bufio.NewReader(stream)
	ackLine, err := br.ReadString('\n')
	if err != nil {
		_ = stream.Close()
		return nil, fmt.Errorf("peer -> relay ack: %w", err)
	}
	var ack relayAck
	if err := json.Unmarshal([]byte(strings.TrimSpace(ackLine)), &ack); err != nil || !ack.OK {
		_ = stream.Close()
		return nil, fmt.Errorf("peer -> relay rejected by %s", pod)
	}
	return &residualConn{br: br, conn: stream}, nil
}

// peerSession 懒建立并缓存到指定副本的 yamux 会话
func (p *PeerRouter) peerSession(ctx context.Context, pod string) (*yamux.Session, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	select {
	case <-p.done:
		return nil, errors.New("peer router closed")
	default:
	}
	if sess, ok := p.sessions[pod]; ok && !sess.IsClosed() {
		return sess, nil
	}
	addr, ok := p.cfg.Addresses[pod]
	if !ok {
		return nil, fmt.Errorf("peer -> unknown peer %q", pod)
	}
	dialURL := &url.URL{Scheme: "ws", Host: addr, Path: "/peer"}
	dialer := &websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	hdr := map[string][]string{"Authorization": {"Bearer " + p.cfg.Token}}
	conn, _, err := dialer.DialContext(ctx, dialURL.String(), hdr)
	if err != nil {
		return nil, fmt.Errorf("peer -> dial %s: %w", addr, err)
	}
	sess, err := yamux.Client(wsconn.New(conn), nil)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("peer -> yamux client: %w", err)
	}
	p.sessions[pod] = sess
	return sess, nil
}

func (p *PeerRouter) dropSession(pod string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if sess, ok := p.sessions[pod]; ok {
		_ = sess.Close()
		delete(p.sessions, pod)
	}
}

// residualConn 把 bufio.Reader 的预读残余与底层连接拼成连续字节流
// （RELAY 头按行读取后，残余必须随流继续——与 BUG-3 同源的丢字节教训）
type residualConn struct {
	br   *bufio.Reader
	conn net.Conn
}

func (c *residualConn) Read(p []byte) (int, error)  { return c.br.Read(p) }
func (c *residualConn) Write(p []byte) (int, error) { return c.conn.Write(p) }
func (c *residualConn) Close() error                { return c.conn.Close() }
func (c *residualConn) LocalAddr() net.Addr         { return c.conn.LocalAddr() }
func (c *residualConn) RemoteAddr() net.Addr        { return c.conn.RemoteAddr() }
func (c *residualConn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}
func (c *residualConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}
func (c *residualConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

// ParsePeerAddresses 解析 PEER_ADDRESSES："name=host:port" 或 "host:port"
// （后者以主机名首段 DNS 标签为 podName，匹配 headless service 命名）。
func ParsePeerAddresses(entries []string) map[string]string {
	out := make(map[string]string)
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if name, addr, found := strings.Cut(e, "="); found {
			if name = strings.TrimSpace(name); name != "" {
				out[name] = strings.TrimSpace(addr)
				continue
			}
		}
		host := e
		if h, _, err := net.SplitHostPort(e); err == nil {
			host = h
		}
		if dot := strings.Index(host, "."); dot > 0 {
			host = host[:dot]
		}
		out[host] = e
	}
	return out
}
