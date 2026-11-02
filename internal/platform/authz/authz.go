// Package authz Casbin 鉴权组件：DB 适配器、语义资源解析与鉴权入口。
//
// 模型（设计文档 §5）：请求四元组 (sub, dom, obj, act)——
//   - dom = 集群 ID；obj = RequestInfo 解析出的语义资源串
//     （<ns>/<resource>[/<subresource>]、cluster/<resource>、nonresource<path>）；
//   - act = K8s verb；匹配器对 act 锚定（FIX-8：`delete` 不得匹配 `deletecollection`）。
//
// 鉴权路径零外部依赖（§7.1 不变量）：策略全量载入内存，PG 仅持久化。
// 多副本策略同步（Redis watcher）属路线图，当前写入后本进程即时生效、
// 其他副本下次重启/重载可见。
package authz

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	goredis "github.com/redis/go-redis/v9"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/request"

	"github.com/TuF3i/kamc/internal/platform/dao"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

// modelText Casbin 模型（§5.2）：
//   - dom 支持 "*" 通配（跨集群角色）；
//   - act 正则经 matcher 内联锚定 "^(" + p.act + ")$"，策略侧书写裸片段。
const modelText = `
[request_definition]
r = sub, dom, obj, act

[policy_definition]
p = sub, dom, obj, act

[role_definition]
g = _, _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub, r.dom) && (r.dom == p.dom || p.dom == "*") && keyMatch2(r.obj, p.obj) && regexMatch(r.act, "^(" + p.act + ")$")
`

// Enforcer 鉴权入口（封装 SyncedEnforcer 与 DB 适配器）
type Enforcer struct {
	e       *casbin.SyncedEnforcer
	watcher *redisWatcher // 跨副本同步（nil = 单副本形态）
}

// Option 鉴权组件选项
type Option func(*enforcerOptions)

type enforcerOptions struct {
	redisCli    goredis.UniversalClient
	syncChannel string
}

// WithSyncChannel 启用跨副本策略同步（Redis pub/sub；未配置则单副本语义）
func WithSyncChannel(cli goredis.UniversalClient, channel string) Option {
	return func(o *enforcerOptions) { o.redisCli, o.syncChannel = cli, channel }
}

// New 构造鉴权组件：建模型 → 装配 DB 适配器 → 全量加载策略 → 跨副本同步。
func New(d *dao.Dao, opts ...Option) (*Enforcer, error) {
	o := &enforcerOptions{syncChannel: DefaultSyncChannel}
	for _, opt := range opts {
		opt(o)
	}

	m, err := model.NewModelFromString(modelText)
	if err != nil {
		return nil, fmt.Errorf("authz -> load model: %w", err)
	}
	e, err := casbin.NewSyncedEnforcer(m, &dbAdapter{d: d})
	if err != nil {
		return nil, fmt.Errorf("authz -> new enforcer: %w", err)
	}
	if err := e.LoadPolicy(); err != nil {
		return nil, fmt.Errorf("authz -> load policy: %w", err)
	}

	en := &Enforcer{e: e}
	if o.redisCli != nil {
		w, err := newRedisWatcher(context.Background(), o.redisCli, o.syncChannel)
		if err != nil {
			return nil, fmt.Errorf("authz -> policy watcher: %w", err)
		}
		en.watcher = w
		// 广播回调：全量重载（本副本自己的修改也会收到——幂等）
		w.SetUpdateCallback(func(string) {
			if err := e.LoadPolicy(); err != nil {
				log.Printf("[authz] policy reload: %v", err)
			}
		})
	}
	return en, nil
}

// notifyWatchers 策略变更后广播（发布失败不影响本副本正确性——变更已落库）
func (en *Enforcer) notifyWatchers() {
	if en.watcher != nil {
		en.watcher.Notify()
	}
}

// Release 释放跨副本同步资源（进程退出时调用；幂等）
func (en *Enforcer) Release() {
	if en.watcher != nil {
		en.watcher.Close()
	}
}

// Enforce 判定 (sub, dom, obj, act) 是否放行。错误向上抛（调用方 fail-closed）。
func (en *Enforcer) Enforce(sub, dom, obj, act string) (bool, error) {
	return en.e.Enforce(sub, dom, obj, act)
}

// AddPolicy 增加策略行（AutoSave 落库 + 广播其他副本重载）
func (en *Enforcer) AddPolicy(sub, dom, obj, act string) (bool, error) {
	added, err := en.e.AddPolicy(sub, dom, obj, act)
	if err != nil {
		return false, fmt.Errorf("authz -> add policy: %w", err)
	}
	if added {
		en.notifyWatchers()
	}
	return added, nil
}

// RemovePolicy 删除策略行（不存在返回 false）
func (en *Enforcer) RemovePolicy(sub, dom, obj, act string) (bool, error) {
	removed, err := en.e.RemovePolicy(sub, dom, obj, act)
	if err != nil {
		return false, fmt.Errorf("authz -> remove policy: %w", err)
	}
	if removed {
		en.notifyWatchers()
	}
	return removed, nil
}

// ListPolicies 全量策略行
func (en *Enforcer) ListPolicies() ([][]string, error) {
	return en.e.GetPolicy()
}

