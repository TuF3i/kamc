package luaauth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TuF3i/kamc/internal/platform/modules"
)

// ---- fakes ----

type fakeScriptStore struct {
	mu      sync.Mutex
	scripts map[string]modules.AuthScript
}

func newFakeScriptStore(scripts ...modules.AuthScript) *fakeScriptStore {
	s := &fakeScriptStore{scripts: make(map[string]modules.AuthScript)}
	for _, sc := range scripts {
		s.scripts[sc.Name] = sc
	}
	return s
}

func (f *fakeScriptStore) ListEnabled(_ context.Context) ([]modules.AuthScript, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []modules.AuthScript
	for _, sc := range f.scripts {
		if sc.Enabled {
			out = append(out, sc)
		}
	}
	// 按 name 排序（与 dao 行为一致）
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Name < out[i].Name {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

func (f *fakeScriptStore) Get(_ context.Context, name string) (*modules.AuthScript, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if sc, ok := f.scripts[name]; ok {
		return &sc, nil
	}
	return nil, errors.New("not found")
}

func (f *fakeScriptStore) ListAll(_ context.Context) ([]modules.AuthScript, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]modules.AuthScript, 0, len(f.scripts))
	for _, sc := range f.scripts {
		out = append(out, sc)
	}
	return out, nil
}

func (f *fakeScriptStore) Upsert(_ context.Context, sc *modules.AuthScript) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scripts[sc.Name] = *sc
	return nil
}

func (f *fakeScriptStore) Delete(_ context.Context, name string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.scripts[name]; !ok {
		return 0, nil
	}
	delete(f.scripts, name)
	return 1, nil
}

type fakeAuthz struct {
	mu       sync.Mutex
	bindings [][3]string
	policies [][4]string
}

func (f *fakeAuthz) AddBinding(user, role, dom string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, b := range f.bindings {
		if b == [3]string{user, role, dom} {
			return false, nil
		}
	}
	f.bindings = append(f.bindings, [3]string{user, role, dom})
	return true, nil
}

