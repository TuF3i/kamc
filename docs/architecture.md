# 系统架构

本文描述 kamc 当前实现（v3.x 行为）。v2.0 的原始设计推导见
[design.md](design.md)，两者冲突时以本文与代码为准。

## 一、定位

kamc 是多集群 Kubernetes 访问管理平台：

- 用户经 Authentik SSO（或 LDAP/本地密码）登录，获得平台 JWT；
- 平台生成 kubeconfig，kubectl 的每个请求都经平台鉴权后转发到目标集群；
- 目标集群只运行 Agent（拨出反向隧道），不向公网暴露 apiserver；
- 权限统一由平台 Casbin 判定，操作与请求全程审计。

## 二、组件

```text
                    ┌──────────────────────────────┐
   浏览器 / kubectl  │  Nginx（TLS、WebSocket、限流） │
                    └──────────────┬───────────────┘
                                   │ HTTPS / WSS
                    ┌──────────────▼───────────────┐
                    │        kamc-platform          │
                    │  HTTP API · 认证 · Casbin     │
                    │  审计 · 隧道管理 · 静态前端    │
                    └───┬───────────┬───────────┬───┘
                        │           │           │
              ┌─────────▼──┐  ┌─────▼─────┐  ┌──▼─────────┐
              │ PostgreSQL │  │   Redis   │  │ MinIO/S3   │
              │ 业务+审计   │  │ 跨副本状态 │  │ 头像(可选) │
              └────────────┘  └───────────┘  └────────────┘
                                   ▲
                        反向隧道（Agent 主动拨出）
                                   │
                    ┌──────────────┴───────────────┐
                    │        kamc-agent             │
                    │  yamux · ReverseProxy · SA     │
                    └──────────────┬───────────────┘
                                   │ 集群内 HTTPS
                    ┌──────────────▼───────────────┐
                    │      kube-apiserver           │
                    └───────────────────────────────┘
```

| 组件 | 代码 | 职责 |
| --- | --- | --- |
| 平台 | `cmd/kamc-platform`、`internal/platform` | HTTP API、认证、Casbin、隧道、审计、静态前端 |
| Agent | `cmd/kamc-agent`、`internal/agent` | 拨出隧道、以 ServiceAccount 访问 apiserver |
| 隧道库 | `internal/tunnel`、`internal/wsconn` | WebSocket → yamux 会话、隧道注册表、peer 转发 |
| exec 插件 | `cmd/kamc` | kubectl 凭证插件：OIDC 设备流程 + Token 兑换 |
| 前端 | `web/` | SolidJS 管理控制台（构建到 `data/web`） |
| 鉴权 | `internal/platform/authz` | Casbin RequestInfo 模型与策略同步 |
| 认证 | `internal/auth`、`internal/platform/oidc` | JWT 签发/校验、JWKS、OIDC |
| 审计 | `internal/platform/audit` | 异步落库、spill、syslog、保留期 |

## 三、请求链路

### 3.1 浏览器访问管理面

```text
浏览器 ──GET /auth/login──▶ 平台 302 ──▶ Authentik
浏览器 ◀─302 /auth/callback?code&state── Authentik
平台   ──code 换 token──▶ Authentik，同步用户/组，执行映射脚本
平台   ──HTML 桥接页（写 localStorage['kamc_token']，跳 /#/）──▶ 浏览器
浏览器 ──Authorization: Bearer <JWT>──▶ /api/v1/**
```

### 3.2 kubectl 数据面

```text
kubectl ──exec 插件 kamc──▶ Authentik 设备授权 ──ID Token──▶ /auth/token-exchange
kubectl ──GET/POST /proxy/{clusterID}/...──▶ 平台
平台    ──JWT 校验 + Casbin(RequestInfo) + 限流──▶ 隧道选择
平台    ──yamux stream──▶ Agent ──SA Token──▶ kube-apiserver
```

平台对用户请求做以下处理：

