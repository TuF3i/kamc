# 配置参考

kamc 使用 Viper 装载配置，优先级：

```text
环境变量 > 配置文件 > 内置默认值
```

配置文件通过 `-config=/path/to/kamc.yaml` 或 `AM_CONFIG_FILE` 指定；未指定时在
当前目录与 `/etc/kamc` 搜索 `kamc.yaml`（Agent 为 `kamc-agent.yaml`）。
示例文件：[config/kamc.example.yaml](../config/kamc.example.yaml)、
[config/kamc-agent.example.yaml](../config/kamc-agent.example.yaml)。

## 一、完整示例（平台）

```yaml
server:
  listen_addr: ":8080"
  env: production                  # production 触发安全护栏
  external_url: https://kamc.example.com

database:
  url: "postgres://kamc:***@postgres:5432/kamc?sslmode=require"

redis:
  addresses: [redis:6379]          # 或逗号分隔字符串；为空=单副本语义
  password: ""
  db: 0

jwt:
  issuer: https://kamc.example.com
  audience: kamc
  keyring_dir: /etc/k8s-am/jwt     # *.pem，文件名即 kid

auth:
  admin_group: am:admin
  password_login:
    enabled: true                  # 生产推荐 false，使用 OIDC/LDAP
    user: sysadmin
    password: ""
  dev_token: false                 # 生产禁止 true

oidc:
  issuer: https://sso.example.com/application/o/kamc/
  client_id: kamc
  client_secret: ""
  redirect_url: https://kamc.example.com/auth/callback

ldap:
  url: ""
  bind_dn: ""
  bind_password: ""
  base_dn: ""
  user_filter: "(uid=%s)"
  username_attr: uid
  email_attr: mail
  nickname_attr: cn
  group_filter: "(member=%s)"
  group_attr: cn
  insecure_skip_verify: false

tunnel:
  pod_id: ""                       # 空则用 hostname
  peer:
    addr: ":9090"
    addresses: ""                  # 多副本：pod=host:port,...
    token: ""
  ttl: 15s
  interval: 5s

audit:
  spill_file: /var/lib/kamc/audit-spill.jsonl
  retention: 720h                  # 0=不清理
  syslog:
    addr: ""                       # udp:// | tcp:// | tls://host:6514
    tag: kamc-cluster-audit
    ca_file: ""
    insecure: false

geoip:
  db: ""                           # MaxMind .mmdb

avatar:
  backend: auto                    # auto | local | minio
  dir: /data/avatar
  secret: ""                       # 生产必填
  minio:
    endpoint: ""                   # 如 minio:9000；auto 下非空即启用
    access_key: ""
    secret_key: ""
    bucket: kamc-avatars           # 不存在时启动自动创建
    use_ssl: false
    region: ""
    prefix: avatars/               # 对象键 <prefix><uid>

rate_limit:
  proxy: 600                       # 每用户每分钟数据面请求数，0=不限
  login: 30                        # 登录尝试/分钟/IP，0=关闭

robot:
  token_ttl: 8760h

kubeconfig:
  ca_file: ""                      # 嵌入 kubeconfig 的平台 CA

agent:
  image_repo: tuf3i/k8s-am-agent   # 不含 tag，tag 随平台版本

web:
  dir: data/web                    # 前端产物目录
```

## 二、环境变量

### 数据库与缓存

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `DATABASE_URL` | 本地 kamc 库 | PostgreSQL DSN；生产必填 |
| `REDIS_ADDR` / `REDIS_ADDRESSES` | — | 单地址 / 多地址（逗号分隔）；未配置=隧道模块不启用、单副本语义 |
| `REDIS_PASSWORD` / `REDIS_DB` | 空 / 0 | Redis 认证 |

### 平台与 JWT

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `AM_LISTEN_ADDR` | `:8080` | HTTP 监听地址 |
| `AM_ENV` | `development` | `production` 触发生产护栏 |
| `AM_EXTERNAL_URL` | 由请求推导 | 对外地址（kubeconfig/安装清单） |
| `JWT_KEYRING_DIR` | 临时密钥 | 密钥环目录；未配置时重启后 Token 失效并告警 |
| `AM_JWT_ISSUER` / `AM_JWT_AUD` | `https://kamc.local` / `kamc` | JWT iss/aud |
| `AM_ADMIN_GROUP` | `am:admin` | 管理面用户组 |
| `AM_DEV_ISSUE_TOKEN` | 关闭 | `/auth/dev-token`；生产拒绝启动 |
| `AM_PASSWORD_LOGIN` | 启用 | 内置账号密码登录（`0` 关闭） |
| `AM_PASSWORD_LOGIN_USER` | `sysadmin` | 内置管理员账号 |
| `AM_PASSWORD_LOGIN_PASSWORD` | `sysadmin123` | 内置管理员密码（生产必须覆盖） |
| `AM_ROBOT_TOKEN_TTL` | `8760h` | 机器人 JWT 有效期 |
| `AM_RATE_LIMIT` | 0 | 数据面每用户每分钟请求数 |
| `AM_LOGIN_RATE_LIMIT` | 30 | 登录尝试/分钟/IP |

