package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/TuF3i/kamc/internal/metrics"
)

// Metrics 采集全部 HTTP 请求指标：
//   - 请求总数（method/route/status）
//   - 耗时直方图（method/route）
//   - 处理中请求数
//   - 响应字节数（method/route）
//
// route 使用 chi 路由模式（如 /api/v1/clusters/{id}），避免原始路径造成高基数；
// 未匹配路由（SPA 回退、404）统一归入 "unmatched"。
func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		metrics.HTTPRequestsInFlight.Inc()
		defer metrics.HTTPRequestsInFlight.Dec()

		next.ServeHTTP(ww, r)

		route := chi.RouteContext(r.Context()).RoutePattern()
		if route == "" {
			route = "unmatched"
		}
		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, route, strconv.Itoa(statusOrOK(ww.Status()))).Inc()
		metrics.HTTPRequestDurationSeconds.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
		metrics.HTTPResponseBytesTotal.WithLabelValues(r.Method, route).Add(float64(ww.BytesWritten()))
	})
}
