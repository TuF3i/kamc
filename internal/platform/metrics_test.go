package platform_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/TuF3i/kamc/internal/platform/errcode"
)

// TestMetricsEndpoint /metrics 暴露（§12.3）：端点可达、kamc_ 前缀指标存在、
// 数据面请求计数随请求递增。
func TestMetricsEndpoint(t *testing.T) {
	trt, rt, _ := setupTunnelPlatform(t, "http://unused")
	defer func() { _ = trt.ReleaseRuntime() }()

	tok := adminToken(t, rt)
	clusterID := fmt.Sprintf("e2e-metrics-%d", time.Now().UnixNano())

	// 拒绝路径：denials 计数
	if resp := callRaw(t, rt, http.MethodGet, "/proxy/"+clusterID+"/api/v1/pods", tok); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("seed deny request: want 403, got %d", resp.StatusCode)
	}

	// 允许路径（集群无 Agent → 503，但请求计数/耗时已产生）：requests_total 获得子标签
	if _, ar := call(t, rt, http.MethodPost, "/api/v1/clusters",
		map[string]string{"id": clusterID, "name": "metrics"}, tok); ar.Status != errcode.CodeSuccess {
		t.Fatalf("create cluster: %+v", ar)
	}
	seedUserPolicy(t, rt, tok, "admin-test", clusterID)
	if resp := callRaw(t, rt, http.MethodGet, "/proxy/"+clusterID+"/api/v1/pods", tok); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("seed allow request: want 503, got %d", resp.StatusCode)
	}

	resp := callRaw(t, rt, http.MethodGet, "/metrics", tok)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/metrics: %d", resp.StatusCode)
	}
	body := string(readAll(t, resp))
	for _, want := range []string{
		"kamc_requests_total",
		"kamc_casbin_denials_total",
		"kamc_tunnel_connected_clusters",
		"kamc_rate_limited_total",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("/metrics missing %q", want)
		}
	}
	// 拒绝计数已递增（403 路径）
	if !strings.Contains(body, `kamc_casbin_denials_total{cluster="`+clusterID+`"} 1`) {
		t.Fatalf("denials counter not present:\n%s", body)
	}
}
