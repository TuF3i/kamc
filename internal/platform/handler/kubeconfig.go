package handler

import (
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/TuF3i/kamc/internal/platform/middleware"
	"github.com/TuF3i/kamc/internal/platform/service"
)

// GetKubeconfig GET /api/v1/clusters/{id}/kubeconfig?mode=static|exec —— §4.4。
// 自服务端点（仅需 JWT，非管理员）：产物只承载调用者自己的身份，
// 数据面授权仍由请求时的 Casbin 决定。响应为 YAML 下载。
func (h *Handler) GetKubeconfig(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromCtx(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	callerToken := bearerToken(r)

	mode := r.URL.Query().Get("mode")
	data, rsp := h.svc.GenerateKubeconfig(r.Context(), service.KubeconfigParams{
		ClusterID:    chi.URLParam(r, "id"),
		Mode:         mode,
		ExternalURL:  externalURL(r),
		CallerToken:  callerToken,
		CallerSub:    claims.Subject,
		CAData:       h.caData,
		ExecIssuer:   h.oidcIssuer,
		ExecClientID: h.oidcClientID,
	})
	if rsp != nil {
		h.auditPlatform(r, "kubeconfig-download", claims.Subject, chi.URLParam(r, "id"), "/api/v1/clusters/"+chi.URLParam(r, "id")+"/kubeconfig", false, rsp.Message)
		http.Error(w, rsp.Message, http.StatusBadRequest)
		return
	}
	h.auditPlatform(r, "kubeconfig-download", claims.Subject, chi.URLParam(r, "id"), "/api/v1/clusters/"+chi.URLParam(r, "id")+"/kubeconfig", true, "mode="+mode)

	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Content-Disposition",
		"attachment; filename=kubeconfig-"+chi.URLParam(r, "id")+".yaml")
	_, _ = w.Write(data)
}

// externalURL 平台对外地址：AM_EXTERNAL_URL 显式优先；否则经
// X-Forwarded-Proto（Nginx 设置，§10.1）+ 请求 Host 推导。
func externalURL(r *http.Request) string {
	if v := os.Getenv("AM_EXTERNAL_URL"); v != "" {
		return v
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		scheme = p
	}
	return scheme + "://" + r.Host
}
