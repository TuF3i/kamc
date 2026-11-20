# 安全模型

## 一、信任边界

```text
[公网] 浏览器 / kubectl
   │  TLS（Nginx）
   ▼
[平台集群] kamc-platform ── JWT / Casbin / 审计
   │  反向隧道（Agent 主动拨出，平台不主动连集群）
   ▼
[目标集群] kamc-agent ── ServiceAccount ── kube-apiserver
```

- 平台是唯一鉴权点；目标集群不信任用户身份，也不做 Impersonation；
- Agent 只持有本集群凭证，不持有平台 JWT；
- PostgreSQL/Redis/MinIO 属平台内网，必须与公网隔离；
- Nginx 是唯一公网入口，`/metrics` 与 peer 端口不对外。

## 二、认证安全

| 项 | 措施 |
| --- | --- |
| OIDC | 授权码 + state（Redis 外置，多副本）；回调只接受校验通过的 code |
| JWT | RS256、kid 密钥环、JWKS；iss/aud/exp/nbf 校验 |
| 密码 | bcrypt 落库；登录失败不区分账号是否存在（防枚举）；恒定时间比较内置密码 |
| LDAP | 先 bind 再搜索；认证失败统一 401 |
| 登录限流 | `AM_LOGIN_RATE_LIMIT`（默认 30 次/分钟/IP） |
| 吊销 | 用户 epoch（`am_ep`）+ jti 黑名单；改密/踢出立即生效 |
| 开发直签 | `AM_DEV_ISSUE_TOKEN` 生产拒绝启动 |

注意：

- 平台 JWT 一旦泄露，在有效期内可访问该用户全部授权资源；生产应缩短有效期并
  依赖吊销机制；
- `AM_PASSWORD_LOGIN` 生产应关闭；若启用必须修改默认账号密码；
- 内置管理员改密后以库内哈希为准，环境变量密码不再生效。

## 三、授权安全

- 数据面完全由 Casbin 判定，管理员也需显式策略；
- 策略主体三类：`user:`、`org:`、`robot:`；用户策略优先并遮蔽组策略；
- namespace 展开由后端完成，前端预览只是展示；
- nonresource 路径单独授权（discovery 必需），不要用通配 `*` 覆盖 `/api` 全部；
- 机器人是独立主体，不应授予超出 CI 所需权限；
- 策略变更通过 Redis 广播，5s 内全副本生效；变更后建议踢出相关用户以刷新
  Token 中的组/epoch 信息。

## 四、数据面与隧道

- 平台在转发前剥离用户提供的 `Impersonate-*` 与 `Authorization` 头，
  防止伪造上游身份；
- Agent 以 ServiceAccount 访问 apiserver，其 RBAC 是平台能力上限；生产建议用
  最小 ClusterRole 替代 `cluster-admin`；
- `/proxy` 仅接受平台 JWT；浏览器 WebSocket 的 `?access_token` 只对 WS 升级生效；
- 限流 `AM_RATE_LIMIT` 按用户滑动窗口，Redis 共享；故障时 fail-open（可用性优先）；
- 审计记录 allow/deny 与请求摘要；exec/port-forward 的允许事件当前不落库，
  仅拒绝事件有审计（已知取舍）。

## 五、密钥与 Secret

| 密钥 | 存放 | 轮换影响 |
| --- | --- | --- |
| JWT 私钥（`jwt-*.pem`） | Secret 挂载 | 新增 kid 可零停机；删除旧 key 使旧 Token 失效 |
| `AM_AVATAR_SECRET` | Secret | 变更使已入库头像 URL 全部失效 |
| `PEER_TOKEN` | Secret | 多副本滚动重启期间短暂不匹配 |
| OIDC client secret | Secret | 重启后生效 |
| MinIO access/secret key | Secret | 重启后生效；建议专用只读写该 bucket 的账号 |
| 数据库密码 | Secret | 滚动重启 |

- 不要把 Secret 写入 values.yaml、命令行历史或镜像；
- Helm `secrets.existingSecret` 优先；Chart 自建 Secret 仅用于 POC；
- MinIO 凭据仅授予 `kamc-avatars` bucket 的读写，禁止 admin 权限。

## 六、头像存储安全

- 上传限制 2MB，Content-Type 白名单 + 文件头探测；
- 对象键使用平台 UID（UUID 风格校验），防止路径/键注入；
- 对外 URL 为 HMAC 签名，token 无过期但不可伪造；URL 可能出现在代理日志，
  属于已知风险；
- 本地文件 0600；MinIO bucket 私有，平台代理读取，不暴露对象地址；
- 删除用户时删除头像对象；管理员修改 `avatar_url` 为外部地址不会自动清理旧对象；
- SSO 登录的 `picture` 可能覆盖平台上传的头像 URL（预期行为，按部署策略评估）。

## 七、生产护栏

`AM_ENV=production` 启动时强制：

- `DATABASE_URL`、`AM_AVATAR_SECRET` 必填；
- `AM_DEV_ISSUE_TOKEN` 必须关闭；
- 默认管理员密码 `sysadmin/sysadmin123` 拒绝启动；
- 密码登录启用但账号/密码为空拒绝启动。

部署侧还需：

- Nginx TLS 1.2+、HSTS、CSP、`X-Content-Type-Options: nosniff`；
- `/metrics` 拒绝公网；peer 端口 NetworkPolicy 限制；
- 平台镜像非 root、只读根文件系统（Helm 默认）；
- 定期更新依赖与镜像（关注 MinIO 社区版已停止维护的上游风险）。

## 八、审计与合规

- 登录、管理操作、策略变更、头像、改密、踢出、数据面允许/拒绝均落审计；
- 审计异步写 PG，PG 故障时 spill 本地文件，恢复后回放；
- 集群审计写 JSONL 文件并由 Promtail 送入 Loki；保留期与 Loki 保留策略可配置；
- 审计包含来源 IP、User-Agent、GeoIP（配置 `.mmdb` 时）；
- 应用日志（Zap）与审计文件都可能含主体/资源信息，日志平台需访问控制；
- 审计正文默认摘要化，详情按需查询，降低敏感信息泄露面。

## 九、上线安全核对表

- [ ] `AM_ENV=production`，`AM_PASSWORD_LOGIN=0` 或已改强密码
- [ ] `AM_DEV_ISSUE_TOKEN` 未启用
- [ ] `AM_AVATAR_SECRET` 为随机 32 字节以上，存于 Secret
- [ ] `JWT_KEYRING_DIR` 稳定挂载，私钥权限 0400/0600
- [ ] OIDC redirect URL 与前端访问源完全一致
- [ ] Redis/PG/MinIO 不暴露公网，访问有网络策略
- [ ] MinIO 凭据为专用账号，仅 bucket 读写
- [ ] Agent RBAC 已最小化（或明确接受 cluster-admin）
- [ ] Nginx 开启 HSTS/CSP/nosniff，`/metrics` 拒绝外部
- [ ] 登录限流、数据面限流按容量配置
- [ ] 审计保留期、Loki 保留与 Promtail 采集已确认，备份可恢复
- [ ] 完成 deploy/README.md 的故障演练与人工验收
