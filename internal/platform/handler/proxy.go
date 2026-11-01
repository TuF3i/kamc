package handler

import (
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	utilproxy "k8s.io/apimachinery/pkg/util/proxy"
	apiproxy "k8s.io/apiserver/pkg/util/proxy"

	"github.com/TuF3i/kamc/internal/platform/authz"
	"github.com/TuF3i/kamc/internal/platform/middleware"
	"github.com/TuF3i/kamc/internal/platform/proxy"
)

// Proxy /proxy/{clusterID}/* —— API 代理（数据面，设计文档 §8.1 完整顺序）：
//
//	JWT 校验（中间件）→ 解析集群 → 剥离 Impersonate-*（防伪造）→
//	Casbin 鉴权（RequestInfo 语义四元组，本地缓存零外部依赖）→
//	注入平台控制的 Impersonate-User/Group → 经隧道转发。
//
// 流式升级（§9）：WebSocket 请求按子协议分流——
//   - `SPDY/3.1+portforward.k8s.io` → TunnelingHandler（SPDY over WS 直通上游）；
//   - `v4/v5.channel.k8s.io`（exec/attach）→ StreamTranslatorHandler（WS→SPDY 翻译）；
//   - 旧版 SPDY 客户端（Upgrade: SPDY/3.1）不走本分支，经 ReverseProxy 原样
//     双向透传（Agent 侧 101 passthrough 已验证）。
//
// 失败语义：无权限 403；集群无隧道（Agent 离线/重连中）由池 ErrorHandler 转 503。
func (h *Handler) Proxy(w http.ResponseWriter, r *http.Request) {
	if h.proxies == nil {
		http.Error(w, "proxy requires tunnels enabled", http.StatusNotImplemented)
		return
	}
	clusterID := chi.URLParam(r, "clusterID")

	// 1. 身份（RequireJWT 中间件已注入）
	claims, ok := middleware.ClaimsFromCtx(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// 2. 剥离用户传入的所有 Impersonate-* 头（必须在鉴权之前，防伪造身份越权）
	stripImpersonateHeaders(r)

	// 3. Casbin 鉴权：基于 strip 后路径解析语义资源（FIX-6 的 strip 在此统一发生）
	cleanPath := strings.TrimPrefix(r.URL.Path, "/proxy/"+clusterID)
	if cleanPath == "" {
		cleanPath = "/"
	}
	obj, act, err := authz.ParseRequest(r.Method, cleanPath)
	if err != nil {
		http.Error(w, "bad request path", http.StatusBadRequest)
		return
	}
	allowed, err := h.svc.Enforce(claims.Subject, clusterID, obj, act)
	if err != nil {
		// fail-closed：鉴权组件故障一律拒绝（§8 双重校验的第一层不能失效开放）
		log.Printf("[platform] casbin enforce error %s@%s %s %s: %v",
			claims.Subject, clusterID, obj, act, err)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if !allowed {
		log.Printf("[platform] casbin denied %s@%s %s %s", claims.Subject, clusterID, obj, act)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// 4. 流式分流（§9）：WebSocket 升级请求交官方 handler；其余走 ReverseProxy
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		h.serveStream(w, r, clusterID, cleanPath, claims.Subject, claims.Groups)
		return
	}

	// 5a. 普通路径：头部注入 Impersonation（§8.1 第 5 步）
	r.Header.Set("Impersonate-User", claims.Subject)
	for _, g := range claims.Groups {
		r.Header.Add("Impersonate-Group", g)
	}

	// 6. 经隧道转发（Authorization 由池 Director 剥离，FIX-7）
	rp, err := h.proxies.For(clusterID)
	if err != nil {
		http.Error(w, "proxy init failed", http.StatusInternalServerError)
		return
	}
	rp.ServeHTTP(w, r)
}

// serveStream 流式升级路径：Impersonation 经 transport 注入（官方 handler 自建
// 上游请求，头部注入不生效），上游地址 = 占位 host + 本次请求的路径与 query。
func (h *Handler) serveStream(w http.ResponseWriter, r *http.Request, clusterID, cleanPath, user string, groups []string) {
	transport, err := h.proxies.Transport(clusterID)
	if err != nil {
		http.Error(w, "proxy init failed", http.StatusInternalServerError)
		return
	}
	impersonated := proxy.NewImpersonationTransport(transport, user, groups)
	base := h.proxies.Location(clusterID)
	if base == nil {
		http.Error(w, "proxy init failed", http.StatusInternalServerError)
		return
	}
	// 上游地址携带本次请求的路径与 query（exec 的 command/stdin 等参数必须透传）
	location := *base
	location.Path = cleanPath
	location.RawQuery = r.URL.RawQuery

	if hasSPDYTunnelSubprotocol(r) {
		// port-forward：SPDY 帧经 WebSocket 直通上游（TunnelingHandler 包装
		// UpgradeAwareHandler 完成上游 SPDY 升级拨号）
		upgradeAware := utilproxy.NewUpgradeAwareHandler(&location, impersonated, false, true, errorResponder{})
		upgradeAware.UpgradeRequired = true
		apiproxy.NewTunnelingHandler(upgradeAware).ServeHTTP(w, r)
		return
	}

	// exec/attach：WS（v4/v5.channel.k8s.io）翻译为 SPDY（兼容 1.23/1.25 仅 SPDY 集群）
	streamHandler := apiproxy.NewStreamTranslatorHandler(&location, impersonated, 0, streamOptions(r))
	streamHandler.ServeHTTP(w, r)
}

// hasSPDYTunnelSubprotocol 识别 port-forward 的 WS 子协议（SPDY/3.1+ 前缀）
func hasSPDYTunnelSubprotocol(r *http.Request) bool {
	for _, proto := range websocket.Subprotocols(r) {
		if strings.HasPrefix(proto, "SPDY/3.1+") {
			return true
		}
	}
	return false
}

// streamOptions 从 query 解析请求的流（stdin/stdout/stderr/tty）
func streamOptions(r *http.Request) apiproxy.Options {
	q := r.URL.Query()
	return apiproxy.Options{
		Stdin:  queryFlag(q, "stdin"),
		Stdout: queryFlag(q, "stdout"),
		Stderr: queryFlag(q, "stderr"),
		Tty:    queryFlag(q, "tty"),
	}
}

func queryFlag(q url.Values, name string) bool {
	v := q.Get(name)
	return v == "1" || strings.EqualFold(v, "true")
}

// errorResponder UpgradeAwareHandler 上游拨号失败的响应出口
type errorResponder struct{}

func (errorResponder) Error(w http.ResponseWriter, _ *http.Request, err error) {
	log.Printf("[platform] stream upgrade failed: %v", err)
	http.Error(w, "upstream unavailable", http.StatusBadGateway)
}

func (errorResponder) ErrorS(w http.ResponseWriter, req *http.Request, err error) {
	errorResponder{}.Error(w, req, err)
}

// stripImpersonateHeaders 剥离用户传入的所有 Impersonate-* 头（§8.1 第 3 步）
func stripImpersonateHeaders(r *http.Request) {
	for key := range r.Header {
		if strings.HasPrefix(strings.ToLower(key), "impersonate-") {
			delete(r.Header, key)
		}
	}
}
