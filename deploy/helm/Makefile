# kamc Helm 部署 Makefile
#
# 常用：
#   make secrets                 # 创建 namespace + JWT/平台 Secret（幂等）
#   make install-platform EXTERNAL_URL=https://kamc.example.com
#   make install-agent CLUSTER_ID=prod-bj PLATFORM_URL=wss://kamc.example.com/tunnel
#   make status
#
# 敏感值通过环境变量传入（不要写进命令行历史）：
#   export DATABASE_URL='postgres://kamc:***@postgres:5432/kamc?sslmode=require'
#   export OIDC_CLIENT_ID=kamc OIDC_CLIENT_SECRET=*** PASSWORD_LOGIN_PASSWORD=***
#   # 可选：启用 MinIO 头像存储时同时传入凭据
#   export MINIO_ACCESS_KEY=*** MINIO_SECRET_KEY=***
#
# Docker 镜像需提前构建并推送到集群可访问的仓库：
#   make -C ../.. image-platform VERSION=v1.0.0

SHELL := /bin/bash

HELM    ?= helm
KUBECTL ?= kubectl

# 命名空间与 release 名
NAMESPACE        ?= k8s-am-system
PLATFORM_RELEASE ?= kamc
AGENT_RELEASE    ?= kamc-agent

# Chart 目录（相对本 Makefile）
PLATFORM_CHART ?= $(CURDIR)/kamc-platform
AGENT_CHART    ?= $(CURDIR)/kamc-agent

# 镜像与版本
VERSION        ?= dev
PLATFORM_IMAGE ?= kamc/k8s-am-platform
AGENT_IMAGE    ?= kamc/k8s-am-agent

# Secret 名称（与 Chart 的 existingSecret 对应）
PLATFORM_SECRET ?= kamc-secrets
JWT_SECRET      ?= kamc-jwt
AGENT_SECRET    ?= kamc-agent-token

# 可选 MinIO 头像存储（设置 MINIO_ENDPOINT 即启用；凭据由 secrets 目标写入）
MINIO_ENDPOINT        ?=
MINIO_BUCKET          ?=
MINIO_USE_SSL         ?=
MINIO_EXISTING_SECRET ?=

# 平台参数
EXTERNAL_URL ?=
REDIS_ADDR   ?=
OIDC_ISSUER  ?=
INGRESS_HOST ?=

# Agent 参数
CLUSTER_ID   ?=
PLATFORM_URL ?=

# 额外 values 文件（可多个：VALUES="a.yaml b.yaml"）
VALUES ?=
# 覆盖 secrets 目标已存在的 Secret（会轮换密钥，谨慎）
FORCE ?=
# 渲染/安装 dry-run：DRY_RUN=server 时 helm 只做服务端校验，不创建资源
DRY_RUN ?=

# 渲染/打包产物目录
DIST := $(CURDIR)/dist

PLATFORM_VALUES_FLAGS := $(foreach f,$(VALUES),-f $(f))
HELM_DRY_RUN := $(if $(DRY_RUN),--dry-run=$(DRY_RUN),)

.PHONY: help lint template template-platform template-agent package \
        secrets install-platform upgrade-platform install-agent upgrade-agent \
        uninstall-platform uninstall-agent status logs-platform logs-agent clean

.DEFAULT_GOAL := help

help: ## 显示可用目标
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

lint: ## helm lint 两个 Chart（使用 ci/test-values.yaml）
	$(HELM) lint $(PLATFORM_CHART) -f $(PLATFORM_CHART)/ci/test-values.yaml
	$(HELM) lint $(AGENT_CHART) -f $(AGENT_CHART)/ci/test-values.yaml

template: template-platform template-agent ## 渲染两个 Chart 到 dist/（仅校验，不安装）

template-platform: ## 渲染平台 Chart 到 dist/kamc-platform.yaml
	@mkdir -p $(DIST)
	$(HELM) template $(PLATFORM_RELEASE) $(PLATFORM_CHART) -n $(NAMESPACE) \
		-f $(PLATFORM_CHART)/ci/test-values.yaml $(PLATFORM_VALUES_FLAGS) > $(DIST)/kamc-platform.yaml
	@echo "rendered: $(DIST)/kamc-platform.yaml"

