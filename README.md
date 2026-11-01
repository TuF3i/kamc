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

## 许可

GPL-3.0，见 [LICENSE](LICENSE)。
