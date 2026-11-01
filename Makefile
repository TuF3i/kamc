# go 不在系统 PATH 中（安装于 /usr/local/go/bin），在此显式补充
export PATH := /usr/local/go/bin:$(PATH)

COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -X github.com/TuF3i/kamc/internal/version.Commit=$(COMMIT)

.PHONY: build platform agent k8s-am-login test vet fmt tidy clean

build: platform agent k8s-am-login

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

clean:
	rm -rf bin
