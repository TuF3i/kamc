package handler

import (
	"net/http"
	"runtime"
	"time"

	"github.com/TuF3i/kamc/internal/platform/errcode"
)

// SystemInfo GET /api/v1/system —— 平台进程运行时信息（管理面）。
// 只暴露聚合后的运行指标，不包含配置/密钥。
func (h *Handler) SystemInfo(w http.ResponseWriter, _ *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	dbStats := h.svc.DBStats()
	uptime := int64(time.Since(h.startedAt).Seconds())
	if uptime < 0 {
		uptime = 0
	}
	errcode.GenFinalResponse(w, map[string]any{
		"go_version":              runtime.Version(),
		"goroutines":              runtime.NumGoroutine(),
		"num_cpu":                 runtime.NumCPU(),
		"heap_alloc_bytes":        m.HeapAlloc,
		"heap_sys_bytes":          m.HeapSys,
		"sys_bytes":               m.Sys,
		"total_alloc_bytes":       m.TotalAlloc,
		"num_gc":                  m.NumGC,
		"uptime_seconds":          uptime,
		"db_open_connections":     dbStats.OpenConnections,
		"db_in_use":               dbStats.InUse,
		"db_idle":                 dbStats.Idle,
		"db_wait_count":           dbStats.WaitCount,
		"db_max_open_connections": dbStats.MaxOpenConnections,
	}, nil)
}
