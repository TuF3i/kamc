# 部署指南

本文覆盖三种部署方式与上线流程。架构背景见 [architecture.md](architecture.md)，
全部配置项见 [configuration.md](configuration.md)。

## 一、方式对比

| 方式 | 目录 | 适用 | 数据依赖 |
| --- | --- | --- | --- |
| Docker Compose | `deploy/docker/` | 本地开发、验收、单机演示 | 容器内 PG/Redis，可选 MinIO |
| 原生 K8s | `deploy/k8s/` | 无 Helm 的集群 | 外部 PG/Redis，可选 MinIO |
| Helm（推荐） | `deploy/helm/` | 生产集群 | 外部 PG/Redis，可选 MinIO |

三种方式使用同一组环境变量与 Secret 契约，行为一致；Helm 额外提供 StatefulSet
稳定 DNS、自动 peer 地址、PDB、NetworkPolicy 与 Secret 模板。

## 二、前置条件

| 依赖 | 要求 |
| --- | --- |
| PostgreSQL | 14+，建议 HA；平台启动时 AutoMigrate |
| Redis | 6+；多副本必须（策略同步/吊销/隧道注册表/限流/state） |
| Authentik | OIDC Provider，Redirect URI `https://<domain>/auth/callback`，scope `openid profile email groups` |
| Nginx/LB | TLS 终结；`/proxy`、`/tunnel` 长超时且关闭 buffering |
| MinIO（可选） | S3 兼容；用于多副本共享头像，替代 RWX 卷 |
| 目标集群 | Kubernetes 1.23+，可出网访问平台 `/tunnel` |

## 三、Docker Compose（本地/验收）

```bash
cp deploy/docker/.env.example deploy/docker/.env
# 编辑 .env：至少修改 POSTGRES_PASSWORD、AM_PASSWORD_LOGIN_PASSWORD、AM_AVATAR_SECRET
docker compose -f deploy/docker/compose.yaml up -d --build
# 或：just compose-up
```

- 入口：http://localhost:8080（`KAMC_PORT` 可改）；
- 默认 `AM_ENV=development`，内置账号 `sysadmin/sysadmin123`；
- 未配置 `JWT_KEYRING_DIR`，平台使用临时密钥，容器重启后需重新登录；
- 数据卷：`pgdata`、`redisdata`、`avatardata`、`auditdata`、`miniodata`；
- 完全重置：`docker compose -f deploy/docker/compose.yaml down -v`。

启用 MinIO：

```dotenv
AM_AVATAR_BACKEND=minio
AM_AVATAR_MINIO_ENDPOINT=minio:9000
AM_AVATAR_MINIO_ACCESS_KEY=kamc
AM_AVATAR_MINIO_SECRET_KEY=kamc-minio-secret
MINIO_ROOT_USER=kamc
MINIO_ROOT_PASSWORD=kamc-minio-secret
```

```bash
docker compose -f deploy/docker/compose.yaml --profile minio up -d --build
# 或：just compose-up-minio
```

MinIO 官方社区镜像已下架，示例使用 `cgr.dev/chainguard/minio`（等价二进制）；
生产可换自建镜像或任意 S3 兼容服务。控制台 http://localhost:9001。

Compose 仅用于本地/验收：不要用它承载生产流量，也不要把 `.env` 提交到仓库。

## 四、原生 Kubernetes

完整占位符与步骤见 [deploy/k8s/README.md](../deploy/k8s/README.md)，要点：

1. 构建并推送镜像：

   ```bash
   just image-platform VERSION=v1.0.0
   just image-agent VERSION=v1.0.0
   docker push <registry>/kamc-platform:v1.0.0
   docker push <registry>/kamc-agent:v1.0.0
   ```

2. 准备 Secret：`platform-db`、`oidc`、`jwt-keyring`、`peer-token`、
   `platform-avatar`（含 `secret`，可选 MinIO 凭据）。
3. 替换 `deploy/k8s/platform.yaml` 占位符并 apply。
4. 在平台注册目标集群并签发 Agent Token。
5. 替换 `deploy/k8s/agent.yaml` 占位符并 apply。
6. 验证 `GET /api/v1/clusters/{id}` 为 `online`，kubeconfig 可执行 kubectl。

静态清单使用 Deployment + 手工枚举 `PEER_ADDRESSES`；Helm 使用 StatefulSet
自动生成 per-pod DNS。

## 五、Helm（生产推荐）