1. 校验平台 JWT（`RequireJWT`）；
2. 构造 `RequestInfo`（verb/resource/namespace/subresource/nonresource path）；
3. Casbin `Enforce(sub, dom, obj, act)`；
4. 剥离用户传入的 `Impersonate-*` 与 `Authorization` 头；
5. 经隧道转发；响应原样返回（Kubernetes JSON，不是平台信封）。

### 3.3 exec / port-forward

- `channel.k8s.io` WebSocket：平台翻译为 SPDY 帧后直接经隧道访问上游；
- `portforward.k8s.io` / SPDY/3.1：`TunnelingHandler` 原样转发；
- 旧版 SPDY：`ReverseProxy` 直通；
- 浏览器 WebSocket 允许 `?access_token=<JWT>`（仅 WS 升级请求），HTTP 请求不能用该参数。

## 四、认证

| 方式 | 入口 | 说明 |
| --- | --- | --- |
| OIDC（推荐） | `/auth/login` → `/auth/callback` | Authentik；state 存 Redis（多副本）或内存；浏览器回调写 localStorage |
| 账号密码 | `/auth/password-login` | 顺序：库内 bcrypt 哈希 → 内置管理员（环境变量）→ LDAP |
| LDAP 直登 | `/auth/ldap-login` | CLI/exec 场景；成功后同步平台用户/组 |
| ID Token 兑换 | `/auth/token-exchange` | exec 插件用；JWKS 验签后签发平台 JWT |
| 开发直签 | `/auth/dev-token` | 仅 `AM_DEV_ISSUE_TOKEN=1`；生产拒绝启动 |
| 机器人 | `/api/v1/robots` 签发 | sub=`robot:<id>`，长期 JWT（默认 365 天），独立策略主体 |

### 用户目录同步

登录成功后在 `platform_users` upsert 用户（username/nickname/email/groups/source），
`source` 为 `sso`/`ldap`/`local`。组信息写入 `platform_groups`，可标记
`is_admin`；管理组声明会注入 JWT `groups`，但**数据面仍只认 Casbin 策略**。

### 映射脚本

登录链路最后执行 Lua 映射脚本（`auth_scripts`，gopher-lua 沙箱，fail-closed）：
脚本可修改 groups/拒绝登录；失败则拒绝签发并写 `script-failed` 审计。

## 五、JWT 与吊销

- RS256，密钥环目录 `JWT_KEYRING_DIR` 下 `*.pem`，文件名即 `kid`；
- `kid` 轮换：新密钥加入目录即可，JWKS 同时暴露多把公钥；
- claims：`iss/sub/aud/exp/iat/jti/groups/am_ep`；
- 吊销两种粒度：
  - 用户级：`POST /api/v1/auth/revoke` 提升 `am_ep` epoch，旧 Token 全失效；
  - 单 Token：`/auth/logout` 把 `jti` 放入黑名单直到过期；
- 多副本通过 Redis 同步（约 5s）；校验路径只查本地缓存，Redis 不可用不影响存量请求；
- 修改密码成功后会吊销该用户全部 JWT。

## 六、授权

Casbin 模型（`internal/platform/authz`）基于 Kubernetes `RequestInfo`：

```text
p, sub, dom, obj, act
```

- `sub`：`user:<username>`、`org:<group>`、`robot:<id>`；
- `dom`：集群 ID；
- `obj`：`<namespace>/<resource>`（可带 subresource）、`cluster/<resource>`、
  或 nonresource 路径（如 `/version`、`/api`、`/apis`）；
- `act`：`get/list/watch/create/update/patch/delete/deletecollection/*` 或
  nonresource 的 HTTP 方法。

判定规则：

1. 用户在该集群下存在任意 `user:` 策略 → 只按用户策略判定；
2. 否则取用户所属全部 `org:` 组策略的并集（任一命中即放行）；
3. 机器人只按自身 `robot:` 策略判定；
4. 管理组 `am:admin` 只用于管理面 API，**数据面仍需显式策略**。

策略在 UI 中按「主体 + 集群 + 资源/命名空间/动作」编辑，后端展开为 Casbin 行：
namespace `*` 生成 `*/resource` 并补充 `cluster/resource`（覆盖全命名空间 list）；
cluster-scoped 资源生成 `cluster/resource`；具体 namespace 生成 `<ns>/resource`。
nonresource 路径同样支持，保证普通用户可以执行 discovery。

