// Package proxy 平台侧 API 代理：按集群缓存 ReverseProxy 与隧道 transport。
//
// 两类转发语义（全部由标准库/k8s 官方组件处理，不手工序列化）：
//   - 普通请求/watch：httputil.ReverseProxy（流式响应即时 flush）；
//   - 流式升级（exec/port-forward，§9）：Transport/Location 供 handler 组装
//     官方 StreamTranslatorHandler / TunnelingHandler，WS→SPDY 翻译后经隧道送达。
//
// 安全收敛（设计文档既定）：Director 剥离 Authorization（FIX-7）；
// Impersonation 对流式路径经 ImpersonationTransport 注入（官方 handler 自建
// 上游请求，头部注入不生效），普通路径由 handler 直写头部。
package proxy

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

// upstreamHost 占位上游主机：DialContext 忽略地址，但 URL 必须绝对。
const upstreamHost = "kube-apiserver"

// poolItem 每集群缓存项
type poolItem struct {
	proxy     *httputil.ReverseProxy
	transport *http.Transport
	location  *url.URL // 占位上游（scheme+host，不含路径）
}

// Pool 集群 → 代理池
type Pool struct {
	mu    sync.Mutex
	items map[string]*poolItem
	open  func(clusterID string) (net.Conn, error) // 取隧道数据流（来自 tunnel.Service.Open）
}

// NewPool 构造代理池
func NewPool(open func(clusterID string) (net.Conn, error)) *Pool {
	return &Pool{items: make(map[string]*poolItem), open: open}
}

// For 取集群的反向代理（普通请求路径；懒建并缓存）
func (p *Pool) For(clusterID string) (*httputil.ReverseProxy, error) {
	item, err := p.item(clusterID)
	if err != nil {
		return nil, err
	}
	return item.proxy, nil
}

// Transport 取集群的隧道 transport（流式 handler 的上游拨号入口）
func (p *Pool) Transport(clusterID string) (http.RoundTripper, error) {
	item, err := p.item(clusterID)
	if err != nil {
		return nil, err
	}
	return item.transport, nil
}

// Location 取集群的占位上游地址（scheme+host；路径由调用方按请求补齐）
func (p *Pool) Location(clusterID string) *url.URL {
	item, _ := p.item(clusterID)
	if item == nil {
		return nil
	}
	return item.location
}

// item 懒建并缓存集群项
func (p *Pool) item(clusterID string) (*poolItem, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if item, ok := p.items[clusterID]; ok {
		return item, nil
	}
	if p.open == nil {
		return nil, fmt.Errorf("proxy -> tunnel dialer not configured")
	}

	transport := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return p.open(clusterID)
		},
		ForceAttemptHTTP2:     false, // 上游为 HTTP/1.1 + SPDY 升级
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	location := &url.URL{Scheme: "http", Host: upstreamHost}

	rp := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// FIX-6: strip /proxy/{clusterID} 前缀，apiserver 收到干净路径。
			// （Casbin 鉴权与 Impersonation 注入在 handler 层完成——必须基于
			// strip 后的语义路径且发生在转发之前。）
			req.URL.Path = strings.TrimPrefix(req.URL.Path, "/proxy/"+clusterID)
			if req.URL.Path == "" {
				req.URL.Path = "/"
			}
			req.URL.RawPath = ""
			// scheme 必须是 http——隧道内为明文 HTTP（TLS 分别终结在 Nginx
			// 与 Agent→apiserver 两端，§3.2 数据流），写 https 会再包一层 TLS。
			req.URL.Scheme = "http"
			req.URL.Host = upstreamHost
			req.Host = upstreamHost

			// FIX-7: 用户凭证不透传（Agent 侧 transport 注入 SA Token）
			req.Header.Del("Authorization")
		},
		Transport: transport,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			// 隧道不可达是最常见错误（集群离线/Agent 重连中），单独可读提示
			log.Printf("[platform] proxy %s %s via %s: %v", r.Method, r.URL.Path, clusterID, err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("cluster unreachable"))
		},
	}

	item := &poolItem{proxy: rp, transport: transport, location: location}
	p.items[clusterID] = item
	return item, nil
}

// ImpersonationTransport 在 transport 层注入平台控制的 Impersonation 头。
// 流式 handler（StreamTranslatorHandler 等）自建上游请求、不透传入站头部，
// 必须以此方式注入（§8.1 第 5 步的流式等价物）。
type ImpersonationTransport struct {
	base   http.RoundTripper
	user   string
	groups []string
}

// NewImpersonationTransport 包装基础 transport
func NewImpersonationTransport(base http.RoundTripper, user string, groups []string) *ImpersonationTransport {
	return &ImpersonationTransport{base: base, user: user, groups: groups}
}

func (t *ImpersonationTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.Header.Set("Impersonate-User", t.user)
	for _, g := range t.groups {
		r2.Header.Add("Impersonate-Group", g)
	}
	return t.base.RoundTrip(r2)
}
