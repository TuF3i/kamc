# API 文档

`openapi.yaml` 是 kamc 平台 HTTP API 的唯一规范（OpenAPI 3.0.3，当前 3.1.0），
覆盖运维端点、认证、集群/Token、kubeconfig、用户目录、映射脚本、权限策略、
机器人、审计、数据面代理与 Agent 入口。

## 约定

### 响应信封

`/api/v1/**` 业务端点返回统一信封，HTTP 通常恒为 200：

```json
{"status": 0, "message": "success", "data": {}}
```

- `status != 0` 即业务错误；`status >= 50000` 时 HTTP 为 500。
- 中间件错误（401/403/429/501/503）为纯文本，不是 JSON。
- 数据面 `/proxy/**` 返回上游 Kubernetes 原始 JSON；`/tunnel` 与
  `/agent-install.yaml` 属 Agent 专用通道。
- 头像 `GET /avatars/{uid}` 返回图片字节，`token` 查询参数即凭证。

业务码分段：0 成功；1xxxx 通用；2xxxx 集群域；3xxxx Token 域；4xxxx 策略域。
接口级业务错误在各操作的 `200` 描述与 `x-kamc-business-errors` 扩展中说明。

### 认证矩阵

| 端点类别 | 凭证 | 说明 |
| --- | --- | --- |
| 管理面 `/api/v1/**` | 平台 JWT | 另需 `am:admin` 组（`AM_ADMIN_GROUP`） |
| 自服务（profile/password/avatar/my-clusters/kubeconfig） | 平台 JWT | 仅需有效身份 |
| `/api/v1/agent/register` | 注册 Token（`am-reg-`） | 一次性交换长期 Token |
| `/tunnel` | 长期 Agent Token（`am-clu-`） | Agent WebSocket |
| `/proxy/{clusterID}/**` | 平台 JWT | 平台 Casbin 鉴权；集群侧不做 Impersonation |
| `/avatars/{uid}` | 查询参数 `token` | HMAC 签名，供 `<img>` 使用 |
| `/healthz`、`/readyz`、`/version`、`/metrics` | 无 | 运维端点，部署层限制访问 |

平台 JWT 获取方式：OIDC（`/auth/login` → `/auth/callback`）、ID Token 兑换
（`/auth/token-exchange`）、账号密码（`/auth/password-login`）、LDAP
（`/auth/ldap-login`）；开发环境可开 `/auth/dev-token`。

### 头像存储

头像字节默认存本地目录（`AM_AVATAR_DIR`），配置 MinIO/S3 后透明切换。
对外 URL 始终是平台签名的 `/avatars/{uid}?token=...`，对象存储地址与凭据不会
出现在响应中。上传上限 2MB，允许 png/jpeg/webp/gif。

## 校验

```bash
npx --yes @redocly/cli@latest lint api/openapi.yaml
```

当前规范可被 Redocly 接受（0 error）。剩余 warning 主要是未填 `operationId`、
示例与 4xx 描述，不阻塞使用；修改接口时请确保不新增 error。

## 更新流程

1. 在 `internal/platform/handler/handler.go` 注册路由，并在对应 handler 文件中
   定义请求/响应结构。
2. 更新 `openapi.yaml` 的 `paths`、`components.schemas` 与 `tags`；新业务错误码
   同步到 `internal/platform/errcode/errcode.go` 和操作描述。
3. 运行 Redocly 校验；涉及认证/错误码的变更同时更新
   [docs/architecture.md](../docs/architecture.md) 与
   [docs/configuration.md](../docs/configuration.md)。
4. 前端调用位于 `web/src/api/`，路径与字段必须与本规范一致。

## 相关文档

- [docs/architecture.md](../docs/architecture.md)：认证、鉴权、隧道与审计设计
- [docs/configuration.md](../docs/configuration.md)：环境变量与生产护栏
- [docs/design.md](../docs/design.md)：v2.0 历史设计记录
