package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Proxy /proxy/{clusterID}/* —— API 代理：
// 经隧道把请求送达目标集群的 kube-apiserver（ReverseProxy 池按集群缓存）。
// 隧道不可达（集群离线/Agent 重连中）由池的 ErrorHandler 转 503。
//
// 鉴权说明：JWT/Casbin 属后续里程碑；当前仅当 AM_ENABLE_UNAUTH_PROXY=1 时
// 装配（仅开发环境），生产环境该路由返回 501，避免暴露无鉴权代理。
func (h *Handler) Proxy(w http.ResponseWriter, r *http.Request) {
	if h.proxies == nil {
		http.Error(w, "proxy requires auth (JWT milestone) or AM_ENABLE_UNAUTH_PROXY=1 for development",
			http.StatusNotImplemented)
		return
	}

	rp, err := h.proxies.For(chi.URLParam(r, "clusterID"))
	if err != nil {
		http.Error(w, "proxy init failed", http.StatusInternalServerError)
		return
	}
	rp.ServeHTTP(w, r)
}
