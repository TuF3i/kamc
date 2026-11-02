// Package luaauth 登录期权限绑定脚本引擎（§5 扩展）：
// 每次 OIDC 登录 / Token 兑换时，按序执行管理员编写的 Lua 脚本，
// 脚本经原生桥接调用 kamc 权限 API（绑定/策略/集群查询），
// 实现按用户名 / 组的动态权限绑定（如"组对应角色不存在则新建"）。
//
// 失败语义 fail-closed：脚本编译错误、运行时错误、超时 → 返回错误，
// 调用方（登录挂载点）拒绝签发 JWT。
//
// 沙箱（纵深防御——脚本是管理员编写的，但不信任其内容）：
//   - gopher-lua SkipOpenLibs，仅开 table/string/math；base 库剥离
//     load/loadstring/dofile/require，不注入 os/io/package
//   - 每脚本独立 LState；执行超时（默认 2s，超时按失败处理）
//     已知限制：gopher-lua 无抢占式中断，超时后该 VM goroutine 泄漏至
//     进程退出（影响面：管理员编写的死循环脚本，重启回收）
package luaauth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	lua "github.com/yuin/gopher-lua"

	"github.com/TuF3i/kamc/internal/platform/modules"
)

// ScriptStore 脚本存储（dao 实现；测试注入内存实现）
type ScriptStore interface {
	ListEnabled(ctx context.Context) ([]modules.AuthScript, error)
	Get(ctx context.Context, name string) (*modules.AuthScript, error)
	ListAll(ctx context.Context) ([]modules.AuthScript, error)
	Upsert(ctx context.Context, s *modules.AuthScript) error
	Delete(ctx context.Context, name string) (int64, error)
}

// AuthzBridge 权限桥接（authz.Enforcer 实现满足；测试注入 fake）
type AuthzBridge interface {
	AddBinding(user, role, dom string) (bool, error)
	RemoveBinding(user, role, dom string) (bool, error)
	ListBindings() ([][]string, error)
	HasPolicy(sub, dom, obj, act string) (bool, error)
	AddPolicy(sub, dom, obj, act string) (bool, error)
	RemovePolicy(sub, dom, obj, act string) (bool, error)
	ListPolicies() ([][]string, error)
}

// ClusterReader 集群读取桥接
type ClusterReader interface {
	GetCluster(ctx context.Context, id string) (*modules.Cluster, error)
	ListClusters(ctx context.Context) ([]modules.Cluster, error)
}

// Claims 登录身份（来自 Authentik 的 ID Token / dev-token 请求）
type Claims struct {
	Sub    string
	Email  string
	Groups []string
}

// Action dry-run 模式下记录的桥接动作
type Action struct {
	API  string   `json:"api"`
	Args []string `json:"args"`
}

const defaultTimeout = 2 * time.Second

// Engine 绑定脚本引擎
type Engine struct {
	scripts  ScriptStore
	authz    AuthzBridge
	clusters ClusterReader
	timeout  time.Duration
}

// Option 引擎选项
type Option func(*Engine)

// WithTimeout 覆盖单脚本执行超时
func WithTimeout(d time.Duration) Option {
	return func(e *Engine) { e.timeout = d }
}

// New 构造引擎
func New(scripts ScriptStore, authz AuthzBridge, clusters ClusterReader, opts ...Option) *Engine {
	e := &Engine{scripts: scripts, authz: authz, clusters: clusters, timeout: defaultTimeout}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// ClaimsOf 从登录处理器提取 Claims
func ClaimsOf(sub, email string, groups []string) Claims {
	return Claims{Sub: sub, Email: email, Groups: groups}
}

// Execute 登录挂载点：顺序执行全部启用脚本（fail-closed——任一失败即返回错误）
func (e *Engine) Execute(ctx context.Context, claims Claims) error {
	scripts, err := e.scripts.ListEnabled(ctx)
	if err != nil {
		return fmt.Errorf("luaauth -> list scripts: %w", err)
	}
	for _, sc := range scripts {
		if _, err := e.runScript(sc.Content, claims, false); err != nil {
			return fmt.Errorf("luaauth -> script %q: %w", sc.Name, err)
		}
	}
	return nil
}

// ExecuteScript 手动执行单个脚本（管理面；dryRun=true 时桥接只记录动作）。
// 返回动作清单（dry-run 的验证闭环）。
func (e *Engine) ExecuteScript(ctx context.Context, name string, claims Claims, dryRun bool) ([]Action, error) {
	sc, err := e.scripts.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("luaauth -> get script: %w", err)
	}
	return e.runScript(sc.Content, claims, dryRun)
}

