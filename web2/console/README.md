# kamc-console（web2）

kamc 管理前端的 SolidJS 重写版：**布局、路由、交互逻辑与中文文案沿用 `web/`**，
组件视觉采用 `web2/example/console`（ret2shell 玻璃/终端风）的设计语言，并改为蓝灰配色，
支持深色 / 浅色双主题。

## 技术栈

- SolidJS 1.9 + `@solidjs/router` 0.15（HashRouter，保持 `/#/...` 路由）
- TypeScript strict + Vite 6 + Tailwind CSS 4
- `@iconify/tailwind4`（Fluent 图标）、`clsx`
- ECharts（概览图表，tree-shaken）
- CodeMirror 6（Lua 映射脚本编辑器）
- dayjs

## 开发

```bash
npm install
npm run dev:mock   # Mock 模式：前端内置数据，无需后端；http://localhost:5174
npm run dev        # 连接平台：默认代理 http://127.0.0.1:8080
KAMC_API=http://127.0.0.1:8080 npm run dev
```

`npm run dev` 的代理规则与 `web/vite.config.ts` 一致：`/api`、`/auth`、`/avatars`、
`/proxy`（WebSocket）、`/.well-known`、`/healthz`、`/readyz`、`/version`。

## 构建

```bash
npm run typecheck
npm run build      # tsc --noEmit + vite build → web2/console/dist
```

当前产物为独立目录，**不接管** `data/web`，也未接入根 `Makefile` / `Dockerfile` / CI；
验收后可再决定是否替换 `web/`。

## 主题

- 深色 `kamc-dark` / 浅色 `kamc-light`，token 定义在 `src/styles/tokens.css`。
- 主题切换在右上角用户菜单内；持久化 key：`localStorage['kamc_theme']`。
- `index.html` 内联脚本在首屏前设置 `data-theme`，避免闪跳。

## 与旧前端的关系

- API、类型、mock 数据、工具函数、权限/审计/策略语义均从 `web/src` 移植。
- 文案一致性通过提取 `web/src` 与 `web2/console/src` 中文字符串做人工比对；
  仅新增主题菜单、日期选择器与表格控件默认文案。
- `web2/example/` 是设计参考样例，不纳入提交（见根 `.gitignore`）。
