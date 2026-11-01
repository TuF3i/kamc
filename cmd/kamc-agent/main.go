// kamc-agent Agent 入口：装载环境变量配置，启动拨出连接循环，信号触发优雅断开。
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/TuF3i/kamc/internal/agent"
)

func main() {
	rt, err := agent.CreateRuntime()
	if err != nil {
		log.Fatalf("[agent] 初始化失败: %v", err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("[agent] 收到 %s，开始断开", sig)

	_ = rt.ReleaseRuntime()
	log.Printf("[agent] 已退出")
}
