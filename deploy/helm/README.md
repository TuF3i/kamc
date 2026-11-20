# Helm Charts

两个独立 Chart，分别对应平台控制面与目标集群 Agent：

| Chart | 部署位置 | 内容 |
| --- | --- | --- |
| `kamc-platform/` | 平台集群（一次） | StatefulSet、Service/headless、PDB、NetworkPolicy、Secret、可选 Ingress/PVC |
| `kamc-agent/` | 每个目标集群（各一次） | Deployment、ServiceAccount、ClusterRoleBinding、Token Secret |

平台多副本使用 **StatefulSet**：peer 端口需要稳定的 Pod DNS
（`<fullname>-<i>.<fullname>-headless`），Chart 在 `config.peerAddresses`
留空时按 `replicaCount` 自动生成。

## 快捷操作（Makefile）

```bash
export DATABASE_URL='postgres://kamc:***@postgres:5432/kamc?sslmode=require'
export OIDC_CLIENT_ID=kamc OIDC_CLIENT_SECRET=*** PASSWORD_LOGIN_PASSWORD=***

make -C deploy/helm secrets            # namespace + JWT/平台 Secret（幂等）
make -C deploy/helm install-platform EXTERNAL_URL=https://kamc.example.com \
  REDIS_ADDR=redis:6379 OIDC_ISSUER=https://sso.example.com/application/o/kamc/
make -C deploy/helm install-agent CLUSTER_ID=prod-bj \
  PLATFORM_URL=wss://kamc.example.com/tunnel
make -C deploy/helm status
```

根目录也可用透传目标：`make helm-help`、`make helm-lint`、`make helm-install-platform ...`。
`DRY_RUN=server` 可让安装目标只做服务端校验；`VALUES=a.yaml` 追加 values 文件。

## 前置条件

- Kubernetes 1.23+
- 镜像已推送到可访问的仓库（`make image-platform` / `make image-agent`）
- PostgreSQL（推荐 HA）、Redis（可选；多副本跨 Pod 转发与吊销同步需要）
- 平台对外 TLS（推荐外部 Nginx + `deploy/extra/nginx/nginx.conf`；WebSocket 需要长读超时）
- OIDC（Authentik）或内置密码登录

## 安装平台

### 1) 准备 Secret（推荐）

敏感值放在预先创建的 Secret 中，键名固定：

```bash
kubectl create namespace k8s-am-system

# JWT 签名密钥环：文件名即 kid，可放多把实现零停机轮换
openssl genrsa -out /tmp/jwt-1.pem 2048
kubectl -n k8s-am-system create secret generic kamc-jwt \
  --from-file=jwt-1.pem=/tmp/jwt-1.pem

kubectl -n k8s-am-system create secret generic kamc-secrets \
  --from-literal=database-url="postgres://kamc:***@postgres:5432/kamc?sslmode=require" \
  --from-literal=oidc-client-id="kamc" \
  --from-literal=oidc-client-secret="***" \
  --from-literal=peer-token="$(openssl rand -hex 32)" \
  --from-literal=avatar-secret="$(openssl rand -hex 32)" \
  --from-literal=password-login-password="***"   # 仅启用密码登录时需要
```

| 键 | 说明 |
| --- | --- |
| `database-url` | PostgreSQL DSN（必填） |
| `oidc-client-id` / `oidc-client-secret` | Authentik Provider 凭证（启用 OIDC 时必填） |
| `peer-token` | 副本间转发令牌（多副本必填） |
| `avatar-secret` | 头像 URL 签名密钥（生产必填） |
| `password-login-password` | 内置密码登录密码（`passwordLogin.enabled=true` 时使用） |

### 2) 安装

```bash
helm upgrade --install kamc deploy/helm/kamc-platform \
  -n k8s-am-system --create-namespace \
  --set image.repository=reg.example.com/kamc/k8s-am-platform \
  --set image.tag=v1.0.0 \
  --set config.externalURL=https://kamc.example.com \
  --set config.redisAddr=redis:6379 \
  --set secrets.existingSecret=kamc-secrets \
  --set jwtKeyring.existingSecret=kamc-jwt \
  --set oidc.issuer=https://sso.example.com/application/o/kamc/
```

开发/POC 可省略 `secrets.existingSecret`，直接 `--set secrets.databaseURL=...`
`--set secrets.avatarSecret=...` `--set jwtKeyring.jwtKeyPEM="$PEM"`，由 Chart 创建 Secret。

### 3) 外部入口

- 默认不创建 Ingress：按 `deploy/extra/nginx/nginx.conf` 在外部 Nginx 终结 TLS，
  反代 `<fullname>:8080`。
- 或 `--set ingress.enabled=true --set ingress.hosts[0].host=kamc.example.com`，
  默认已带 WebSocket 所需的 `proxy-read-timeout/send-timeout=3600` 与关闭 buffering。

### 4) 多副本注意事项

- 头像存储在卷上：默认 `emptyDir`，各 Pod 互不可见。
  多副本请提供 **ReadWriteMany** 存储：
  `--set avatar.persistence.enabled=true --set avatar.persistence.storageClassName=<rwx-sc>`
  或 `--set avatar.persistence.existingClaim=<现有 RWX PVC>`。
- `PEER_ADDRESSES` 留空时自动生成；若使用外部服务发现可显式覆盖。
- PDB 默认 `minAvailable: 2`（`replicaCount=3`）。

## 安装 Agent

Agent Token 在平台「集群 → Agent 安装」签发（`am-clu-` 前缀）。

```bash
# 目标集群（同一个 namespace 名称便于管理）
kubectl create namespace k8s-am-system
kubectl -n k8s-am-system create secret generic kamc-agent-token \
  --from-literal=token=am-clu-xxxxxxxx

helm upgrade --install kamc-agent deploy/helm/kamc-agent \
  -n k8s-am-system \
  --set image.repository=reg.example.com/kamc/k8s-am-agent \
  --set image.tag=v1.0.0 \
  --set clusterID=prod-bj \
  --set platformURL=wss://kamc.example.com/tunnel \
  --set existingSecret=kamc-agent-token
```

安装后平台侧集群状态应变为 `online`。Token 轮换只需更新 Secret，Agent 会按
文件 mtime 自动重读并重连。

## 校验

```bash
helm lint deploy/helm/kamc-platform -f deploy/helm/kamc-platform/ci/test-values.yaml
helm lint deploy/helm/kamc-agent -f deploy/helm/kamc-agent/ci/test-values.yaml
helm template kamc deploy/helm/kamc-platform -n k8s-am-system -f deploy/helm/kamc-platform/ci/test-values.yaml
```

## 与静态清单的关系

`deploy/k8s/platform.yaml` 与 `deploy/k8s/agent.yaml` 保留作为无 Helm
环境的最小模板；两者字段保持一致。生产建议使用 Helm（Secret 管理、升级与回滚更可控）。
