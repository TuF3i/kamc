# kamc-web

kamc 平台管理前端（Vue 3 + TypeScript + Vite + Pinia），交互风格对齐 Rancher Dashboard（深色壳 + 浅色内容区、卡片数据表、状态徽章、Growl 通知），主题自绘、无 Rancher 代码依赖。

## 快速开始

```bash
cd web
npm install
KAMC_API=http://127.0.0.1:8080 npm run dev   # 默认代理到 127.0.0.1:8080
```

后端需开启开发直签以便脱离 Authentik 登录：

```bash
DATABASE_URL="postgres://postgres:postgres@127.0.0.1:5432/kamc?sslmode=disable" \
AM_DEV_ISSUE_TOKEN=1 ./bin/kamc-platform
```

打开 http://localhost:5173 → 「开发直签登录」→ sub 填 `admin`、组填 `am:admin`。
生产登录走「使用 SSO 登录（OIDC）」按钮（后端 `/auth/login` 302 → Authentik → 回调桥接页写 localStorage → 回 SPA）。

```bash
npm run build   # vue-tsc 严格类型检查 + 产物输出 dist/
```

## 页面清单

| 路由 | 说明 |
| --- | --- |
| `/login` | SSO 跳转 + 开发直签（SPA 路由；不可用 `/auth/login`，该前缀在代理层转发后端） |
| `/` | 概览：集群统计卡片（管理员）或快速开始 + 身份（普通用户） |
| `/clusters` | 注册 / 注销集群（输入 ID 二次确认） |
| `/clusters/:id` | 详情四页签：概况 / Agent Token（签发-轮换-吊销、一次性明文弹窗）/ 终端 / kubeconfig |
| `/access` | Casbin 绑定与策略增删、踢出用户（epoch 吊销） |
| `/scripts`、`/scripts/:name` | 登录期 Lua 绑定脚本：启停 / CodeMirror 编辑（保存即语法校验）/ mock claims dry-run 动作清单 |
| `/audit` | 审计查询（用户/集群/条数过滤，允许-拒绝着色） |
| `/account` | claims 展示、kubeconfig 下载（自服务）、登出 |

## exec 终端协议

集群详情 → 终端：浏览器直连平台 `/proxy/{clusterID}/api/v1/namespaces/{ns}/pods/{pod}/exec`
（WebSocket 子协议 `v4.channel.k8s.io`，与 kubectl/Rancher 同构）。帧格式为
**首字节流号 + 载荷**：0 stdin / 1 stdout / 2 stderr / 3 error / 4 resize（载荷 JSON
`{"Width":cols,"Height":rows}`）。协议封装见 `src/terminal/exec.ts`。

浏览器 WebSocket 无法携带 Authorization 头：JWT 经 `access_token` 查询参数传递
（后端仅对 WS 升级请求接受该参数，见 `internal/platform/middleware/jwt.go`）。
数据面链路：JWT → Casbin `pods/exec` 鉴权（fail-closed）→ Impersonation 注入 →
隧道转发 → 上游 apiserver RBAC；创建事件入审计。

## 生产部署

`npm run build` 产出 `dist/`，任意静态服务器托管并以同源反代后端即可（与
vite dev 代理同规则）：

```nginx
# nginx 参考片段（/tunnel 为 Agent WS 专用，勿与前端路由混淆）
location / { try_files $uri $uri/ /index.html; }   # SPA history 路由回退
location /api/    { proxy_pass http://kamc-backend; }
location /auth/   { proxy_pass http://kamc-backend; }
location /proxy/  { proxy_pass http://kamc-backend; proxy_http_version 1.1;
                    proxy_set_header Upgrade $http_upgrade;   # exec/port-forward WS
                    proxy_set_header Connection "upgrade"; proxy_read_timeout 1h; }
location /.well-known/ { proxy_pass http://kamc-backend; }
location ~ ^/(healthz|readyz|version)$ { proxy_pass http://kamc-backend; }
```

token 存储于同源 localStorage（XSS 面与 Rancher 的 cookie 方案相当，均依赖前端
无注入；桥接页已做 JSON 字面量转义）。退出登录为整页硬导航并吊销 jti。

## 结构

```
web/src/
├── api/          # fetch 封装：信封解包（HTTP 200 + status 业务码）、ApiError、401 统一回登录
├── stores/       # Pinia：auth（restore/probeAdmin/signout）、growl（通知）
├── components/   # Card/DataTable/Modal/Banner/StatusBadge/Tabs/GrowlStack/
│                 # LabeledInput/LabeledSelect/CopyButton/EmptyState/CodeEditor/ExecTerminal
├── pages/        # Login/Home/Clusters/ClusterDetail/Access/Scripts/ScriptEdit/Audit/Account
├── terminal/     # exec WS 帧协议（buildExecUrl/stdinFrame/resizeFrame/parseFrame）
└── styles/       # 主题变量（variables.scss 经 vite additionalData 全局注入）+ 基础样式
```

路由守卫（`src/router.ts`）：`meta.public` 免登录；`meta.admin` 需管理员——
管理员判定不是解析 claims（平台无法感知 AM_ADMIN_GROUP 取值），而是 restore 时以
`GET /api/v1/clusters` 探测（403 = 普通用户）。
