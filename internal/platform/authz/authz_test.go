package authz

import (
	"testing"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
)

// newMemEnforcer 内存策略的测试 Enforcer（绕过 DB 适配器，纯匹配语义验证）。
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
		if r[0] != "p" {
			t.Fatalf("only p rules are supported, got %v", r)
		}
		if _, err := e.AddPolicy(r[1], r[2], r[3], r[4]); err != nil {
			t.Fatalf("add policy %v: %v", r, err)
		}
	}
	return e
}

func enforceWith(
	t *testing.T,
	e *casbin.SyncedEnforcer,
	sub string,
	groups []string,
	dom, method, path, rawQuery string,
) bool {
	t.Helper()
	obj, act, err := ParseRequest(method, path, rawQuery)
	if err != nil {
		t.Fatalf("parse %s %s: %v", method, path, err)
	}
	en := &Enforcer{e: e}
	ok, err := en.Enforce(sub, groups, dom, obj, act)
	if err != nil {
		t.Fatalf("enforce: %v", err)
	}
	return ok
}

// TestUserPolicy 直接用户主体：本命名空间放行、跨命名空间/跨集群拒绝。
func TestUserPolicy(t *testing.T) {
	e := newMemEnforcer(t, [][]string{
		{"p", "user:bob", "cluster-a", "dev/*", "get|list|watch|create|update|patch|delete|deletecollection"},
	})

	if !enforceWith(t, e, "bob", nil, "cluster-a", "GET", "/api/v1/namespaces/dev/pods", "") {
		t.Fatal("list pods in dev should be allowed")
	}
	if enforceWith(t, e, "bob", nil, "cluster-a", "GET", "/api/v1/namespaces/prod/pods", "") {
		t.Fatal("list pods in prod should be denied")
	}
	if enforceWith(t, e, "bob", nil, "cluster-b", "GET", "/api/v1/namespaces/dev/pods", "") {
		t.Fatal("policy is per-dom; cluster-b should be denied")
	}
}

// TestOrgPolicy JWT groups 中的组织主体应自动匹配 org:<group> 策略。
func TestOrgPolicy(t *testing.T) {
	e := newMemEnforcer(t, [][]string{
		{"p", "org:dev-team", "cluster-a", "dev/*", "get|list"},
	})

	if !enforceWith(t, e, "bob", []string{"dev-team", "other"}, "cluster-a", "GET", "/api/v1/namespaces/dev/pods", "") {
		t.Fatal("org policy should match JWT groups")
	}
	if enforceWith(t, e, "bob", []string{"other"}, "cluster-a", "GET", "/api/v1/namespaces/dev/pods", "") {
		t.Fatal("org policy should not match unrelated groups")
	}
	if enforceWith(t, e, "bob", nil, "cluster-a", "GET", "/api/v1/namespaces/dev/pods", "") {
		t.Fatal("org policy should not match a user without groups")
	}
}

// TestFix8ActAnchoring FIX-8 回归：`delete` 策略不得匹配 `deletecollection`。
func TestFix8ActAnchoring(t *testing.T) {
	e := newMemEnforcer(t, [][]string{
		{"p", "user:carol", "cluster-a", "dev/pods", "delete"},
	})

	if !enforceWith(t, e, "carol", nil, "cluster-a", "DELETE", "/api/v1/namespaces/dev/pods/pod-1", "") {
		t.Fatal("single delete should be allowed")
	}
	if enforceWith(t, e, "carol", nil, "cluster-a", "DELETE", "/api/v1/namespaces/dev/pods", "") {
		t.Fatal("deletecollection must NOT match a delete-only policy (FIX-8)")
	}
}

// TestSubresourcePolicy 子资源精确匹配：exec 允许、list pods 不涉及。
func TestSubresourcePolicy(t *testing.T) {
	e := newMemEnforcer(t, [][]string{
		{"p", "user:dave", "cluster-a", "dev/pods/exec", "create"},
	})

	if !enforceWith(t, e, "dave", nil, "cluster-a", "POST", "/api/v1/namespaces/dev/pods/pod-1/exec", "") {
		t.Fatal("exec should be allowed")
	}
	if enforceWith(t, e, "dave", nil, "cluster-a", "GET", "/api/v1/namespaces/dev/pods", "") {
		t.Fatal("list pods should be denied (policy covers exec only)")
	}
}

// TestClusterScopedAndNonResource 集群级资源、子资源与非资源路径。
func TestClusterScopedAndNonResource(t *testing.T) {
	e := newMemEnforcer(t, [][]string{
		{"p", "user:erin", "cluster-a", "cluster/nodes", "get|list"},
		{"p", "user:erin", "cluster-a", "cluster/nodes/proxy", "get"},
		{"p", "user:erin", "cluster-a", "nonresource/version", "get"},
	})

	if !enforceWith(t, e, "erin", nil, "cluster-a", "GET", "/apis/metrics.k8s.io/v1beta1/nodes", "") {
		t.Fatal("cluster-scoped nodes list should be allowed")
	}
	if enforceWith(t, e, "erin", nil, "cluster-a", "DELETE", "/api/v1/nodes/node-1", "") {
		t.Fatal("node delete should be denied")
	}
	if !enforceWith(t, e, "erin", nil, "cluster-a", "GET", "/api/v1/nodes/node-1/proxy", "") {
		t.Fatal("cluster-scoped subresource proxy should be allowed")
	}
	if !enforceWith(t, e, "erin", nil, "cluster-a", "GET", "/version", "") {
		t.Fatal("nonresource /version should be allowed")
	}
}

// TestParseRequestGrammar 语义资源串语法（§5.1 表格逐项验证）。
func TestParseRequestGrammar(t *testing.T) {
	cases := []struct {
		method, path, rawQuery string
		wantObj                string
		wantAct                string
	}{
		{"GET", "/api/v1/namespaces/dev/pods", "", "dev/pods", "list"},
		{"GET", "/api/v1/namespaces/dev/pods/foo", "", "dev/pods", "get"},
		{"POST", "/api/v1/namespaces/dev/pods/foo/exec", "", "dev/pods/exec", "create"},
		{"GET", "/api/v1/namespaces/dev/pods/foo/log", "", "dev/pods/log", "get"},
		{"GET", "/apis/apps/v1/deployments", "", "cluster/deployments", "list"},
		{"GET", "/api/v1/nodes/node-1/proxy", "", "cluster/nodes/proxy", "get"},
		{"GET", "/api/v1/namespaces/dev/pods", "watch=true", "dev/pods", "watch"},
		{"GET", "/version", "", "nonresource/version", "get"},
	}
	for _, c := range cases {
		obj, act, err := ParseRequest(c.method, c.path, c.rawQuery)
		if err != nil {
			t.Fatalf("parse %s %s: %v", c.method, c.path, err)
		}
		if obj != c.wantObj || act != c.wantAct {
			t.Fatalf("%s %s?%s: got (%s, %s), want (%s, %s)",
				c.method, c.path, c.rawQuery, obj, act, c.wantObj, c.wantAct)
		}
	}
}

// TestWildcardPolicy 资源 * 与动作 .* 覆盖所有请求。
func TestWildcardPolicy(t *testing.T) {
	e := newMemEnforcer(t, [][]string{
		{"p", "user:root", "cluster-a", "*", ".*"},
	})

	if !enforceWith(t, e, "root", nil, "cluster-a", "DELETE", "/api/v1/namespaces/dev/pods", "") {
		t.Fatal("wildcard policy should allow any resource/action")
	}
}