// SyntaxCheck 仅编译不执行（脚本保存前的语法校验）
func SyntaxCheck(content string) error {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()
	if _, err := L.LoadString(content); err != nil {
		return fmt.Errorf("luaauth -> compile: %w", err)
	}
	return nil
}

// ── 脚本管理（管理面 API 的业务层）──

// UpsertScript 保存脚本（先语法校验；校验失败不落库）
func (e *Engine) UpsertScript(ctx context.Context, name, content string, enabled bool) error {
	if strings.TrimSpace(name) == "" || len(name) > 128 {
		return errors.New("luaauth -> script name invalid")
	}
	if err := SyntaxCheck(content); err != nil {
		return err
	}
	return e.scripts.Upsert(ctx, &modules.AuthScript{Name: name, Content: content, Enabled: enabled})
}

// DeleteScript 删除脚本
func (e *Engine) DeleteScript(ctx context.Context, name string) (bool, error) {
	rows, err := e.scripts.Delete(ctx, name)
	if err != nil {
		return false, fmt.Errorf("luaauth -> delete script: %w", err)
	}
	return rows > 0, nil
}

// ListScripts 脚本列表（includeDisabled=false 仅启用的）
func (e *Engine) ListScripts(ctx context.Context, includeDisabled bool) ([]modules.AuthScript, error) {
	if includeDisabled {
		return e.scripts.ListAll(ctx)
	}
	return e.scripts.ListEnabled(ctx)
}

// GetScript 脚本详情
func (e *Engine) GetScript(ctx context.Context, name string) (*modules.AuthScript, error) {
	return e.scripts.Get(ctx, name)
}

// runScript 编译 → 沙箱构造 → 桥接注册 → 超时执行。
// 超时后放弃该 VM（不 Close 仍在运行的 LState，避免数据竞态），
// goroutine 与内存随进程生命周期持有——已知限制，见包注释。
func (e *Engine) runScript(content string, claims Claims, dryRun bool) ([]Action, error) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	var leaked atomic.Bool
	defer func() {
		if !leaked.Load() {
			L.Close()
		}
	}()

	e.openSafeLibs(L)

	var (
		recMu   sync.Mutex
		actions []Action
	)
	record := func(api string, args []string) {
		recMu.Lock()
		actions = append(actions, Action{API: api, Args: args})
		recMu.Unlock()
	}
	e.registerBridge(L, claims, dryRun, record)

	// 全局 claims 表
	if err := L.DoString("claims = " + claimsLiteral(claims)); err != nil {
		return nil, fmt.Errorf("luaauth -> set claims: %w", err)
	}

	fn, err := L.LoadString(content)
	if err != nil {
		return nil, fmt.Errorf("luaauth -> compile: %w", err)
	}
	L.Push(fn)

	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("panic: %v", r) // 桥接侧 Go bug 兜底
			}
		}()
		done <- L.PCall(0, 0, nil) // Lua 错误（error()/运行时错误）以 error 返回
	}()
	select {
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("luaauth -> run: %w", err)
		}
		return actions, nil
	case <-time.After(e.timeout):
		leaked.Store(true)
		return nil, errors.New("luaauth -> script timeout")
	}
}

// openSafeLibs 白名单库装载 + base 危险函数剥离
func (e *Engine) openSafeLibs(L *lua.LState) {
	lua.OpenBase(L)
	lua.OpenTable(L)
	lua.OpenString(L)
	lua.OpenMath(L)
	// base 剥离：动态编译/加载与文件相关入口
	for _, name := range []string{"load", "loadstring", "dofile", "loadfile", "require", "print", "collectgarbage"} {
		L.SetGlobal(name, lua.LNil)
	}
}

// claimsLiteral 生成 claims 的 Lua 表字面量（字段经 strconv 安全转义）
func claimsLiteral(c Claims) string {
	q := func(s string) string {
		return "'" + strings.NewReplacer("\\", "\\\\", "'", "\\'", "\n", "\\n").Replace(s) + "'"
	}
	groups := make([]string, 0, len(c.Groups))
	for _, g := range c.Groups {
		groups = append(groups, q(g))
	}
	return fmt.Sprintf("{sub=%s, email=%s, groups={%s}}",
		q(c.Sub), q(c.Email), strings.Join(groups, ","))
}
