# kubectl exec 命令审计实施计划（WS ↔ SPDY tee）

> 状态：已取消（2026-11-04 决定不实施会话录制；本文仅留档，勿按此实现）
> 目标：在平台侧代理链路上审计 `kubectl exec` 的初始命令与交互式会话输入，支持命令检索与会话回放。

## 1. 目标与非目标

### 目标

- 审计每一次 `kubectl exec`：谁、在哪个集群/命名空间/Pod/容器、执行了什么、何时开始结束、退出码。
- 非交互命令（`kubectl exec pod -- cmd`）100% 覆盖，命令直接来自请求 query。
- 交互式 TTY 会话（`kubectl exec -it pod -- sh`）：捕获 stdin/stdout/stderr/resize，支持回放，并按行提取“尝试输入”的命令（best-effort）。
- 审计记录与平台身份（JWT sub/groups）、来源 IP、UA、GeoIP、Casbin 判定结果关联。
- 录制失败不影响 exec 本身（fail-open for recording；权限判定仍 fail-closed）。

### 非目标

- 不做容器内 execve 级进程审计（eBPF/auditd），后续可作为补充。
- 不修改用户命令、不注入 shell wrapper、不要求业务镜像配合。
- 不做输出内容脱敏（只做访问控制、大小限制和保留期管理）。
- 不覆盖用户绕过平台直连 apiserver 的场景（由网络策略/部署约束保证）。

## 2. 现状与关键结论

当前 exec 链路（`internal/platform/handler/proxy.go:115`）：

1. `kubectl exec` 携带 `Upgrade: websocket` 与 `v5.channel.k8s.io` 子协议进入 `/proxy/{cluster}/...`。
2. 平台 Casbin 已放行（集群侧不再注入 Impersonation；Agent 以 SA 身份访问上游）。
3. 使用 `k8s.io/apiserver/pkg/util/proxy.NewStreamTranslatorHandler` 直接经隧道 transport 完成：
   - 客户端 WebSocket（channel 0=stdin、1=stdout、2=stderr、3=error、4=resize）
   - ↔ 上游 SPDY（`remotecommand.NewSPDYExecutor`）
4. 平台目前只记录 `allowed-stream`（status 101），不读取流内容。

关键结论：

- 非交互命令在 exec 请求 URL 的 `command` query 中（`command=ls&command=/tmp`），平台已能看到（现有审计 `path` 已包含完整 query），只是没有结构化解析。
- 交互式后续命令只在 **stdin channel** 中，必须在翻译层 tee。
- 上游 `StreamTranslatorHandler` 没有流 hook；但它的 `ServeHTTP` 里 `websocketStreams.stdinStream/stdoutStream/stderrStream/resizeChan` 正好是 tee 点。
- `websocketStreams` 与 `webSocketServerStreams` 是 `k8s.io/apiserver` 的未导出实现，无法从外部包装，需要 fork 一份（Apache-2.0）或自实现同等逻辑。

## 3. 方案选择

| 方案 | 结论 |
| --- | --- |
| Fork `StreamTranslatorHandler`，在构造 `remotecommand.StreamOptions` 时 tee | **采用**：改动集中、保留 Kubernetes 官方协议语义 |
| 包装上游 SPDY `Transport` 解析 SPDY 帧 | 不采用：SPDY 多 stream 映射复杂、易随版本变化 |
| 在客户端 WS 外面再套一层 WS 代理 | 不采用：需二次终止/重建 WS，复杂且多一跳 |
| 注入 shell wrapper / 改造容器 | 不采用：可被 `exec /bin/sh` 绕过，侵入业务 |
| 仅依赖 apiserver audit | 作为补充：只能拿到初始 command，且要改集群审计配置 |

## 4. 总体架构

