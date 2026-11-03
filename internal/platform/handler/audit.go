package handler

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/middleware"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

const (
	// maxAuditBody 请求/响应体在审计报文中的截断上限（字节）
	maxAuditBody = 16 * 1024
	// maxAuditHeaderBytes 报文头总长上限（防止异常超长头撑爆记录）
	maxAuditHeaderBytes = 4 * 1024
)

// sensitiveHeaders 敏感头遮蔽：审计保留头结构但不泄露凭证
var sensitiveHeaders = map[string]bool{
	"authorization":       true,
	"proxy-authorization": true,
	"cookie":              true,
	"set-cookie":          true,
	"x-api-key":           true,
	"x-auth-token":        true,
	"impersonate-token":   true,
	"impersonate-uid":     true,
}

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
// 完整 HTTP 报文（请求/响应），kind=cluster 落库（不落 stdio）+ 可选 syslog。
func (h *Handler) auditDataPlane(r *http.Request, clusterID, obj, verb string, allowed bool, reason string, start time.Time, reqDump, respDump string, status int) {
	ev := h.clusterEvent(r, allowed, reason)
	ev.Cluster = clusterID
	ev.Path = r.URL.RequestURI() // 原始请求（含 query，审计溯源）
	ev.Object = obj              // 鉴权语义资源串
	ev.Verb = verb
	ev.Method = r.Method
	ev.LatencyMS = time.Since(start).Milliseconds()
	ev.RequestBody = reqDump
	ev.ResponseBody = respDump
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

// ── HTTP 报文捕获与格式化 ──

// maskHeader 敏感头遮蔽（保留键名便于排查）
func maskHeader(key, value string) string {
	if sensitiveHeaders[strings.ToLower(key)] {
		return "***"
	}
	return truncate(value, 512)
}

// buildHTTPMessage 组装可读的完整 HTTP 报文：起始行 + 头 + 格式化 body。
func buildHTTPMessage(startLine string, header http.Header, body []byte, truncated, decompressed bool) string {
	var b strings.Builder
	b.WriteString(startLine)
	b.WriteString("\r\n")

	keys := make([]string, 0, len(header))
	for k := range header {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	headerBytes := 0
	for _, k := range keys {
		for _, v := range header[k] {
			line := k + ": " + maskHeader(k, v) + "\r\n"
			if headerBytes+len(line) > maxAuditHeaderBytes {
				b.WriteString("…(headers truncated)\r\n")
				headerBytes = -1
				break
			}
			b.WriteString(line)
			headerBytes += len(line)
		}
		if headerBytes < 0 {
			break
		}
	}
	b.WriteString("\r\n")
	b.WriteString(formatAuditBody(header.Get("Content-Type"), body, truncated, decompressed))
	return b.String()
}

// formatAuditBody body 美化：JSON（按 Content-Type 或内容嗅探）缩进 2 空格，
// 其余原样；截断/解压状态以注释行标注。
func formatAuditBody(contentType string, raw []byte, truncated, decompressed bool) string {
	var sb strings.Builder
	if decompressed {
		sb.WriteString("(body 已从 gzip 解压展示)\n")
	}
	if len(raw) == 0 {
		sb.WriteString("(无 body)")
	} else if looksLikeJSON(contentType, raw) && !truncated {
		var buf bytes.Buffer
		if err := json.Indent(&buf, raw, "", "  "); err == nil {
			sb.Write(buf.Bytes())
		} else {
			sb.Write(raw)
		}
	} else {
		sb.Write(raw)
	}
	if truncated {
		sb.WriteString("\n…(truncated)")
	}
	return sb.String()
}

// looksLikeJSON Content-Type 含 json，或 body 以 { / [ 开头
func looksLikeJSON(contentType string, raw []byte) bool {
	if strings.Contains(strings.ToLower(contentType), "json") {
		return true
	}
	for _, c := range raw {
		switch c {
		case ' ', '\t', '\r', '\n':
			continue
		case '{', '[':
			return true
		default:
			return false
		}
	}
	return false
}

// decodeBody gzip 解压（压缩由客户端 Accept-Encoding 触发，审计需可读内容）
func decodeBody(header http.Header, raw []byte) (body []byte, truncated, decompressed bool) {
	enc := strings.ToLower(header.Get("Content-Encoding"))
	if !strings.Contains(enc, "gzip") || len(raw) == 0 {
		return raw, false, false
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return raw, false, false
	}
	defer zr.Close()
	dec, err := io.ReadAll(io.LimitReader(zr, maxAuditBody+1))
	if err != nil {
		return raw, false, false
	}
	if len(dec) > maxAuditBody {
		dec = dec[:maxAuditBody]
		truncated = true
	}
	return dec, truncated, true
}

// captureRequestDump 捕获完整请求报文（起始行 + 头 + 格式化 body），
// 同时把请求体还原给代理继续转发。请求头在此刻快照（早于剥离/注入）。
func captureRequestDump(r *http.Request) string {
	header := r.Header.Clone()
	var raw []byte
	truncated := false
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		// 无 body
	default:
		if r.Body != nil && r.Body != http.NoBody {
			prefix, err := io.ReadAll(io.LimitReader(r.Body, maxAuditBody+1))
			if err == nil {
				if len(prefix) > maxAuditBody {
					prefix = prefix[:maxAuditBody]
					truncated = true
				}
				raw = prefix
				rest := r.Body
				// 还原：prefix + 剩余流（代理继续消费；Close 透传给原 Body）
				r.Body = struct {
					io.Reader
					io.Closer
				}{Reader: io.MultiReader(bytes.NewReader(raw), rest), Closer: rest}
			}
		}
	}
	body, gzTrunc, gzDec := decodeBody(header, raw)
	startLine := fmt.Sprintf("%s %s %s", r.Method, r.URL.RequestURI(), r.Proto)
	return buildHTTPMessage(startLine, header, body, truncated || gzTrunc, gzDec)
}

// captureWriter 响应捕获包装：记录状态码/响应头/前 maxAuditBody 字节，
// 其余行为原样透传（含 Flush，保证流式响应不被破坏）。
type captureWriter struct {
	http.ResponseWriter
	proto         string
	status        int
	header        http.Header
	body          bytes.Buffer
	bodyTruncated bool
}

func newCaptureWriter(w http.ResponseWriter, proto string) *captureWriter {
	return &captureWriter{ResponseWriter: w, proto: proto, status: http.StatusOK}
}

func (w *captureWriter) WriteHeader(code int) {
	w.status = code
	w.header = w.Header().Clone() // 快照（之后头不再变）
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

// Dump 组装完整响应报文（状态行 + 头 + 格式化 body）
func (w *captureWriter) Dump() string {
	header := w.header
	if header == nil {
		header = w.Header().Clone()
	}
	body, gzTrunc, gzDec := decodeBody(header, w.body.Bytes())
	startLine := fmt.Sprintf("%s %d %s", w.proto, w.status, http.StatusText(w.status))
	return buildHTTPMessage(startLine, header, body, w.bodyTruncated || gzTrunc, gzDec)
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
