package authz

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
)

// newMemEnforcer 内存策略的测试 Enforcer（绕过 DB 适配器，纯匹配语义验证）
func newMemEnforcer(t *testing.T, rules [][]string) *casbin.SyncedEnforcer {
	t.Helper()
	m, err := model.NewModelFromString(modelText)
	if err != nil {
		t.Fatalf("model: %v", err)
	}
	e, err := casbin.NewSyncedEnforcer(m)
	if err != nil {
		t.Fatalf("enforcer: %v", err)
	}
	for _, r := range rules {
		var err error
		switch r[0] {
		case "p":
			_, err = e.AddPolicy(r[1], r[2], r[3], r[4])
		case "g":
			_, err = e.AddGroupingPolicy(r[1], r[2], r[3])
		default:
			t.Fatalf("unknown rule type %q", r[0])
		}
		if err != nil {
			t.Fatalf("add policy %v: %v", r, err)
		}
	}
	return e
}

// developerRBAC 设计文档 §5.3 的开发者角色
var developerRBAC = [][]string{
	{"p", "role:developer", "cluster-a", "dev/*", "get|list|watch|create|update|patch|delete|deletecollection"},
	{"g", "bob", "role:developer", "cluster-a"},
}

func enforceWith(t *testing.T, e *casbin.SyncedEnforcer, sub, dom, method, path string) bool {
	t.Helper()
	obj, act, err := ParseRequest(method, path)
	if err != nil {
		t.Fatalf("parse %s %s: %v", method, path, err)
	}
	ok, err := e.Enforce(sub, dom, obj, act)
	if err != nil {
		t.Fatalf("enforce: %v", err)
	}
	return ok
}

// TestDeveloperPolicy 开发者角色：本命名空间放行、跨命名空间拒绝。
func TestDeveloperPolicy(t *testing.T) {
	e := newMemEnforcer(t, developerRBAC)

	if !enforceWith(t, e, "bob", "cluster-a", "GET", "/api/v1/namespaces/dev/pods") {
		t.Fatal("list pods in dev should be allowed")
	}
	if enforceWith(t, e, "bob", "cluster-a", "GET", "/api/v1/namespaces/prod/pods") {
		t.Fatal("list pods in prod should be denied")
	}
	if enforceWith(t, e, "bob", "cluster-b", "GET", "/api/v1/namespaces/dev/pods") {
		t.Fatal("policy is per-dom; cluster-b should be denied")
	}
}

// TestFix8ActAnchoring FIX-8 回归：`delete` 策略不得匹配 `deletecollection`
// （未锚定 regexMatch 的越权漏洞）。
func TestFix8ActAnchoring(t *testing.T) {
	e := newMemEnforcer(t, [][]string{
		{"p", "role:limited", "cluster-a", "dev/pods", "delete"},
		{"g", "carol", "role:limited", "cluster-a"},
	})

	if !enforceWith(t, e, "carol", "cluster-a", "DELETE", "/api/v1/namespaces/dev/pods/pod-1") {
		t.Fatal("single delete should be allowed")
	}
	if enforceWith(t, e, "carol", "cluster-a", "DELETE", "/api/v1/namespaces/dev/pods") {
		t.Fatal("deletecollection must NOT match a delete-only policy (FIX-8)")
	}
}

// TestSubresourcePolicy 子资源精确匹配：exec 允许、log 不涉及。
func TestSubresourcePolicy(t *testing.T) {
	e := newMemEnforcer(t, [][]string{
		{"p", "role:execer", "cluster-a", "dev/pods/exec", "create"},
		{"g", "dave", "role:execer", "cluster-a"},
	})

	if !enforceWith(t, e, "dave", "cluster-a", "POST", "/api/v1/namespaces/dev/pods/pod-1/exec") {
		t.Fatal("exec should be allowed")
	}
	if enforceWith(t, e, "dave", "cluster-a", "GET", "/api/v1/namespaces/dev/pods") {
		t.Fatal("list pods should be denied (policy covers exec only)")
	}
}

// TestClusterScopedAndNonResource 集群级资源与非资源路径。
func TestClusterScopedAndNonResource(t *testing.T) {
	e := newMemEnforcer(t, [][]string{
		{"p", "role:viewer", "cluster-a", "cluster/nodes", "get|list"},
		{"p", "role:any", "*", "nonresource/version", "get"},
		{"g", "erin", "role:viewer", "cluster-a"},
		{"g", "erin", "role:any", "cluster-a"},
	})

	if !enforceWith(t, e, "erin", "cluster-a", "GET", "/apis/metrics.k8s.io/v1beta1/nodes") {
		t.Fatal("cluster-scoped nodes list should be allowed")
	}
	if enforceWith(t, e, "erin", "cluster-a", "DELETE", "/api/v1/nodes/node-1") {
		t.Fatal("node delete should be denied")
	}
	if !enforceWith(t, e, "erin", "cluster-a", "GET", "/version") {
		t.Fatal("nonresource /version should be allowed")
	}
}

// TestParseRequestGrammar 语义资源串语法（§5.1 表格逐项验证）。
func TestParseRequestGrammar(t *testing.T) {
	cases := []struct {
		method, path string
		wantObj      string
		wantAct      string
	}{
		{"GET", "/api/v1/namespaces/dev/pods", "dev/pods", "list"},
		{"GET", "/api/v1/namespaces/dev/pods/foo", "dev/pods", "get"},
		{"POST", "/api/v1/namespaces/dev/pods/foo/exec", "dev/pods/exec", "create"},
		{"GET", "/api/v1/namespaces/dev/pods/foo/log", "dev/pods/log", "get"},
		{"GET", "/apis/apps/v1/deployments", "cluster/deployments", "list"},
		{"GET", "/version", "nonresource/version", "get"},
	}
	for _, c := range cases {
		obj, act, err := ParseRequest(c.method, c.path)
		if err != nil {
			t.Fatalf("parse %s %s: %v", c.method, c.path, err)
		}
		if obj != c.wantObj || act != c.wantAct {
			t.Fatalf("%s %s: got (%s, %s), want (%s, %s)",
				c.method, c.path, obj, act, c.wantObj, c.wantAct)
		}
	}
}

// TestWatchVerbDerived watch 动词由 query 参数推导（client-go watch 请求）。
func TestWatchVerbDerived(t *testing.T) {
	r := &http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/api/v1/namespaces/dev/pods", RawQuery: "watch=true"},
	}
	info, err := reqResolver.NewRequestInfo(r)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if info.Verb != "watch" {
		t.Fatalf("verb: got %q, want watch", info.Verb)
	}
}
