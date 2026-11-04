# AGENT.md — kamc 项目指南（面向 AI 与新成员）

## 项目定位

kamc（Kubernetes Access Manager Center）是基于 Go 的多集群 Kubernetes 访问管理平台：
用户经 Authentik SSO 登录，下载平台生成的 kubeconfig，kubectl 请求经平台 Casbin 鉴权后，
经平台 Casbin 授权后，通过 Agent 反向隧道（WebSocket + yamux）由 Agent 以 ServiceAccount 身份送达目标集群的 kube-apiserver（集群侧不做 Impersonation）。

**动手前必读设计文档：[docs/design.md](docs/design.md)**（v2.0，含全部架构决策、
v1.0 缺陷修复记录 BUG-1~8、高可用设计与故障模式分析）。

## 技术栈

Go 1.24+ / chi v5 / gorm(PostgreSQL) / go-redis v9 / gorilla/websocket / hashicorp/yamux
golang-jwt v5 + go-oidc v3（认证） / casbin v2 + k8s.io/apiserver（RequestInfo 鉴权）
前端：React 18 + Ant Design 5 + TypeScript + Vite（web/，见 web/README.md）

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
api/
  openapi.yaml       平台 HTTP API 的 OpenAPI 3.0 规范（与代码包分离）
deploy/              部署产物（nginx/manifests/README，见其内部说明）
web/                 管理前端（React 18 + Ant Design 5 + Vite + TypeScript；见 web/README.md）
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
| `AM_PASSWORD_LOGIN` | 启用 | 内置账号密码登录开关（`0` 关闭） |
| `AM_PASSWORD_LOGIN_USER` | `sysadmin` | 内置管理员账号（生产必须覆盖） |
| `AM_PASSWORD_LOGIN_PASSWORD` | `sysadmin123` | 内置管理员密码（生产必须覆盖） |
| `AM_ROBOT_TOKEN_TTL` | `8760h` | Robot 长期 JWT 有效期（Go duration，默认 365 天） |
| `AM_DEV_ISSUE_TOKEN` | — | 开发直签 Token 开关（`/auth/dev-token`，生产勿开） |
| `OIDC_ISSUER` / `OIDC_CLIENT_ID` / `OIDC_CLIENT_SECRET` / `OIDC_REDIRECT_URL` | — | Authentik OIDC（未配置则 /auth/login 503） |
| `APISERVER_URL`（Agent） | in-cluster 探测 | 上游 kube-apiserver 显式覆盖（开发/测试） |
| `PEER_ADDRESSES` | — | 可拨副本列表，逗号分隔（`pod=host:port` 或 `host:port`）；未配置=单副本形态 |
| `PEER_TOKEN` | — | 副本间转发认证令牌（Secret 下发） |
| `AM_PEER_ADDR` | `:9090` | peer 内部监听地址（仅集群内网可达，NetworkPolicy 限平台 Pod） |
| `AM_EXTERNAL_URL` | X-Forwarded-Proto 推导 | 平台对外地址（kubeconfig 生成用） |
| `AM_KUBECONFIG_CA_FILE` | — | kubeconfig 嵌入的平台 CA（可选） |
| `PLATFORM_INSECURE`（Agent） | — | 跳过平台证书校验（开发环境自签） |
| `AM_RATE_LIMIT` | 0（不限） | 数据面每用户每分钟最大请求数（Redis 共享窗口） |
| `AM_AUDIT_SPILL_FILE` | `/tmp/kamc-audit-spill.jsonl` | 审计本地暂存文件（PG 故障时） |
| `AM_GEOIP_DB` | — | MaxMind GeoLite2/GeoIP2 `.mmdb` 路径（审计地理信息；未配置不解析） |
| `AM_SYSLOG_ADDR` | — | 集群审计 syslog 导出地址，`udp://host:514` 或 `tcp://host:514`（空=不导出） |
| `AM_SYSLOG_TAG` | `kamc-cluster-audit` | syslog APP-NAME |
| `AM_AVATAR_DIR` | `data/avatar` | 用户头像文件存储目录 |
| `AM_AVATAR_SECRET` | 派生 | 头像 URL 签名密钥（不配则由部署配置派生，保证重启后 URL 有效） |
| `AM_WEB_DIR` | `data/web` | 前端构建产物目录（后端同进程托管页面；不存在则纯 API） |
| `AM_AGENT_IMAGE_REPO` | `tuf3i/k8s-am-agent` | Agent 镜像仓库完整地址（不含 tag；tag 随平台版本，一键安装清单用） |
| `TEST_REDIS_ADDR` | — | Redis 集成测试门控（watcher/state store/限流，仅测试用） |
| `TEST_PG_DSN` | — | 数据库集成测试门控（仅测试用） |
| `KAMC_API` | `http://127.0.0.1:8080` | 前端开发时 Vite 代理目标（仅 web 开发用） |

## 构建与测试

```bash
make build   # bin/kamc-platform + bin/kamc-agent
make test    # 全量测试
make vet

cd web && npm install
npm run build   # tsc 严格类型检查 + vite build → web/dist/
KAMC_API=http://127.0.0.1:8080 npm run dev
```

## 路线图（对应设计文档章节）

