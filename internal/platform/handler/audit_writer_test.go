package handler

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// hijackableRecorder 模拟真实 http.ResponseWriter 的 Hijack 能力。
type hijackableRecorder struct {
	*httptest.ResponseRecorder
	conn net.Conn
}

func (h *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return h.conn, bufio.NewReadWriter(bufio.NewReader(h.conn), bufio.NewWriter(h.conn)), nil
}

// TestCaptureWriterHijack 覆盖 SPDY 升级（port-forward / 旧版 kubectl exec）
// 依赖的 Hijack 透传：包装后必须仍可被 ReverseProxy/UpgradeAwareHandler 识别。
func TestCaptureWriterHijack(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	base := &hijackableRecorder{ResponseRecorder: httptest.NewRecorder(), conn: server}
	cw := newCaptureWriter(base, "HTTP/1.1")
	ww := chimw.NewWrapResponseWriter(cw, 1)
	hj, ok := ww.(http.Hijacker)
	if !ok {
		t.Fatalf("wrapped writer %T lost http.Hijacker", ww)
	}
	conn, _, err := hj.Hijack()
	if err != nil || conn == nil {
		t.Fatalf("hijack: conn=%v err=%v", conn, err)
	}
}

// TestCaptureWriterHijackUnsupported 底层不支持 Hijack 时返回明确错误，
// 避免静默降级成半开连接。
func TestCaptureWriterHijackUnsupported(t *testing.T) {
	cw := newCaptureWriter(httptest.NewRecorder(), "HTTP/1.1")
	if _, _, err := cw.Hijack(); err == nil || !strings.Contains(err.Error(), "not http.Hijacker") {
		t.Fatalf("want not-Hijacker error, got %v", err)
	}
}
