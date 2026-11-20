# 观测栈（Prometheus + Loki + Promtail + Grafana）

数据流：

```text
platform /metrics ──scrape──▶ Prometheus ──▶ Grafana（指标面板）
platform 集群审计 JSONL ──tail──▶ Promtail ──push──▶ Loki ──▶ Grafana（日志面板）
```

平台应用日志（Zap JSON）输出到容器 stdout，由 Docker/K8s 日志驱动收集；
集群审计（kubectl 数据面事件）单独写文件，避免与进程日志混在一起。

## 本地一键启动

观测栈作为主 compose 的 `observability` profile，与平台共享 `clusterlog` 卷：

```bash
# 平台 + PG/Redis + 观测栈
docker compose -f deploy/docker/compose.yaml --profile observability up -d --build

# 需要 MinIO 时叠加 profile
docker compose -f deploy/docker/compose.yaml --profile observability --profile minio up -d --build
```

| 服务 | 地址 | 说明 |
| --- | --- | --- |
| Grafana | http://localhost:3000 | 默认 admin/admin（`GRAFANA_ADMIN_USER/PASSWORD` 覆盖） |
| Prometheus | http://localhost:9090 | 抓取 `platform:8080/metrics` |
| Loki | http://localhost:3100 | Promtail 推送目标 |
| Promtail | 内部 9080 | 读取 `/var/log/kamc/*.jsonl*` |

仪表盘会自动导入「kamc 平台概览」，包含 HTTP、Casbin、登录、隧道、审计、
头像与 Go 运行时面板，以及 Loki 审计日志面板。

## 目录

```text
deploy/observability/
├── prometheus/prometheus.yml              # 抓取配置
├── loki/loki-config.yaml                  # 单机文件系统存储 + 保留 168h
├── promtail/promtail-config.yaml          # 审计 JSONL 采集与标签提取
└── grafana/
    ├── provisioning/{datasources,dashboards}/
    └── dashboards/kamc-overview.json
```

## Kubernetes

- 平台 Helm Chart 支持 `promtail.enabled=true` 以 sidecar 方式采集同 Pod 的
  `/var/log/kamc/cluster-audit.jsonl`，并推送到 `promtail.lokiURL`；
- 也可使用 Promtail DaemonSet 读取共享卷（RWX）或 hostPath，路径匹配
  `/var/log/kamc/*.jsonl*`；
- Loki/Grafana/Prometheus 推荐使用各自官方 Helm Chart 或外部服务；
  Prometheus 抓取目标为平台 Service 的 `/metrics`（默认 8080）；
- 审计文件由平台按 `AM_AUDIT_LOG_MAX_MB`/`AM_AUDIT_LOG_BACKUPS` 轮转，
  Promtail 的 `*.jsonl*` 通配会继续采集轮转文件。

## 指标清单（/metrics）

| 指标 | 标签 | 说明 |
| --- | --- | --- |
| `kamc_http_requests_total` | method/route/status | 平台 HTTP 请求 |
| `kamc_http_request_duration_seconds` | method/route | 请求耗时直方图 |
| `kamc_http_requests_in_flight` | — | 处理中请求 |
| `kamc_http_response_bytes_total` | method/route | 响应字节 |
| `kamc_auth_attempts_total` | method/result | 登录尝试 |
| `kamc_jwt_auth_failures_total` | reason | JWT 校验失败 |
| `kamc_casbin_decisions_total` | cluster/result | allow/deny/error |
| `kamc_casbin_decision_duration_seconds` | cluster | 判定耗时 |
| `kamc_requests_total` | cluster/verb/code | 数据面代理请求 |
| `kamc_proxy_duration_seconds` | cluster | 数据面耗时 |
| `kamc_rate_limited_total` | — | 限流 |
| `kamc_tunnel_connected_clusters` / `kamc_tunnel_attached_agents` | — | 在线隧道 |
| `kamc_tunnel_attach_total` / `kamc_tunnel_detach_total` | cluster | 隧道事件 |
| `kamc_peer_relay_total` | owner | 跨副本转发 |
| `kamc_audit_events_total` | kind/result | 审计写入/落盘/丢弃/回放 |
| `kamc_audit_queue_depth` | — | 审计队列深度 |
| `kamc_audit_write_duration_seconds` | — | 审计写 PG 耗时 |
| `kamc_avatar_operations_total` | backend/operation/result | 头像操作 |
| `kamc_build_info` | version/commit | 构建信息 |
| `kamc_ready` | — | 数据库就绪状态 |

Go 运行时与进程指标由 Prometheus client 默认暴露（`go_*`、`process_*`）。

## 注意

- 示例 Grafana 默认密码仅用于本地，生产必须修改并启用 TLS；
- Loki 单机配置只适合本地/小规模；生产使用对象存储与多副本；
- Promtail 读取的审计文件包含用户/集群/请求摘要，注意访问控制；
- 应用日志为 Zap JSON（stdout），可另用 Promtail 的 Docker/K8s 日志采集，
  或在 `AM_AUDIT_LOG_FILE` 之外增加日志文件输出（当前未默认开启）。
