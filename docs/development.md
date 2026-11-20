# 开发指南

## 一、环境要求

| 工具 | 版本 | 说明 |
| --- | --- | --- |
| Go | 1.26+ | 安装于 `/usr/local/go/bin`（just/Makefile 已补 PATH） |
| Node.js | 20+（推荐 22/24） | 前端构建 |
| npm | 10+ | 使用 lockfile |
| PostgreSQL | 14+ | 本地可用容器 |
| Redis | 6+ | 可选；多副本/限流/隧道需要 |
| just | 1.x | 日常命令入口（Makefile 保留） |
| Helm / kubectl / Docker | 可选 | 部署与集成验证 |

## 二、命令入口

```bash
just --list          # 查看全部命令
just build           # bin/kamc-platform、bin/kamc-agent、bin/kamc
just release         # 前端 data/web + 后端
just test            # go test -count=1 ./...
just vet             # go vet ./...
just fmt             # gofmt -w
just fmt-check       # 检查格式（不写入）
just tidy            # go mod tidy
just dev-web         # 前端连接 127.0.0.1:8080
just dev-web-mock    # 前端 mock 模式（5174，无需后端）
just compose-up      # 本地全栈（deploy/docker）
just helm-lint       # 转发 Helm Makefile
```

等价 Makefile 目标全部保留：`make build`、`make release`、`make test`、
`make helm-*` 等。

## 三、本地后端

### 1. 准备 PostgreSQL

```bash
docker run -d --name kamc-pg \
  -e POSTGRES_USER=kamc -e POSTGRES_PASSWORD=kamc -e POSTGRES_DB=kamc \
  -p 127.0.0.1:5432:5432 postgres:16-alpine

# 集成测试建议单独建库，避免污染开发数据
docker exec kamc-pg createdb -U kamc kamc_test
```

Redis 可选：

```bash
docker run -d --name kamc-redis -p 127.0.0.1:6379:6379 redis:7-alpine
```

### 2. 启动平台

```bash
export DATABASE_URL='postgres://kamc:kamc@127.0.0.1:5432/kamc?sslmode=disable'
export AM_ENV=development
export AM_PASSWORD_LOGIN=1
export AM_AVATAR_DIR=data/avatar
./bin/kamc-platform
```

- 默认 `:8080`；启动时 AutoMigrate 建表；
- 未配置 `JWT_KEYRING_DIR` 会用临时密钥，重启后需重新登录；
- 未配置 `AM_AVATAR_SECRET` 会派生开发密钥并告警；
- 前端产物 `data/web` 不存在时只提供 API。

### 3. 运行 Agent（可选）

```bash
PLATFORM_URL=ws://127.0.0.1:8080/tunnel \
CLUSTER_ID=dev \
TOKEN_FILE=/tmp/agent-token \
PLATFORM_INSECURE=1 \
./bin/kamc-agent
```

Token 从平台「集群 → Agent Token」签发后写入文件。

## 四、本地前端

```bash
cd web
npm install
npm run dev        # release 模式，代理到 127.0.0.1:8080，端口 5174
npm run dev:mock   # mock 模式，内置数据（admin/alice 可切换）
npm run build      # 产物 data/web
```

- `KAMC_API` 可改代理目标：`KAMC_API=http://127.0.0.1:8080 npm run dev`；
- 前端使用 hash 路由（`/#/...`），与后端桥接页约定一致；
- 修改前端后若 8080 由平台托管页面，需要 `just web` 重新构建；
- 修改后端接口时同步 `web/src/api/types.ts` 与 mock 数据。

## 五、日志与观测

- 应用日志：Zap JSON（默认）→ stdout；本地调试可 `AM_LOG_FORMAT=console`，
  级别 `AM_LOG_LEVEL=debug`；
- 访问日志：结构化字段（method/path/status/latency/request_id）；
- 集群审计：`AM_AUDIT_LOG_FILE`（默认 `data/cluster-audit.jsonl`）JSONL，
  自动按大小轮转；
- 指标：`curl -s localhost:8080/metrics | grep kamc_`；
- 观测栈：`just compose-up-observability` 或
  `docker compose -f deploy/docker/compose.yaml --profile observability up -d`，
  见 [deploy/observability/README.md](../deploy/observability/README.md)。

## 六、测试
### 单元测试

```bash
just test
# 或
go test -count=1 ./...
```

### 数据库集成测试

```bash
TEST_PG_DSN='postgres://kamc:kamc@127.0.0.1:5432/kamc_test?sslmode=disable' \
  go test -count=1 ./internal/platform/...
```

