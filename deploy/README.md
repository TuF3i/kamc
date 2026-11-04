# kamc 部署指南

对应设计文档 §10（接入层）/§11（部署）。**上线前必须完成文末人工验收清单**
（本仓库的自动化测试不覆盖真实集群演练）。

## 目录

| 文件 | 内容 |
| --- | --- |
| `nginx/nginx.conf` | 接入层配置（TLS 终结、$connection_upgrade、三级超时路由） |
| `platform/manifests.yaml` | 平台 Deployment/Service/headless/PDB/NetworkPolicy/Secret 模板 |
| `agent/manifests.yaml` | Agent SA/RBAC/Secret/Deployment（双副本） |

## 镜像构建

```bash
make image-platform VERSION=v1.0.0
make image-agent VERSION=v1.0.0
```

`kamc-platform` 镜像已包含 `data/web` 前端产物；Agent 镜像只含隧道客户端。

## 前置条件

- 平台集群（运行主控）、目标集群 1.23+（目标集群内部署 Agent）
- PostgreSQL（HA）、Redis（Sentinel 或单机）
- Authentik（OIDC Provider）：创建 Application，Redirect URI
  `https://__DOMAIN__/auth/callback`，Scope `openid profile email groups`
- Nginx ×2 + keepalived VIP（或云 LB）

## 占位符清单

| 位置 | 占位符 | 说明 |
| --- | --- | --- |
| nginx.conf | `__AM_DOMAIN__` `__AM_CERT__` `__AM_KEY__` `__PLATFORM_ENDPOINTS__` | 域名/证书/平台副本地址 |
| platform | `__IMAGE__` `__PG_DSN__` `__REDIS_ADDR__` `__OIDC_*__` `__DOMAIN__` `__PEER_ADDRESSES__` `__PEER_TOKEN__` `__JWT1_PEM_BASE64__` | 见文件头注释 |
| agent | `__IMAGE__` `__PLATFORM_URL__` `__AGENT_TOKEN__` `__CLUSTER_ID__` | Token 经平台签发 |

密钥生成：

```bash
openssl genrsa -out jwt-1.pem 2048          # JWT 签名密钥（kid=jwt-1）
openssl rand -hex 32                        # PEER_TOKEN
base64 -w0 jwt-1.pem                        # __JWT1_PEM_BASE64__
```

## 部署顺序（§11.4）

1. Authentik 配置 OIDC Provider（双副本 + LB）
2. PostgreSQL（HA）、Redis
3. 平台集群应用 `deploy/platform/manifests.yaml`（替换占位符后）
4. Nginx ×2 + keepalived，加载 `deploy/nginx/nginx.conf`
5. 平台注册目标集群 → 签发 Token
6. 目标集群应用 `deploy/agent/manifests.yaml`（替换占位符后）
7. 验证：`GET /api/v1/clusters/{id}` 状态 `online`；kubeconfig 下载后 kubectl 可用

## 运维要点

- **Agent SA 权限即平台上限**：集群侧不做用户级 Impersonation，用户授权全部由平台 Casbin 完成。
  当前 Agent 模板绑定 `cluster-admin`（最省事）；安全要求高时请替换为覆盖所需 API 的专用
  ClusterRole，并保留 `Impersonate-*` 请求头剥离（这是防提权的必要条件）。

- **滚动升级**：平台 `maxUnavailable: 0`，Agent 隧道经 1001 通知自动重连其他副本；
  进行中的 exec/port-forward 会断（§7.5 已知取舍）
- **JWT 轮换**：新密钥 `jwt-2.pem` 加入 Secret → 存量过期后移除 `jwt-1.pem`（§4.3）
- **Token 轮换**：重新签发即旧 Token 进 24h 宽限期；彻底失效后 Agent 自动停止
  重试并等待新 Token（更新 Secret 即恢复，§6.5）
- **踢出用户**：`POST /api/v1/auth/revoke {"sub":"..."}`（5s 内同步到全部副本）

## 上线前人工验收清单（真实环境，本仓库自动化不覆盖）

> 以下演练需要真实集群与 kubectl 操作，**务必在上线前执行**（设计文档 §13）。

- [ ] kill 单个平台 Pod：Agent 隧道 < 30s 内迁移到其他副本；kubectl 请求恢复
- [ ] 平台滚动更新：控制面零中断；确认 exec 会话断连符合预期
- [ ] Nginx 主节点故障：VIP 漂移，Agent/kubectl 自动重连
- [ ] 目标集群 Agent 单副本故障：请求无感切换到另一隧道
- [ ] Redis 停机：存量 kubectl 访问与鉴权不受影响（§7.1 不变量）；恢复后自愈
- [ ] PostgreSQL 停机：kubectl 访问不受影响；审计落本地 spill 并恢复回放
- [ ] Authentik 停机：存量 JWT 有效期内可用
- [ ] exec / port-forward 双协议验证（kubectl 1.31+ WebSocket 与降级 SPDY）
- [ ] 限流阈值验证（429 + Retry-After；数据面 `AM_RATE_LIMIT`、登录 `AM_LOGIN_RATE_LIMIT`）
- [ ] 确认 `AM_PASSWORD_LOGIN=0` 或已配置非默认密码（生产清单默认关闭）
- [ ] 审计保留期 `AM_AUDIT_RETENTION` 与 syslog TLS 目标已确认
- [ ] 踢出用户后全部副本 5s 内生效
