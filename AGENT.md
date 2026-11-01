# AGENT.md — kamc 项目指南（面向 AI 与新成员）

## 项目定位

kamc（Kubernetes Access Manager Center）是基于 Go 的多集群 Kubernetes 访问管理平台：
用户经 Authentik SSO 登录，下载平台生成的 kubeconfig，kubectl 请求经平台 Casbin 鉴权后，
通过 Agent 反向隧道（WebSocket + yamux）以 Impersonation 方式送达目标集群的 kube-apiserver。

**动手前必读设计文档：[docs/design.md](docs/design.md)**（v2.0，含全部架构决策、
v1.0 缺陷修复记录 BUG-1~8、高可用设计与故障模式分析）。

## 技术栈

Go 1.24+ / chi v5 / gorm(PostgreSQL) / go-redis v9 / gorilla/websocket / hashicorp/yamux
golang-jwt v5 + go-oidc v3（认证） / casbin v2 + k8s.io/apiserver（RequestInfo 鉴权）

## 目录结构

```text
cmd/
  kamc-platform/     平台主控服务入口
  kamc-agent/        Agent 入口（部署在目标集群）
internal/
  version/           构建版本信息（-ldflags 注入，叶包）
  wsconn/            WebSocket → net.Conn 适配器（yamux 依赖它，平台/Agent 共用）
  tunnel/            平台侧隧道模块：隧道管理器 + Redis 注册表
  platform/          平台 HTTP 模块：认证/鉴权/集群管理 API（当前为基础框架）
  agent/             Agent 模块：拨出连接循环、Token 轮询、流处理
infrastructure/
  postgres/          gorm PostgreSQL 连接统一构建
  redis/             go-redis UniversalClient 统一构建（单机/哨兵/集群）
docs/
  design.md          设计文档 v2.0（架构权威来源）
```

## 架构约定（摘要）

1. **单向依赖**：`handler → service → dao → db`；`modules/`、`errcode/`、`internal/version` 是叶包，被依赖但不依赖他人。
2. **装配与业务分离**：每个模块顶层唯一的 `runtime.go` 只负责组装与生命周期（包注释即模块架构文档）；业务方法全部在 Service 上。
3. **依赖注入**：模块边界用 Functional Options（`WithXxx`）；模块内部用构造函数注入链（`NewXxx`），不引入 wire/dig。
4. **所有权明确**：依赖"注入优先，否则自建"；自建资源由模块自己的 `ReleaseRuntime` 释放；一切 Release/Close **幂等**。
5. **错误处理两套模式，不混用**：
   - 模式 A（标准 Go）：sentinel `ErrXxx` + `%w` 包装并携带操作路径前缀（如 `fmt.Errorf("registry -> report: %w", err)`）——用于 dao/infra/tunnel/agent；
   - 模式 B（Response 即 error）：业务错误为 errcode 包级 var，层间 `(data, rsp)` 元组上抛，handler 统一出口 `GenFinalResponse`——仅用于 platform 的 HTTP API 层。
6. **并发**：done-channel + WaitGroup；管道写入一律双 select；所有等待带超时；goroutine 退出路径必须有 `wg.Done` 或明确所有者。
7. **注释与命名**：注释统一中文；导出符号必有注释；日志带 `[模块名]` 前缀且只记一次性事件；常量**必须带类型**；构造 `NewXxx`、选项 `WithXxx`、幂等初始化 `SetupXxx`、sentinel `ErrXxx`。
8. **测试**：与被测包同目录；数据库用例用 `TEST_PG_DSN` 环境变量门控（未设置则 `t.Skip`）；所有等待带超时；提交前 `go build ./... && go vet ./... && go test -count=1 ./...`。

