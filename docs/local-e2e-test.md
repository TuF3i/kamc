# 本地端到端联调指南

面向真实集群的联调步骤：补齐单元测试覆盖不到的三项——**exec 模式 kubeconfig（OIDC Device Flow）**、
**port-forward / SPDY 降级**、**GeoIP 富化**。全部工具在本目录，不依赖外网镜像。

> 生产环境不需要这些：TLS 由 Nginx 终结、OIDC 用 Authentik、GeoIP 用正式 mmdb。

## 0. 前置

1. 平台按联调参数启动（关键：`REDIS_ADDR` 决定隧道模块是否启用，缺省时 `/tunnel` 会拒绝）：

```bash
docker start kamc-postgres kamc-redis   # 依赖容器

# 自签证书（证书内容含 SAN IP:127.0.0.1）
mkdir -p /tmp/kamc-tls
openssl req -x509 -newkey rsa:2048 -nodes -keyout /tmp/kamc-tls/key.pem \
  -out /tmp/kamc-tls/cert.pem -days 2 -subj "/CN=127.0.0.1" \
  -addext "subjectAltName=IP:127.0.0.1,DNS:localhost"

# 本地 TLS 终结（kubectl 不向 http:// 发送 Bearer Token，必须 https）
go run ./hack/e2e/tlsproxy \
  -listen 127.0.0.1:8443 -upstream 127.0.0.1:8080 \
  -cert /tmp/kamc-tls/cert.pem -key /tmp/kamc-tls/key.pem

# 平台（另开终端；OIDC/GEOIP 两项可按需省略）
SSL_CERT_FILE=/tmp/kamc-tls/cert.pem \
AM_DEV_ISSUE_TOKEN=1 REDIS_ADDR=127.0.0.1:6379 \
AM_EXTERNAL_URL=https://127.0.0.1:8443 \
AM_KUBECONFIG_CA_FILE=/tmp/kamc-tls/cert.pem \
OIDC_ISSUER=https://127.0.0.1:9100 OIDC_CLIENT_ID=kamc OIDC_CLIENT_SECRET=mock-secret \
OIDC_REDIRECT_URL=https://127.0.0.1:8443/auth/callback \
AM_GEOIP_DB=/tmp/kamc-geoip/GeoLite2-City-Test.mmdb \
./bin/kamc-platform
```

2. 集群接入（详见上一轮联调记录）：平台注册集群 → 签发 longterm Token → 目标集群建
   `k8s-am-system` SA/CRB/Token → 启动 Agent：

```bash
PLATFORM_URL=ws://127.0.0.1:8080/tunnel CLUSTER_ID=<集群ID> \
TOKEN_FILE=/tmp/kamc-agent-token \
APISERVER_URL=https://<apiserver>:6443 \
SA_CA_FILE=/tmp/kamc-sa-ca.crt SA_TOKEN_FILE=/tmp/kamc-sa-token \
./bin/kamc-agent
```

## 1. exec 模式 kubeconfig（Device Flow）

```bash
# 1) 启动 mock OIDC（stdlib，仅本地；issuer 必须与平台 OIDC_ISSUER 完全一致）
cd hack/e2e/mock-oidc && go run .
# 固定账号：sub=alice，groups=[dev-team]

# 2) 编译插件并放进 PATH
go build -o /tmp/kamc-bin/k8s-am-login ./cmd/k8s-am-login

# 3) 下载 exec kubeconfig（需管理员 JWT）
curl -sk "https://127.0.0.1:8443/api/v1/clusters/<集群ID>/kubeconfig?mode=exec" \
  -H "Authorization: Bearer $ADMIN" -o /tmp/kc-exec.yaml

# 4) 使用（SSL_CERT_FILE 让插件信任自签平台证书；插件会打印设备码并自动完成轮询）
rm -rf ~/.config/kamc        # 清缓存，强制走一次设备流程
PATH=/tmp/kamc-bin:$PATH SSL_CERT_FILE=/tmp/kamc-tls/cert.pem \
  KUBECONFIG=/tmp/kc-exec.yaml kubectl get pods -n default
```

预期：stderr 打印 `设备码: KAMC-TEST`；插件经 `/auth/token-exchange` 换到平台 JWT；
audit 出现 `login-success alice /auth/token-exchange`。授权仍按 alice 的 Casbin 策略判定。

## 2. port-forward 与 SPDY 降级

```bash
# port-forward（kubectl 1.31+ 走 WebSocket+SPDY 隧道）
kubectl -n <ns> port-forward pod/<pod> 19000:9000

# 强制 SPDY（模拟旧版 kubectl，验证 Hijack 路径）
KUBECTL_REMOTE_COMMAND_WEBSOCKETS=false \
  kubectl -n <ns> exec <pod> -- sh -c 'echo spdy-ok'
```

> 注意：`captureWriter` 必须透传 `http.Hijacker`，否则 SPDY 升级报
> `can't switch protocols using non-Hijacker ResponseWriter type`，port-forward 直接 503。

## 3. GeoIP 富化

```bash
# 生成合成库（203.0.113.0/24=US/California，198.51.100.0/24=CN/Shanghai）
cd hack/e2e/testgeo && go run .
# 输出 /tmp/kamc-geoip/GeoLite2-City-Test.mmdb

# 平台带 AM_GEOIP_DB 重启后，带 XFF 请求一次
curl -sk -H "Authorization: Bearer $ADMIN" -H "X-Forwarded-For: 203.0.113.77" \
  "https://127.0.0.1:8443/proxy/<集群ID>/api/v1/namespaces/default/pods"
# 审计详情应出现 geo_country=US / California / Los Angeles
```

## 4. 尚未覆盖

- **集群内 Agent Deployment**：需要目标集群能从镜像仓库拉取 `kamc-agent`（仓库凭证或可达
  registry）。本地可先用进程方式验证隧道与数据面；镜像化部署走 `deploy/agent/manifests.yaml`。
- **LDAP 登录**：需要 LDAP 服务与 `LDAP_*` 配置。
- **exec 命令审计/录制**：明确不做（见 `docs/exec-command-audit-plan.md`）。
