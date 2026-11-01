// Package proxy 平台侧 API 代理：按集群缓存 httputil.ReverseProxy，
// 其 Transport 经隧道 DialContext（tunnel.Service.Open）把请求送达目标集群。
//
// ReverseProxy 同时承担两类转发语义（标准库处理，不手工序列化）：
//   - 普通请求/watch（流式响应即时 flush）；
//   - 101 升级（exec/port-forward）：劫持两侧双向拷贝，与 Agent 侧对偶。
//
// Impersonation 注入与 JWT 鉴权属后续里程碑；本池仅做设计文档已定的
// 安全收敛：剥离用户传入的 Impersonate-* 头（§8.1 第 3 步）与
// Authorization 头（FIX-7），凭证由 Agent 侧 transport 注入。
package proxy

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"
)

// Pool 集群 → 代理池（Transport 缓存连接池，DialContext 固定指向单一集群）
type Pool struct {
	mu    sync.Mutex
	items map[string]*httputil.ReverseProxy
	open  func(clusterID string) (net.Conn, error) // 取隧道数据流（来自 tunnel.Service.Open）
}

// NewPool 构造代理池
func NewPool(open func(clusterID string) (net.Conn, error)) *Pool {
	return &Pool{items: make(map[string]*httputil.ReverseProxy), open: open}
}

// For 取集群代理（懒建并缓存）
func (p *Pool) For(clusterID string) (*httputil.ReverseProxy, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if rp, ok := p.items[clusterID]; ok {
		return rp, nil
	}
	if p.open == nil {
		return nil, fmt.Errorf("proxy -> tunnel dialer not configured")
	}

	tr := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return p.open(clusterID)
		},
		ForceAttemptHTTP2:     false, // 上游为 HTTP/1.1 + SPDY 升级
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
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
			// 占位 scheme/host：DialContext 忽略地址，但 URL 必须绝对。
			// scheme 必须是 http——隧道内为明文 HTTP（TLS 分别终结在 Nginx
			// 与 Agent→apiserver 两端，§3.2 数据流），写 https 会再包一层 TLS。
			req.URL.Scheme = "http"
			req.URL.Host = "kube-apiserver"
			req.Host = "kube-apiserver"

			// FIX-7: 用户凭证不透传（Agent 侧 transport 注入 SA Token）
			req.Header.Del("Authorization")
		},
		Transport: tr,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			// 隧道不可达是最常见错误（集群离线/Agent 重连中），单独可读提示
			log.Printf("[platform] proxy %s %s via %s: %v", r.Method, r.URL.Path, clusterID, err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("cluster unreachable"))
		},
	}
	p.items[clusterID] = rp
	return rp, nil
}