### OIDC / LDAP

| 变量 | 说明 |
| --- | --- |
| `OIDC_ISSUER` / `OIDC_CLIENT_ID` / `OIDC_CLIENT_SECRET` / `OIDC_REDIRECT_URL` | Authentik Provider；未配置则 `/auth/login` 503 |
| `LDAP_URL` / `LDAP_BIND_DN` / `LDAP_BIND_PASSWORD` / `LDAP_BASE_DN` | LDAP 连接与绑定 |
| `LDAP_USER_FILTER` / `LDAP_USERNAME_ATTR` / `LDAP_EMAIL_ATTR` / `LDAP_NICKNAME_ATTR` | 用户搜索 |
| `LDAP_GROUP_FILTER` / `LDAP_GROUP_ATTR` | 组搜索 |
| `LDAP_INSECURE_SKIP_VERIFY` | 跳过 LDAP TLS 校验（仅开发） |

### 隧道与副本

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `PEER_ADDRESSES` | — | 可拨副本列表（`pod=host:port` 或 `host:port`） |
| `PEER_TOKEN` | — | 副本间转发令牌（多副本必填） |
| `AM_PEER_ADDR` | `:9090` | peer 监听地址 |
| `POD_ID` | hostname | 隧道注册表实例标识 |
| `AM_TUNNEL_TTL` / `AM_TUNNEL_INTERVAL` | `15s` / `5s` | 隧道注册 TTL / 刷新间隔 |

### 审计

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `AM_AUDIT_SPILL_FILE` | `/tmp/kamc-audit-spill.jsonl` | PG 故障暂存文件 |
| `AM_AUDIT_RETENTION` | `720h` | 保留期，0=不清理 |
| `AM_SYSLOG_ADDR` | — | `udp://`、`tcp://`、`tls://host:6514` |
| `AM_SYSLOG_TAG` | `kamc-cluster-audit` | syslog APP-NAME |
| `AM_SYSLOG_CA_FILE` | 系统根证书 | TLS CA |
| `AM_SYSLOG_INSECURE` | off | TLS 跳过校验（仅开发） |
| `AM_GEOIP_DB` | — | MaxMind `.mmdb`，未配置不解析 |

### 头像与前端

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `AM_AVATAR_BACKEND` | `auto` | `auto/local/minio` |
| `AM_AVATAR_DIR` | `data/avatar` | 本地头像目录 |
| `AM_AVATAR_SECRET` | 派生 | URL 签名密钥；生产必填 |
| `AM_AVATAR_MINIO_ENDPOINT` | — | 如 `minio:9000`；auto 下非空即启用 MinIO |
| `AM_AVATAR_MINIO_ACCESS_KEY` | — | 访问密钥（启用 MinIO 必填） |
| `AM_AVATAR_MINIO_SECRET_KEY` | — | 私钥（启用 MinIO 必填） |
| `AM_AVATAR_MINIO_BUCKET` | `kamc-avatars` | bucket，不存在时自动创建 |
| `AM_AVATAR_MINIO_USE_SSL` | `false` | 是否使用 HTTPS |
| `AM_AVATAR_MINIO_REGION` | — | S3 region（MinIO 可空） |
| `AM_AVATAR_MINIO_PREFIX` | `avatars/` | 对象键前缀 |
| `AM_WEB_DIR` | `data/web` | 前端产物目录 |
| `AM_AGENT_IMAGE_REPO` | `tuf3i/k8s-am-agent` | Agent 镜像仓库（不含 tag） |
| `AM_KUBECONFIG_CA_FILE` | — | 嵌入 kubeconfig 的 CA |

### Agent

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `PLATFORM_URL` | 必填 | 如 `wss://kamc.example.com/tunnel` |
| `CLUSTER_ID` | 必填 | Casbin domain |
| `AGENT_ID` | hostname | 实例标识（双副本须不同） |
| `TOKEN_FILE` | `/etc/k8s-am/token/token` | Agent Token 文件 |
| `HEARTBEAT_PING_INTERVAL` | `20s` | 心跳间隔 |
| `APISERVER_URL` | 集群内探测 | 上游 apiserver 覆盖 |
| `PLATFORM_INSECURE` | — | 跳过平台证书校验（仅开发） |
| `SA_TOKEN_FILE` | — | 指定 ServiceAccount Token 路径 |
| `BACKOFF_BASE` / `BACKOFF_MAX` | `1s` / `30s` | 重连退避区间 |