```text
kubectl exec
   │  WebSocket (v5.channel.k8s.io)
   ▼
kamc-platform /proxy/{cluster}/...
   │
   ├─ 请求级审计：解析 query(command/container/stdin/tty) ──► audit_events(exec-start)
   │
   ├─ fork 的 StreamTranslatorHandler
   │      ├─ stdin  ── tee ──► SessionRecorder.CommandAssembler
   │      ├─ stdout ── tee ──► SessionRecorder.Cast(输出帧)
   │      ├─ stderr ── tee ──► SessionRecorder.Cast(错误帧)
   │      └─ resize ────────► SessionRecorder.Cast(resize 事件)
   │
   ▼
上游 apiserver / SPDY（Agent SA 身份，平台已完成用户级授权）
   │
   └─ 会话结束 ──► exec_sessions / exec_commands 落库
                    录制文件 data/sessions/<id>.cast.gz
```

## 5. 详细设计

### 5.1 Fork 翻译层

新增包：`internal/platform/execstream/`

来源（固定版本，保留许可证头）：

- `k8s.io/apiserver@v0.37.2/pkg/util/proxy/streamtranslator.go`
- `k8s.io/apiserver@v0.37.2/pkg/util/proxy/websocket.go`

处理：

- 去掉对 `k8s.io/apiserver/pkg/util/proxy/metrics` 的依赖，换成平台 `metrics` 或 no-op。
- `Options` 改为包内类型（Stdin/Stdout/Stderr/Tty），行为与上游一致。
- `StreamTranslatorHandler` 增加 `TapFactory func(*http.Request) Tap` 字段；`Tap` 接口：

```go
type Tap interface {
    // 以下 Writer 必须非阻塞或有界阻塞；禁止因为录制拖死 exec
    Stdin() io.Writer
    Stdout() io.Writer
    Stderr() io.Writer
    Resize(cols, rows uint16)
    Close(exitCode int, execErr error)
}
```

- `ServeHTTP` 内只改流装配部分：

```go
tap := h.TapFactory(req) // 可为 nil（功能关闭）
if tap != nil {
    stdin = io.TeeReader(stdin, tap.Stdin())
    stdout = io.MultiWriter(stdout, tap.Stdout())
    stderr = io.MultiWriter(stderr, tap.Stderr())
    terminalSizeQueue = &tapSizeQueue{inner: innerQueue, tap: tap}
}
```

> 注意：`io.MultiWriter`/`io.TeeReader` 会在 tap 写入阻塞时反压到 exec。
> 因此 `Tap` 的 Writer 必须写入**有界 ring/channel**，队列满时截断并置
> `truncated=true`，绝不等待落盘。

- 保留上游行为：
  - 仅支持 v5 子协议（与当前一致）
  - 4 小时 idle timeout
  - resize JSON 解码、error channel 写 Status
  - `exec.CodeExitError` → exit code；StatusError → 错误状态
  - 退出时先 `tap.Close(exitCode, err)` 再写状态（保证审计 session 收尾）

- 许可证与升级：目录下放 `NOTICE`/源码头注释，记录上游版本；后续升级时 diff 同步。

### 5.2 Session 生命周期

在 `serveStream` 内、调用翻译层之前：

1. 解析 exec 元数据：
   - `clusterID`、`namespace`、`pod`、`container`
   - `command[]`（query 中可重复）
   - `stdin/stdout/stderr/tty`
   - claims.Subject、claims.Groups、来源 IP（XFF）、UA、GeoIP
2. 生成 `sessionID`（UUIDv4），启动 `SessionRecorder`：
   - 创建临时文件 `data/sessions/.tmp/<id>.jsonl`
   - 写 asciinema v2 header（默认 80x24；resize 事件会更新播放尺寸）
   - 数据库插入 `exec_sessions`（status=running）
   - 写审计事件 `exec-start`（带 session_id）
3. 调用 `execstream.Handler`，Tap 绑定到 recorder。
4. 会话结束：
   - `Close(exitCode)`：flush 队列 → gzip 压缩 → 原子 rename 到 `data/sessions/<id>.cast.gz` → 更新 `exec_sessions`（ended_at/exit_code/bytes/truncated/recording_path）
   - 写审计事件 `exec-end`（或只更新 session 行，减少事件量）
   - 删除临时文件；异常退出由启动清理任务处理孤儿文件