func (f *fakeAuthz) RemoveBinding(user, role, dom string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, b := range f.bindings {
		if b == [3]string{user, role, dom} {
			f.bindings = append(f.bindings[:i], f.bindings[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeAuthz) ListBindings() ([][]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]string, 0, len(f.bindings))
	for _, b := range f.bindings {
		out = append(out, []string{b[0], b[1], b[2]})
	}
	return out, nil
}

func (f *fakeAuthz) HasPolicy(sub, dom, obj, act string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.policies {
		if p == [4]string{sub, dom, obj, act} {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeAuthz) AddPolicy(sub, dom, obj, act string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.policies {
		if p == [4]string{sub, dom, obj, act} {
			return false, nil
		}
	}
	f.policies = append(f.policies, [4]string{sub, dom, obj, act})
	return true, nil
}

func (f *fakeAuthz) RemovePolicy(sub, dom, obj, act string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, p := range f.policies {
		if p == [4]string{sub, dom, obj, act} {
			f.policies = append(f.policies[:i], f.policies[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeAuthz) ListPolicies() ([][]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]string, 0, len(f.policies))
	for _, p := range f.policies {
		out = append(out, []string{p[0], p[1], p[2], p[3]})
	}
	return out, nil
}

type fakeClusters struct{}

func (fakeClusters) GetCluster(_ context.Context, id string) (*modules.Cluster, error) {
	if id == "cluster-a" {
		return &modules.Cluster{ID: id, Name: "A"}, nil
	}
	return nil, errors.New("not found")
}

func (fakeClusters) ListClusters(_ context.Context) ([]modules.Cluster, error) {
	return []modules.Cluster{{ID: "cluster-a", Name: "A"}}, nil
}

// ---- fixtures ----

func newEngine(t *testing.T, store *fakeScriptStore, authz *fakeAuthz, opts ...Option) *Engine {
	t.Helper()
	return New(store, authz, fakeClusters{}, opts...)
}

// 组绑定示例脚本（用户场景：Authentik 组 → kamc 角色，组不存在则新建）
const groupBindingScript = `
local DOM = "cluster-a"
local role_map = {
  ["authentik-devs"] = "role:devs",
  ["authentik-sre"]  = "role:sre",
}
for _, g in ipairs(claims.groups) do
  local role = role_map[g]
  if role then
    if not kamc.has_policy(role, DOM, DOM .. "/*", "get|list|watch") then
      kamc.add_policy(role, DOM, DOM .. "/*", "get|list|watch")
    end
    if not kamc.cluster_exists(DOM) then
      error("cluster missing: " .. DOM)
    end
    kamc.add_binding(claims.sub, role, DOM)
  end
end
`

// TestGroupBindingScenario 用户场景全链路：
// Authentik 组映射角色 → 角色策略不存在则新建 → 绑定幂等。
func TestGroupBindingScenario(t *testing.T) {
	store := newFakeScriptStore(modules.AuthScript{Name: "01-groups", Content: groupBindingScript, Enabled: true})
	authz := &fakeAuthz{}
	e := newEngine(t, store, authz)

	if err := e.Execute(context.Background(), ClaimsOf("bob", "bob@x.com", []string{"authentik-devs"})); err != nil {
		t.Fatalf("execute: %v", err)
	}

	// 角色策略已新建；绑定已建立（幂等——重复执行不报错）
	if ok, _ := authz.HasPolicy("role:devs", "cluster-a", "cluster-a/*", "get|list|watch"); !ok {
		t.Fatal("role policy should be created")
	}
	if err := e.Execute(context.Background(), ClaimsOf("bob", "bob@x.com", []string{"authentik-devs"})); err != nil {
		t.Fatalf("idempotent re-execute: %v", err)
	}
	rows, _ := authz.ListBindings()
	if len(rows) != 1 || rows[0][0] != "bob" {
		t.Fatalf("bindings: %v", rows)
	}

	// 不相关组 → 无动作
	if err := e.Execute(context.Background(), ClaimsOf("eve", "", []string{"other-group"})); err != nil {
		t.Fatalf("unrelated group: %v", err)
	}
	rows2, _ := authz.ListBindings()
	if len(rows2) != 1 {
		t.Fatalf("unrelated group must not add bindings: %v", rows2)
	}
}

// TestFailClosed 脚本运行时错误 → Execute 返回错误（登录挂载点据此拒绝）。
func TestFailClosed(t *testing.T) {
	store := newFakeScriptStore(
		modules.AuthScript{Name: "01-bad", Content: `error("boom")`, Enabled: true},
	)
	e := newEngine(t, store, &fakeAuthz{})

	if err := e.Execute(context.Background(), ClaimsOf("bob", "", nil)); err == nil {
		t.Fatal("script error must fail-closed")
	}
}

// TestCompileCheck 语法错误在编译期即失败（保存前校验的依据）。
func TestCompileCheck(t *testing.T) {
	if err := SyntaxCheck("this is not lua"); err == nil {
		t.Fatal("syntax error should be caught")
	}
	if err := SyntaxCheck("return 1"); err != nil {
		t.Fatalf("valid lua: %v", err)
	}
}

// TestSandboxBlocksEscape 危险能力不可用：os/io/load/dofile 均不存在。
func TestSandboxBlocksEscape(t *testing.T) {
	for _, probe := range []string{
		`os.execute("echo pwned")`,
		`io.open("/etc/passwd")`,
		`load("return 1")`,
		`loadstring("return 1")`,
		`dofile("/etc/passwd")`,
		`require("os")`,
	} {
		store := newFakeScriptStore(modules.AuthScript{Name: "s", Content: probe, Enabled: true})
		e := newEngine(t, store, &fakeAuthz{})
		if err := e.Execute(context.Background(), ClaimsOf("bob", "", nil)); err == nil {
			t.Fatalf("sandbox escape must fail: %s", probe)
		}
	}
}

// TestScriptTimeout 死循环脚本按超时失败（已知限制：VM goroutine 泄漏至进程退出）。
func TestScriptTimeout(t *testing.T) {
	store := newFakeScriptStore(modules.AuthScript{Name: "s", Content: `while true do end`, Enabled: true})
	e := newEngine(t, store, &fakeAuthz{}, WithTimeout(50*time.Millisecond))

	if err := e.Execute(context.Background(), ClaimsOf("bob", "", nil)); err == nil {
		t.Fatal("infinite loop should hit timeout")
	}
}

// TestDryRun 手动执行：dry-run 只记录动作不落地。
func TestDryRun(t *testing.T) {
	store := newFakeScriptStore(modules.AuthScript{Name: "s", Content: groupBindingScript, Enabled: true})
	authz := &fakeAuthz{}
	e := newEngine(t, store, authz)

	actions, err := e.ExecuteScript(context.Background(), "s", ClaimsOf("bob", "", []string{"authentik-devs"}), true)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(actions) != 2 { // add_policy + add_binding
		t.Fatalf("actions: %+v", actions)
	}
	if ok, _ := authz.HasPolicy("role:devs", "cluster-a", "cluster-a/*", "get|list|watch"); ok {
		t.Fatal("dry-run must not create policy")
	}
	if len(authz.bindings) != 0 {
		t.Fatal("dry-run must not create binding")
	}

	// 非 dry-run 落地
	if _, err := e.ExecuteScript(context.Background(), "s", ClaimsOf("bob", "", []string{"authentik-devs"}), false); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(authz.bindings) != 1 {
		t.Fatalf("apply should create binding: %v", authz.bindings)
	}
}

// TestBridgeNames 文档与测试的桥接清单锚定。
func TestBridgeNames(t *testing.T) {
	want := strings.Join([]string{
		"list_clusters", "cluster_exists",
		"list_bindings", "add_binding", "remove_binding",
		"has_policy", "add_policy", "remove_policy", "list_policies",
	}, ",")
	if got := strings.Join(bridgeNames(), ","); got != want {
		t.Fatalf("bridge surface changed: %s", got)
	}
}
