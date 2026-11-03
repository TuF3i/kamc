package handler

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/middleware"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

// maxAuditBody 原始请求/响应体在审计中的截断上限（字节前缀）
const maxAuditBody = 4096

// ListAudit GET /api/v1/audit?kind=&user=&cluster=&limit= —— 审计查询（管理面）。
// kind=platform|cluster 过滤；缺省返回全部（兼容旧调用）。
func (h *Handler) ListAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	events, err := h.svc.ListAudit(r.Context(),
		r.URL.Query().Get("kind"),
		r.URL.Query().Get("user"),
		r.URL.Query().Get("cluster"),
		limit)
	if err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ServerInternal(err))
		return
	}
	if events == nil {
		events = []modules.AuditEvent{}
	}
	errcode.GenFinalResponse(w, events, nil)
}

// requestMeta 对端上下文：IP（X-Forwarded-For → X-Real-IP → 连接地址）、
// 原始转发链、UA、GeoIP（未配置数据库时为空）。
type requestMeta struct {
	IP           string
	ForwardedFor string
	UserAgent    string
	GeoCountry   string
	GeoRegion    string
	GeoCity      string
}

// meta 解析一次请求的对端信息。
// X-Forwarded-For 取最左（原始客户端）且保留完整链用于展示；IP 选择顺序：
// XFF 首项 → X-Real-IP → RemoteAddr。部署层（Nginx）负责覆盖伪造头。
func (h *Handler) meta(r *http.Request) requestMeta {
	m := requestMeta{UserAgent: truncate(r.UserAgent(), 512)}
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		m.ForwardedFor = truncate(xff, 512)
		if first, _, _ := strings.Cut(xff, ","); strings.TrimSpace(first) != "" {
			m.IP = strings.TrimSpace(first)
		}
	}
	if m.IP == "" {
		if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" {
			m.IP = xr
		}
	}
	if m.IP == "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			m.IP = r.RemoteAddr
		} else {
			m.IP = host
		}
	}
	if h.geo != nil {
		loc := h.geo.Lookup(m.IP)
		m.GeoCountry, m.GeoRegion, m.GeoCity = loc.Country, loc.Region, loc.City
	}
	return m
}

// auditPlatform 平台事件（登录/Token/下载/管理动作）：kind=platform，
// 输出到 stdio 并落库。失败不阻塞业务（Writer 内部丢弃并计数）。
func (h *Handler) auditPlatform(r *http.Request, event, user, cluster, path string, allowed bool, reason string) {
	if h.audit == nil {
		return
	}
	m := h.meta(r)
	if user == "" {
		if claims, ok := middleware.ClaimsFromCtx(r.Context()); ok {
			user = claims.Subject
		}
	}
	h.audit.Log(&modules.AuditEvent{
		Kind:         modules.AuditKindPlatform,
		Event:        event,
		Timestamp:    time.Now(),
		User:         user,
		Cluster:      cluster,
		Method:       r.Method,
		Path:         path,
		Allowed:      allowed,
		Reason:       reason,
		SourceIP:     m.IP,
		ForwardedFor: m.ForwardedFor,
		UserAgent:    m.UserAgent,
		GeoCountry:   m.GeoCountry,
		GeoRegion:    m.GeoRegion,
		GeoCity:      m.GeoCity,
	})
}

// clusterEvent 组装集群操作事件（kind=cluster；调用方填细节后 auditCluster）。
func (h *Handler) clusterEvent(r *http.Request, allowed bool, reason string) *modules.AuditEvent {
	m := h.meta(r)
	ev := &modules.AuditEvent{
		Kind:         modules.AuditKindCluster,
		Event:        "k8s-api",
		Timestamp:    time.Now(),
		Method:       r.Method,
		Path:         r.URL.RequestURI(),
		Allowed:      allowed,
		Reason:       reason,
		SourceIP:     m.IP,
		ForwardedFor: m.ForwardedFor,
		UserAgent:    m.UserAgent,
		GeoCountry:   m.GeoCountry,
		GeoRegion:    m.GeoRegion,
		GeoCity:      m.GeoCity,
	}
	if claims, ok := middleware.ClaimsFromCtx(r.Context()); ok {
		ev.User = claims.Subject
		ev.Groups = strings.Join(claims.Groups, ",")
	}
	return ev
}

