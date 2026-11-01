// k8s-am-login 平台 kubeconfig 的 exec credential plugin（§4.4）：
// OIDC Device Flow → /auth/token-exchange → 向 stdout 输出 ExecCredential。
//
// 用法（由平台生成的 kubeconfig 自动调用）：
//
//	k8s-am-login --server=https://am.example.com --cluster-id=cluster-a \
//	    --oidc-issuer=https://authentik.example.com/application/o/kamc/ --oidc-client-id=kamc
//
// stdout 仅输出 ExecCredential JSON；全部提示走 stderr。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/TuF3i/kamc/internal/login"
)

func main() {
	var opts login.Options
	flag.StringVar(&opts.Server, "server", "", "平台地址（如 https://access-manager.example.com）")
	flag.StringVar(&opts.ClusterID, "cluster-id", "", "集群 ID（缓存键；平台 JWT 跨集群通用）")
	flag.StringVar(&opts.Issuer, "oidc-issuer", "", "OIDC issuer（Authentik application URL）")
	flag.StringVar(&opts.ClientID, "oidc-client-id", "", "OIDC client id")
	flag.StringVar(&opts.DeviceAuthUR, "device-auth-url", "", "设备授权端点覆盖（默认经 discovery）")
	flag.BoolVar(&opts.NoCache, "no-cache", false, "禁用本地凭证缓存")
	flag.BoolVar(&opts.Insecure, "insecure", false, "跳过 TLS 校验（仅开发环境）")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cred, err := login.Run(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "k8s-am-login: %v\n", err)
		os.Exit(1)
	}
	_, _ = os.Stdout.Write(cred)
	_, _ = os.Stdout.WriteString("\n")
}
