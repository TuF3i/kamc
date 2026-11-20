# kamc 文档

kamc（Kubernetes Access Manager Center）是基于 Go + Casbin + Authentik SSO 的
多集群 Kubernetes 访问管理平台。本目录面向使用者、运维与二次开发者。

## 文档地图

| 文档 | 内容 | 适合谁 |
| --- | --- | --- |
| [architecture.md](architecture.md) | 组件、认证、授权、隧道、审计与高可用设计 | 所有人（先读） |
| [deployment.md](deployment.md) | Docker Compose、原生 K8s、Helm 三种部署方式 | 运维 |
| [configuration.md](configuration.md) | 全部配置项、环境变量、生产护栏、MinIO | 运维、开发 |
| [operations.md](operations.md) | 日常运维、备份、轮换、故障排查、验收 | 运维 |
| [development.md](development.md) | 本地开发、just/make、测试、代码约定 | 开发者 |
| [security.md](security.md) | 信任边界、威胁模型、上线安全核对表 | 安全、运维 |
| [design.md](design.md) | v2.0 设计决策记录（历史，部分内容已演进） | 架构参考 |
| [../api/README.md](../api/README.md) | OpenAPI 使用与更新流程 | 前端、集成方 |
| [../web/README.md](../web/README.md) | 前端开发、页面与 API 约定 | 前端 |

## 推荐阅读顺序

1. 新同学：architecture → development → api/README
2. 部署上线：architecture → deployment → configuration → operations → security
3. 排障：operations 的「故障模式」→ configuration 的「生产护栏」→ architecture 的「请求链路」

## 文档约定

- 中文正文，代码/字段/命令保持英文。
- 架构与流程优先用 `text` 代码块画图，便于终端阅读。
- 代码是唯一事实来源；文档描述与代码不一致时，以代码为准并提 issue/PR 修正。
- 修改 API、配置、部署产物时，同步更新对应文档：
  - 新环境变量：configuration.md、AGENT.md、config/kamc.example.yaml
  - 新接口：api/openapi.yaml + api/README.md + web/src/api
  - 新部署参数：deployment.md、deploy/README.md、Helm values
- `design.md` 保留为 v2.0 设计记录，不再逐条同步；当前行为看
  architecture/configuration/operations。

## 快速索引

- 本地一键环境：`docs/deployment.md` 的「Docker Compose」
- 生产 Helm：`deploy/helm/README.md`
- 全部命令：`just --list`（Makefile 保留兼容）
- 认证/授权模型：`docs/architecture.md` 的「认证」「授权」
- MinIO 头像：`docs/configuration.md` 的「头像存储」