策略变更通过 Redis pub/sub 广播，各副本全量 `LoadPolicy`。

## 七、隧道数据面

- Agent 主动连接 `wss://<platform>/tunnel`，Bearer 长期 Token；
- WebSocket 升级后建立 yamux 会话；平台按集群选择可用会话；
- 多副本：隧道注册表存 Redis；请求落到非持有副本时经 peer 端口（默认 9090）
  使用 RELAY 协议转发到持有副本；
- 平台下线时对 Agent 发 WS 1001，Agent 自动重连其他副本；
- Agent 侧以 ServiceAccount 访问 apiserver；平台已剥离用户身份头，
  **集群侧不再注入 Impersonation**，授权完全在平台完成；
- Agent ServiceAccount 的 RBAC 即平台能力上限，生产建议最小化 ClusterRole。

## 八、审计

| 类型 | 来源 | 内容 |
| --- | --- | --- |
| 平台审计 | 登录、登出、管理 CRUD、策略、机器人、头像、改密、踢出 | 操作者、路径、结果、原因、组 |
| 集群审计 | 经隧道的 kubectl 请求 | 集群、verb/resource/namespace、allow/deny、请求/响应摘要、GeoIP |

- 异步批量写 PostgreSQL；PG 故障时写本地 spill 文件并在恢复后回放；
- 可选 syslog 导出（UDP/TCP/TLS）；
- 保留期 `AM_AUDIT_RETENTION`（默认 720h，每小时清理）；
- 列表默认不返回报文正文（`summary=1`），详情走 `/api/v1/audit/{id}`。

## 九、数据模型（主要表）

| 表 | 内容 |
| --- | --- |
| `platform_users` | uid、username、nickname、email、source、groups、avatar_url、password_hash、metadata |
| `platform_groups` | 组名、成员、`is_admin`、来源 |
| `clusters` | 集群 ID、名称、描述、状态、Agent 信息 |
| `agent_tokens` | Token 哈希、类型、宽限期、吊销状态 |
| `policy_rules` | Casbin 策略行（主体/域/对象/动作） |
| `robots` | 机器人 ID、名称、集群、Token 元数据 |
| `audit_events` | 平台/集群审计事件 |
| `auth_scripts` | Lua 映射脚本（按名称） |

表结构由 GORM `AutoMigrate` 在启动时创建/补齐；破坏性变更需手工 SQL
（见 `deploy/extra/migrations/`）。

## 十、高可用

- 平台 3 副本（StatefulSet 或 Deployment），PDB `minAvailable: 2`；
- peer 端口 9090 仅同 Release Pod 可达（NetworkPolicy）；
- Redis 承载：策略同步、吊销 epoch/黑名单、限流窗口、OIDC state、隧道注册表；
- PostgreSQL 承载业务与审计，建议外部 HA；
- Nginx/keepalived 终结 TLS，`/proxy` 与 `/tunnel` 使用长超时且关闭 buffering；
- Agent 双副本反亲和，隧道断开自动重连其他副本；
- 头像多副本：MinIO 或 ReadWriteMany 卷（见 configuration.md）。

## 十一、扩展点

- 新认证方式：实现 `service.Identity` 生成逻辑并复用 `issueJWTWithSync`；
- 新存储后端：实现 `avatarstore.Store`（`Put/Get/Delete`），在
  `buildAvatarStore` 选择；
- 新权限语义：改 Casbin model 与 `authz` 的 RequestInfo 构造；
- 新审计出口：实现 `audit.Sink`；
- 新部署形态：`deploy/` 下新增清单，保持环境变量契约不变。

## 十二、与 design.md 的关系

`design.md` 是 v2.0 的设计推导与缺陷修复记录（BUG-1..8、FIX-6..8），
对理解隧道/HA 取舍仍有价值。后续新增的用户目录、组管理、机器人、映射脚本、
头像（含 MinIO）、修改密码等能力未回写到该文档，当前行为以本目录其他文档与
代码为准。
