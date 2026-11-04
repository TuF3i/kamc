# kamc Web 前端

kamc 管理控制台：React 18 + Ant Design 5 + TypeScript + Vite + Recharts。
深色悬浮导航运维控制台风格，覆盖集群、Agent Token、Casbin 权限、审计、
机器人、kubeconfig 与平台信息页。

## 生产构建（前后端一起启动）

```bash
make release        # 在仓库根目录执行：构建前端到 data/web，再编译后端
./bin/kamc-platform # 打开 http://127.0.0.1:8080 即为管理前端（与 API 同源）
```

后端运行时从 `AM_WEB_DIR`（默认 `data/web`）读取前端产物并映射成路由：
SPA 未知路径回退 `index.html`，`assets/` 长缓存；`/api`、`/auth`、`/proxy`
等后端命名空间不参与回退。前端产物不进入二进制，部署时与后端一起发布即可。

## 快速开始（两种模式）

```bash
cd web
npm install

npm run dev        # release 模式：连接后端（KAMC_API 指定平台地址，默认 127.0.0.1:8080）
npm run dev:mock   # mock 模式：完全脱离后端，使用前端内置模拟数据（刷新重置）
```

打开 <http://localhost:5173>：

- 默认显示 **SSO 登录**。
- 点击「使用用户名和密码」切换到内置账号登录，默认 `sysadmin / sysadmin123`
  （`AM_PASSWORD_LOGIN_USER` / `AM_PASSWORD_LOGIN_PASSWORD` 可覆盖，
  `AM_PASSWORD_LOGIN=0` 关闭）。
- 开发直签登录已从界面移除；`/auth/dev-token` 端点仍保留给测试/CLI 使用。

生产登录优先走 Authentik SSO：前端跳转后端 `/auth/login`，回调桥接页写入
`localStorage['kamc_token']` 后跳回 SPA 根路径。

### 开发代理

平台没有 CORS，浏览器必须与 API 同源。Vite 开发代理（`vite.config.ts`）：

| 路径 | 说明 |
| --- | --- |
| `/api` | 管理面 API |
| `/auth` | OIDC / 用户名密码登录 / me / logout |
| `/proxy` | 数据面（保留 `ws: true` 以透传 WebSocket，如 kubectl exec） |
| `/.well-known` | JWKS |
| `/healthz`、`/readyz`、`/version` | 运维端点 |

`KAMC_API` 只影响开发代理目标，不是前端运行时 baseURL。

> OIDC 联调：`OIDC_REDIRECT_URL` 应与前端访问源一致（如
> `http://localhost:5173/auth/callback`），否则桥接页会把 token 写到另一个源的
> localStorage。

## 页面

| 路由 | 页面 | 权限 |
| --- | --- | --- |
| `/login` | 登录（默认 SSO，可切换用户名密码） | 公开 |
| `/` | 概览（系统统计 + 近 7 天登录/操作/拦截折线图） | 管理员 |
| `/clusters` | 集群列表：注册、搜索、注销（输入完整 ID 确认） | 管理员 |
| `/clusters/:id` | 集群详情：概况 / Agent Token / Agent 安装 / kubeconfig | 管理员 |
| `/platform` | 平台信息：设置 / SSO / 数据库 / 缓存 / Authentik / Go 运行时 | 管理员 |
| `/access` | 权限策略列表与四段式策略编辑器 | 管理员 |
| `/audit` | 审计日志查询（用户/集群/条数） | 管理员 |
| `/robots` | CICD 机器人：签发长期 kubeconfig、配置策略、下载、吊销 | 管理员 |
| `/kubeconfig` | 独立 kubeconfig 下载页（普通用户默认落地页） | 登录 |
| `/account` | 当前身份详细信息 | 登录 |

## 权限模型与策略编辑器

权限页只保留策略，不再有用户/角色/绑定 Tab。策略主体直接是：

- **组织**：Authentik Group，存为 `org:<group name>`；
- **用户**：Authentik User，存为 `user:<subject>`（Robot 也是 `user:robot:<id>`）。

创建策略的 Drawer 从上到下四段：

1. **主体**：用户/组织单选；组织与用户来自 Authentik API，用户下拉额外合并本平台 Robot。
2. **集群**：单选；选择后调用 `GET /api/v1/clusters/{id}/resources` 加载 GVR、动词和 namespace。
3. **资源与动作**：可添加多条；每条选择资源（可含 subresource）、namespace（`*` 或具体值）和多选动作；资源 `*` 自动授予全部动作 `*`。
4. **通配与预览**：展示最终展开的 `p, sub, dom, obj, act` 行。

后端展开规则：namespace `*` 生成 `*/resource` 并补充 `cluster/resource` 覆盖全命名空间 list；cluster-scoped 资源生成 `cluster/resource`；具体 namespace 生成 `<ns>/resource`。

## Authentik 目录

策略页的组织/用户下拉由平台后端代理 Authentik API：