template-agent: ## 渲染 Agent Chart 到 dist/kamc-agent.yaml
	@mkdir -p $(DIST)
	$(HELM) template $(AGENT_RELEASE) $(AGENT_CHART) -n $(NAMESPACE) \
		-f $(AGENT_CHART)/ci/test-values.yaml $(PLATFORM_VALUES_FLAGS) > $(DIST)/kamc-agent.yaml
	@echo "rendered: $(DIST)/kamc-agent.yaml"

package: ## 打包两个 Chart 到 dist/
	@mkdir -p $(DIST)
	$(HELM) package $(PLATFORM_CHART) -d $(DIST)
	$(HELM) package $(AGENT_CHART) -d $(DIST)

# 创建 namespace 与平台所需 Secret（幂等；FORCE=1 时轮换）
secrets: ## 创建 namespace/JWT/平台 Secret（需要 DATABASE_URL 环境变量）
	@command -v openssl >/dev/null || { echo "openssl is required" >&2; exit 1; }
	@: $${DATABASE_URL:?DATABASE_URL is required, e.g. export DATABASE_URL='postgres://...'}
	@$(KUBECTL) create namespace $(NAMESPACE) --dry-run=client -o yaml | $(KUBECTL) apply -f -
	@if $(KUBECTL) -n $(NAMESPACE) get secret $(JWT_SECRET) >/dev/null 2>&1 && [ "$(FORCE)" != "1" ]; then \
		echo "secret/$(JWT_SECRET) exists, skip (FORCE=1 to rotate)"; \
	else \
		tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
		openssl genrsa -out "$$tmp/jwt-1.pem" 2048; \
		$(KUBECTL) -n $(NAMESPACE) create secret generic $(JWT_SECRET) \
			--from-file=jwt-1.pem="$$tmp/jwt-1.pem" \
			--dry-run=client -o yaml | $(KUBECTL) apply -f -; \
		echo "secret/$(JWT_SECRET) applied"; \
	fi
	@if $(KUBECTL) -n $(NAMESPACE) get secret $(PLATFORM_SECRET) >/dev/null 2>&1 && [ "$(FORCE)" != "1" ]; then \
		echo "secret/$(PLATFORM_SECRET) exists, skip (FORCE=1 to rotate)"; \
	else \
		peer=$${PEER_TOKEN:-$$(openssl rand -hex 32)}; \
		avatar=$${AVATAR_SECRET:-$$(openssl rand -hex 32)}; \
		minio_args=""; \
		if [ -n "$$MINIO_ACCESS_KEY" ] && [ -n "$$MINIO_SECRET_KEY" ]; then \
			minio_args="--from-literal=minio-access-key=$$MINIO_ACCESS_KEY --from-literal=minio-secret-key=$$MINIO_SECRET_KEY"; \
		fi; \
		$(KUBECTL) -n $(NAMESPACE) create secret generic $(PLATFORM_SECRET) \
			--from-literal=database-url="$$DATABASE_URL" \
			--from-literal=oidc-client-id="$${OIDC_CLIENT_ID:-}" \
			--from-literal=oidc-client-secret="$${OIDC_CLIENT_SECRET:-}" \
			--from-literal=peer-token="$$peer" \
			--from-literal=avatar-secret="$$avatar" \
			--from-literal=password-login-password="$${PASSWORD_LOGIN_PASSWORD:-}" \
			$$minio_args \
			--dry-run=client -o yaml | $(KUBECTL) apply -f -; \
		echo "secret/$(PLATFORM_SECRET) applied"; \
	fi

install-platform: upgrade-platform ## 安装/升级平台（helm upgrade --install）

