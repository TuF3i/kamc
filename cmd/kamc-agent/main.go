// kamc-agent Agent 入口：装载配置（环境变量/配置文件），启动拨出连接循环，信号触发优雅断开。
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/TuF3i/kamc/internal/agent"
)

func main() {
	configPath := flag.String("config", "", "配置文件路径（默认 AM_CONFIG_FILE，或 ./kamc-agent.yaml）")
	flag.Parse()

	cfg, err := agent.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("[agent] 配置装载失败: %v", err)
	}
	rt, err := agent.CreateRuntime(agent.WithConfig(cfg))
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