### 5.3 会话录制格式

采用 asciinema v2（JSONL）便于现成播放器与后续离线分析：

```json
{"version":2,"width":80,"height":24,"timestamp":1790000000,"env":{"SHELL":"/bin/bash"}}
[0.120, "i", "ls\r"]
[0.310, "o", "app  bin  etc\r\n"]
[1.020, "r", "120x32"]
```

- `i` = stdin（用户输入）
- `o` = stdout（合并 stderr 可加 `e` 事件或单独 type）
- `r` = resize（`"120x32"`）
- 时间戳为相对会话开始的秒数（保留 3 位小数）

压缩：`compress/gzip`（标准库，避免新依赖），文件权限 `0600`。
播放：前端引入 `asciinema-player`（或按格式自研 `<pre>` 播放器）。

### 5.4 命令提取（best-effort）

非 TTY：

- stdin 按 `\n` 切分，去掉 `\r`；每条即命令（适用于 `kubectl exec -i ... < script`）。

TTY：

- 维护 `[]rune` 行缓冲 + 光标位置，处理：
  - `\r`/`\n`：提交一行
  - `\x7f`/`\b`：退格
  - `\x15` Ctrl-U：清空当前行
  - `\x17` Ctrl-W：删一个单词
  - `\x03` Ctrl-C：放弃当前行并记录中断
  - `\x04` Ctrl-D：结束输入
  - `ESC[A/B`：上/下历史（无法从 stdin 还原历史内容，标 `confidence=low`）
  - `ESC[C/D`：左右移动光标（维护索引）
  - `ESC[200~ ... ESC[201~`：bracketed paste，按换行拆分为多条
  - UTF-8 多字节跨帧拼接
- 初始命令（query `command[]`）作为 `source=initial, confidence=high` 单独记录；若与 stdin 第一条重复则去重。
- 提取结果标 `confidence=high|best-effort`；不能承诺与 shell 实际执行完全一致（补全、别名、`eval`、粘贴换行等）。

限制：

- 单条命令最大 4096 字符，超出截断
- 每会话最多 1000 条命令，超出标记 truncated
- 二进制/超大 stdin 只保留录制，不尝试提取

### 5.5 数据模型

新增两张表（PG）：

```sql
CREATE TABLE exec_sessions (
  id               uuid PRIMARY KEY,
  cluster          varchar(64)  NOT NULL,
  namespace        varchar(128) NOT NULL,
  pod              varchar(253) NOT NULL,
  container        varchar(128),
  username         varchar(128) NOT NULL,
  groups           text,
  command          text,          -- JSON 数组，query 初始命令
  tty              boolean NOT NULL DEFAULT false,
  stdin            boolean NOT NULL DEFAULT false,
  stdout           boolean NOT NULL DEFAULT false,
  stderr           boolean NOT NULL DEFAULT false,
  started_at       timestamptz NOT NULL,
  ended_at         timestamptz,
  exit_code        int,
  bytes_in         bigint NOT NULL DEFAULT 0,
  bytes_out        bigint NOT NULL DEFAULT 0,
  truncated        boolean NOT NULL DEFAULT false,
  recording_path   text,
  recording_bytes  bigint NOT NULL DEFAULT 0,
  source_ip        varchar(64),
  user_agent       varchar(512),
  geo_country      varchar(8),
  geo_region       varchar(128),
  geo_city         varchar(128),
  created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE exec_commands (
  session_id  uuid NOT NULL REFERENCES exec_sessions(id) ON DELETE CASCADE,
  seq         int  NOT NULL,
  at_ms       bigint NOT NULL,   -- 相对会话开始毫秒
  command     text NOT NULL,
  source      varchar(16) NOT NULL,     -- initial | stdin
  confidence  varchar(16) NOT NULL,     -- high | best-effort
  PRIMARY KEY (session_id, seq)
);
CREATE INDEX idx_exec_sessions_user_time ON exec_sessions(username, started_at DESC);
CREATE INDEX idx_exec_sessions_cluster_time ON exec_sessions(cluster, started_at DESC);
-- 可选：命令模糊搜索依赖 pg_trgm 扩展
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX idx_exec_commands_cmd ON exec_commands USING gin (command gin_trgm_ops);
```

