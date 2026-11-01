package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/TuF3i/kamc/internal/auth"
)

// withTestClaims 模拟 RequireJWT 注入身份（claimsKey 同包可访问）
func withTestClaims(ctx context.Context, sub string) context.Context {
	return context.WithValue(ctx, claimsKey, &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: sub},
	})
}

// TestMemoryRateLimiterWindow 滑动窗口：限内放行、超限拒绝、滑出后恢复。
func TestMemoryRateLimiterWindow(t *testing.T) {
	now := time.Now()
	l := NewMemoryRateLimiter(time.Minute, 3)
	l.SetNow(func() time.Time { return now })
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		ok, err := l.Allow(ctx, "user:alice")
		if err != nil || !ok {
			t.Fatalf("request %d should pass: ok=%v err=%v", i+1, ok, err)
		}
	}
	if ok, _ := l.Allow(ctx, "user:alice"); ok {
		t.Fatal("4th request within window should be blocked")
	}

	// 窗口滑出（时钟前移 61s）→ 恢复
	now = now.Add(61 * time.Second)
	if ok, _ := l.Allow(ctx, "user:alice"); !ok {
		t.Fatal("request after window slide should pass")
	}

	// 不同 key 独立计数
	if ok, _ := l.Allow(ctx, "user:bob"); !ok {
		t.Fatal("different key should have its own window")
	}
}

// TestRateLimitMiddleware 中间件：限内 200、超限 429 + Retry-After、未启用直通。
func TestRateLimitMiddleware(t *testing.T) {
	claimsInjector := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 模拟 RequireJWT 注入身份（限流按用户计数的前置）
			r = r.WithContext(withTestClaims(r.Context(), "alice"))
			next.ServeHTTP(w, r)
		})
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	l := NewMemoryRateLimiter(time.Minute, 2)
	srv := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claimsInjector(RateLimit(l, 2, time.Minute)(handler)).ServeHTTP(w, r)
	})

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/proxy/x", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: want 200, got %d", i+1, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/proxy/x", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("3rd request: want 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") != "60" {
		t.Fatalf("Retry-After: %q", rec.Header().Get("Retry-After"))
	}

	// 未启用（nil limiter）→ 直通
	passthrough := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claimsInjector(RateLimit(nil, 0, time.Minute)(handler)).ServeHTTP(w, r)
	})
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		passthrough.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/proxy/x", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("disabled limiter request %d: got %d", i+1, rec.Code)
		}
	}
}
