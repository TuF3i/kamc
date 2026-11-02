package platform_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TuF3i/kamc/internal/platform/errcode"
)

const luaGroupBindingScript = `
local DOM = "cluster-a"
for _, g in ipairs(claims.groups) do
  if g == "e2e-devs" then
    if not kamc.cluster_exists(DOM) then error("cluster missing") end
    if not kamc.has_policy("role:e2e-devs", DOM, DOM .. "/*", "get|list|watch") then
      kamc.add_policy("role:e2e-devs", DOM, DOM .. "/*", "get|list|watch")
    end
    kamc.add_binding(claims.sub, "role:e2e-devs", DOM)
  end
end
`

// TestLuaBindingE2E 登录期权限绑定全链路（§5 扩展）：
// 上传脚本 → dev-token 登录（fail-closed 挂载）→ 绑定/策略自动创建 →
// 坏脚本拒绝登录 → 禁用脚本恢复 → dry-run 只验证不落地。
func TestLuaBindingE2E(t *testing.T) {
	trt, rt, _ := setupTunnelPlatform(t, "http://unused")
	defer func() { _ = trt.ReleaseRuntime() }()

	clusterID := fmt.Sprintf("e2e-lua-%d", time.Now().UnixNano())
	adminTok := adminToken(t, rt)
	if _, ar := call(t, rt, http.MethodPost, "/api/v1/clusters",
		map[string]string{"id": clusterID, "name": "lua"}, adminTok); ar.Status != errcode.CodeSuccess {
		t.Fatalf("create cluster: %+v", ar)
	}
	t.Cleanup(func() { _, _ = call(t, rt, http.MethodDelete, "/api/v1/clusters/"+clusterID, nil, adminTok) })

	// 脚本里的集群名用占位，测试时替换为真实集群 ID
	script := strings.ReplaceAll(luaGroupBindingScript, `"cluster-a"`, fmt.Sprintf("%q", clusterID))

	// ── 1. 保存脚本（语法校验通过）──
	resp, ar := call(t, rt, http.MethodPut, "/api/v1/auth-scripts/e2e-groups",
		map[string]any{"content": script, "enabled": true}, adminTok)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("put script: http=%d body=%+v", resp.StatusCode, ar)
	}
	t.Cleanup(func() { _, _ = call(t, rt, http.MethodDelete, "/api/v1/auth-scripts/e2e-groups", nil, adminTok) })

	// 语法错误 → 保存失败（携带编译信息）
	resp, ar = call(t, rt, http.MethodPut, "/api/v1/auth-scripts/e2e-bad",
		map[string]any{"content": "not lua at all", "enabled": true}, adminTok)
	if ar.Status == errcode.CodeSuccess {
		t.Fatal("syntax error must reject save")
	}
	_, _ = call(t, rt, http.MethodDelete, "/api/v1/auth-scripts/e2e-bad", nil, adminTok)

	// ── 2. 登录触发绑定 ──
	tokDev := devToken(t, rt, "lua-user", []string{"e2e-devs"})
	_, ar = call(t, rt, http.MethodGet, "/auth/me", nil, tokDev)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("login with script: %+v", ar)
	}

	// 绑定与策略已自动创建
	resp, ar = call(t, rt, http.MethodGet, "/api/v1/bindings", nil, adminTok)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("list bindings: %+v", ar)
	}
	if raw := string(ar.Data); !strings.Contains(raw, `"lua-user"`) || !strings.Contains(raw, clusterID) {
		t.Fatalf("binding not created: %s", raw)
	}
	resp, ar = call(t, rt, http.MethodGet, "/api/v1/policies", nil, adminTok)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("list policies: %+v", ar)
	}
	if raw := string(ar.Data); !strings.Contains(raw, "role:e2e-devs") {
		t.Fatalf("policy not created: %s", raw)
	}

	// 数据面真实验证：绑定的角色允许 list pods（集群无 Agent → 503 = 已过 Casbin）
	if resp := callRaw(t, rt, http.MethodGet, "/proxy/"+clusterID+"/api/v1/namespaces/"+clusterID+"/pods", tokDev); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("data plane via script binding: want 503, got %d", resp.StatusCode)
	}

	// ── 3. 坏脚本 → fail-closed ──
	if _, ar = call(t, rt, http.MethodPut, "/api/v1/auth-scripts/e2e-groups",
		map[string]any{"content": `error("broken on purpose")`, "enabled": true}, adminTok); ar.Status != errcode.CodeSuccess {
		t.Fatalf("update to broken script: %+v", ar)
	}
	// fail-closed：脚本错误 → 403 纯文本（客户端可感知登录被拒及原因类别）
	req, _ := http.NewRequest(http.MethodPost, "/auth/dev-token",
		strings.NewReader(`{"sub":"lua-user","groups":["e2e-devs"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	rt.Engine().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("broken script: want 403, got %d body=%s", rec.Code, rec.Body.String())
	}

	// ── 4. 禁用脚本 → 恢复登录 ──
	if _, ar = call(t, rt, http.MethodPut, "/api/v1/auth-scripts/e2e-groups",
		map[string]any{"content": script, "enabled": false}, adminTok); ar.Status != errcode.CodeSuccess {
		t.Fatalf("disable script: %+v", ar)
	}
	tokRecovered := devToken(t, rt, "lua-user", []string{"e2e-devs"})
	if _, ar = call(t, rt, http.MethodGet, "/auth/me", nil, tokRecovered); ar.Status != errcode.CodeSuccess {
		t.Fatalf("login after disable: %+v", ar)
	}

	// ── 5. dry-run：动作清单、无副作用 ──
	// 先记录当前绑定数；重新启用脚本（不影响既有绑定——add 幂等）
	if _, ar = call(t, rt, http.MethodPut, "/api/v1/auth-scripts/e2e-groups",
		map[string]any{"content": script, "enabled": true}, adminTok); ar.Status != errcode.CodeSuccess {
		t.Fatalf("re-enable: %+v", ar)
	}
	resp, ar = call(t, rt, http.MethodPost,
		"/api/v1/auth-scripts/e2e-groups/execute?dry_run=1",
		map[string]any{"sub": "dry-user", "groups": []string{"e2e-devs"}}, adminTok)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("dry run: http=%d body=%+v", resp.StatusCode, ar)
	}
	var result struct {
		Actions []map[string]any `json:"actions"`
	}
	if err := json.Unmarshal(ar.Data, &result); err != nil {
		t.Fatalf("decode actions: %v", err)
	}
	// 策略已存在（前面 lua-user 登录时创建）→ 仅记录 dry-user 的 add_binding
	if len(result.Actions) != 1 {
		t.Fatalf("dry-run actions: %+v", result.Actions)
	}
	if result.Actions[0]["api"] != "add_binding" {
		t.Fatalf("action: %+v", result.Actions[0])
	}
	// dry-run 无副作用
	_, arB := call(t, rt, http.MethodGet, "/api/v1/bindings", nil, adminTok)
	if arB.Status != errcode.CodeSuccess {
		t.Fatalf("list bindings: %+v", arB)
	}
	if strings.Contains(string(arB.Data), "dry-user") {
		t.Fatal("dry-run must not create bindings")
	}
}