// HasPolicy 精确匹配策略行是否存在（Lua 桥接的"组是否存在则新建"语义）
func (en *Enforcer) HasPolicy(sub, dom, obj, act string) (bool, error) {
	rules, err := en.e.GetPolicy()
	if err != nil {
		return false, fmt.Errorf("authz -> has policy: %w", err)
	}
	for _, r := range rules {
		if len(r) == 4 && r[0] == sub && r[1] == dom && r[2] == obj && r[3] == act {
			return true, nil
		}
	}
	return false, nil
}

// AddBinding 用户-角色绑定（g, user, role, dom）
func (en *Enforcer) AddBinding(user, role, dom string) (bool, error) {
	added, err := en.e.AddGroupingPolicy(user, role, dom)
	if err != nil {
		return false, fmt.Errorf("authz -> add binding: %w", err)
	}
	if added {
		en.notifyWatchers()
	}
	return added, nil
}

// RemoveBinding 删除绑定（不存在返回 false）
func (en *Enforcer) RemoveBinding(user, role, dom string) (bool, error) {
	removed, err := en.e.RemoveGroupingPolicy(user, role, dom)
	if err != nil {
		return false, fmt.Errorf("authz -> remove binding: %w", err)
	}
	if removed {
		en.notifyWatchers()
	}
	return removed, nil
}

// ListBindings 全量绑定
func (en *Enforcer) ListBindings() ([][]string, error) {
	return en.e.GetGroupingPolicy()
}

// dbAdapter casbin persist.Adapter 的 DB 实现（AutoSave 生效于 Add/Remove；
// SavePolicy 全量写不启用——策略管理全部走增量 API）
type dbAdapter struct {
	d *dao.Dao
}

// LoadPolicy 全量加载（启动与显式重载）
func (a *dbAdapter) LoadPolicy(m model.Model) error {
	rules, err := a.d.ListPolicyRules(context.Background())
	if err != nil {
		return fmt.Errorf("authz -> adapter load: %w", err)
	}
	for _, r := range rules {
		if err := loadPolicyLine(r.Ptype, ruleValues(&r), m); err != nil {
			return err
		}
	}
	return nil
}

// SavePolicy 不支持（策略管理只走增量 API，避免全量覆盖的竞态）
func (a *dbAdapter) SavePolicy(_ model.Model) error {
	return fmt.Errorf("authz -> SavePolicy unsupported: use incremental policy APIs")
}

// AddPolicy 落库一条
func (a *dbAdapter) AddPolicy(_ string, ptype string, rule []string) error {
	return a.d.AddPolicyRule(context.Background(), toRule(ptype, rule))
}

// RemovePolicy 按字段匹配删除
func (a *dbAdapter) RemovePolicy(_ string, ptype string, rule []string) error {
	_, err := a.d.RemovePolicyRule(context.Background(), toRule(ptype, rule))
	return err
}

// RemoveFilteredPolicy 不支持（管理 API 无过滤删除场景）
func (a *dbAdapter) RemoveFilteredPolicy(_ string, _ string, _ int, _ ...string) error {
	return fmt.Errorf("authz -> RemoveFilteredPolicy unsupported")
}

// ---- 解析与工具 ----

var reqResolver = &request.RequestInfoFactory{
	APIPrefixes:          sets.NewString("api", "apis"),
	GrouplessAPIPrefixes: sets.NewString("api"),
}

// ParseRequest 把已 strip 前缀的 API 路径解析为 Casbin 的 (obj, act)（§5.1）。
// 非 K8s 资源请求（/version、/openapi 等）解析为 nonresource 对象。
func ParseRequest(method, path string) (obj, act string, err error) {
	r := &http.Request{Method: method, URL: &url.URL{Path: path}}
	info, err := reqResolver.NewRequestInfo(r)
	if err != nil {
		return "", "", fmt.Errorf("authz -> parse request: %w", err)
	}
	if !info.IsResourceRequest {
		return "nonresource" + info.Path, info.Verb, nil
	}
	if info.Namespace != "" {
		obj = info.Namespace + "/" + info.Resource
		if info.Subresource != "" {
			obj += "/" + info.Subresource
		}
	} else {
		obj = "cluster/" + info.Resource
	}
	return obj, info.Verb, nil
}

// toRule 字段数组 → 存储行（空字段由 gorm 列默认值兜底）
func toRule(ptype string, rule []string) *modules.PolicyRule {
	r := &modules.PolicyRule{Ptype: ptype}
	fields := []*string{&r.V0, &r.V1, &r.V2, &r.V3, &r.V4, &r.V5}
	for i, v := range rule {
		if i >= len(fields) {
			break
		}
		*fields[i] = v
	}
	return r
}

// ruleValues 存储行 → 字段数组（截断空尾字段，与 casbin 行为一致）
func ruleValues(r *modules.PolicyRule) []string {
	vals := []string{r.V0, r.V1, r.V2, r.V3, r.V4, r.V5}
	for len(vals) > 0 && vals[len(vals)-1] == "" {
		vals = vals[:len(vals)-1]
	}
	return vals
}

// loadPolicyLine 单行策略加载（与 persist.LoadPolicyArray 语义一致）
func loadPolicyLine(ptype string, vals []string, m model.Model) error {
	sec := ptype[:1] // "p" → 策略段，"g" → 角色段
	if err := m.AddPolicy(sec, ptype, vals); err != nil {
		return fmt.Errorf("authz -> load policy line %s %v: %w", ptype, vals, err)
	}
	return nil
}
