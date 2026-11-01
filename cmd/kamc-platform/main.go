// kamc-platform 平台主控服务入口：装配隧道模块与 HTTP 服务，等待退出信号后优雅关停。
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/TuF3i/kamc/internal/platform"
	"github.com/TuF3i/kamc/internal/tunnel"
)

func main() {
	cfg := platform.LoadConfig()

	// 隧道模块：未配置 Redis 时降级启动（仅本地隧道，跨 Pod 路由不可用，§7.2）
	var trt *tunnel.Runtime
	if os.Getenv("REDIS_ADDRESSES") != "" || os.Getenv("REDIS_ADDR") != "" {
		var err error
		trt, err = tunnel.CreateRuntime()
		if err != nil {
			log.Fatalf("[platform] 隧道模块初始化失败: %v", err)
		}
	} else {
		log.Printf("[platform] 未配置 Redis，隧道模块未启用（跨 Pod 路由不可用）")
	}

	prt, err := platform.CreateRuntime(platform.WithAddr(cfg.Addr), platform.WithTunnels(trt))
	if err != nil {
		log.Fatalf("[platform] 初始化失败: %v", err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("[platform] 收到 %s，开始优雅关停", sig)

	// 释放顺序：先停平台（摘流 + 排空请求），再释放隧道（下线归属 + 关会话）
	_ = prt.ReleaseRuntime()
	if trt != nil {
		_ = trt.ReleaseRuntime()
	}
	log.Printf("[platform] 已退出")
}
