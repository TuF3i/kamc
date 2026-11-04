# go 不在系统 PATH 中（安装于 /usr/local/go/bin），在此显式补充
export PATH := /usr/local/go/bin:$(PATH)

# 版本与提交：VERSION 可用 `make release VERSION=v1.0.0` 指定（同时作为 Agent 镜像 tag）
VERSION ?= dev
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -X github.com/TuF3i/kamc/internal/version.Version=$(VERSION) \
           -X github.com/TuF3i/kamc/internal/version.Commit=$(COMMIT)

WEB_DIR  := web
WEB_OUT  := data/web

.PHONY: build release web platform agent k8s-am-login image-platform image-agent test vet fmt tidy clean

# 仅后端三件套（开发迭代用）
build: platform agent k8s-am-login

# 一键生产构建：先构建前端（data/web），再编译后端；
# 运行时后端从 AM_WEB_DIR（默认 data/web）读取前端产物，同一进程托管页面与 API。
release: web platform agent k8s-am-login

web: $(WEB_OUT)/index.html

# 前端构建（npm ci 优先，保证 lockfile 一致；产物时间戳作为增量依据）
$(WEB_OUT)/index.html: $(shell find $(WEB_DIR)/src $(WEB_DIR)/public $(WEB_DIR)/index.html $(WEB_DIR)/package.json $(WEB_DIR)/vite.config.ts $(WEB_DIR)/tsconfig*.json 2>/dev/null)
	cd $(WEB_DIR) && (test -f package-lock.json && npm ci || npm install)
	cd $(WEB_DIR) && npm run build

platform:
	go build -ldflags "$(LDFLAGS)" -o bin/kamc-platform ./cmd/kamc-platform

agent:
	go build -ldflags "$(LDFLAGS)" -o bin/kamc-agent ./cmd/kamc-agent

k8s-am-login:
	go build -ldflags "$(LDFLAGS)" -o bin/k8s-am-login ./cmd/k8s-am-login

# 容器镜像（需要本机 docker；tag 默认 VERSION）
image-platform:
	docker build -t kamc-platform:$(VERSION) \
		--build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -f Dockerfile .

image-agent:
	docker build -t kamc-agent:$(VERSION) \
		--build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -f Dockerfile.agent .

test:
	go test -count=1 ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

tidy:
	go mod tidy

clean:
	rm -rf bin