未设置 `TEST_PG_DSN` 时相关用例 `t.Skip`。测试会创建/清理自身数据；建议使用
独立库。

### Redis 集成测试

```bash
TEST_REDIS_ADDR=127.0.0.1:6379 go test -count=1 ./internal/tunnel/... ./internal/platform/middleware/...
```

### MinIO 集成测试

```bash
docker run -d --name kamc-minio -p 19000:9000 --user 0 \
  -e MINIO_ROOT_USER=minioadmin -e MINIO_ROOT_PASSWORD=minioadmin \
  cgr.dev/chainguard/minio server /data --console-address :9001

TEST_MINIO_ENDPOINT=127.0.0.1:19000 \
TEST_MINIO_ACCESS_KEY=minioadmin \
TEST_MINIO_SECRET_KEY=minioadmin \
  go test -count=1 ./internal/platform/avatarstore -run TestMinIOIntegration -v
```

### 部署产物校验

```bash
go test ./deploy          # YAML 可解析与关键字段
just compose-config       # docker compose 配置
just helm-lint            # 两个 Chart
npx --yes @redocly/cli@latest lint api/openapi.yaml
```

## 七、代码结构约定

```text
cmd/                入口（kamc-platform / kamc-agent / kamc）
internal/
  platform/         平台模块
    handler/        参数绑定与错误映射（不写业务）
    service/        业务逻辑
    dao/            gorm 访问
    modules/        模型/DTO（叶包）
    middleware/     认证/鉴权/限流/日志/指标
    audit/          审计写入、文件出口与回放
    avatarstore/    头像对象存储（local/minio）
    webstatic/      前端静态托管
  metrics/          Prometheus 指标定义
  logging/          Zap 初始化（JSON/Console、级别）
  agent/            Agent 模块
  tunnel/           隧道管理器与注册表
  auth/             JWT 服务
  login/            exec 插件登录缓存
web/                SolidJS 前端
deploy/             helm / k8s / docker / observability / extra
docs/               文档
api/                OpenAPI
```

约定摘要：

- 单向依赖 `handler → service → dao → db`；`modules`/`errcode` 是叶包；
- 每个模块一个 `runtime.go` 负责组装与生命周期；
- 模块边界用 Functional Options（`WithXxx`），内部用构造函数注入；
- 业务错误用 `errcode.Response` 元组上抛，handler 统一出口；
- 非业务错误用 sentinel + `%w` 包装；
- 注释中文，导出符号必须有注释。

## 八、新增一个 API 的步骤

1. `internal/platform/modules` 定义模型/DTO（如需要）；
2. `dao` 增加读写方法，`service` 实现业务；
3. `handler` 绑定参数、调用 service、统一 `errcode.GenFinalResponse`；
4. `handler/handler.go` 在正确分组注册路由（自服务/管理面）；
5. 更新 `api/openapi.yaml` 与 `web/src/api`；
6. 增加单测/集成测试，`just test` 通过；
7. 涉及权限的接口确认 Casbin 行为（数据面资源级 vs 管理面组）。

## 九、常见问题

| 现象 | 原因/处理 |
| --- | --- |
| `502`/`503` 登录失败 | OIDC 未配置或回调地址不一致；检查 `OIDC_REDIRECT_URL` 与访问源 |
| 重启后全部掉登录 | 未配置 `JWT_KEYRING_DIR`，临时密钥变化 |
| 修改密码后环境变量密码仍可登录 | 本地哈希已写入；有哈希时不再回退内置密码（预期） |
| `kubectl` 不发送 Bearer Token | 平台必须 HTTPS；明文连接 client-go 不加载凭证 |
| exec/port-forward 失败 | Nginx 未透传 Upgrade/长超时；检查 `/proxy` 配置 |
| 前端 504 Outdated Optimize Dep | `rm -rf web/node_modules/.vite` 后 `npm run dev -- --force` |
| 8080 页面仍旧 | 构建产物在 `data/web`；改前端后 `just web` 并刷新 |
| 多副本头像互不可见 | 启用 MinIO 或挂载 RWX 卷 |
| 普通用户 discovery 403 | 策略需包含 nonresource 路径（`/api`、`/apis` 等） |

## 十、提交前检查

```bash
just fmt-check
just vet
just test
just web            # 前端类型检查 + 构建
go test ./deploy
```

涉及 UI 的改动建议在 mock 与真实后端各走一遍管理员/普通用户流程。
