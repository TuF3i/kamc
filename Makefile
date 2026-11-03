# go 不在系统 PATH 中（安装于 /usr/local/go/bin），在此显式补充
export PATH := /usr/local/go/bin:$(PATH)

# 版本与提交：VERSION 可用 `make release VERSION=v1.0.0` 指定（同时作为 Agent 镜像 tag）
VERSION ?= dev
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -X github.com/TuF3i/kamc/internal/version.Version=$(VERSION) \
           -X github.com/TuF3i/kamc/internal/version.Commit=$(COMMIT)

WEB_DIR   := web
WEB_DIST  := $(WEB_DIR)/dist
EMBED_DIR := internal/platform/webui/dist

.PHONY: build release web embed-web platform agent k8s-am-login test vet fmt tidy clean

# 仅后端三件套（开发迭代用，不重编前端）
build: platform agent k8s-am-login

# 一键生产构建：前端构建 → 复制产物进 embed 目录 → 编译后端（单二进制含前端）
release: embed-web platform agent k8s-am-login

web: $(WEB_DIST)/index.html

# 前端构建（npm ci 优先，保证 lockfile 一致；产物时间戳作为增量依据）
$(WEB_DIST)/index.html: $(shell find $(WEB_DIR)/src $(WEB_DIR)/public $(WEB_DIR)/index.html $(WEB_DIR)/package.json $(WEB_DIR)/vite.config.ts $(WEB_DIR)/tsconfig*.json 2>/dev/null)
	cd $(WEB_DIR) && (test -f package-lock.json && npm ci || npm install)
	cd $(WEB_DIR) && npm run build

# 把 web/dist 复制到 Go embed 目录（embed 不允许引用包目录之外的文件）
embed-web: web
	rm -rf $(EMBED_DIR)
	mkdir -p $(EMBED_DIR)
	cp -r $(WEB_DIST)/. $(EMBED_DIR)/
	touch $(EMBED_DIR)/.gitkeep
	@echo "embedded web assets: $$(du -sh $(EMBED_DIR) | cut -f1)"

platform:
	go build -ldflags "$(LDFLAGS)" -o bin/kamc-platform ./cmd/kamc-platform

agent:
	go build -ldflags "$(LDFLAGS)" -o bin/kamc-agent ./cmd/kamc-agent

k8s-am-login:
	go build -ldflags "$(LDFLAGS)" -o bin/k8s-am-login ./cmd/k8s-am-login

test:
	go test -count=1 ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

tidy:
	go mod tidy

# 清理二进制与嵌入产物（保留 .gitkeep 以便 go:embed 编译）
clean:
	rm -rf bin
	rm -rf $(EMBED_DIR)
	mkdir -p $(EMBED_DIR)
	touch $(EMBED_DIR)/.gitkeep
