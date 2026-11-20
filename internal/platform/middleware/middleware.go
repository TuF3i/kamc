// Package middleware 平台 HTTP 横切关注点：只做日志、指标与 ctx 注入，不做业务判断。
package middleware

import (
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

// AccessLog 每请求一行结构化访问日志：method/path/status/耗时/来源/request_id。
// 访问日志是 HTTP 服务的常规一次性事件，不受"循环内禁刷日志"约束。
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		defer func() {
			zap.L().Info("http request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", statusOrOK(ww.Status())),
				zap.Duration("latency", time.Since(start).Round(time.Millisecond)),
				zap.String("remote", r.RemoteAddr),
				zap.String("request_id", chimw.GetReqID(r.Context())),
			)
		}()
		next.ServeHTTP(ww, r)
	})
}

func statusOrOK(status int) int {
	if status == 0 {
		return http.StatusOK
	}
	return status
}
