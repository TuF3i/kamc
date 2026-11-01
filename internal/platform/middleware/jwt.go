// Package middleware 平台 HTTP 横切关注点：JWT 认证与管理员校验。
// 只做校验与 ctx 注入，不做业务判断。
package middleware

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/TuF3i/kamc/internal/auth"
)

type claimsKeyType int

const claimsKey claimsKeyType = iota

// ClaimsFromCtx 取当前请求的 JWT claims（RequireJWT 之后可用）
func ClaimsFromCtx(ctx context.Context) (*auth.Claims, bool) {
	c, ok := ctx.Value(claimsKey).(*auth.Claims)
	return c, ok && c != nil
}

// RequireJWT 强制 Bearer 平台 JWT：验证失败一律 401（不区分过期/无效，防枚举；
// WWW-Authenticate 头供客户端刷新语义）。claims 注入 ctx 供下游使用。
func RequireJWT(as *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerToken(r)
			if token == "" || as == nil {
				log.Printf("[platform] jwt missing (remote=%s path=%s)", r.RemoteAddr, r.URL.Path)
				unauthorized(w)
				return
			}
			claims, err := as.Verify(token)
			if err != nil {
				log.Printf("[platform] jwt verify failed: %v", err)
				unauthorized(w)
				return
			}
			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin 管理员组校验：须在 RequireJWT 之后使用。
// 管理面（集群/Token/策略管理）按最小授权原则限定管理员组；
// 数据面 /proxy 的资源级鉴权由 Casbin 承担（后续里程碑）。
func RequireAdmin(group string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromCtx(r.Context())
			if !ok || !hasGroup(claims.Groups, group) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="kamc"`)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func hasGroup(groups []string, want string) bool {
	for _, g := range groups {
		if g == want {
			return true
		}
	}
	return false
}