## 环境变量

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `DATABASE_URL` | 本地 kamc 库 | PostgreSQL 连接串（gorm DSN） |
| `AM_LISTEN_ADDR` | `:8080` | 平台 HTTP 监听地址 |
| `REDIS_ADDRESSES` | — | Redis 地址，逗号分隔（哨兵/集群多地址）；未配置则隧道模块不启用 |
| `REDIS_ADDR` | — | 单地址 Redis（与上二选一） |
| `REDIS_PASSWORD` / `REDIS_DB` | 空 / 0 | Redis 认证 |
| `PLATFORM_URL` | 必填(Agent) | 平台隧道入口，如 `wss://access-manager.example.com/tunnel` |
| `CLUSTER_ID` | 必填(Agent) | 本集群 ID（Casbin dom） |
| `AGENT_ID` | hostname | Agent 实例标识（双副本时须不同） |
| `TOKEN_FILE` | `/etc/k8s-am/token/token` | Agent Token 文件（Secret 挂载） |
| `HEARTBEAT_PING_INTERVAL` | `20s` | WS 心跳 ping 间隔 |
| `JWT_KEYRING_DIR` | 临时密钥 | JWT 签名密钥环目录（Secret 挂载，*.pem 文件名即 kid；未配置用临时密钥并告警） |
| `AM_JWT_ISSUER` / `AM_JWT_AUD` | `https://kamc.local` / `kamc` | 平台 JWT iss/aud |
| `AM_ADMIN_GROUP` | `am:admin` | 管理面用户组（JWT groups 内） |
| `AM_DEV_ISSUE_TOKEN` | — | 开发直签 Token 开关（`/auth/dev-token`，生产勿开） |
| `OIDC_ISSUER` / `OIDC_CLIENT_ID` / `OIDC_CLIENT_SECRET` / `OIDC_REDIRECT_URL` | — | Authentik OIDC（未配置则 /auth/login 503） |
| `APISERVER_URL`（Agent） | in-cluster 探测 | 上游 kube-apiserver 显式覆盖（开发/测试） |
| `TEST_PG_DSN` | — | 数据库集成测试门控（仅测试用） |

## 构建与测试

```bash
make build   # bin/kamc-platform + bin/kamc-agent
make test    # 全量测试
make vet
```

## 路线图（对应设计文档章节）

- [x] 基础框架：目录结构、装配层、健康检查、集群注册 CRUD、wsconn 适配器、隧道管理器、Agent 连接循环
- [x] Agent Token 管理：签发/列表/吊销、SHA256 哈希、轮换宽限期、注册 Token 1h TTL
- [x] 隧道全链路：/tunnel 入口（Token 校验+WS 升级+Attach）→ Agent 数据面
      （ReverseProxy 反代 apiserver，101 升级标准库处理）→ /proxy/{clusterID} 代理
      （strip 前缀 FIX-6、剥离 Impersonate-*/Authorization FIX-7）
- [x] Agent Token 认证收尾：注册 Token 换长期 Token 一次性交换端点（§6.1）
- [x] JWT 认证：auth 模块（RS256 密钥环/kid 零停机轮换/JWKS）+ OIDC 登录
      （/auth/login、/auth/callback）+ RequireJWT/RequireAdmin 中间件
- [x] Casbin RequestInfo 鉴权（§5，act 锚定 FIX-8）+ /proxy Impersonation 注入
      （§8.1 完整顺序，无鉴权开关已移除）
- [ ] exec/port-forward 流式入口：官方 StreamTranslatorHandler / TunnelingHandler
      接入 /proxy 路由（§9；Agent 侧 101 反代已就绪，平台侧待接）
- [ ] 跨 Pod peer 转发：peer yamux 会话 + RELAY 拼接（§7.2.4）
- [ ] kubeconfig 生成与 k8s-am-login exec plugin（§4.4）
- [ ] 多副本策略同步：casbin Redis watcher（§7.6）；OIDC state 外置 Redis
- [ ] Agent 收到 401 后停止重试等管理员的重新注册流程（§6.5）
- [ ] HA 收尾：审计异步落库与本地暂存（§12.2）、注册表驱动的跨副本集群状态收敛、
      平台侧隧道读超时看门狗（当前依赖 Agent 侧 ping 判死）
