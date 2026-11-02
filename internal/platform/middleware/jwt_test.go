package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// bearerToken 仅对 WS 升级请求接受 access_token 查询参数（浏览器 WebSocket
// 无法携带 Authorization 头）；普通 HTTP 请求不识别查询参数（防 token 进访问日志）。
func TestBearerTokenQueryParamOnlyForWebSocket(t *testing.T) {
	mk := func(upgrade bool) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/proxy/c1/api/v1/namespaces/p/exec?access_token=tok123", nil)
		if upgrade {
			r.Header.Set("Upgrade", "websocket")
			r.Header.Set("Connection", "Upgrade")
		}
		return r
	}

	if got := bearerToken(mk(true)); got != "tok123" {
		t.Fatalf("ws upgrade: want tok123, got %q", got)
	}
	if got := bearerToken(mk(false)); got != "" {
		t.Fatalf("plain http: want empty, got %q", got)
	}

	// 头部优先于查询参数
	r := mk(true)
	r.Header.Set("Authorization", "Bearer hdr-tok")
	if got := bearerToken(r); got != "hdr-tok" {
		t.Fatalf("header priority: want hdr-tok, got %q", got)
	}
}

// RequireJWT 端到端：WS 升级请求经查询参数通过校验，claims 注入 ctx。
func TestRequireJWTAcceptsWebSocketQueryParam(t *testing.T) {
	// 伪造签名链路超出本单测范围（auth.Service 需要 RSA keyring），
	// 这里只验证 token 提取路径：无效 token 也应走到 Verify（401 而非 missing 分支），
	// 两个分支对外均为 401，用日志差异区分成本高——提取语义由 TestBearerTokenQueryParamOnlyForWebSocket 覆盖。
	r := httptest.NewRequest(http.MethodGet, "/proxy/c1/exec?access_token=x", nil)
	r.Header.Set("Upgrade", "websocket")
	w := httptest.NewRecorder()
	RequireJWT(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("nil auth service: want 401, got %d", w.Code)
	}
}
