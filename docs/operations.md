# 运维手册

## 一、健康与观测

| 端点 | 用途 | 说明 |
| --- | --- | --- |
| `GET /healthz` | 存活 | 进程存活即 200，不检查依赖 |
| `GET /readyz` | 就绪 | 数据库不可达返回 503，LB/编排据此摘流 |
| `GET /version` | 版本 | 版本、commit、组件名 |
| `GET /metrics` | Prometheus | 平台指标；Nginx 默认拒绝外部访问 |

建议监控：

- 平台 Pod 重启次数、`readyz` 失败、HTTP 5xx 比例；
- 隧道在线数（概览页「在线隧道」/指标）；
- 审计写入失败与 spill 文件增长；
- PostgreSQL 连接数/慢查询、Redis 内存与延迟；
- MinIO bucket 容量与错误率（启用时）。

## 二、日志

- 平台日志带 `[模块]` 前缀：`[platform]`、`[tunnel]`、`[postgres]`、`[redis]`；
- 请求日志由 chi 中间件输出：方法、路径、状态、耗时、request_id；
- 敏感值（DSN 密码、OIDC secret、MinIO key）不会打印；
- Kubernetes：`kubectl -n k8s-am-system logs -l app.kubernetes.io/instance=kamc`；
- Compose：`docker compose -f deploy/docker/compose.yaml logs -f platform`。

启动日志会明确输出：监听地址、web 目录、头像后端（local/minio 与 endpoint/bucket）、
Redis 是否启用、LDAP 是否启用、syslog 是否启用、审计保留期。

## 三、审计

| 项 | 说明 |
| --- | --- |
| 存储 | PostgreSQL `audit_events`，异步批量写入 |
| PG 故障 | 写入 `AM_AUDIT_SPILL_FILE`，恢复后回放 |
| syslog | `AM_SYSLOG_ADDR` 可导出到外部 SIEM |
| 保留 | `AM_AUDIT_RETENTION`（默认 720h，每小时清理） |
| 查询 | 管理面 `/api/v1/audit`，列表可 `summary=1` 省流量 |
| 详情 | `GET /api/v1/audit/{id}` 含请求/响应摘要 |

排障顺序：先看审计页筛选失败事件（`allowed=false`），再对照平台日志的
request_id。

## 四、备份与恢复

### PostgreSQL

```bash
pg_dump "$DATABASE_URL" -Fc -f kamc-$(date +%F).dump
pg_restore -d "$DATABASE_URL" --clean --if-exists kamc-$(date +%F).dump
```

备份包含：用户/组、集群、Token 哈希、策略、机器人、审计、映射脚本。

### 头像

- 本地模式：备份 `AM_AVATAR_DIR`（或 RWX 卷快照）；
- MinIO：使用 `mc mirror` 或 bucket 版本/复制策略；
- `avatar_url` 存于数据库，恢复时与对象存储需配套。

### JWT 密钥环

- `JWT_KEYRING_DIR` 内的 `*.pem` 必须纳入 Secret 备份；
- 丢失后所有存量 Token 失效（用户需重新登录），但不会影响数据。

### Redis

Redis 承载的是可重建状态（策略缓存、吊销 epoch/黑名单、注册表、限流窗口）。
丢失后：

- 策略缓存会在下次变更/启动时重载；
- 吊销黑名单丢失 → 已登出 Token 可能恢复可用直到过期（安全影响，需评估）；
- 建议开启 AOF 并纳入备份。

## 五、密钥与凭据轮换

| 对象 | 步骤 |
| --- | --- |
| JWT 签名密钥 | 新增 `jwt-2.pem` 到 Secret → 滚动重启（JWKS 同时暴露新旧）→ 存量过期后删除旧文件 |
| Agent Token | 平台重新签发 → 旧 Token 24h 宽限 → 更新 Agent Secret（滚动） |
| MinIO 凭据 | 更新 Secret 中 `minio-access-key/secret-key` → 滚动重启平台 |
| 头像签名密钥 | 修改 `AM_AVATAR_SECRET` 会使已入库 URL 失效；需同时刷新 `avatar_url`（重新上传或批量重签） |
| 内置管理员密码 | 登录个人中心修改（bcrypt 落库，旧 JWT 吊销）；或更新 `AM_PASSWORD_LOGIN_PASSWORD` 后重启（仅未落库时生效） |
| 数据库密码 | 更新 PG 与 Secret → 滚动重启平台 |

