# Docker Compose 本地环境

定位：**本地开发 / 验收**。一条命令拉起平台、PostgreSQL、Redis（可选 MinIO），
不适合生产（生产请用 `deploy/helm` 或 `deploy/k8s`）。

## 快速开始

```bash
cp deploy/docker/.env.example deploy/docker/.env
docker compose -f deploy/docker/compose.yaml up -d --build
# 或：just compose-up
```

打开 http://localhost:8080，使用 `.env` 中的内置账号登录
（默认 `sysadmin` / `sysadmin123`，仅 `AM_ENV=development` 下可用）。

查看状态与日志：

```bash
docker compose -f deploy/docker/compose.yaml ps
docker compose -f deploy/docker/compose.yaml logs -f platform
just compose-logs
```

## 目录与卷

| 卷 | 用途 |
| --- | --- |
| `pgdata` | PostgreSQL 数据 |
| `redisdata` | Redis AOF |
| `avatardata` | 本地头像文件（`AM_AVATAR_DIR`） |
| `auditdata` | 审计 spill（PG 故障暂存） |
| `miniodata` | MinIO 对象数据（启用 profile 时） |

完全重置（会删除全部数据）：

```bash
docker compose -f deploy/docker/compose.yaml down -v
```

## 启用 MinIO

1. 在 `deploy/docker/.env` 中设置：

   ```dotenv
   AM_AVATAR_BACKEND=minio
   AM_AVATAR_MINIO_ENDPOINT=minio:9000
   AM_AVATAR_MINIO_ACCESS_KEY=kamc
   AM_AVATAR_MINIO_SECRET_KEY=kamc-minio-secret
   MINIO_ROOT_USER=kamc
   MINIO_ROOT_PASSWORD=kamc-minio-secret
   ```

2. 启动时带 `minio` profile：

   ```bash
   docker compose -f deploy/docker/compose.yaml --profile minio up -d --build
   # 或：just compose-up-minio
   ```

3. 控制台 http://localhost:9001（账号同上）；平台启动时自动创建
   `AM_AVATAR_MINIO_BUCKET`（默认 `kamc-avatars`）。

`AM_AVATAR_BACKEND=auto` 时：配置了 `AM_AVATAR_MINIO_ENDPOINT` 即走 MinIO，
否则使用本地卷。

## 说明

- 未配置 `JWT_KEYRING_DIR`，平台使用临时签名密钥，**容器重启后需要重新登录**。
  需要保持会话时，在 compose 中挂载一个含 `jwt-1.pem` 的目录并设置
  `JWT_KEYRING_DIR`。
- `AM_ENV=development` 允许内置账号密码登录；生产必须改为 `production` 并配置
  `AM_AVATAR_SECRET`、关闭密码登录或更换默认密码。
- 平台镜像由仓库根 `Dockerfile` 构建，已包含 `data/web` 前端产物。
- 时区、审计 syslog、GeoIP、LDAP 等配置可通过 `.env` 追加同名环境变量并加入
  `compose.yaml` 的 `environment` 段。
