package platform_test

import (
	"net/http"
	"testing"

	"github.com/TuF3i/kamc/internal/platform/errcode"
)

// TestRevocationE2E Token 吊销语义（进程内存储路径，Redis 同步由 auth 单测覆盖）：
//  1. 踢出用户：标记前的 Token 401，新签发的可用
//  2. 登出：单 Token 失效，同用户其他 Token 不受影响
func TestRevocationE2E(t *testing.T) {
	rt := newTestRuntime(t)
	tokA := devToken(t, rt, "admin-test", []string{"am:admin"})

	// 基线：可用
	if _, ar := call(t, rt, http.MethodGet, "/auth/me", nil, tokA); ar.Status != errcode.CodeSuccess {
		t.Fatalf("baseline me: %+v", ar)
	}

	// ── 1. 踢出用户 ──
	_, ar := call(t, rt, http.MethodPost, "/api/v1/auth/revoke",
		map[string]string{"sub": "admin-test"}, tokA)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("revoke user: %+v", ar)
	}
	if resp := callRaw(t, rt, http.MethodGet, "/auth/me", tokA); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked token: want 401, got %d", resp.StatusCode)
	}

	// 新签发（iat 晚于标记）→ 可用
	tokNew := devToken(t, rt, "admin-test", []string{"am:admin"})
	if _, ar = call(t, rt, http.MethodGet, "/auth/me", nil, tokNew); ar.Status != errcode.CodeSuccess {
		t.Fatalf("token issued after marker should work: %+v", ar)
	}

	// ── 2. 登出（jti 级）──
	tokB := devToken(t, rt, "bob", nil)
	tokC := devToken(t, rt, "bob", nil)
	resp, ar := call(t, rt, http.MethodPost, "/auth/logout", nil, tokB)
	if resp.StatusCode != http.StatusOK || ar.Status != errcode.CodeSuccess {
		t.Fatalf("logout: http=%d body=%+v", resp.StatusCode, ar)
	}
	if resp := callRaw(t, rt, http.MethodGet, "/auth/me", tokB); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("logged-out token: want 401, got %d", resp.StatusCode)
	}
	if _, ar = call(t, rt, http.MethodGet, "/auth/me", nil, tokC); ar.Status != errcode.CodeSuccess {
		t.Fatalf("sibling token must survive logout: %+v", ar)
	}

	// 参数缺失 → 业务码错误
	_, ar = call(t, rt, http.MethodPost, "/api/v1/auth/revoke",
		map[string]string{"sub": ""}, tokNew)
	if ar.Status != errcode.CodeInvalidParam {
		t.Fatalf("empty sub: %+v", ar)
	}
}