## 六、扩缩容

- 平台：无状态（会话在 JWT），可水平扩容；多副本需 Redis + `PEER_ADDRESSES`；
- Agent：每集群建议 2 副本，反亲和；扩容无需改平台；
- 隧道容量：单 Agent 连接对应一个 yamux 会话，请求按会话分发；
- 审计写入是异步批量，扩容前关注 PG 写入能力；
- 头像：本地模式多副本必须共享存储，推荐 MinIO。

## 七、故障模式

| 故障 | 影响 | 处理 |
| --- | --- | --- |
| 单个平台 Pod 挂 | 隧道迁移到其他副本（<30s）；存量 exec 断开 | 等待重连/重启 Pod |
| 平台滚动升级 | 控制面短暂重连；exec 会话断开 | 预期行为，选低峰执行 |
| Redis 不可用 | 存量 kubectl 鉴权不受影响；策略同步/吊销跨副本暂停；限流退化 | 恢复 Redis；必要时重启副本 |
| PostgreSQL 不可用 | kubectl 数据面不受影响；管理面写操作失败；审计写 spill | 恢复 PG，确认 spill 回放 |
| Authentik 不可用 | 新登录失败；存量 JWT 有效期内可用 | 恢复 IdP |
| Agent 单副本故障 | 请求切到另一隧道 | 重启 Agent |
| 全部 Agent 故障 | 该集群 `offline`，kubectl 503 | 检查 Token/网络/平台 URL |
| Nginx 故障 | 入口不可用 | VIP 漂移/切流 |
| MinIO 不可用 | 头像上传/读取失败（其余功能正常） | 恢复对象存储；检查凭据/网络 |
| 审计 spill 磁盘满 | 审计可能丢事件 | 清理/扩容，恢复 PG 写入 |

## 八、用户与权限运维

- **踢出用户**：`POST /api/v1/auth/revoke {"sub":"username"}`，5s 内全副本生效；
- **删除用户**：管理面删除会吊销 Token 并清理头像对象；
- **组管理员**：组 `is_admin=true` 的成员登录后进入管理组；数据面仍需策略；
- **策略排障**：确认主体类型（user/org/robot）、集群 domain、namespace 展开规则；
  普通用户 discovery 403 通常是缺少 nonresource 策略；
- **机器人**：删除会吊销 JWT 并清理 `robot:<id>` 策略；轮换需重新签发 kubeconfig。

## 九、升级与回滚

1. 备份 PG、JWT keyring、头像；
2. 执行需要的 `deploy/extra/migrations/*.sql`；
3. 更新镜像并滚动（Helm `helm upgrade` / `kubectl set image`）；
4. 观察 `readyz`、隧道在线数、审计写入；
5. 回滚：`helm rollback` 或改回旧镜像；数据库变更需人工评估。

Agent 升级可与平台独立；Token 不变时隧道自动重连。

## 十、容量参考

| 资源 | 说明 |
| --- | --- |
| 平台 CPU/内存 | 默认 requests 100m/128Mi，limits 500m/512Mi；按 QPS 调整 |
| PG | 审计增长最快；按 `AM_AUDIT_RETENTION` 估算 |
| Redis | 吊销/策略/注册表，通常很小；限流 ZSET 与用户数相关 |
| 头像 | 单文件 ≤2MB；按用户数 × 平均大小估算 |
| 隧道 | 每集群 2 条；并发受 Agent 与 apiserver 限制 |

## 十一、日常检查清单

- [ ] 平台所有副本 `readyz` 正常，无频繁重启
- [ ] 集群状态无长期 `offline`
- [ ] 审计无持续写入失败，spill 文件未增长
- [ ] Redis/PG 磁盘与连接数正常
- [ ] 证书（Nginx/MinIO）未临近过期
- [ ] JWT keyring 轮换计划与 Token 过期时间匹配
- [ ] 备份任务成功，恢复演练定期执行
- [ ] 生产未启用默认密码与 dev-token