审计事件：

- `audit_events` 增加 `session_id uuid NULL`（索引），exec 相关事件带 session_id。
- `exec-start`：kind=cluster、event=exec、object=`<ns>/pods/<pod>/exec`、verb=create、status=101、reason=`tty=<bool>`
- `exec-end`：可配置是否单独记；默认只更新 session 行，避免事件翻倍
- 可选：`exec-command` 不写 audit_events，命令统一从 exec_commands 查（避免每条命令一条审计）

### 5.6 API

```text
GET /api/v1/exec/sessions?user=&cluster=&pod=&command=&from=&to=&limit=
GET /api/v1/exec/sessions/{id}          # 元数据 + 命令列表
GET /api/v1/exec/sessions/{id}/recording # 下载 .cast.gz（管理面 JWT）
DELETE /api/v1/exec/sessions/{id}       # 手动清理（可选）
```

- 全部管理面（JWT + admin group）。
- 列表默认按 started_at 倒序，limit 50/100/200。
- 录制下载设置 `Content-Disposition`，支持前端播放器拉取。

### 5.7 UI

审计页新增第三个 Tab「Exec 会话」：

- 表格：时间、用户、集群、Pod/容器、命令摘要、TTY、时长、退出码、大小、状态
- 详情 Drawer：
  - 元数据（用户/组/IP/Geo/UA/策略结果）
  - 命令时间线（来源、置信度、时间）
  - 回放器（asciinema-player，支持暂停/倍速/复制文本）
  - 下载录制
- 搜索：命令文本（trigram）、用户、Pod
- 概览页后续可加 exec 统计卡片（可选，不在本计划范围）

### 5.8 配置

| 环境变量 | 默认 | 说明 |
| --- | --- | --- |
| `AM_EXEC_AUDIT` | `commands` | `off` / `commands`（仅命令）/ `session`（命令+录制） |
| `AM_EXEC_RECORD_DIR` | `data/sessions` | 录制文件目录；HA 需共享存储/对象存储 |
| `AM_EXEC_RECORD_MAX_BYTES` | `8388608` | 单会话录制上限（压缩前） |
| `AM_EXEC_INPUT_MAX_BYTES` | `1048576` | stdin 捕获上限 |
| `AM_EXEC_OUTPUT_MAX_BYTES` | `8388608` | stdout+stderr 捕获上限 |
| `AM_EXEC_COMMAND_MAX_LEN` | `4096` | 单条命令截断长度 |
| `AM_EXEC_RECORD_RETENTION` | `720h` | 保留期，后台清理 |

### 5.9 安全与合规

- 会话元数据与录制仅管理员可见；不提供匿名/签名公开 URL，下载走 JWT。
- 文件权限 `0600`，目录 `0700`；数据库连接已有权限边界。
- 录制可能包含密钥/敏感输出：
  - 明确保留期与访问审计（谁下载了录制也应记审计）
  - 可配置命令参数正则脱敏（如 `--password=xxx`、`Authorization`），但输出不保证
- 平台故障时录制可缺失，但 `exec-start` 事件仍应尽量落库；录制失败不得影响 exec。
- 多副本部署：录制文件必须放共享存储（PVC RWX / S3），否则在其他副本查询时 404；本计划先支持本地目录 + 单副本，HA 作为部署前置条件。

## 6. 失败与边界处理