> 全部完成（含 Web 前端）。上线前须完成 deploy/README.md 的人工验收清单（真实集群演练）。

- [x] 基础框架：目录结构、装配层、健康检查、集群注册 CRUD、wsconn 适配器、隧道管理器、Agent 连接循环
- [x] Agent Token 管理：签发/列表/吊销、SHA256 哈希、轮换宽限期、注册 Token 1h TTL
- [x] 隧道全链路：/tunnel 入口（Token 校验+WS 升级+Attach）→ Agent 数据面
      （ReverseProxy 反代 apiserver，101 升级标准库处理）→ /proxy/{clusterID} 代理
      （strip 前缀 FIX-6、剥离用户传入的 Impersonate-*/Authorization FIX-7）
- [x] Agent Token 认证收尾：注册 Token 换长期 Token 一次性交换端点（§6.1）
- [x] JWT 认证：auth 模块（RS256 密钥环/kid 零停机轮换/JWKS）+ OIDC 登录
      （/auth/login、/auth/callback）+ RequireJWT/RequireAdmin 中间件
- [x] Casbin RequestInfo 鉴权（§5，act 锚定 FIX-8）+ /proxy 平台侧授权
      （集群侧不注入 Impersonation；无鉴权开关已移除）
- [x] exec/port-forward 流式入口（§9）：WS 升级按子协议分流——channel.k8s.io →
      StreamTranslatorHandler（WS→SPDY，直接经隧道 transport 访问上游，无本地中继）；
      SPDY/3.1+portforward.k8s.io → TunnelingHandler；旧版 SPDY 经 ReverseProxy 直通。
      注意：UpgradeAwareHandler 必须开 UseRequestLocation（否则路径被 location 替换）
- [x] 跨 Pod peer 转发（§7.2.4）：PeerRouter（RELAY 协议：头+回执+字节拼接；
      注册表择优/失败剔除）+ Manager.Open fallback + 内部监听（AM_PEER_ADDR，
      默认 :9090）。多副本部署需配置：PEER_ADDRESSES、PEER_TOKEN（Secret）、
      headless service（§11.2 manifest 属部署里程碑）
- [x] kubeconfig 生成与 k8s-am-login exec plugin（§4.4）：/auth/token-exchange
      （ID Token 验签兑换）、GET /api/v1/clusters/{id}/kubeconfig（static/exec 两模式，
      自服务 JWT 组）、cmd/k8s-am-login（RFC 8628 设备流程 + 本地缓存）。
      注意：client-go 对明文连接不加载凭证——生产平台必须 https
- [x] JWT 吊销（§4.2）：用户 epoch 踢出（am_ep 声明，同秒重签不误杀）+ jti
      黑名单（登出）；校验只查本地缓存（§7.1 不变量），Redis HINCRBY/ZSET
      跨副本惰性同步（5s 窗口）
- [x] 数据面限流（§7）：/proxy 按用户 ZSET 滑动窗口（AM_RATE_LIMIT 每分钟，
      0=不限）；限流器故障 fail-open；Redis 跨副本 / 内存单副本双实现
- [x] 优雅下线显式通知（§7.5）：Release 对 Agent 隧道发 WS 1001 GoingAway
- [x] 多副本策略同步（§7.6）：自写 Redis pub/sub watcher（不引第三方），
      策略增删广播 → 各副本全量 LoadPolicy；装配在 authz.New(WithSyncChannel)
- [x] OIDC state 外置 Redis（§4.1）：StateStore 接口（内存/GETDEL 双实现）
- [x] Agent 401 停止重试（§6.5）：DialFunc 携带响应，401 驻留等待 Token 变更恢复
- [x] 平台侧读活性看门狗（§6.3）：wsconn LastActivity + watchdog（25s/75s）
- [x] 审计落库（§12.2）：异步批量写 PG、spill 本地暂存回放、GET /api/v1/audit
- [x] Prometheus 指标（§12.3）：/metrics（平台侧；Agent 侧无 HTTP 端口属后续）
- [x] 部署产物：deploy/（nginx、平台/Agent manifests、NetworkPolicy、README
      含上线前人工验收清单——真实集群演练属上线前人工项）
- [x] 权限策略重构：策略主体分三类——`user:<username>`（平台用户目录）、
      `org:<group>`（IdP/平台组）、`robot:<robot id>`（平台签发，独立主体，
      只在机器人页配置）；用户策略优先并完全遮蔽组策略，多组策略取并集；
      组可标记 `is_admin`，成员登录时在 JWT 中注入管理组声明。集群 GVR/动词
      通过 Agent 隧道实时 discovery（5 分钟缓存）。
- [x] Web 前端（web/，React 18 + Ant Design 5 + TypeScript + Vite）：
      登录（SSO + 用户名密码）、概览、集群/Token 管理、权限策略编辑器、
      机器人、审计、kubeconfig 下载、exec 终端（xterm + v5.channel.k8s.io WS，
      access_token 查询参数凭证——仅 WS 升级接受）。浏览器接入后端配合：
      /auth/callback 浏览器导航返回桥接页（写 localStorage['kamc_token'] 后跳
      SPA）。开发：KAMC_API=… npm run dev；生产构建 dist/ + 同源反代
      （web/README.md）