// auditDataPlane 数据面审计（Proxy 允许/拒绝统一出口）：组装集群事件 +
// 原始请求/响应前缀，kind=cluster 落库（不落 stdio）+ 可选 syslog 导出。
func (h *Handler) auditDataPlane(r *http.Request, clusterID, obj, verb string, allowed bool, reason string, start time.Time, reqBody, respBody string, status int) {
	ev := h.clusterEvent(r, allowed, reason)
	ev.Cluster = clusterID
	ev.Path = r.URL.RequestURI() // 原始请求（含 query，审计溯源）
	ev.Object = obj              // 鉴权语义资源串
	ev.Verb = verb
	ev.Method = r.Method
	ev.LatencyMS = time.Since(start).Milliseconds()
	ev.RequestBody = reqBody
	ev.ResponseBody = respBody
	ev.StatusCode = status
	h.auditCluster(ev)
}

// auditCluster 提交集群操作事件
func (h *Handler) auditCluster(ev *modules.AuditEvent) {
	if h.audit == nil || ev == nil {
		return
	}
	h.audit.Log(ev)
}

// truncateBody 审计体截断（附标记）
func truncateBody(b []byte) string {
	if len(b) <= maxAuditBody {
		return string(b)
	}
	return string(b[:maxAuditBody]) + "…(truncated)"
}

// captureRequestBody 读取请求体前缀后还原（供代理继续转发）。
// GET/HEAD/OPTIONS 不捕获；其余方法读取至多 maxAuditBody+1 字节。
func captureRequestBody(r *http.Request) string {
	if r.Body == nil || r.Body == http.NoBody {
		return ""
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return ""
	}
	prefix, err := io.ReadAll(io.LimitReader(r.Body, maxAuditBody+1))
	if err != nil {
		return ""
	}
	rest := r.Body
	// 还原：prefix + 剩余流（代理继续消费；Close 透传给原 Body）
	r.Body = struct {
		io.Reader
		io.Closer
	}{Reader: io.MultiReader(bytes.NewReader(prefix), rest), Closer: rest}
	return truncateBody(prefix)
}

// captureWriter 响应捕获包装：记录状态码与前 maxAuditBody 字节，
// 其余行为原样透传（含 Flush，保证流式响应不被破坏）。
type captureWriter struct {
	http.ResponseWriter
	status        int
	body          bytes.Buffer
	bodyTruncated bool
}

func newCaptureWriter(w http.ResponseWriter) *captureWriter {
	return &captureWriter{ResponseWriter: w, status: http.StatusOK}
}

func (w *captureWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *captureWriter) Write(b []byte) (int, error) {
	if !w.bodyTruncated {
		if room := maxAuditBody + 1 - w.body.Len(); room > 0 {
			if len(b) > room {
				w.body.Write(b[:room])
				w.bodyTruncated = true
			} else {
				w.body.Write(b)
			}
		}
	}
	return w.ResponseWriter.Write(b)
}

// Flush 透传 http.Flusher（流式/长响应）
func (w *captureWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Body 返回捕获的响应体（truncated）；gzip 等压缩响应标记为不可读
func (w *captureWriter) Body() string {
	if enc := w.Header().Get("Content-Encoding"); enc != "" && enc != "identity" {
		return "(响应体经 " + enc + " 压缩，不记录)"
	}
	return truncateBody(w.body.Bytes())
}

// Status 返回响应状态码
func (w *captureWriter) Status() int { return w.status }

// truncate 字符串截断（按字节，用于头部值）
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
