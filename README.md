# kamc
kubernetes Access Manager Center

基于 Go + Casbin + Authentik SSO 的多集群 Kubernetes 访问管理平台：
用户经 SSO 登录后用标准 kubectl 访问任意纳管集群，权限由平台统一控制，操作全程可审计。

- 文档索引：[docs/README.md](docs/README.md)（架构、部署、配置、开发、运维、安全）
- 项目指南（AI/新成员必读）：[AGENT.md](AGENT.md)
- API 规范：[api/openapi.yaml](api/openapi.yaml)、[api/README.md](api/README.md)

## 快速开始

```bash
just build   # 仅后端：bin/kamc-platform、bin/kamc-agent、bin/kamc
just release # 生产构建：先构建前端（data/web），再编译后端
just test
# Makefile 保留兼容：make build / make release / make test
```

`just release` 构建前端产物到 `data/web/`；后端运行时从 `AM_WEB_DIR`
（默认 `data/web`）读取并托管页面与 API，同一进程启动即可访问管理前端，
无需额外静态服务器。用户头像默认保存到 `AM_AVATAR_DIR`（默认 `data/avatar`），
配置 MinIO 后自动切换对象存储（见 [docs/configuration.md](docs/configuration.md)）。
`VERSION=v1.0.0 just release` 同时设定版本号（Agent 镜像 tag 使用同一版本）。

本地运行平台（需要 PostgreSQL）：

```bash
export DATABASE_URL="host=127.0.0.1 port=5432 user=postgres password=postgres dbname=kamc sslmode=disable"
./bin/kamc-platform
curl http://127.0.0.1:8080/healthz
```

## 容器镜像

```bash
make image-platform VERSION=v1.0.0   # kamc-platform:v1.0.0（含 data/web）
make image-agent    VERSION=v1.0.0   # kamc-agent:v1.0.0
```

镜像构建需要本机 Docker；`deploy/` 清单中的 `__IMAGE__` 可直接使用上述 tag。

## 前端

管理前端在 [web/](web/README.md)，技术栈 SolidJS 1.9 + Tailwind CSS 4 + TypeScript + Vite：

```bash
cd web
npm install
npm run dev        # 代理 127.0.0.1:8080，http://localhost:5174
npm run dev:mock   # mock 模式（无需后端）
npm run build      # 产出 data/web/
# 根目录也可：just dev-web / just dev-web-mock / just web
```

开发登录可使用内置账号 `sysadmin / sysadmin123`（`AM_PASSWORD_LOGIN_USER` /
`AM_PASSWORD_LOGIN_PASSWORD` 可覆盖）；生产清单默认 `AM_PASSWORD_LOGIN=0` 并禁止
默认凭据启动，生产使用 Authentik SSO。
前端必须与平台同源部署（平台无 CORS），生产反代配置见 web/README.md。

## 部署

三种方式（本地 Compose、原生 K8s、Helm）见 [docs/deployment.md](docs/deployment.md)；
目录与占位符说明见 [deploy/README.md](deploy/README.md)。生产推荐
`deploy/helm`，本地一键环境用 `deploy/docker`。
观测栈（Prometheus + Loki + Promtail + Grafana）见
[deploy/observability/README.md](deploy/observability/README.md)。

## 许可

GPL-3.0，见 [LICENSE](LICENSE)。
