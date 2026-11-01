package handler

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/middleware"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

// ListAudit GET /api/v1/audit?user=&cluster=&limit= —— 审计查询（管理面）
func (h *Handler) ListAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	events, err := h.svc.ListAudit(r.Context(),
		r.URL.Query().Get("user"), r.URL.Query().Get("cluster"), limit)
	if err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ServerInternal(err))
		return
	}
	if events == nil {
		events = []modules.AuditEvent{}
	}
	errcode.GenFinalResponse(w, events, nil)
}

// auditDataPlane 数据面审计（Proxy 允许/拒绝统一出口）
func (h *Handler) auditDataPlane(r *http.Request, clusterID, obj, verb string, allowed bool, reason string, start time.Time) {
	if h.audit == nil {
		return
	}
	var sub, groups string
	if claims, ok := middleware.ClaimsFromCtx(r.Context()); ok {
		sub = claims.Subject
		groups = strings.Join(claims.Groups, ",")
	}
	h.audit.Log(&modules.AuditEvent{
		Timestamp: time.Now(),
		User:      sub,
		Groups:    groups,
		Cluster:   clusterID,
		Method:    r.Method,
		Path:      obj,
		Verb:      verb,
		Allowed:   allowed,
		Reason:    reason,
		SourceIP:  clientIP(r),
		LatencyMS: time.Since(start).Milliseconds(),
	})
}

// auditSimple 简单事件审计（隧道接入/踢出/登录等）
func (h *Handler) auditSimple(r *http.Request, user, cluster, path, reason string, allowed bool) {
	if h.audit == nil {
		return
	}
	h.audit.Log(&modules.AuditEvent{
		Timestamp: time.Now(),
		User:      user,
		Cluster:   cluster,
		Method:    r.Method,
		Path:      path,
		Allowed:   allowed,
		Reason:    reason,
		SourceIP:  clientIP(r),
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
