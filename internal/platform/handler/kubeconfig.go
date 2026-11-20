package handler

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/middleware"
	"github.com/TuF3i/kamc/internal/platform/modules"
	"github.com/TuF3i/kamc/internal/platform/service"
	"go.uber.org/zap"
)

// ListMyClusters GET /api/v1/my/clusters —— 当前用户可下载 kubeconfig 的集群。
// 管理员返回全部；普通用户只返回 Casbin 用户/组策略覆盖到的集群。
func (h *Handler) ListMyClusters(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromCtx(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	list, rsp := h.svc.ListMyClusters(r.Context(), claims.Subject, claims.Groups, h.claimsAdmin(claims.Groups))
	if list == nil {
		list = []*modules.ClusterDTO{}
	}
	errcode.GenFinalResponse(w, list, rsp)
}

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
	clusterID := chi.URLParam(r, "id")
	isAdmin := h.claimsAdmin(claims.Groups)

	// 只允许下载自己有数据面权限的集群（管理员例外）；避免靠猜集群 ID 探测。
	allowed, err := h.svc.HasClusterAccess(claims.Subject, claims.Groups, clusterID, isAdmin)
	if err != nil {
		zap.S().Infof("[platform] kubeconfig access check %s@%s: %v", claims.Subject, clusterID, err)
		h.auditPlatform(r, "kubeconfig-download", claims.Subject, clusterID, "/api/v1/clusters/"+clusterID+"/kubeconfig", false, "access-check-error")
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if !allowed {
		h.auditPlatform(r, "kubeconfig-download", claims.Subject, clusterID, "/api/v1/clusters/"+clusterID+"/kubeconfig", false, "no-cluster-access")
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	mode := r.URL.Query().Get("mode")
	data, rsp := h.svc.GenerateKubeconfig(r.Context(), service.KubeconfigParams{
		ClusterID:    clusterID,
		Mode:         mode,
		ExternalURL:  h.externalURL(r),
		CallerToken:  callerToken,
		CallerSub:    claims.Subject,
		CAData:       h.caData,
		ExecIssuer:   h.oidcIssuer,
		ExecClientID: h.oidcClientID,
	})
	if rsp != nil {
		h.auditPlatform(r, "kubeconfig-download", claims.Subject, clusterID, "/api/v1/clusters/"+clusterID+"/kubeconfig", false, rsp.Message)
		http.Error(w, rsp.Message, http.StatusBadRequest)
		return
	}
	h.auditPlatform(r, "kubeconfig-download", claims.Subject, clusterID, "/api/v1/clusters/"+clusterID+"/kubeconfig", true, "mode="+mode)

	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Content-Disposition",
		"attachment; filename=kubeconfig-"+clusterID+".yaml")
	_, _ = w.Write(data)
}

// claimsAdmin 当前 JWT 是否带管理组声明
func (h *Handler) claimsAdmin(groups []string) bool {
	for _, g := range groups {
		if g == h.adminGroup {
			return true
		}
	}
	return false
}

// externalURL 平台对外地址：配置（AM_EXTERNAL_URL / 配置文件）显式优先；
// 否则经 X-Forwarded-Proto（Nginx 设置，§10.1）+ 请求 Host 推导。
func (h *Handler) externalURL(r *http.Request) string {
	if v := strings.TrimSpace(h.settings.ExternalURL); v != "" {
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
