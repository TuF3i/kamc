package platform_test

import (
	"net/http"
	"testing"

	"github.com/TuF3i/kamc/internal/platform/errcode"
)

// TestRateLimitE2E 数据面限流（AM_RATE_LIMIT=2）：
// 同一用户第 3 个 /proxy 请求 429 + Retry-After；管理面不受限。
func TestRateLimitE2E(t *testing.T) {
	// 限流参数在 CreateRuntime 装配时读取
	t.Setenv("AM_RATE_LIMIT", "2")
	trt, rt, srv := setupTunnelPlatform(t, "http://unused")
	defer func() { _ = trt.ReleaseRuntime() }()
	defer func() { _ = rt.ReleaseRuntime() }()
	_ = srv

	tok := devToken(t, rt, "rl-user", nil)

	// /proxy 对未授权集群是 403（Casbin 先行）——限流在鉴权之后但计数在
	// Allow 调用（无论 403/503 都计数）。直接打 /proxy 计数。
	for i := 0; i < 2; i++ {
		resp := callRaw(t, rt, http.MethodGet, "/proxy/some-cluster/api/v1/pods", tok)
		if resp.StatusCode == http.StatusTooManyRequests {
			t.Fatalf("request %d within limit should not be 429", i+1)
		}
	}
	resp := callRaw(t, rt, http.MethodGet, "/proxy/some-cluster/api/v1/pods", tok)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("3rd request: want 429, got %d", resp.StatusCode)
	}

	// 管理面不受数据面限流影响
	if _, ar := call(t, rt, http.MethodGet, "/auth/me", nil, tok); ar.Status != errcode.CodeSuccess {
		t.Fatalf("auth endpoints should not be rate limited: %+v", ar)
	}

	// 不同用户独立窗口
	tokOther := devToken(t, rt, "rl-user-2", nil)
	if resp := callRaw(t, rt, http.MethodGet, "/proxy/some-cluster/api/v1/pods", tokOther); resp.StatusCode == http.StatusTooManyRequests {
		t.Fatal("different user should have independent window")
	}
}
