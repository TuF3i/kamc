# kamc 平台镜像：前端构建产物 data/web + 平台/exec 插件二进制
# 构建：docker build -t kamc-platform:dev --build-arg VERSION=dev --build-arg COMMIT=$(git rev-parse --short HEAD) .
ARG NODE_IMAGE=node:24-alpine
ARG GO_IMAGE=golang:1.26-alpine

FROM ${NODE_IMAGE} AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM ${GO_IMAGE} AS build
ARG VERSION=dev
ARG COMMIT=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/TuF3i/kamc/internal/version.Version=${VERSION} -X github.com/TuF3i/kamc/internal/version.Commit=${COMMIT}" \
      -o /out/kamc-platform ./cmd/kamc-platform \
 && CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/TuF3i/kamc/internal/version.Version=${VERSION} -X github.com/TuF3i/kamc/internal/version.Commit=${COMMIT}" \
      -o /out/k8s-am-login ./cmd/k8s-am-login

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -H -u 10001 kamc
WORKDIR /app
COPY --from=build /out/kamc-platform /out/k8s-am-login /app/
COPY --from=web /src/data/web /app/web
RUN mkdir -p /data/avatar && chown -R kamc:kamc /app /data
USER kamc
ENV AM_WEB_DIR=/app/web \
    AM_AVATAR_DIR=/data/avatar \
    AM_AUDIT_SPILL_FILE=/data/audit-spill.jsonl
EXPOSE 8080
ENTRYPOINT ["/app/kamc-platform"]
