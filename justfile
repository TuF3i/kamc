# kamc 日常命令入口（just）。Makefile 保留作为兼容入口，两者目标一致。
#
# 常用：
#   just build                     # 后端三件套（开发迭代）
#   just release                   # 生产构建：前端 data/web + 后端
#   just test                      # 全量 Go 测试
#   just compose-up                # 本地 compose 起平台（PG/Redis）
#   just compose-up-minio          # 额外启用 MinIO（需在 deploy/docker/.env 配置）
#   just helm-lint                 # 转发到 deploy/helm/Makefile

set shell := ["bash", "-c"]

version := env_var_or_default("VERSION", "dev")
commit := `git rev-parse --short HEAD 2>/dev/null || echo none`
ldflags := "-X github.com/TuF3i/kamc/internal/version.Version=" + version + " -X github.com/TuF3i/kamc/internal/version.Commit=" + commit

# Go 不在系统 PATH 中（安装于 /usr/local/go/bin），显式补充
export PATH := "/usr/local/go/bin:" + env_var_or_default("PATH", "/usr/local/bin:/usr/bin:/bin")

# 列出可用命令
default:
    @just --list

# 构建后端三件套（platform / agent / kamc）
build: platform agent kamc

# 生产构建：先构建前端到 data/web，再编译后端
release: web build

# 构建前端（npm ci 优先，失败回退 npm install）
web:
    cd web && if test -f package-lock.json; then npm ci; else npm install; fi
    cd web && npm run build

# 编译平台
platform:
    go build -ldflags "{{ldflags}}" -o bin/kamc-platform ./cmd/kamc-platform

# 编译 Agent
agent:
    go build -ldflags "{{ldflags}}" -o bin/kamc-agent ./cmd/kamc-agent

# 编译 kubectl exec 插件
kamc:
    go build -ldflags "{{ldflags}}" -o bin/kamc ./cmd/kamc

# 跨平台构建 exec 插件（默认 linux/windows amd64；KAMC_TARGETS 可覆盖）
kamc-cross:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p bin
    targets="${KAMC_TARGETS:-linux/amd64 windows/amd64}"
    for target in $targets; do
      os="${target%/*}"
      arch="${target#*/}"
      ext=""
      if [ "$os" = "windows" ]; then ext=".exe"; fi
      out="bin/kamc-${os}-${arch}${ext}"
      echo "==> $out"
      GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 go build -trimpath -ldflags "{{ldflags}}" -o "$out" ./cmd/kamc
    done

# 全量测试（数据库用例需 TEST_PG_DSN）
test:
    go test -count=1 ./...

# 静态检查
vet:
    go vet ./...

# 格式化（就地写入）
fmt:
    gofmt -l -w .

# 检查格式（不写入，CI 可用）
fmt-check:
    test -z "$(gofmt -l .)"

# 整理依赖
tidy:
    go mod tidy

# 清理构建产物
clean:
    rm -rf bin

# 构建平台镜像
image-platform:
    docker build -t kamc-platform:{{version}} \
      --build-arg VERSION={{version}} --build-arg COMMIT={{commit}} \
      -f Dockerfile .

# 构建 Agent 镜像
image-agent:
    docker build -t kamc-agent:{{version}} \
      --build-arg VERSION={{version}} --build-arg COMMIT={{commit}} \
      -f Dockerfile.agent .

# 前端开发（连接平台，默认 127.0.0.1:8080）
dev-web:
    cd web && npm run dev

# 前端开发（mock 模式，无需后端）
dev-web-mock:
    cd web && npm run dev:mock

# Helm：语法检查
helm-lint:
    make -C deploy/helm lint

# Helm：渲染两个 Chart 到 deploy/helm/dist
helm-template:
    make -C deploy/helm template

# Helm：创建 namespace 与 Secret（幂等，FORCE=1 轮换）
helm-secrets:
    make -C deploy/helm secrets

# Helm：安装/升级平台（EXTERNAL_URL 等通过环境变量传入）
helm-install-platform:
    make -C deploy/helm install-platform

# Helm：安装/升级 Agent（CLUSTER_ID / PLATFORM_URL 通过环境变量传入）
helm-install-agent:
    make -C deploy/helm install-agent

# Helm：查看平台与 Agent 状态
helm-status:
    make -C deploy/helm status

# Helm：查看平台日志
helm-logs-platform:
    make -C deploy/helm logs-platform

# Helm：查看 Agent 日志
helm-logs-agent:
    make -C deploy/helm logs-agent

# Docker Compose：启动本地平台（PostgreSQL + Redis）
compose-up:
    docker compose -f deploy/docker/compose.yaml up -d --build

# Docker Compose：启动本地平台并启用 MinIO profile
compose-up-minio:
    docker compose -f deploy/docker/compose.yaml --profile minio up -d --build

# Docker Compose：启动本地平台 + 观测栈（Prometheus/Loki/Promtail/Grafana）
compose-up-observability:
    docker compose -f deploy/docker/compose.yaml --profile observability up -d --build

# Docker Compose：停止并移除容器
compose-down:
    docker compose -f deploy/docker/compose.yaml down

# Docker Compose：跟踪日志
compose-logs:
    docker compose -f deploy/docker/compose.yaml logs -f

# Docker Compose：校验并输出最终配置
compose-config:
    docker compose -f deploy/docker/compose.yaml config
