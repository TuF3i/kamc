package handler

import (
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/TuF3i/kamc/internal/platform/authz"
	"github.com/TuF3i/kamc/internal/platform/middleware"
)

// Proxy /proxy/{clusterID}/* —— API 代理（数据面，设计文档 §8.1 完整顺序）：
//
//	JWT 校验（中间件）→ 解析集群 → 剥离 Impersonate-*（防伪造）→
//	Casbin 鉴权（RequestInfo 语义四元组，本地缓存零外部依赖）→
//	注入平台控制的 Impersonate-User/Group → 经隧道转发。
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

	// 4. 注入平台控制的 Impersonation（API Server RBAC 以此身份做第二道校验）
	r.Header.Set("Impersonate-User", claims.Subject)
	for _, g := range claims.Groups {
		r.Header.Add("Impersonate-Group", g)
	}

	// 5. 经隧道转发（Authorization 由池 Director 剥离，FIX-7）
	rp, err := h.proxies.For(clusterID)
	if err != nil {
		http.Error(w, "proxy init failed", http.StatusInternalServerError)
		return
	}
	rp.ServeHTTP(w, r)
}

// stripImpersonateHeaders 剥离用户传入的所有 Impersonate-* 头（§8.1 第 3 步）
func stripImpersonateHeaders(r *http.Request) {
	for key := range r.Header {
		if strings.HasPrefix(strings.ToLower(key), "impersonate-") {
			delete(r.Header, key)
		}
	}
}