upgrade-platform: ## 安装/升级平台（需要 EXTERNAL_URL）
	@test -n "$(EXTERNAL_URL)" || { echo "EXTERNAL_URL is required, e.g. make install-platform EXTERNAL_URL=https://kamc.example.com" >&2; exit 1; }
	@if [ -n "$(PLATFORM_SECRET)" ]; then \
		$(KUBECTL) -n $(NAMESPACE) get secret $(PLATFORM_SECRET) >/dev/null 2>&1 || { echo "secret/$(PLATFORM_SECRET) not found: run 'make secrets' first" >&2; exit 1; }; \
		$(KUBECTL) -n $(NAMESPACE) get secret $(JWT_SECRET) >/dev/null 2>&1 || { echo "secret/$(JWT_SECRET) not found: run 'make secrets' first" >&2; exit 1; }; \
	fi
	$(HELM) upgrade --install $(PLATFORM_RELEASE) $(PLATFORM_CHART) \
		-n $(NAMESPACE) --create-namespace --timeout 5m $(HELM_DRY_RUN) \
		--set image.repository=$(PLATFORM_IMAGE) \
		--set image.tag=$(VERSION) \
		--set config.externalURL=$(EXTERNAL_URL) \
		$(if $(REDIS_ADDR),--set config.redisAddr=$(REDIS_ADDR),) \
		$(if $(OIDC_ISSUER),--set oidc.issuer=$(OIDC_ISSUER),) \
		$(if $(PLATFORM_SECRET),--set secrets.existingSecret=$(PLATFORM_SECRET),) \
		$(if $(JWT_SECRET),--set jwtKeyring.existingSecret=$(JWT_SECRET),) \
		$(if $(MINIO_ENDPOINT),--set avatar.minio.endpoint=$(MINIO_ENDPOINT),) \
		$(if $(MINIO_BUCKET),--set avatar.minio.bucket=$(MINIO_BUCKET),) \
		$(if $(MINIO_USE_SSL),--set avatar.minio.useSSL=$(MINIO_USE_SSL),) \
		$(if $(MINIO_EXISTING_SECRET),--set avatar.minio.existingSecret=$(MINIO_EXISTING_SECRET),) \
		$(if $(INGRESS_HOST),--set ingress.enabled=true --set "ingress.hosts[0].host=$(INGRESS_HOST)",) \
		$(PLATFORM_VALUES_FLAGS)

install-agent: upgrade-agent ## 安装/升级 Agent（helm upgrade --install）

upgrade-agent: ## 安装/升级 Agent（需要 CLUSTER_ID/PLATFORM_URL）
	@test -n "$(CLUSTER_ID)" || { echo "CLUSTER_ID is required" >&2; exit 1; }
	@test -n "$(PLATFORM_URL)" || { echo "PLATFORM_URL is required, e.g. wss://kamc.example.com/tunnel" >&2; exit 1; }
	@if [ -n "$(AGENT_SECRET)" ]; then \
		$(KUBECTL) -n $(NAMESPACE) get secret $(AGENT_SECRET) >/dev/null 2>&1 || { echo "secret/$(AGENT_SECRET) not found: create it with the token issued by the platform" >&2; exit 1; }; \
	fi
	$(HELM) upgrade --install $(AGENT_RELEASE) $(AGENT_CHART) \
		-n $(NAMESPACE) --create-namespace --timeout 5m $(HELM_DRY_RUN) \
		--set image.repository=$(AGENT_IMAGE) \
		--set image.tag=$(VERSION) \
		--set clusterID=$(CLUSTER_ID) \
		--set platformURL=$(PLATFORM_URL) \
		$(if $(AGENT_SECRET),--set existingSecret=$(AGENT_SECRET),) \
		$(PLATFORM_VALUES_FLAGS)

uninstall-platform: ## 卸载平台（不删除 PVC/Secret）
	$(HELM) uninstall $(PLATFORM_RELEASE) -n $(NAMESPACE)

uninstall-agent: ## 卸载 Agent（不删除 Secret）
	$(HELM) uninstall $(AGENT_RELEASE) -n $(NAMESPACE)

status: ## 查看平台/Agent 工作负载与 Pod
	@echo "== platform =="; $(KUBECTL) -n $(NAMESPACE) get statefulset,svc,pods -l app.kubernetes.io/instance=$(PLATFORM_RELEASE) 2>/dev/null || true
	@echo "== agent =="; $(KUBECTL) -n $(NAMESPACE) get deploy,pods -l app.kubernetes.io/instance=$(AGENT_RELEASE) 2>/dev/null || true

logs-platform: ## 跟踪平台日志
	$(KUBECTL) -n $(NAMESPACE) logs -l app.kubernetes.io/instance=$(PLATFORM_RELEASE) --tail=100 -f

logs-agent: ## 跟踪 Agent 日志
	$(KUBECTL) -n $(NAMESPACE) logs -l app.kubernetes.io/instance=$(AGENT_RELEASE) --tail=100 -f

clean: ## 清理渲染/打包产物
	rm -rf $(DIST)
