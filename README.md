# kamc
kubernetes Access Manager Center

基于 Go + Casbin + Authentik SSO 的多集群 Kubernetes 访问管理平台：
用户经 SSO 登录后用标准 kubectl 访问任意纳管集群，权限由平台统一控制，操作全程可审计。

- 设计文档：[docs/design.md](docs/design.md)
- 项目指南（AI/新成员必读）：[AGENT.md](AGENT.md)

## 快速开始

```bash
make build   # 产出 bin/kamc-platform、bin/kamc-agent
make test
```

本地运行平台（需要 PostgreSQL）：

```bash
export DATABASE_URL="host=127.0.0.1 port=5432 user=postgres password=postgres dbname=kamc sslmode=disable"
./bin/kamc-platform
curl http://127.0.0.1:8080/healthz
```

## 前端

管理前端在 [web/](web/README.md)，技术栈 React 18 + Ant Design 5 + TypeScript + Vite：

```bash
cd web
npm install
KAMC_API=http://127.0.0.1:8080 npm run dev   # http://localhost:5173
npm run build                                # 产出 web/dist/
```

开发登录可使用内置账号 `sysadmin / sysadmin123`（`AM_PASSWORD_LOGIN_USER` /
`AM_PASSWORD_LOGIN_PASSWORD` 可覆盖，生产必须修改）；生产使用 Authentik SSO。
前端必须与平台同源部署（平台无 CORS），生产反代配置见 web/README.md。

## 部署

见 [deploy/README.md](deploy/README.md)：Nginx 接入层、平台/Agent manifests、
部署顺序、上线前人工验收清单。

## 许可

GPL-3.0，见 [LICENSE](LICENSE)。
