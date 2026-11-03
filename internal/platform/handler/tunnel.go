package handler

import (
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
	"github.com/TuF3i/kamc/internal/wsconn"
)

// tunnelUpgrader Agent 不是浏览器，Origin 校验无意义，直接放行；
// Token 认证在升级前完成（升级后的连接不再校验，与 Token 语义一致）。
var tunnelUpgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true },
}

// TunnelWS GET /tunnel —— Agent 隧道入口：
// Bearer Token 校验（哈希 + 宽限期）→ WS 升级 → yamux 服务端会话接管。
// 升级成功后本 handler 返回，连接交由隧道管理器监控（断开自动注销 + 分离回调）。
func (h *Handler) TunnelWS(w http.ResponseWriter, r *http.Request) {
	if h.tunnels == nil {
		http.Error(w, "tunnel module disabled", http.StatusServiceUnavailable)
		return
	}

	token := bearerToken(r)
	if token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	clusterID, _, err := h.svc.LookupToken(r.Context(), token)
	if err != nil {
		// 不区分原因（无效/过期/吊销），防枚举；细节在审计日志补齐（路线图）
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	// Agent 上报的集群必须与 Token 归属一致，防跨集群误注册
	if r.Header.Get("X-Cluster-ID") != clusterID {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	agentID := strings.TrimSpace(r.Header.Get("X-Agent-ID"))
	if agentID == "" {
		http.Error(w, "X-Agent-ID required", http.StatusBadRequest)
		return
	}

	conn, err := tunnelUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[platform] tunnel upgrade %s/%s: %v", clusterID, agentID, err)
		return
	}

	if err := h.tunnels.Attach(clusterID, agentID, wsconn.New(conn)); err != nil {
		log.Printf("[platform] tunnel attach %s/%s: %v", clusterID, agentID, err)
		_ = conn.Close()
		return
	}

	// 接入成功：集群转 online（设计文档 §11.4 验证点）
	if _, rsp := h.svc.SetClusterStatus(r.Context(), clusterID, modules.ClusterStatusOnline); !errcode.IsSuccess(rsp) {
		log.Printf("[platform] mark cluster %s online: %s", clusterID, rsp)
	}
	h.auditPlatform(r, "tunnel-attached", "", clusterID, "/tunnel", true, "attached")
	log.Printf("[platform] tunnel attached: cluster=%s agent=%s", clusterID, agentID)
}

// bearerToken 提取 Authorization: Bearer <token>
func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return ""
}