- `AUTHENTIK_API_URL`（为空时从 `OIDC_ISSUER` origin 推导）
- `AUTHENTIK_API_TOKEN`（必填，读取 core users/groups）
- `AUTHENTIK_USER_SUB_FIELD`（默认 `uid`，可选 `username`/`pk`）
- `AUTHENTIK_CACHE_TTL`（默认 `60s`）

未配置 Token 时目录接口返回 503，编辑器会显示配置错误。

## Robot 权限

Robot 不再绑定角色。创建机器人只填名称与集群，之后：

- 在权限页为 `user:robot:<id>` 创建策略；或
- 在机器人列表点击「配置策略」，自动跳转权限页并预填机器人主体。

删除机器人会吊销 JWT、清理该主体的全部直接策略并删除记录。

## 认证与 API 约定

- **统一信封**：业务响应为 `{status, message, data}`；`status !== 0` 即业务错误，
  HTTP 通常仍为 200；`status >= 50000` 才是 HTTP 500。
- **纯文本错误**：401 / 403 / 429 与 kubeconfig 400 不是 JSON，HTTP 封装按
  `Content-Type` 分支处理。
- **Token**：`localStorage['kamc_token']`；HTTP 请求使用
  `Authorization: Bearer <token>`；浏览器 WebSocket 使用 `?access_token=<token>`
  （仅 WS 升级请求有效）。
- **密码登录**：`POST /auth/password-login`（`{username,password}`），默认
  `sysadmin/sysadmin123`，返回与 OIDC 相同的 `IssuedCredential`。
- **管理员判定**：`GET /api/v1/clusters` 返回 200 = 管理员，403 = 普通用户；
  `/auth/me` 没有 `is_admin` 字段。
- **401**：清理 token 并跳 `/#/login?expired=1`；403 不视为登录失效。
- **kubeconfig**：必须带 Authorization 头 `fetch` 后以 Blob 下载，不能
  `window.open`；文件名取 `Content-Disposition`。
- **数据面**：`/proxy/{clusterID}/...` 返回原生 Kubernetes JSON，不是平台信封。
- **审计**：`groups` 是逗号分隔字符串；`limit` ≤ 0 或 > 1000 会被后端强制为 100；
  异步批量落库，查询可能有约 0.5s 延迟。

## 目录结构

```text
web/src/
  api/         http 封装与各资源 API（types.ts 为后端 JSON 字段）
  stores/      auth（token/claims/管理员探测）
  layouts/     AppLayout（悬浮三段式导航：Logo / 标签页选择器 / 用户面板）
  components/  PageHeader / StatusTag / CopyButton / PolicyEditor / 路由守卫
  pages/       Login / Dashboard / Clusters / ClusterDetail / Access /
               Audit / Robots / Kubeconfig / Platform / Account / NotFound
  utils/       时间与剩余有效期格式化
  theme.ts     Ant Design ConfigProvider 主题
```

## 构建与部署

```bash
npm run build     # tsc --noEmit + vite build → web/dist/
npm run preview   # 本地预览 dist（无后端代理，仅静态）
```

生产必须同源部署：Nginx 托管 `dist/` 的 SPA 回退，并显式反代 API/认证/数据面。
现有 `deploy/nginx/nginx.conf` 只反代平台、未托管静态文件，可参考以下片段整合：

```nginx
root /usr/share/nginx/html/kamc;
location / {
    try_files $uri $uri/ /index.html;
}

location /api/    { proxy_pass http://kamc_platform; proxy_set_header Host $host; }
location /auth/   { proxy_pass http://kamc_platform; proxy_set_header Host $host; }
location /.well-known/ { proxy_pass http://kamc_platform; }
location ~ ^/(healthz|readyz|version)$ { proxy_pass http://kamc_platform; }

location /proxy/ {
    proxy_pass http://kamc_platform;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection $connection_upgrade;
    proxy_set_header Host $host;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
    proxy_buffering off;
    proxy_request_buffering off;
}
```

`/tunnel` 是 Agent 专用路径，不要暴露给浏览器或落到 SPA 回退。

## 已知后端行为

- 权限模型迁移需执行 `deploy/migrations/2026-11-02-policy-reset.sql`（清空旧策略、删除 roles/auth_scripts、移除 robots.role）。
- Robot 长期 Token 默认 365 天（`AM_ROBOT_TOKEN_TTL`）；删除机器人时吊销 jti 并清理其策略。
  未配置 Redis 时吊销只在当前进程生效，重启后黑名单丢失；生产应启用 Redis。
- `/auth/callback` 浏览器分支固定跳 `/`，不支持子路径部署或回跳原页面。
- 平台不提供静态文件服务、没有 CORS；不要尝试跨域直连。
- 审计中允许的 exec/port-forward WebSocket 事件当前不落库（仅拒绝事件有审计）。
- `access_token` 会随 query 转发到上游 apiserver，生产需注意日志脱敏。

## 验证

```bash
npm run build          # 类型检查 + 生产构建
go build ./... && go vet ./... && go test -count=1 ./...
```
