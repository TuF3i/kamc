// Package middleware 平台 HTTP 横切关注点：只做日志与 ctx 注入，不做业务判断。
// 认证/鉴权类中间件（JWT、Casbin）落地后也放本包。
package middleware

import (
	"log"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// AccessLog 每请求一行访问日志：method path status 耗时 来源 request_id。
// 访问日志是 HTTP 服务的常规一次性事件，不受"循环内禁刷日志"约束。
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		defer func() {
			log.Printf("[platform] %s %s %d %s %s req_id=%s",
				r.Method, r.URL.Path, ww.Status(),
				time.Since(start).Round(time.Millisecond), r.RemoteAddr,
				chimw.GetReqID(r.Context()))
		}()
		next.ServeHTTP(ww, r)
	})
}
