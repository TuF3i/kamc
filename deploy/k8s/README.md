# 原生 Kubernetes 清单

不依赖 Helm 的部署方式，适合无法安装 Helm 的集群。生产推荐使用
[`deploy/helm`](../helm/README.md)（StatefulSet + 自动 peer 地址 + Secret 契约）。

| 文件 | 内容 |
| --- | --- |
| `platform.yaml` | 平台 Deployment（3 副本）、Service/headless、PDB、NetworkPolicy、Secret 模板 |
| `agent.yaml` | Agent ServiceAccount/RBAC/Secret/Deployment（2 副本） |

## 使用步骤

1. 构建并推送镜像：

   ```bash
   make image-platform VERSION=v1.0.0
   make image-agent VERSION=v1.0.0
   docker push <registry>/kamc-platform:v1.0.0
   docker push <registry>/kamc-agent:v1.0.0
   ```

2. 替换 `platform.yaml` 中的占位符（文件头有完整清单）：
   `__IMAGE__`、`__PG_DSN__`、`__REDIS_ADDR__`、`__OIDC_*__`、`__DOMAIN__`、
   `__PEER_ADDRESSES__`、`__PEER_TOKEN__`、`__JWT1_PEM_BASE64__`、`__AVATAR_SECRET__`。
3. 应用平台清单：

   ```bash
   kubectl apply -f deploy/k8s/platform.yaml
   ```

4. 平台注册目标集群并签发 Agent Token 后，替换 `agent.yaml` 的
   `__IMAGE__`、`__PLATFORM_URL__`、`__AGENT_TOKEN__`、`__CLUSTER_ID__` 并应用：

   ```bash
   kubectl apply -f deploy/k8s/agent.yaml
   ```

5. 验证：`GET /api/v1/clusters/{id}` 状态为 `online`，kubeconfig 下载后 kubectl 可用。

## 头像存储与 MinIO

默认使用本地目录（镜像内 `/data/avatar`，Pod 本地临时卷）。多副本部署时各 Pod
看不到彼此的头像，生产建议启用 MinIO 或挂载 RWX 卷。启用 MinIO 时在平台
Deployment 中追加：

```yaml
env:
  - name: AM_AVATAR_BACKEND
    value: minio
  - name: AM_AVATAR_MINIO_ENDPOINT
    value: minio.example.com:9000
  - name: AM_AVATAR_MINIO_BUCKET
    value: kamc-avatars
  - name: AM_AVATAR_MINIO_USE_SSL
    value: "true"
  - name: AM_AVATAR_MINIO_ACCESS_KEY
    valueFrom:
      secretKeyRef: {name: platform-avatar, key: minio-access-key}
  - name: AM_AVATAR_MINIO_SECRET_KEY
    valueFrom:
      secretKeyRef: {name: platform-avatar, key: minio-secret-key}
```

`platform-avatar` Secret 中同时保留 `secret`（签名密钥，生产必填）。启用 MinIO 后
不再需要头像卷；对象由平台代理读取，浏览器仍访问 `/avatars/{uid}?token=...`。

## 审计日志与 Promtail

平台把集群审计写到 `/var/log/kamc/cluster-audit.jsonl`（`AM_AUDIT_LOG_FILE`），
清单挂载 `cluster-log` emptyDir。采集方式：

- **sidecar（推荐）**：在同一 Pod 增加 Promtail 容器，挂载 `cluster-log` 只读，
  配置 `__path__: /var/log/kamc/*.jsonl*`，推送到 Loki；
- **DaemonSet**：使用 RWX 卷或 hostPath 让 Promtail 读取同一路径；
- 应用日志（Zap JSON）在容器 stdout，可用节点级 Promtail/K8s 日志采集。

完整配置见 [deploy/observability/README.md](../observability/README.md)。

## 注意

- 静态清单使用 Deployment + 手工枚举 `PEER_ADDRESSES`；Helm Chart 使用
  StatefulSet 自动生成 per-pod DNS，二者语义一致但更新方式不同。
- `AM_ENV=production` 时平台强制要求 `DATABASE_URL` 与 `AM_AVATAR_SECRET`，
  并拒绝 `AM_DEV_ISSUE_TOKEN=1`。
- Agent 的 ServiceAccount 权限即平台能力上限；按需把 `cluster-admin` 换成最小
  ClusterRole。
