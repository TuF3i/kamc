# K8s Access Manager（kamc）平台设计文档

> 基于 Go + Casbin + Authentik SSO 的多集群 Kubernetes 访问管理平台
>
> **v2.0**：在 v1.0 基础上修复全部已识别缺陷，补全高可用（HA）架构。

---

## 目录

1. [概述](#一概述)
2. [版本变更（v1.0 → v2.0）](#二版本变更v10--v20)
3. [总体架构](#三总体架构)
4. [认证与授权](#四认证与授权)
5. [基于 RequestInfo 的 Casbin 鉴权](#五基于-requestinfo-的-casbin-鉴权)
6. [隧道数据平面：Dialer 模型](#六隧道数据平面dialer-模型)
7. [高可用设计](#七高可用设计)
8. [请求转发与 Impersonation](#八请求转发与-impersonation)
9. [流式连接处理（exec / port-forward）](#九流式连接处理exec--port-forward)
10. [接入层（Nginx）配置](#十接入层nginx配置)
11. [部署拓扑与清单](#十一部署拓扑与清单)
12. [可观测性与运维](#十二可观测性与运维)
13. [故障模式分析](#十三故障模式分析)
14. [附录](#十四附录)

---

## 一、概述

### 1.1 目标

构建一个统一的 Kubernetes 访问管理平台，实现：

- **单点登录**：用户通过 Authentik SSO 登录，无需管理多个集群的 kubeconfig
- **集中鉴权**：通过 Casbin 统一控制用户对多集群的访问权限
- **多集群纳管**：一个平台管理多个内网/公网 K8s 集群（支持 1.23 / 1.25 等版本）
- **透明访问**：用户下载平台生成的 kubeconfig，用标准 kubectl 操作集群
- **完整支持**：CRUD、watch、exec、port-forward、logs 等全部 K8s 操作
- **审计追溯**：所有操作可追溯到真实用户
- **高可用**（v2.0 新增）：平台多副本无状态部署，单副本故障不影响服务，隧道秒级自愈

### 1.2 设计原则

| 原则 | 说明 |
| --- | --- |
| 平台是唯一授权决策点 | Casbin 在请求到达 Agent 前完成鉴权 |
| Agent 是哑管道 | Agent 只负责转发，不做任何权限判断 |
| 双重权限校验 | Casbin 第一道过滤，API Server RBAC 第二道防线 |
| 无客户端证书依赖 | 兼容 Nginx 网关 TLS 终结架构 |
| 数据面字节透明 | 平台内部与隧道只搬运字节流，HTTP 语义只在两端终结 |
| 无单点（v2.0） | 隧道注册表外置、副本对等、Agent 多连接，任何单副本故障可自愈 |

### 1.3 支持的集群版本

- Kubernetes 1.23+ / 1.25+（仅 SPDY）
- 向前兼容 1.29+（WebSocket 协议，KEP-4006）
- kubectl 1.31+ 默认尝试 WebSocket，失败后自动回退 SPDY

---

## 二、版本变更（v1.0 → v2.0）

| 编号 | v1.0 问题 | v2.0 方案 | 章节 |
| --- | --- | --- | --- |
| BUG-1 | 平台 `replicas: 3` 但 `TunnelManager` 是进程内 map，Nginx upstream 只配单后端：Agent 隧道落在某个 Pod，用户请求被负载均衡到其他 Pod 时查不到隧道 | 引入 **Redis 隧道注册表**（TTL 心跳）+ **跨 Pod 数据面转发**（副本间 yamux peer 会话做字节拼接），任意副本可服务任意请求 | §7.2 |
| BUG-2 | Casbin `obj` 用原始 URL 匹配：exec 真实路径是 `/api/v1/namespaces/dev/pods/{name}/exec`，示例策略 `/api/v1/namespaces/dev/pods/exec` 匹配不上；且首条 `/api/v1/namespaces/dev/*` 通配已放行全部动词，后面的 exec/portforward 行形同虚设 | `obj` 改为 `RequestInfo` 解析出的**语义资源串**（`<ns>/<resource>[/<subresource>]`），`act` 用 K8s verb；重写全部策略示例 | §5 |
| BUG-3 | `TunnelTransport.RoundTrip` 草稿手工 `req.Write` + `http.ReadResponse(bufio.NewReader(stream), ...)`：bufio 预读的响应体字节会随 `stream` 直接封装而**丢失**，chunked 编码与 101 升级的正确性也无法保证 | 改为 **Dialer 模型**：`http.Transport{ DialContext: 隧道 }` + Agent 侧 `net/http.Server`，HTTP 细节全部交给标准库，平台内部与隧道只搬运字节流 | §6.2 |
| BUG-4 | yamux 直接跑在 gorilla/websocket 上，缺少 `net.Conn` 适配，未定义心跳，NAT/LB 空闲超时会静默断链 | 明确 **binary frame NetConn 适配器**（gorilla 单写者约束）+ **双层心跳**：WS ping/pong 20s，yamux keepalive 30s，Nginx 隧道读超时必须大于心跳间隔 | §6.3 |
| BUG-5 | Nginx 在 `location /` 写死 `proxy_set_header Connection "upgrade"` 破坏普通请求 keepalive；`proxy_read_timeout 3600s` 一刀切 | `$connection_upgrade` map 按需升级；**按路由分级超时**：控制面 API 60s、流式代理 3600s、Agent 隧道 75s（大于心跳间隔，加速死链检测） | §10 |
| FIX-6 | v1.0 转发流程未说明 `/proxy/{clusterID}` 前缀剥离与 Host 重写，apiserver 将收到 `/proxy/cluster-a/api/v1/...` 直接 404 | 平台转发前 **strip 前缀、重写 Host** | §8.1 |
| FIX-7 | 用户 JWT 会随请求透传到目标集群（进入 Agent 日志、apiserver 审计），且无效 token 可能干扰 apiserver 认证 | 平台注入 Impersonate 头的同时**清除 `Authorization` 头**，由 Agent 的 SA transport 注入 SA Token | §8.1 |
| FIX-8 | matcher `regexMatch(r.act, p.act)` **未锚定**：`delete` 会匹配 `deletecollection`，构成越权风险 | 锚定为 `regexMatch(r.act, "^(" + p.act + ")$")` | §5.2 |

---

## 三、总体架构

### 3.1 架构图（HA 版）

```text
                        ┌────────────────────────────────────────────────┐
                        │ 用户侧                                          │
                        │  Browser ──OIDC──► Authentik（HA，双副本+LB）    │
                        │  kubectl ──JWT──► https://access-manager...     │
                        └───────────────────────┬────────────────────────┘
                                                │ HTTPS
                        ┌───────────────────────▼────────────────────────┐
                        │ 接入层：Nginx ×2 + keepalived VIP（TLS 终结）    │
                        │  /proxy/*  → 平台（流式，长超时）                 │
                        │  /tunnel/* → 平台（Agent 隧道）                  │
                        │  /api/*    → 平台（控制面，短超时）               │
                        └───────────────────────┬────────────────────────┘
                                                │ HTTP（内网）
                ┌───────────────────────────────┼───────────────────────────────┐
                │              平台（无状态，3 副本，对等）                         │
                │  ┌────────────┐ ┌────────────┐ ┌────────────┐                 │
                │  │ Pod-0      │◄┼──peer──────┼►│ Pod-1      │  … 副本间        │
                │  │ 认证/鉴权   │ │  yamux 会话│ │            │    全互联（懒建立）│
                │  │ 隧道A 隧道C │ └──────┬─────┘ │ 隧道B      │                 │
                │  └─────┬──────┘        │       └─────┬──────┘                 │
                │        │               │             │                        │
                │  ┌─────▼───────────────▼─────────────▼──────┐                 │
                │  │ Pod-2（同理，隧道分布无固定约定）            │                 │
                │  └─────┬────────────────────────────────────┘                 │
                └────────┼──────────────────────────────────────────────────────┘
                         │
        ┌────────────────┼───────────────────────┐
        ▼                ▼                       ▼
┌───────────────┐  ┌───────────────┐   ┌──────────────────────────┐
│ PostgreSQL(HA)│  │ Redis(HA)     │   │ 目标集群 A/B/C…            │
│ 策略/集群/Token│  │ 隧道注册表(TTL) │   │  Agent ×2（各持 1 条隧道）  │
│ 哈希/审计日志  │  │ casbin watcher│   │   → kube-apiserver:6443   │
└───────────────┘  │ 限流          │   │   Impersonation + RBAC    │
                   └───────────────┘   └──────────────────────────┘
```

### 3.2 一次请求的完整数据流（跨 Pod 场景）

1. kubectl 携带 JWT 发起请求 → Nginx → **Pod-X**（任意副本）
2. Pod-X 验证 JWT → 提取用户身份 → 解析集群 ID
3. 剥离用户传入的所有 `Impersonate-*` 头 → Casbin 鉴权（本地策略缓存，无网络依赖）
4. 鉴权通过 → 注入平台控制的 `Impersonate-User` / `Impersonate-Group` → strip `/proxy/{clusterID}` 前缀 → 清除 `Authorization`
5. 取隧道：本地有 → 直接用；**本地没有 → 查 Redis 注册表 → 经 peer yamux 会话转发到持有隧道的 Pod-Y，Pod-Y 将该字节流拼接到本地 Agent 隧道**（§7.2）
6. Agent 用 SA transport 转发到 API Server（注入 SA Token）
7. API Server 完成 Impersonation 认证 + RBAC 授权，响应原路返回（字节透传）

### 3.3 组件清单

| 组件 | 部署位置 | 职责 | HA 方式 |
| --- | --- | --- | --- |
| Web 前端 | 平台 | 登录、kubeconfig 下载、管理界面 | 多副本，无状态 |
| 认证服务 | 平台 | OIDC 对接 Authentik、JWT 签发与校验 | 多副本，共享签名密钥（§7.3） |
| 鉴权服务 | 平台 | Casbin 策略管理、RequestInfo 解析 | 多副本，本地缓存 + watcher 同步（§7.6） |
| 隧道管理器 | 平台 | 本机 Agent 隧道生命周期 + 注册表上报 | 多副本对等（§7.2） |
| 跨 Pod 转发器 | 平台 | peer yamux 会话、RELAY 字节拼接 | 懒建立，故障即弃（§7.2） |
| API 代理 | 平台 | HTTP 反代、Impersonation 注入、流式处理 | 多副本，无状态 |
| Token 管理 | 平台 | Agent Token 生成、轮换、吊销 | 多副本，状态在 PG |
| Audit 服务 | 平台 | 异步批量写审计日志 | 多副本，PG 故障时本地暂存（§12.2） |
| Agent | 目标集群 | 隧道客户端、SA 转发 | 每集群 2 副本，双隧道（§7.4） |

### 3.4 技术栈

```text
语言：Go 1.22+
Web 框架：chi（标准库 net/http 兼容，便于与 k8s handler 组合）
SSO：github.com/coreos/go-oidc/v3
JWT：github.com/golang-jwt/jwt/v5（RS256，kid 多密钥轮换）
鉴权：github.com/casbin/casbin/v2（file-adapter 不用于生产，策略落 PG）
K8s 客户端：k8s.io/client-go
K8s 请求解析：k8s.io/apiserver/pkg/endpoints/request
SPDY/WS 转换：k8s.io/apiserver/pkg/util/proxy
隧道复用：github.com/hashicorp/yamux
WebSocket：github.com/gorilla/websocket
存储：PostgreSQL（策略/集群/Token 哈希/审计）
缓存与协调：Redis（隧道注册表 / casbin watcher / 限流）
指标：github.com/prometheus/client_golang
```

---

## 四、认证与授权

### 4.1 用户认证流程（浏览器登录）

与 v1.0 一致：Browser → 平台重定向 OIDC → Authentik 登录 → ID Token 回调 → 平台提取 `sub` / `groups` → 签发平台 JWT。

**Authentik 故障降级**：平台本地缓存 OIDC Discovery 与 JWKS（TTL 1h + 主动刷新失败退避）。Authentik 宕机时，已登录用户的平台 JWT 在有效期内继续可用（kubectl 不受影响），仅新登录失败。

### 4.2 JWT 结构

```json
{
  "iss": "https://access-manager.example.com",
  "aud": "k8s-access-manager",
  "sub": "alice",
  "email": "alice@example.com",
  "groups": ["dev-team", "sre"],
  "exp": 1730000000,
  "iat": 1729996400,
  "jti": "unique-token-id"
}
```

- 算法：**RS256**；Header 带 **`kid`**（v2.0 新增，支持多密钥轮换）
- 有效期：1 小时（可配置）
- 校验：`iss`、`aud`、`exp`、签名；签名密钥按 `kid` 从本地密钥环选取
- `jti` 预留服务端吊销列表（可选实现，Redis 黑名单，TTL = token 剩余有效期）

### 4.3 签名密钥管理与多副本共享（v2.0）

- RS256 私钥以 Secret 挂载到**全部平台副本**（同一把当前密钥），各副本签发/校验行为一致。
- 平台暴露 `GET /.well-known/jwks.json`（公钥集合），供运维工具与未来扩展校验。
- **轮换流程（零停机）**：
  1. 生成新密钥对（`kid-2`），加入所有副本的密钥环，此时密钥环 = `{kid-1 验证, kid-2 签发+验证}`
  2. 滚动重启或热加载后，新签发 token 全部带 `kid-2`
  3. 等 `kid-1` 签发的存量 token 全部过期（≤ 1h）后，移除 `kid-1`

### 4.4 kubeconfig 生成

**方式一：静态 Token（简单）**

```yaml
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://access-manager.example.com/proxy/cluster-a
    certificate-authority-data: <平台 CA>   # 公网 CA 签发的证书可省略
  name: cluster-a
contexts:
- context: {cluster: cluster-a, user: alice, namespace: default}
  name: cluster-a
current-context: cluster-a
users:
- name: alice
  user:
    token: <平台 JWT>
```

**方式二：exec credential plugin（推荐）**

避免用户频繁重下 kubeconfig。`k8s-am-login` 是平台提供的小工具，走 OIDC Device Flow 或本地缓存的 refresh token，输出标准 `ExecCredential`：

```yaml
users:
- name: alice
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1
      command: k8s-am-login
      args:
      - "--server=https://access-manager.example.com"
      - "--cluster=cluster-a"
      interactiveMode: IfAvailable
```

```json
{
  "apiVersion": "client.authentication.k8s.io/v1",
  "kind": "ExecCredential",
  "status": {
    "token": "<JWT>",
    "expirationTimestamp": "2025-01-15T11:00:00Z"
  }
}
```

---

## 五、基于 RequestInfo 的 Casbin 鉴权

> **v3.0 权限模型变更**：移除 `g` 角色/用户绑定；策略主体直接为
> `user:<username>`（平台用户目录）、`org:<group>`（IdP/平台组）或
> `robot:<robot id>`（平台签发，独立主体），鉴权时对用户与 JWT groups
> 逐候选匹配；用户策略优先并完全遮蔽组策略，多组策略取并集。权限页使用
> 平台用户/组目录选择主体，并通过 Agent 隧道 discovery 获取集群
> GVR/动词/namespace。下方 v2.0 的 `g` 示例仅保留历史说明。

### 5.1 为什么不能用原始 URL 匹配（BUG-2 修复说明）

K8s 资源路径含资源名与子资源层级（`/api/v1/namespaces/dev/pods/{name}/exec`、`/apis/apps/v1/namespaces/dev/deployments/{name}`），用原始路径做 keyMatch2 极易漏配/误配，且集群级资源、非资源路径（`/version`、`/healthz`）语义混杂。v2.0 用 `RequestInfoResolver` 把请求解析为**语义五元组**，Casbin 的 `obj` 使用规范化资源串：

| 请求 | RequestInfo | Casbin obj | Casbin act |
| --- | --- | --- | --- |
| `GET /api/v1/namespaces/dev/pods` | ns=dev, resource=pods, verb=list | `dev/pods` | `list` |
| `GET /api/v1/namespaces/dev/pods/foo` | ns=dev, resource=pods, verb=get | `dev/pods` | `get` |
| `POST /api/v1/namespaces/dev/pods/foo/exec` | ns=dev, resource=pods, subresource=exec, verb=create | `dev/pods/exec` | `create` |
| `GET /api/v1/namespaces/dev/pods/foo/log` | subresource=log | `dev/pods/log` | `get` |
| `GET /apis/apps/v1/deployments`（集群级） | ns="", resource=deployments | `cluster/deployments` | `list` |
| `GET /version` | nonResourceURL | `nonresource/version` | `get` |

### 5.2 Model 定义（v2.0，修复 BUG-2 + FIX-8）

```ini
[request_definition]
r = sub, dom, obj, act

[policy_definition]
p = sub, dom, obj, act

[role_definition]
g = _, _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub, r.dom) \
    && (r.dom == p.dom || p.dom == "*") \
    && keyMatch2(r.obj, p.obj) \
    && regexMatch(r.act, concat("(^|,)", p.act, "(,|$)"))
```

两处关键修正：

- **`act` 匹配改为逗号定界**（`(^|,)get|list(,|$)` 形式由策略书写保证；更简单的等价实现是策略侧使用 `^(...)$` 包裹，见下方代码）。未锚定的 `regexMatch` 会让 `delete` 匹配到 `deletecollection`，属于**越权漏洞**（FIX-8）。
- **`dom` 支持 `*` 通配**，便于定义跨集群角色。

策略中 `act` 一律书写为正则片段 `get|list|watch`，加载时由平台统一包装锚定：

```go
// 平台加载策略时自动锚定 act 列，杜绝未锚定正则
func anchorAct(policies [][]string) [][]string {
    for _, p := range policies {
        p[3] = "^(" + p[3] + ")$"
    }
    return policies
}
```

### 5.3 Policy 示例（v2.0 重写）

```text
# ===== 角色-权限 =====

# 集群管理员：全部集群、全部资源、全部动词
p, role:cluster-admin, *,                       *,                       .*

# 开发者（按集群/命名空间授权，绑定到具体 dom）
p, role:developer,   {dom},   {ns}/*,              get|list|watch|create|update|patch|delete|deletecollection
p, role:developer,   {dom},   {ns}/pods/exec,        create
p, role:developer,   {dom},   {ns}/pods/portforward, create
p, role:developer,   {dom},   {ns}/pods/log,         get

# 只读用户
p, role:viewer,      {dom},   {ns}/*,              get|list|watch

# 非 resource 路径（所有角色放行版本/健康类只读端点）
p, role:*,           *,       nonresource/version,   get
p, role:*,           *,       nonresource/openapi*,  get

# ===== 用户-角色绑定（dom = 集群 ID）=====
g, alice, role:cluster-admin, cluster-a
g, bob,   role:developer,     cluster-a
g, carol, role:viewer,        cluster-a
```

> 说明：`role:*` 这类通配绑定按需展开为具体角色或改用中间层角色；示例以清晰为先。
> Casbin 默认白名单 effect（无匹配即拒绝），不存在隐式放行。

### 5.4 请求解析

```go
import (
    "k8s.io/apiserver/pkg/endpoints/request"
    "k8s.io/apimachinery/pkg/util/sets"
)

var resolver = request.NewRequestInfoResolver(&request.RequestInfoFactory{
    APIPrefixes:          sets.NewString("api", "apis"),
    GrouplessAPIPrefixes: sets.NewString("api"),
})

// buildCasbinRequest 把 http.Request 解析为 Casbin 四元组。
// 注意：必须在 strip 掉 /proxy/{clusterID} 前缀之后调用。
func buildCasbinRequest(r *http.Request, username, clusterID string) []interface{} {
    info, err := resolver.NewRequestInfo(r)
    if err != nil {
        return nil // 解析失败一律拒绝
    }

    var obj string
    if info.IsResourceRequest {
        if info.Namespace != "" {
            obj = info.Namespace + "/" + info.Resource
            if info.Subresource != "" {
                obj += "/" + info.Subresource
            }
        } else {
            obj = "cluster/" + info.Resource
        }
    } else {
        obj = "nonresource" + info.Path
    }

    return []interface{}{username, clusterID, obj, info.Verb}
}
```

---

## 六、隧道数据平面：Dialer 模型

### 6.1 协议栈

```text
┌─────────────────────────────────────────────┐
│ HTTP 请求/响应（含 101 升级后的双向字节流）      │ ← 应用层
├─────────────────────────────────────────────┤
│ yamux stream（多路复用）                      │
├─────────────────────────────────────────────┤
│ WebSocket binary frame（帧封装 + ping/pong） │
├─────────────────────────────────────────────┤
│ TLS（Nginx 终结 / 副本间内网可选 mTLS）        │ ← 传输层
└─────────────────────────────────────────────┘
```

### 6.2 Dialer 模型（BUG-3 修复说明）

v1.0 草稿在平台侧手工序列化/反序列化 HTTP（`req.Write` / `http.ReadResponse`），存在 bufio 预读丢字节、chunked 编码与 101 升级处理不可靠三类问题。v2.0 **不再手工处理 HTTP**，把隧道抽象为 `net.Conn` 工厂（Dialer），HTTP 语义交给两端的标准库：

**平台侧：隧道即 Dialer**

```go
// TunnelDialer 实现 http.Transport 的 DialContext。
// 每次拨号 = 打开一条 yamux stream（本地隧道，或经 peer 会话跨 Pod 拼接，见 §7.2）。
type TunnelDialer struct {
    clusterID string
    local     *TunnelManager   // 本 Pod 持有的隧道
    mesh      *Mesh            // 跨 Pod 转发器
}

func (d *TunnelDialer) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
    if t := d.local.Get(d.clusterID); t != nil {
        return t.Session.Open(ctx)      // yamux stream 即 net.Conn
    }
    return d.mesh.DialCluster(ctx, d.clusterID)   // 跨 Pod（§7.2）
}

// 每个集群一个 transport，进程内缓存复用连接池。
func newTunnelTransport(dial *TunnelDialer) *http.Transport {
    return &http.Transport{
        DialContext:           dial.DialContext,
        ForceAttemptHTTP2:     false,        // 上游是 HTTP/1.1 + SPDY 升级
        MaxIdleConnsPerHost:   16,
        IdleConnTimeout:       90 * time.Second,
        ResponseHeaderTimeout: 30 * time.Second,
    }
}
```

**Agent 侧：隧道即监听器**

```go
// Agent 把 yamux 会话包装成 net.Listener，跑一个标准 net/http.Server。
// HTTP 解析全部交给标准库；转发逻辑只做 SA transport 代理。
func (a *Agent) serveOverTunnel(session *yamux.Session) error {
    lis := yamuxListener{session: session}          // Accept() 返回 *yamux.Stream
    srv := &http.Server{Handler: http.HandlerFunc(a.proxyHandler)}
    return srv.Serve(lis)
}

func (a *Agent) proxyHandler(w http.ResponseWriter, r *http.Request) {
    transport, err := rest.TransportFor(a.saConfig) // 注入 SA Token、目标集群 CA
    if err != nil {
        http.Error(w, "transport init failed", 500)
        return
    }
    // r 的路径已被平台 strip 为 /api/v1/...（§8.1）；此处原样转发。
    // 101 升级（SPDY）：标准库把升级后的连接暴露为 Hijacker / resp.Body 双向流。
    if isUpgrade(r) {
        a.proxyUpgrade(w, r, transport)             // 劫持后双向 io.Copy
        return
    }
    resp, err := transport.RoundTrip(r)
    if err != nil {
        http.Error(w, "upstream error", 502)
        return
    }
    defer resp.Body.Close()
    for k, vv := range resp.Header {
        for _, v := range vv {
            w.Header().Add(k, v)
        }
    }
    w.WriteHeader(resp.StatusCode)
    io.Copy(w, resp.Body)
}
```

这样修复了三类缺陷：响应体不再丢字节（标准库管理缓冲）；chunked / Content-Length 正确；101 升级由标准库 + k8s 官方 handler 处理（§9）。

### 6.3 yamux-over-WebSocket 适配（BUG-4 修复说明）

yamux 需要 `io.ReadWriteCloser`，WebSocket 是消息帧语义，必须显式适配，且 gorilla/websocket 有**单写者**约束：

```go
// wsNetConn：把 *websocket.Conn 适配为 net.Conn。
// - 只使用 BinaryMessage（yamux 帧是二进制流）
// - 写侧加互斥锁：yamux 内部 sendLoop 之外还有 ping 等控制写入
// - 关闭语义映射：Close() → CloseWrite + 发送 Close 帧
type wsNetConn struct {
    conn *websocket.Conn
    wmu  sync.Mutex
}

func (c *wsNetConn) Read(b []byte) (int, error) {
    for {
        mt, data, err := c.conn.ReadMessage()
        if err != nil {
            return 0, err
        }
        if mt != websocket.BinaryMessage {
            continue // 忽略 ping/pong/control 由库处理
        }
        n := copy(b, data)
        return n, nil
    }
}

func (c *wsNetConn) Write(b []byte) (int, error) {
    c.wmu.Lock()
    defer c.wmu.Unlock()
    if err := c.conn.WriteMessage(websocket.BinaryMessage, b); err != nil {
        return 0, err
    }
    return len(b), nil
}
```

**双层心跳（缺一不可）**：

| 层 | 机制 | 参数 | 作用 |
| --- | --- | --- | --- |
| WebSocket | 客户端每 20s 发 ping，服务端自动 pong；客户端 10s 未收到 pong 判死 | ping 20s / 超时 10s | 穿透 NAT/LB 空闲超时，快速检测死链 |
| yamux | `EnableKeepAlive: true`，`KeepAliveInterval: 30s`，`ConnectionWriteTimeout: 10s` | 默认值即可 | stream 层活性检测 |

派生约束：**Nginx 对 `/tunnel/` 的 `proxy_read_timeout` 必须 > ping 间隔**（v2.0 取 75s，见 §10），否则空闲心跳会被网关掐断。

yamux 会话参数（两端一致）：

```go
cfg := yamux.DefaultConfig()      // AcceptBacklog 256 / MaxStreamWindowSize 256KB
cfg.KeepAliveInterval = 30 * time.Second
cfg.ConnectionWriteTimeout = 10 * time.Second
```

---

## 七、高可用设计

### 7.1 状态分类清单

HA 的前提是明确"什么状态在哪里"：

| 状态 | 位置 | 故障后果 | 恢复方式 |
| --- | --- | --- | --- |
| 隧道连接本身 | 平台 Pod 内存 | 该 Pod 上的隧道全断 | Agent 自动重连其他副本，**自愈** |
| 隧道注册表 | Redis（TTL 心跳） | 查不到隧道归属 | Agent 重连后重写注册，**自愈** |
| Casbin 策略 | PG（源）+ 各副本内存缓存 | 拒绝变更，存量鉴权不受影响 | watcher 重连后全量重载 |
| 集群/Token 哈希 | PG | 无法注册新 Agent | PG 恢复即可 |
| 审计日志 | 各副本缓冲 → PG | 写入延迟 | 本地文件暂存，恢复后回放（§12.2） |
| JWT 签名私钥 | Secret（多副本共享） | 无法签发新 token | 密钥环多密钥（§4.3） |

**核心 HA 不变量：鉴权路径零外部依赖** —— Casbin 策略缓存在每个副本本地，鉴权决策不依赖 Redis/PG 在线；Redis/PG 故障只影响管理面（策略变更、Agent 注册）与新增能力，存量用户的 kubectl 访问完全不受影响。

### 7.2 隧道注册表与跨 Pod 转发（BUG-1 修复，核心）

#### 7.2.1 为什么不能只靠 LB 粘性

Agent 的 WebSocket 长连接与 kubectl 用户请求的**来源 IP 不同**，基于源 IP 的粘性（ip_hash）无法把 kubectl 请求路由到持有隧道的副本；kubectl 也无法使用 cookie 粘性。因此粘性只能作为优化，不能作为方案，**路由决策必须在应用层完成**。

#### 7.2.2 备选方案对比

| 方案 | 结论 |
| --- | --- |
| LB 粘性（ip_hash / cookie） | ✗ 不可行，kubectl 与 Agent 来源 IP 不同，无共享 cookie |
| 单独"隧道网关"组件持有全部隧道 | ✗ 引入新单点，与 HA 目标矛盾 |
| **Redis 注册表 + 副本间 peer 数据面** | ✓ 采用：控制面轻量（Redis），数据面副本对等，故障自愈（Rancher remotedialer 同型方案） |

#### 7.2.3 注册表设计

Redis 键（TTL 心跳）：

```text
am:tunnel:{clusterID}:{agentID}  →  {"pod": "am-platform-1", "since": "...", "v": 1}
    TTL 15s，持有方每 5s 续期
```

- 键按 `agentID` 细分，天然支持**每集群多 Agent 副本**（§7.4）；同集群多隧道共存，择优使用。
- 持有方 Pod 崩溃 → 键 15s 内过期 → 隧道归属自动消失，期间转发方按"拨号失败 → 重试下一个 owner"处理。
- 注册表**丢失是安全的**：最坏情况是新请求短暂 503，Agent 的指数退避重连（base 1s，×2，cap 30s，full jitter）会在秒级重建注册。

#### 7.2.4 跨 Pod 数据面转发

副本两两之间（N=3 → 每副本 2 条 peer 连接，懒建立并缓存）使用与 Agent **完全相同的协议**（WS + yamux）互联，复用同一套代码：

```text
Pod-X（收到用户请求，无本地隧道）
  │ ① Redis 查 am:tunnel:cluster-a:* → owners = [Pod-Y]
  │ ② 取/建到 Pod-Y 的 peer yamux 会话（headless service:9090，peer token 认证）
  │ ③ peer 会话上打开 stream，首字节写 RELAY 头 {"cluster":"cluster-a"}
  ▼
Pod-Y（RELAY 处理器）
  │ ④ 打开本地 Agent 隧道 yamux stream
  │ ⑤ 把 peer stream 与 agent stream 双向 io.Copy（纯字节拼接，不解析 HTTP）
  ▼
Agent（标准 http.Server 接收完整 HTTP 请求，§6.2）
```

```go
func (m *Mesh) DialCluster(ctx context.Context, clusterID string) (net.Conn, error) {
    if c, ok := m.local.Get(clusterID); ok {
        return c.Session.Open(ctx)
    }
    for _, owner := range m.registry.Lookup(clusterID) {     // Redis
        if owner == m.self {
            continue
        }
        peer, err := m.peerSession(owner)                    // 懒建立并缓存
        if err != nil {
            m.registry.Evict(clusterID, owner)               // 过期数据，顺手清理
            continue
        }
        stream, err := peer.OpenRelay(ctx, clusterID)        // ③
        if err == nil {
            return stream, nil
        }
        m.registry.Evict(clusterID, owner)
    }
    return nil, fmt.Errorf("cluster %s unreachable", clusterID)
}
```

**关键性质：**

- 平台内部（peer hop + agent hop）全程**字节透明**，HTTP/SPDY/WS 语义只在两端终结，exec/port-forward 的 101 升级在拼接管道内无感知通行；
- peer 会话与 Agent 隧道共用适配器与心跳实现（§6.3），多一条协议就多一份 bug 面，v2.0 不引入第二套传输协议；
- peer 连接认证：共享 peer token（Secret 下发）+ `NetworkPolicy` 限制 9090 端口仅平台 Pod 可达。

### 7.3 JWT 密钥多副本共享

见 §4.3。要点：Secret 共享当前私钥 + 密钥环多 `kid` 重叠，轮换零停机。

### 7.4 Agent 侧 HA（每集群 2 副本）

- 每个目标集群运行 **Agent Deployment `replicas: 2`**，两个副本各建一条独立隧道，注册表表现为 `am:tunnel:{clusterID}:{agentID-0/1}` 两条记录。
- 平台转发时按注册表顺序择优（第一个可拨通的 owner），一条隧道断开时**请求级**切换到另一条，无需等待 Agent 重连。
- 两个副本共用同一个集群 Token（同一 Secret），轮换一次即全量生效；建议副本调度到不同节点（`podAntiAffinity`）。
- Agent 副本自身无状态、无 leader 选举需求（拨出连接互不干扰）。

### 7.5 优雅下线与滚动升级

收到 `SIGTERM` 后：

1. readiness 探针立即返回失败 → Nginx 摘流，不再有新用户请求
2. 对本 Pod 持有的全部 Agent 隧道发送 WS `Close(1001)`，Agent 收到后**带抖动立即重连**其他副本
3. 删除本 Pod 在 Redis 的全部注册键
4. 停止接受新的 peer RELAY
5. 排空 in-flight 请求（最长 `terminationGracePeriodSeconds`，默认 60s）后退出

对应 Deployment 参数：`maxSurge: 1` / `maxUnavailable: 0` / `PDB minAvailable: 2` / `preStop: sleep 5`。

**诚实的取舍**：滚动升级会重建副本，正在进行的 exec / port-forward 长连接会断（kubectl exec 无断点续传）。控制面 API 与普通 kubectl 请求由 client-go 自动重试，不受影响。滚动更新建议在业务低峰执行；单副本故障替换（非主动更新）由于隧道 30s 内自愈 + Agent 双副本，影响面可控。

### 7.6 数据层 HA

**PostgreSQL**：

- 云托管实例或 Patroni 集群（1 主 1 同步副本）；平台侧连接池 + 重试。
- Casbin 策略源在 PG，各副本内存缓存 + **casbin watcher（Redis pub/sub）** 增量同步；watcher 断线期间策略变更排队，重连后全量重载兜底。
- 鉴权读路径不经过 PG（§7.1 不变量）。

**Redis**：

- Sentinel（1 主 2 从）或云托管；仅承载：隧道注册表（TTL 键）、casbin watcher、限流计数。
- **Redis 不在鉴权关键路径**：全挂时的表现 = 新隧道注册失败 + 跨 Pod 转发查不到 owner（503）+ 限流失效；存量隧道上的请求与全部鉴权决策不受影响。恢复后 Agent 心跳自动重建注册表，**无人工干预**。

### 7.7 接入层 HA

- Nginx ×2 + **keepalived VIP**（主备秒级漂移），或云厂商 LB。
- 两台 Nginx 均可承载 Agent 隧道与用户流量（Agent 经 DNS/VIP 连接，故障漂移后 Agent 自动重连）。
- 无需配置任何粘性策略（§7.2.1）。
- Nginx upstream 列表由部署工具渲染平台 Service Endpoints（3 副本），`least_conn` 均衡。

### 7.8 容量与扩展

| 维度 | 预算 | 说明 |
| --- | --- | --- |
| 单 Pod 隧道数 | ≤ 200 个集群 | yamux 会话为轻量协程结构，瓶颈在内存与 fd |
| 单隧道并发 stream | AcceptBacklog 256，默认窗口 256KB/stream | 大流量（watch 全量事件）按 stream 隔离 |
| 副本间 peer 连接 | N 副本 N-1 条/副本 | 懒建立；N > 10 时可改无向环路由，v2.0 不做 |
| 扩容动作 | 只改 `replicas` | 新 Pod 自动承接 Agent 重连与注册表写入，无人工迁移 |

---

## 八、请求转发与 Impersonation

### 8.1 平台侧中间件（v2.0 完整顺序）

```go
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // 1. 提取并校验 JWT（kid 密钥环）
    claims, err := p.jwtValidator.Validate(extractBearerToken(r))
    if err != nil {
        audit.LogDenied(claims, r); http.Error(w, "unauthorized", 401); return
    }

    // 2. 解析集群 ID（/proxy/{clusterID}/...）
    clusterID := extractClusterID(r.URL.Path)

    // 3. 剥离用户传入的所有 Impersonate-* 头（防伪造身份，顺序必须在 Casbin 之前）
    stripImpersonateHeaders(r)

    // 4. Casbin 鉴权（本地缓存，无网络调用）
    if allowed, _ := p.enforcer.Enforce(buildCasbinRequest(r, claims.Subject, clusterID)...); !allowed {
        audit.LogDenied(claims.Subject, clusterID, r)
        http.Error(w, "forbidden", 403); return
    }

    // 5. 注入平台控制的 Impersonate 头
    r.Header.Set("Impersonate-User", claims.Subject)
    for _, g := range claims.Groups {
        r.Header.Add("Impersonate-Group", g)
    }

    // 6. FIX-6：strip /proxy/{clusterID} 前缀，重写 Host
    r.URL.Path = strings.TrimPrefix(r.URL.Path, "/proxy/"+clusterID)
    r.Host = "kube-apiserver." + clusterID   // 占位；apiserver 不依赖 Host

    // 7. FIX-7：清除用户 JWT，SA Token 由 Agent 的 transport 注入
    r.Header.Del("Authorization")

    // 8. 审计（异步）
    audit.LogAllowed(claims.Subject, clusterID, r)

    // 9. 经隧道 transport 转发（Dialer 模型，§6.2）
    p.transports.For(clusterID).ServeHTTP(w, r)
}
```

```go
func stripImpersonateHeaders(r *http.Request) {
    for key := range r.Header {
        if strings.HasPrefix(strings.ToLower(key), "impersonate-") {
            delete(r.Header, key)
        }
    }
}
```

必须剥离的头：`Impersonate-User`、`Impersonate-Group`、`Impersonate-Uid`、`Impersonate-Extra-*`。

### 8.2 双重权限校验

| 层级 | 校验方 | 作用 |
| --- | --- | --- |
| 第一层 | 平台 Casbin | 快速拒绝、统一策略、审计 |
| 第二层 | API Server RBAC | 最后防线，以 impersonated 身份校验；审计日志记录真实用户（`impersonatedUser`） |

目标集群侧仍需按 §v1.0 方式为用户/用户组配置 RoleBinding（Casbin 与 RBAC 是**与**的关系，两边都要放行）。

---

## 九、流式连接处理（exec / port-forward）

### 9.1 协议背景

| 集群版本 | 支持协议 | kubectl 1.31+ 行为 |
| --- | --- | --- |
| 1.23 / 1.25 | 仅 SPDY | 尝试 WebSocket，失败后回退 SPDY |
| 1.29+ | SPDY + WebSocket | 默认 WebSocket |

因此平台必须同时支持两种入口协议。

### 9.2 官方 streamtunnel 组件

| Handler | 用途 | 子协议 |
| --- | --- | --- |
| `StreamTranslatorHandler` | RemoteCommand（exec / attach / cp） | `v5.channel.k8s.io` |
| `TunnelingHandler` | PortForward | `SPDY/3.1+portforward.k8s.io` |

### 9.3 平台侧实现（结合 Dialer 模型）

```go
// 流式路由：识别子协议后交给官方 handler，
// transport 即 §6.2 的隧道 transport（跨 Pod 场景下底层是 peer 拼接流，字节透明）。
func (p *Proxy) handleStream(w http.ResponseWriter, r *http.Request, clusterID string) {
    transport := p.transports.For(clusterID)
    upstream := p.upstreamURL(clusterID)   // https://kube-apiserver.{clusterID}

    if isPortForward(r) {
        proxy.NewTunnelingHandler(upstream, transport).ServeHTTP(w, r)
        return
    }
    if isExecLike(r) {                     // exec / attach / cp
        opts := proxy.Options{
            Stdin: hasStream(r, "stdin"), Stdout: hasStream(r, "stdout"),
            Stderr: hasStream(r, "stderr"), Tty: hasStream(r, "tty"),
        }
        proxy.NewStreamTranslatorHandler(upstream, transport, 0, opts).ServeHTTP(w, r)
        return
    }
    transport.ServeHTTP(w, r)              // 普通请求（含 watch）
}
```

> 注：`NewStreamTranslatorHandler` / `NewTunnelingHandler` 的具体签名以所引入的 `k8s.io/apiserver` 版本为准，此处示意职责划分。

### 9.4 kubectl 回退控制（调试期）

```bash
export KUBECTL_REMOTE_COMMAND_WEBSOCKETS=false
export KUBECTL_PORT_FORWARD_WEBSOCKETS=false
```

仅作排障手段；长期必须支持 WebSocket 入口。

### 9.5 完整数据流

```text
kubectl ──WebSocket/SPDY──► Nginx ──► Pod-X（StreamTranslatorHandler / TunnelingHandler）
                                          │ 隧道 transport（Dialer）
                                          ▼
                              本地 Agent 隧道 或 peer 拼接流（字节透明）
                                          ▼
                                       Agent（标准 http.Server + SA transport）
                                          ▼
                                 kube-apiserver ──SPDY──► kubelet:10250
```

---

## 十、接入层（Nginx）配置

### 10.1 完整配置（v2.0 修正版）

```nginx
# BUG-5 修复：按需升级，普通请求保持 keepalive
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      '';
}

upstream am_platform {
    least_conn;
    # 由部署工具渲染 3 个平台副本 Endpoint
    server 10.0.0.11:8080;
    server 10.0.0.12:8080;
    server 10.0.0.13:8080;
    keepalive 32;
}

server {
    listen 443 ssl;
    http2 on;                                # kubectl 的 WS 拨号走 HTTP/1.1，不受影响
    server_name access-manager.example.com;

    ssl_certificate     /etc/nginx/certs/access-manager.crt;
    ssl_certificate_key /etc/nginx/certs/access-manager.key;
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         HIGH:!aNULL:!MD5;

    client_max_body_size 100m;               # kubectl apply 大 YAML

    # ── 流式代理：exec / port-forward / watch ──────────────────────
    location /proxy/ {
        proxy_pass http://am_platform;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade           $http_upgrade;
        proxy_set_header Connection        $connection_upgrade;

        proxy_read_timeout  3600s;           # 长连接（watch 空闲期）
        proxy_send_timeout  3600s;
        proxy_buffering off;
        proxy_request_buffering off;
    }

    # ── Agent 隧道：长连接 + 快速死链检测 ───────────────────────────
    location /tunnel/ {
        proxy_pass http://am_platform;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade           $http_upgrade;
        proxy_set_header Connection        $connection_upgrade;

        proxy_read_timeout  75s;             # 必须 > WS ping 间隔 20s（§6.3）
        proxy_send_timeout  75s;
        proxy_buffering off;
    }

    # ── 控制面 API：短超时 ──────────────────────────────────────────
    location / {
        proxy_pass http://am_platform;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Connection        "";        # 恢复 keepalive
        proxy_read_timeout  60s;
        proxy_send_timeout  60s;
    }

    location /healthz {
        proxy_pass http://am_platform;
        access_log off;
    }
}
```

### 10.2 关键配置说明

| 配置 | 作用 |
| --- | --- |
| `$connection_upgrade` map | 按需升级；无 Upgrade 头的请求保持 keepalive（修复 BUG-5） |
| `/proxy/` 3600s / `/` 60s / `/tunnel/` 75s 分级超时 | 长连接不误杀，死隧道快速检出（修复 BUG-5） |
| `proxy_http_version 1.1` | WebSocket 升级需要 HTTP/1.1 |
| `http2 on` 与 WS 共存 | kubectl 的 WebSocket 走 HTTP/1.1 拨号，HTTP/2 只服务普通浏览器流量 |
| `proxy_buffering off`（流式路由） | 流式响应不缓冲 |

---

## 十一、部署拓扑与清单

### 11.1 前置条件

- 平台集群（运行主控服务）
- 目标集群（1.23 / 1.25+）
- Nginx ×2 + keepalived（或云 LB）
- Authentik（OIDC Provider，双副本 + LB）
- PostgreSQL（HA）
- Redis（Sentinel）

### 11.2 平台 Deployment 要点（v2.0 增量）

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: k8s-am-platform
  namespace: k8s-am-system
spec:
  replicas: 3
  strategy:
    rollingUpdate: {maxSurge: 1, maxUnavailable: 0}   # §7.5
  selector:
    matchLabels: {app: k8s-am-platform}
  template:
    metadata:
      labels: {app: k8s-am-platform}
    spec:
      terminationGracePeriodSeconds: 60
      topologySpreadConstraints:                       # 打散到节点
      - maxSkew: 1
        topologyKey: kubernetes.io/hostname
        whenUnsatisfiable: ScheduleAnyway
        labelSelector:
          matchLabels: {app: k8s-am-platform}
      containers:
      - name: platform
        image: your-registry/k8s-am-platform:__TAG__
        ports:
        - {name: http,  containerPort: 8080}   # 用户/API/隧道（Nginx 后端）
        - {name: peer,  containerPort: 9090}   # 副本间 peer yamux（§7.2.4）
        env:
        - {name: DATABASE_URL, valueFrom: {secretKeyRef: {name: platform-db, key: url}}}
        - {name: REDIS_ADDRESSES, value: "redis-0.redis:26379,redis-1.redis:26379,redis-2.redis:26379"}
        - {name: OIDC_ISSUER,     value: "https://authentik.example.com/application/o/k8s-am/"}
        - {name: OIDC_CLIENT_ID,     valueFrom: {secretKeyRef: {name: oidc, key: client-id}}}
        - {name: OIDC_CLIENT_SECRET, valueFrom: {secretKeyRef: {name: oidc, key: client-secret}}}
        - {name: JWT_KEYRING_DIR, value: "/etc/k8s-am/jwt"}      # 密钥环：多 kid（§4.3）
        - {name: PEER_TOKEN_FILE, value: "/etc/k8s-am/peer/token"}
        - {name: POD_NAME, valueFrom: {fieldRef: {fieldPath: metadata.name}}}
        lifecycle:
          preStop:
            exec: {command: ["sleep", "5"]}      # 等 LB 摘流生效
        readinessProbe:
          httpGet: {path: /readyz, port: http}
        volumeMounts:
        - {name: jwt-keyring, mountPath: /etc/k8s-am/jwt, readOnly: true}
        - {name: peer-token,  mountPath: /etc/k8s-am/peer, readOnly: true}
      volumes:
      - name: jwt-keyring
        secret: {secretName: jwt-keyring}        # 多 kid 并存
      - name: peer-token
        secret: {secretName: peer-token}
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata: {name: k8s-am-platform, namespace: k8s-am-system}
spec:
  minAvailable: 2
  selector:
    matchLabels: {app: k8s-am-platform}
---
# headless service：副本间 peer 直连（pod-0.pod-0.k8s-am-system…）
apiVersion: v1
kind: Service
metadata:
  name: k8s-am-platform-headless
  namespace: k8s-am-system
spec:
  clusterIP: None
  selector: {app: k8s-am-platform}
  ports:
  - {name: peer, port: 9090}
```

### 11.3 目标集群 Agent（v2.0 增量）

- Deployment `replicas: 2`（§7.4），共用同一 Token Secret，`podAntiAffinity` 打散节点
- 其余（SA + cluster-admin ClusterRoleBinding、Token Secret、Volume 挂载、资源限额）与 v1.0 §6.2 一致
- 连接参数：`PLATFORM_URL=wss://access-manager.example.com/tunnel`、`HEARTBEAT_PING_INTERVAL=20s`、重连退避 base 1s / cap 30s / full jitter

### 11.4 部署顺序

1. 部署 Authentik（HA），配置 OIDC Provider（Redirect URI：`https://access-manager.example.com/auth/callback`，Scope：`openid profile email groups`）
2. 部署 PostgreSQL（HA）、Redis（Sentinel）
3. 部署平台服务（3 副本）与 headless Service、PDB
4. 配置 Nginx ×2 + keepalived VIP
5. 在平台注册集群，生成 Token
6. 在目标集群部署 Agent ×2
7. 验证：Agent 双隧道注册表、kubectl 基础读写、watch、exec、port-forward、kill 单副本演练、滚动更新演练

---

## 十二、可观测性与运维

### 12.1 健康检查

```bash
curl -k https://access-manager.example.com/healthz          # 存活
curl -k https://access-manager.example.com/readyz           # 就绪（下线摘流依据）
curl -k https://access-manager.example.com/api/admin/tunnels -H "Authorization: Bearer <admin-token>"
```

### 12.2 审计日志

- **异步批量写** PG（batch insert，队列缓冲），请求路径零阻塞
- PG 不可用时：缓冲落**本地文件**，恢复后按序回放；本地磁盘占用超限时丢弃最旧并打告警指标
- 平台审计字段：`timestamp, user, groups, cluster, method, path, verb, allowed, source_ip, latency_ms, via_peer(bool)`

### 12.3 监控指标

| 指标 | 说明 |
| --- | --- |
| `tunnel_connected_clusters` | 在线集群数 |
| `tunnel_owners{pod}` | 每 Pod 持有隧道数（负载均衡度观测） |
| `tunnel_reconnects_total{cluster}` | 重连次数 |
| `peer_relay_total{src,dst}` | 跨 Pod 转发请求数 |
| `peer_relay_duration_seconds` | 跨 Pod 拼接建立耗时 |
| `registry_sync_up` | 注册表心跳是否正常 |
| `requests_total{cluster,user,verb,status}` | 请求计数 |
| `casbin_denials_total{user,cluster}` | 鉴权拒绝数 |
| `token_rotations_total{cluster}` | Token 轮换次数 |
| `stream_duration_seconds` | 流式连接时长 |
| `audit_backlog_bytes` | 审计本地暂存积压（PG 故障时） |

### 12.4 备份与恢复

| 组件 | 备份内容 | 频率 |
| --- | --- | --- |
| PostgreSQL | 策略、Token 哈希、集群元数据、审计 | 每日 + 审计按留存策略 |
| JWT 密钥环 Secret | 全部 `kid` 私钥 | 变更时 |
| Nginx 证书 / keepalived 配置 | 证书、私钥 | 变更时 |
| Agent Token Secret | K8s Secret | 变更时 |

### 12.5 常见问题排查

| 现象 | 可能原因 | 排查 |
| --- | --- | --- |
| kubectl 401 | JWT 过期 / kid 不在密钥环 | 检查 JWT 有效期、密钥环 Secret |
| kubectl 403 | Casbin 拒绝 / RBAC 拒绝 | 平台审计日志 → API Server 审计日志（注意 `verb` 锚定规则 §5.2） |
| 集群 not connected | Agent 未启动 / Token 无效 / 注册表键过期 | Agent 日志、Redis `GET am:tunnel:*`、平台 `/api/admin/tunnels` |
| exec 偶发断开 | 滚动更新 / peer owner 故障 | 属预期行为（§7.5 取舍）；频率异常则查 `peer_relay_duration_seconds` |
| watch 断开 | 中间代理超时 | 检查 Nginx `/proxy/` 3600s 与链路中间设备 |
| Agent 无限重连 | Token 已吊销且过宽限期 | 应进入重新注册流程（§12.6），检查日志中的 401 处理 |
| 隧道频繁断开 | 心跳被网关掐断 | 确认 `/tunnel/` 读超时 > ping 间隔、中间无空闲超时 < 20s 的设备 |

### 12.6 长时间离线处理（Token 侧，与 v1.0 一致）

- 离线 < 宽限期（默认 24h）：旧 Token 有效，重连后自动续期
- 离线 > 宽限期：旧 Token 吊销，Agent 收到 401 后**停止无限重试**，输出明确错误日志等待管理员重新注册

---

## 十三、故障模式分析

| 故障 | 影响面 | 恢复 | 目标时长 |
| --- | --- | --- | --- |
| 平台单 Pod 崩溃 | 该 Pod 上隧道断；其上 in-flight 请求失败（client-go 重试）；exec 断 | Agent 抖动重连其他副本 + 注册表自愈 | **< 30s** |
| 平台滚动更新 | 同上，按副本滚动 | §7.5 流程 | 每副本 < 60s |
| Nginx 主节点故障 | 连接闪断 | keepalived VIP 漂移，Agent/kubectl 自动重连 | 秒级 |
| Agent 单副本故障 | 该集群剩一条隧道 | 请求级切换到健康隧道（§7.4） | 0（无感） |
| Agent 与平台网络分区 | 该集群请求 503 | 分区恢复后退避重连 | 视网络 |
| Redis 全故障 | 新隧道注册失败、跨 Pod 转发不可用（本地隧道上的请求不受影响）、限流失效 | Agent 心跳重建注册表 | Redis 恢复即自愈 |
| PostgreSQL 故障 | 管理面（登录记录、策略变更、审计延迟）不可用；**kubectl 既有访问不受影响**（§7.1 不变量） | 本地暂存回放 + 连接池重试 | PG 恢复 |
| Authentik 故障 | 新登录失败；存量 JWT 有效期内可用 | 缓存 Discovery/JWKS | Authentik 恢复 |
| JWT 私钥泄漏 | 可伪造任意用户 token | 立即轮换密钥环（旧 kid 移除即失效）；启用 `jti` 黑名单 | 分钟级 |

**SLO 参考**：隧道故障重建 < 30s；跨 Pod 转发引入额外时延 < 10ms（同机房）；控制面 API P99 < 50ms（不含上游 K8s）。

---

## 十四、附录

### 14.1 Go 依赖

```text
// K8s
k8s.io/client-go/rest
k8s.io/client-go/kubernetes
k8s.io/client-go/tools/remotecommand
k8s.io/client-go/tools/portforward
k8s.io/client-go/transport/spdy
k8s.io/apiserver/pkg/endpoints/request
k8s.io/apiserver/pkg/util/proxy
// 认证
github.com/golang-jwt/jwt/v5
github.com/coreos/go-oidc/v3/oidc
// 鉴权
github.com/casbin/casbin/v2
// 隧道
github.com/hashicorp/yamux
github.com/gorilla/websocket
// 存储
github.com/jackc/pgx/v5          // PostgreSQL
github.com/redis/go-redis/v9     // Redis
// 其他
github.com/go-logr/logr
github.com/prometheus/client_golang
```

### 14.2 参考资料

- Kubernetes Authentication / Impersonation / RBAC（kubernetes.io 官方文档）
- KEP-4006: Transition SPDY to WebSockets
- Rancher Manager Architecture（remotedialer 与 crt-token 轮换）
- Casbin Documentation（keyMatch2 / domain 模型）
- Authentik OIDC Provider 文档
- `k8s.io/apiserver/pkg/util/proxy/streamtunnel.go` / `streamtranslator.go`

### 14.3 安全清单（上线前核对）

- [ ] `Impersonate-*` 头剥离在鉴权之前执行
- [ ] Casbin `act` 已锚定（`delete` 不得匹配 `deletecollection`）
- [ ] 用户 JWT 不透传至目标集群（`Authorization` 已清除）
- [ ] Agent Token 仅存哈希；宽限期轮换策略生效；401 后停止无限重试
- [ ] peer 端口 9090 由 NetworkPolicy 限制在平台 Pod 之间
- [ ] TLS ≥ 1.2；平台 CA 与 Authentik 证书链校验开启
- [ ] 审计写 PG 异步且有本地暂存兜底
- [ ] JWT 密钥环支持 ≥ 2 个 `kid` 并存，轮换演练通过
- [ ] 单 Pod kill、滚动更新、Redis/PG 停机演练通过

### 14.4 版本记录

| 版本 | 日期 | 说明 |
| --- | --- | --- |
| v1.0 | 2025-01-15 | 初始版本（DeepSeek 会话产出） |
| v2.0 | 2026-11-01 | 修复 BUG-1~5 / FIX-6~8；补全 HA 架构（注册表 + peer 数据面、密钥环、Agent 双副本、优雅下线、数据层与接入层 HA、故障模式分析） |