| 场景 | 处理 |
| --- | --- |
| 录制队列满 | 丢弃后续帧，`truncated=true`，不阻塞 exec |
| 磁盘写失败 | 关闭录制，记 warning 日志，exec 继续 |
| 平台进程崩溃 | 临时文件残留，启动时清理 `.tmp`；session 标 `aborted`（启动扫描 running 超时） |
| 上游 exec 失败 | 仍 `Close(exitCode/err)`，记录失败原因与状态 |
| 非 TTY 大输出 | 达到 output cap 后停止录制，继续转发 |
| WebSocket 断开 | 正常 Close，记录 ended_at 与 bytes |
| 多副本 | 依赖共享存储；否则限制单副本或返回 404 |
| 旧版客户端 v4 协议 | 当前翻译层只支持 v5；维持现状（不支持则无法 exec） |

## 7. 测试计划

### 单元测试

- 命令提取状态机：普通输入、退格、Ctrl-U/W/C、方向键、Home/End、bracketed paste、UTF-8 多字节、超长截断、二进制数据。
- SessionRecorder：容量上限、并发写、gzip 原子落盘、truncated 标记、清理。
- 元数据解析：query `command` 重复参数、container、tty/stdin 组合。

### 翻译层测试

移植上游 `streamtranslator_test.go` 的 loopback 用例：

- fake SPDY 上游 + `remotecommand.NewWebSocketExecutor`
- 验证 tee 后 stdin/stdout 内容仍原样透传
- 验证 resize、exit code、StatusError 行为不变
- 验证 Tap 写入阻塞/队列满时 exec 不挂起

### 集成/E2E

- 本地 kind/k3s + 真实 apiserver：
  - `kubectl exec pod -- echo hi` → session 行 + 初始命令 + 简短录制
  - `kubectl exec -it pod -- sh` → 输入 `echo hello`、`exit` → exec_commands 包含命令，录制可播放
  - `kubectl exec -i ... < script` → 非 TTY 多行命令提取
  - 大输出（`yes | head -c 20M`）→ 截断、exec 正常结束
- 回归：exec/port-forward（SPDY 子协议）、attach 不受影响；功能关闭时行为与当前完全一致。

## 8. 里程碑与工作量

| 阶段 | 内容 | 预估 |
| --- | --- | --- |
| M0 | 请求级命令审计（query 解析 + audit 字段/UI） | 0.5–1 天 |
| M1 | fork 翻译层 + Tap 接口 + 透传回归测试 | 2–3 天 |
| M2 | SessionRecorder + 命令提取 + 录制落盘 + 数据表 | 3–4 天 |
| M3 | API + 审计页 Exec 会话 Tab + 播放器 | 2–3 天 |
| M4 | 容量/清理/HA 存储/安全加固 + E2E | 2–3 天 |
| 合计 | | 约 10–14 人天 |

建议先交付 M0（零风险、立即覆盖非交互 exec），M1–M3 作为一次迭代，M4 与部署形态一起确定。

## 9. 风险与开放问题

1. **fork 维护成本**：上游 `StreamTranslatorHandler` 随 Kubernetes 版本变化；固定 apiserver v0.37.2，升级时对照 diff。可评估向上游提交 hook 的 PR，长期减少分叉。
2. **TTY 命令提取准确性**：行编辑/补全/历史无法完全还原；以录制回放为准，命令列表标注置信度。
3. **录制存储成本**：交互式会话可能产生大量输出；必须默认限额+保留期，避免磁盘打满。
4. **敏感信息**：录制和命令可能含密钥；需管理员权限、保留期、下载审计，必要时接对象存储加密。
5. **HA**：本地文件只适用于单副本/共享盘；多副本需 PVC RWX 或 S3 兼容对象存储，影响部署方案。
6. **执行边界**：平台只审计经过代理的 exec；必须用 NetworkPolicy/安全组禁止用户直连 apiserver，否则方案可被绕过。

## 10. 验收标准

- 功能关闭时 exec 行为、延迟、错误码与当前完全一致。
- 非交互 exec 的 command/container/tty 字段 100% 落库，审计页可检索。
- 交互式会话可回放；命令列表可搜索；退出码/时长/字节数正确。
- 录制达到上限时 exec 不中断，`truncated=true` 可见。
- 管理面权限校验：普通用户无法读取会话/录制。
- 现有 `go test ./...` 与前端 typecheck 全绿。