```bash
export DATABASE_URL='postgres://kamc:***@postgres:5432/kamc?sslmode=require'
export OIDC_CLIENT_ID=kamc OIDC_CLIENT_SECRET=*** PASSWORD_LOGIN_PASSWORD=***

make -C deploy/helm secrets            # 或 just helm-secrets
make -C deploy/helm install-platform EXTERNAL_URL=https://kamc.example.com \
  REDIS_ADDR=redis:6379 OIDC_ISSUER=https://sso.example.com/application/o/kamc/
make -C deploy/helm install-agent CLUSTER_ID=prod-bj \
  PLATFORM_URL=wss://kamc.example.com/tunnel
make -C deploy/helm status
```

### 5.1 Secret 契约

`kamc-secrets`（或 `secrets.existingSecret`）键：

| 键 | 必填 | 说明 |
| --- | --- | --- |
| `database-url` | 是 | PostgreSQL DSN |
| `oidc-client-id` / `oidc-client-secret` | 启用 OIDC 时 | Authentik Provider |
| `peer-token` | 多副本 | 副本间转发令牌 |
| `avatar-secret` | 生产 | 头像 URL 签名密钥 |
| `password-login-password` | 启用密码登录时 | 内置账号密码 |
| `minio-access-key` / `minio-secret-key` | 启用 MinIO 时 | 对象存储凭据 |

`kamc-jwt`：`jwt-1.pem`（文件名即 kid，可放多把轮换）。

### 5.2 MinIO

```bash
--set avatar.minio.endpoint=minio.k8s-am-system:9000 \
--set avatar.minio.bucket=kamc-avatars \
--set avatar.minio.useSSL=false \
--set avatar.minio.accessKey=*** --set avatar.minio.secretKey=***
# 或专用 Secret
--set avatar.minio.existingSecret=kamc-minio
```

`avatar.backend=auto`（默认）下配置 endpoint 即启用 MinIO；`local|minio` 可显式
指定。MinIO 模式不再渲染头像 PVC/卷，bucket 不存在时平台启动自动创建。
详见 [deploy/helm/README.md](../deploy/helm/README.md)。

## 六、接入层（Nginx）

模板：[deploy/extra/nginx/nginx.conf](../deploy/extra/nginx/nginx.conf)

| 路径 | 超时/要点 |
| --- | --- |
| `/proxy/` | 3600s 读写超时、`proxy_buffering off`、WebSocket Upgrade |
| `/tunnel` | 75s（必须大于 Agent 20s 心跳） |
| `/` | 60s，keepalive；静态前端由平台托管 |
| `= /metrics` | deny all（集群内网采集） |

TLS 1.2/1.3、HSTS、CSP（`img-src 'self' data: https:`）、
`client_max_body_size 100m`。若由 Nginx 直接托管前端 `data/web`，需补 SPA 回退
（见 [web/README.md](../web/README.md)）。

## 七、数据库迁移

表结构由 GORM AutoMigrate 自动创建/补齐。破坏性/一次性变更需手工执行
`deploy/extra/migrations/` 下 SQL，例如：

```bash
psql "$DATABASE_URL" -f deploy/extra/migrations/2026-11-02-policy-reset.sql
```

迁移前先备份；升级顺序：备份 → 迁移 SQL → 滚动更新平台 → 验证。

## 八、升级与回滚

### 平台

- Helm：修改镜像 tag 后 `helm upgrade`；StatefulSet `maxUnavailable: 0`，
  Pod 滚动更新，隧道经 1001 通知迁移；
- 原生：`kubectl set image` 或重新 apply；
- 升级前确认 `AM_AVATAR_SECRET`/JWT keyring 未变，否则存量 URL/Token 失效；
- 回滚：`helm rollback` 或改回旧镜像；数据库迁移需人工评估（无自动 down）。

### Agent

- 重新签发 Token 会进入 24h 宽限期，旧 Token 到期后 Agent 停止重试并等待新
  Token；更新 Secret 即恢复；
- 双副本滚动，单副本故障不影响请求。

## 九、上线验收

部署完成后至少验证：

- [ ] `/healthz`、`/readyz` 正常；`/version` 版本正确
- [ ] SSO 登录与回调桥接页正常；`/auth/me` claims 正确
- [ ] 普通用户 kubeconfig 可 `kubectl get ns`，未授权资源 403
- [ ] 管理员策略编辑后 5s 内全部副本生效
- [ ] 审计页能看到登录与 kubectl 请求；PG 故障时 spill 可回放
- [ ] 头像上传/展示正常（本地或 MinIO）
- [ ] Nginx 断连、平台单 Pod 故障、Agent 单副本故障演练
- [ ] `AM_PASSWORD_LOGIN=0` 或已改默认密码；`AM_AVATAR_SECRET` 已设置

完整人工验收清单见 [deploy/README.md](../deploy/README.md)。
