package service

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/hashicorp/yamux"
)

// yamuxListener 把 yamux 会话适配为 net.Listener：每条流即一条 HTTP 连接，
// 交由 http.Server 解析（含 101 升级的 Hijack 语义）。
type yamuxListener struct {
	session *yamux.Session
}

// Accept 等待下一条流
func (l *yamuxListener) Accept() (net.Conn, error) { return l.session.Accept() }

// Close 关闭会话（连接级关闭；幂等由 yamux 保证）
func (l *yamuxListener) Close() error { return l.session.Close() }

// Addr 隧道监听地址（仅示意，无真实网络含义）
func (l *yamuxListener) Addr() net.Addr { return tunnelAddr{} }

// tunnelAddr 隧道监听地址
type tunnelAddr struct{}

func (tunnelAddr) Network() string { return "tunnel" }
func (tunnelAddr) String() string  { return "tunnel" }

// newUpstreamProxy 上游反向代理：路径已被平台 strip（设计文档 FIX-6），
// 此处只改写 scheme/host；101 升级（exec/port-forward 的 SPDY 流）由
// ReverseProxy 标准路径处理——劫持两侧后双向拷贝，无需手工序列化。
func newUpstreamProxy(upstream *url.URL, tr http.RoundTripper) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(upstream)
			pr.Out.Host = upstream.Host
			// FIX-7 的 Agent 侧兜底：用户凭证不透传（SA 凭证由 transport 层注入）
			pr.Out.Header.Del("Authorization")
		},
		Transport: tr,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("[agent] 上游转发失败 %s %s: %v", r.Method, r.URL.Path, err)
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
		},
	}
}

// BuildUpstream 解析上游 kube-apiserver：
//  1. 显式 APIServerURL 优先（开发/测试场景，可用 http 上游）；
//  2. 否则探测 in-cluster SA 文件（token + ca.crt），目标 https://kubernetes.default.svc；
//  3. 都不可用返回 (nil, nil, nil)——数据面未启用，隧道仅保活。
//
// SA Token 采用静态读取的 Bearer transport；投射 Token 的动态轮换加载属路线图
// （届时替换为 rest.TransportFor(saConfig)，调用方无需改动）。
func BuildUpstream(cfg Config) (*url.URL, http.RoundTripper, error) {
	saTokenFile := orDefault(cfg.SATokenFile, "/var/run/secrets/kubernetes.io/serviceaccount/token")
	saCAFile := orDefault(cfg.SACAFile, "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")

	target := cfg.APIServerURL
	if target == "" {
		if !fileExists(saTokenFile) {
			return nil, nil, nil // 非集群环境且未显式配置：数据面未启用
		}
		target = "https://kubernetes.default.svc"
	}

	u, err := url.Parse(target)
	if err != nil {
		return nil, nil, fmt.Errorf("agent -> parse apiserver url: %w", err)
	}

	// ForceAttemptHTTP2 必须为 false：apiserver 经 TLS ALPN 协商出 h2 后，
	// Connection: Upgrade（SPDY exec/port-forward）无法在 h2 上发起。
	// exec 的 SPDY 升级与普通请求都走 HTTP/1.1。
	base := &http.Transport{ForceAttemptHTTP2: false}
	caFile := cfg.SACAFile
	if caFile == "" && fileExists(saCAFile) {
		caFile = saCAFile
	}
	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, nil, fmt.Errorf("agent -> read apiserver ca: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, nil, fmt.Errorf("agent -> apiserver ca %s: no valid certs", caFile)
		}
		base.TLSClientConfig = &tls.Config{RootCAs: pool}
	}

	var tr http.RoundTripper = base
	if b, err := os.ReadFile(saTokenFile); err == nil && len(strings.TrimSpace(string(b))) > 0 {
		tr = &bearerTransport{base: base, token: strings.TrimSpace(string(b))}
	}
	return u, tr, nil
}

// bearerTransport 静态 Bearer Token 注入（SA 凭证不落请求头之外的地方）
type bearerTransport struct {
	base  http.RoundTripper
	token string
}

func (b *bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(r2)
}

func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
