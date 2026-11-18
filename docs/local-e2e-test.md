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

## 4. 真实 Authentik SSO 接入

平台侧只需要三个参数：`OIDC_ISSUER`（Application 的 issuer URL）、`OIDC_CLIENT_ID`、
`OIDC_CLIENT_SECRET`，以及与之**逐字符一致**的 `OIDC_REDIRECT_URL`。

在 Authentik（2026.5 实测）创建 Provider 时注意两处默认值陷阱：

1. `redirect_uris` 不是字符串，而是 `RedirectURI(matching_mode=STRICT, url=..., redirect_uri_type=AUTHORIZATION)` 列表；
2. 新建 Provider 的 `grant_types` 默认是**空数组**，必须显式开启
   `authorization_code` / `refresh_token`（exec 模式还需 `urn:ietf:params:oauth:grant-type:device_code`），
   否则 authorize 直接返回 `invalid_request`。

```python
# kubectl -n authentik exec deploy/authentik-worker -- /lifecycle/ak shell -c "..."
from authentik.providers.oauth2.models import (
    OAuth2Provider, GrantType, RedirectURI, RedirectURIMatchingMode, RedirectURIType,
)
from authentik.core.models import Application
from authentik.crypto.models import CertificateKeyPair
from authentik.flows.models import Flow

provider = OAuth2Provider.objects.create(
    name="kamc",
    authorization_flow=Flow.objects.get(slug="default-provider-authorization-implicit-consent"),
    invalidation_flow=Flow.objects.get(slug="default-invalidation-flow"),
    client_type="confidential",
    client_id="kamc",
    client_secret="<secret>",
    redirect_uris=[RedirectURI(
        matching_mode=RedirectURIMatchingMode.STRICT,
        url="https://<platform>/auth/callback",
        redirect_uri_type=RedirectURIType.AUTHORIZATION,
    )],
    signing_key=CertificateKeyPair.objects.get(name="authentik Self-signed Certificate"),
)
provider.grant_types = [GrantType.AUTHORIZATION_CODE, GrantType.REFRESH_TOKEN, GrantType.DEVICE_CODE]
provider.save()
Application.objects.create(name="kamc", slug="kamc", provider=provider)
```

- issuer 形如 `https://sso.example.com/application/o/kamc/`（由 Application slug 决定）。
- `groups` 声明来自 Authentik 默认 `profile` scope（`request.user.groups`），平台请求
  `openid profile email groups`；组名与平台 `am:admin` 对齐后 SSO 用户即获得管理员权限。
- 平台用户名取 ID Token 的 `sub`（Authentik 中是哈希串），审计与策略主体均按该值记录。
- 本地浏览器联调建议用 `http://127.0.0.1:8080/auth/callback` 作为 redirect（并在 Provider 中登记），
  避免浏览器对自签 8443 证书的拦截；生产使用 https 域名证书即可。

## 5. 集群内 Agent 的网络前提

Agent 是**反向**拨出：目标集群的节点必须能访问平台入口（`wss://<platform>/tunnel`）。
本机开发时平台通常位于 Wi-Fi/NAT 之后，集群节点往往无法回连（表现为
`No route to host`/超时），此时：

- 继续用**本机进程方式**运行 Agent（本指南第 0 节），验证隧道与数据面；
- 或把平台部署进集群（`deploy/platform/manifests.yaml`），让 Agent 走集群内部 Service；
- 镜像可先推到目标集群可访问的 Harbor（如 `reg.cqupt.gay/kamc/k8s-am-agent:dev`），
  并在平台设置 `AM_AGENT_IMAGE_REPO`，一键安装清单会使用该仓库。


## 6. 尚未覆盖

- **LDAP 登录**：需要 LDAP 服务与 `LDAP_*` 配置。
- **exec 命令审计/录制**：明确不做（见 `docs/exec-command-audit-plan.md`）。
