package middleware

import "net/http"

// LimitBody 限制请求体大小（JSON/表单 API 用；/proxy 流式请求不能套）。
// 超限时读取返回 *http.MaxBytesError，由各 handler 按参数错误处理。
func LimitBody(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if maxBytes > 0 && r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}