### 测试与前端开发

| 变量 | 说明 |
| --- | --- |
| `TEST_PG_DSN` | 数据库集成测试门控；未设置则跳过 |
| `TEST_REDIS_ADDR` | Redis 集成测试门控 |
| `TEST_MINIO_ENDPOINT` / `TEST_MINIO_ACCESS_KEY` / `TEST_MINIO_SECRET_KEY` / `TEST_MINIO_BUCKET` | MinIO 集成测试门控 |
| `KAMC_API` | Vite 开发代理目标（默认 `http://127.0.0.1:8080`） |

## 三、生产护栏

`AM_ENV=production` 时平台启动阶段强制：

- `DATABASE_URL` 必填；
- `AM_AVATAR_SECRET` 必填（不派生开发密钥）；
- `AM_DEV_ISSUE_TOKEN` 必须关闭（否则拒绝启动）；
- 内置密码登录若启用且仍是 `sysadmin/sysadmin123`，拒绝启动；
- `AM_PASSWORD_LOGIN` 启用但账号/密码为空，拒绝启动。

此外：

- `JWT_KEYRING_DIR` 未配置会使用临时密钥，重启后全部 Token 失效；
- Redis 未配置时吊销/策略同步只在单进程生效，生产必须配置；
- MinIO 模式缺少 endpoint/凭据时启动失败，不会静默回退本地。

## 四、头像存储

`avatar.backend`：

- `auto`（默认）：`avatar.minio.endpoint` 非空 → MinIO，否则本地；
- `local`：始终本地目录；
- `minio`：强制对象存储，缺少配置直接失败。

本地模式文件为 `<AM_AVATAR_DIR>/<uid>`（0600）；MinIO 模式对象为
`<prefix><uid>`，Content-Type 随对象保存。浏览器始终访问
`/avatars/{uid}?token=...`，切换后端不影响前端/数据库。

从本地迁移到 MinIO（一次性）：

```bash
mc alias set kamc https://minio.example.com "$ACCESS" "$SECRET"
mc mb --ignore-existing kamc/kamc-avatars
mc mirror --overwrite /data/avatar kamc/kamc-avatars/avatars
```

`avatar_url` 无需修改（仍是平台签名 URL）。旧对象未迁移时读取会 404。

## 五、典型配置

### 本地开发

```bash
AM_ENV=development
DATABASE_URL=postgres://kamc:kamc@127.0.0.1:5432/kamc?sslmode=disable
AM_PASSWORD_LOGIN=1
AM_PASSWORD_LOGIN_USER=sysadmin
AM_PASSWORD_LOGIN_PASSWORD=sysadmin123
AM_AVATAR_DIR=data/avatar
```

### 生产（OIDC + MinIO）

```bash
AM_ENV=production
AM_EXTERNAL_URL=https://kamc.example.com
DATABASE_URL=postgres://...?sslmode=require
REDIS_ADDRESSES=redis-0:6379,redis-1:6379
JWT_KEYRING_DIR=/etc/k8s-am/jwt
OIDC_ISSUER=https://sso.example.com/application/o/kamc/
OIDC_CLIENT_ID=kamc
OIDC_CLIENT_SECRET=***
OIDC_REDIRECT_URL=https://kamc.example.com/auth/callback
AM_PASSWORD_LOGIN=0
AM_AVATAR_SECRET=***
AM_AVATAR_BACKEND=minio
AM_AVATAR_MINIO_ENDPOINT=minio.storage:9000
AM_AVATAR_MINIO_BUCKET=kamc-avatars
AM_AVATAR_MINIO_USE_SSL=true
AM_AVATAR_MINIO_ACCESS_KEY=***
AM_AVATAR_MINIO_SECRET_KEY=***
AM_AUDIT_RETENTION=2160h
```

## 六、排查配置是否生效

- 启动日志会打印监听地址、web 目录、头像后端（不含密钥）、Redis/LDAP 状态；
- `GET /version` 返回版本与 commit；
- `GET /api/v1/platform/info`（管理员）返回脱敏后的 DSN/Redis/Agent 镜像等；
- 配置错误大多在启动阶段直接失败，优先看日志第一条 `[platform] ...` 错误。
