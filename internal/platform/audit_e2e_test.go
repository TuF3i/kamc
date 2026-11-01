package platform_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

// TestAuditE2E 审计落库（§12.2）：数据面允许/拒绝事件、踢出事件
// 异步批量写入 PG，管理面查询 API 可读回（字段与过滤断言）。
func TestAuditE2E(t *testing.T) {
	trt, rt, _ := setupTunnelPlatform(t, "http://unused")
	defer func() { _ = trt.ReleaseRuntime() }()
	tok := adminToken(t, rt)
	id := fmt.Sprintf("e2e-audit-%d", time.Now().UnixNano())

	// 建集群（无 Agent → /proxy 走到 503；先拒绝再允许的场景用 Casbin 拒绝）
	_, ar := call(t, rt, http.MethodPost, "/api/v1/clusters",
		map[string]string{"id": id, "name": "audit"}, tok)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("create cluster: %+v", ar)
	}
	t.Cleanup(func() { _, _ = call(t, rt, http.MethodDelete, "/api/v1/clusters/"+id, nil, tok) })

	// 未授权用户 → Casbin 拒绝（denied 事件）
	viewer := devToken(t, rt, "audit-viewer", nil)
	if resp := callRaw(t, rt, http.MethodGet, "/proxy/"+id+"/api/v1/pods", viewer); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer: want 403, got %d", resp.StatusCode)
	}
	// 授权绑定 → 允许（allowed 事件；集群无 Agent → 503，但事件已产生）
	if _, ar = call(t, rt, http.MethodPost, "/api/v1/policies",
		map[string]string{"sub": "role:a", "dom": id, "obj": "*", "act": ".*"}, tok); ar.Status != errcode.CodeSuccess {
		t.Fatalf("policy: %+v", ar)
	}
	if _, ar = call(t, rt, http.MethodPost, "/api/v1/bindings",
		map[string]string{"user": "audit-viewer", "role": "role:a", "dom": id}, tok); ar.Status != errcode.CodeSuccess {
		t.Fatalf("binding: %+v", ar)
	}
	if resp := callRaw(t, rt, http.MethodGet, "/proxy/"+id+"/api/v1/pods", viewer); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("no-agent cluster: want 503, got %d", resp.StatusCode)
	}

	// 踢出事件
	if _, ar = call(t, rt, http.MethodPost, "/api/v1/auth/revoke",
		map[string]string{"sub": "audit-viewer"}, tok); ar.Status != errcode.CodeSuccess {
		t.Fatalf("revoke: %+v", ar)
	}

	// 等待异步批量落库，查询 API 读回
	deadline := time.Now().Add(5 * time.Second)
	var events []modules.AuditEvent
	for time.Now().Before(deadline) {
		_, ar := call(t, rt, http.MethodGet, "/api/v1/audit?cluster="+id+"&limit=100", nil, tok)
		if ar.Status != errcode.CodeSuccess {
			t.Fatalf("list audit: %+v", ar)
		}
		if err := json.Unmarshal(ar.Data, &events); err != nil {
			t.Fatalf("decode events: %v", err)
		}
		if len(events) >= 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(events) < 2 {
		t.Fatalf("want >=2 audit events, got %d", len(events))
	}

	var sawDenied, sawAllowed bool
	for _, ev := range events {
		if ev.Cluster != id {
			t.Fatalf("cluster filter failed: %+v", ev)
		}
		switch {
		case ev.Reason == "denied-casbin":
			sawDenied = true
			if ev.Allowed || ev.User != "audit-viewer" || ev.Verb != "list" {
				t.Fatalf("denied event mismatch: %+v", ev)
			}
		case ev.Reason == "allowed":
			sawAllowed = true
			if !ev.Allowed || ev.User != "audit-viewer" {
				t.Fatalf("allowed event mismatch: %+v", ev)
			}
		}
	}
	if !sawDenied || !sawAllowed {
		t.Fatalf("want denied+allowed events, got denied=%v allowed=%v (%+v)", sawDenied, sawAllowed, events)
	}

	// 用户过滤
	_, ar = call(t, rt, http.MethodGet, "/api/v1/audit?user=audit-viewer&limit=100", nil, tok)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("user filter query: %+v", ar)
	}
	if err := json.Unmarshal(ar.Data, &events); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, ev := range events {
		if ev.User != "audit-viewer" {
			t.Fatalf("user filter failed: %+v", ev)
		}
	}
}
