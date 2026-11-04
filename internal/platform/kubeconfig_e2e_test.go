package platform_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	clientcmd "k8s.io/client-go/tools/clientcmd"
)

// TestKubeconfigEndToEnd 用户侧闭环（§4.4）：生成 kubeconfig（static/exec 两模式）
// → client-go 按产物构建客户端 → 经 /proxy 穿隧道到假 apiserver 完成 Pod 列表。
// 这是"管理员下载 → 用户 kubectl 可用"的自动化等价物。
func TestKubeconfigEndToEnd(t *testing.T) {
	// 假 apiserver：返回合法空 PodList，并捕获身份头
	var gotImpersonateUser, gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotImpersonateUser = r.Header.Get("Impersonate-User")
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"PodList","apiVersion":"v1","metadata":{"resourceVersion":"1"},"items":[]}`))
	}))
	defer upstream.Close()

	trt, rt, _ := setupTunnelPlatform(t, upstream.URL)
	srv := httptest.NewTLSServer(rt.Engine()) // kubectl 面走 https（明文连接 client-go 不发凭证）
	defer srv.Close()
	defer func() { _ = trt.ReleaseRuntime() }()
	defer func() { _ = rt.ReleaseRuntime() }()

	clusterID := fmt.Sprintf("e2e-kubeconfig-%d", time.Now().UnixNano())
	adminTok, agentToken := seedClusterAndPolicies(t, rt, srv, clusterID)
	startAgent(t, clusterID, "wss://"+srv.Listener.Addr().String()+"/tunnel", agentToken, upstream.URL)
	waitForClusterStatus(t, rt, adminTok, clusterID, "online", 5*time.Second)

	// 对外地址固定为 httptest 监听地址（生产由 AM_EXTERNAL_URL / X-Forwarded-Proto 决定）
	t.Setenv("AM_EXTERNAL_URL", srv.URL)

	// ── static 模式：产物可被 client-go 解析 ──
	resp := callRaw(t, rt, http.MethodGet, "/api/v1/clusters/"+clusterID+"/kubeconfig?mode=static", adminTok)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("kubeconfig static: %d", resp.StatusCode)
	}
	staticYAML := readAll(t, resp)

	// ── exec 模式：插件命令行正确 ──
	resp = callRaw(t, rt, http.MethodGet, "/api/v1/clusters/"+clusterID+"/kubeconfig?mode=exec", adminTok)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("kubeconfig exec: %d", resp.StatusCode)
	}
	execYAML := readAll(t, resp)
	for _, want := range []string{
		"command: k8s-am-login",
		"--server=" + srv.URL,
		"--cluster-id=" + clusterID,
		"client.authentication.k8s.io/v1",
	} {
		if !strings.Contains(string(execYAML), want) {
			t.Fatalf("exec kubeconfig missing %q:\n%s", want, execYAML)
		}
	}

	// ── 皇冠断言：static 产物直接驱动 client-go 走完整链路 ──
	apiCfg, err := clientcmd.Load(staticYAML)
	if err != nil {
		t.Fatalf("load kubeconfig: %v", err)
	}
	// 注意：程序化消费解析产物用 NewNonInteractiveClientConfig（不做文件合并）；
	// NewDefaultClientConfig 会把本机 ~/.kube/config 合并进来，覆盖身份与上下文。
	restCfg, err := clientcmd.NewNonInteractiveClientConfig(*apiCfg, apiCfg.CurrentContext, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
	if err != nil {
		t.Fatalf("rest config: %v", err)
	}
	if len(restCfg.BearerToken) == 0 {
		t.Fatal("https kubeconfig must carry the bearer token")
	}
	// 测试证书未嵌 CA（生产经 certificate-authority-data 信任）；
	// TLSClientConfig 为值类型，直接置位即可
	restCfg.TLSClientConfig.Insecure = true
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pods, err := clientset.CoreV1().Pods("dev").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list pods via kubeconfig: %v", err)
	}
	if len(pods.Items) != 0 {
		t.Fatalf("want empty PodList, got %d", len(pods.Items))
	}
	// 数据面语义：集群侧不注入 Impersonate、用户凭证不透传（Authorization 由 Agent 剥离）
	if gotImpersonateUser != "" {
		t.Fatalf("Impersonate-User should be empty, got %q", gotImpersonateUser)
	}
	if gotAuth != "" {
		t.Fatalf("Authorization leaked: %q", gotAuth)
	}

	// 未认证 → 401
	if unauth := callRaw(t, rt, http.MethodGet, "/api/v1/clusters/"+clusterID+"/kubeconfig"); unauth.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauth kubeconfig: %d", unauth.StatusCode)
	}
}

func readAll(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return b
}
