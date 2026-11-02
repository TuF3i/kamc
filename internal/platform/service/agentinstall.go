package service

import (
	"context"
	"strings"
	"text/template"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

// AgentInstallParams 渲染 Agent 一键安装清单的参数（路由层收集，业务在此层落地）。
type AgentInstallParams struct {
	Token       string // Agent 长期 Token 明文（仅签发响应中出现，平台只存哈希）
	PlatformURL string // 平台对外地址（http/https，渲染时转为 ws/wss 并拼 /tunnel）
	ImageRepo   string // Agent 镜像仓库完整地址（不含 tag）
	ImageTag    string // 镜像 tag（与平台版本一致）
}

// agentInstallTpl 与 deploy/agent/manifests.yaml 保持一致（§11.3 + §6.2）：
// 双副本 HA + 反亲和打散，Token 经 Secret 挂载，Agent 只做字节转发。
var agentInstallTpl = template.Must(template.New("agent-install").Parse(`# kamc Agent 一键安装清单（由平台生成）
# 集群：{{.ClusterID}} · 镜像：{{.Image}} · 隧道：{{.TunnelURL}}
# 说明：replicas=2（双隧道 HA），podAntiAffinity 打散节点；
#       平台证书为自签时给 Deployment 增加 PLATFORM_INSECURE=1 环境变量（仅开发/POC）。

apiVersion: v1
kind: Namespace
metadata:
  name: k8s-am-system
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: k8s-am-agent
  namespace: k8s-am-system
---
# Agent 用 SA Token 访问 kube-apiserver（Impersonation + RBAC 第二道防线，§8）
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: k8s-am-agent
subjects:
- kind: ServiceAccount
  name: k8s-am-agent
  namespace: k8s-am-system
roleRef:
  kind: ClusterRole
  name: cluster-admin
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: v1
kind: Secret
metadata:
  name: k8s-am-agent-token
  namespace: k8s-am-system
type: Opaque
stringData:
  token: "{{.Token}}"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: k8s-am-agent
  namespace: k8s-am-system
  labels: {app: k8s-am-agent}
spec:
  replicas: 2                              # 双隧道 HA（§7.4）
  strategy:
    rollingUpdate: {maxSurge: 1, maxUnavailable: 0}
  selector:
    matchLabels: {app: k8s-am-agent}
  template:
    metadata:
      labels: {app: k8s-am-agent}
    spec:
      serviceAccountName: k8s-am-agent
      terminationGracePeriodSeconds: 30
      affinity:
        podAntiAffinity:                    # 双副本打散节点
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              topologyKey: kubernetes.io/hostname
              labelSelector:
                matchLabels: {app: k8s-am-agent}
      containers:
      - name: agent
        image: {{.Image}}
        env:
        - {name: PLATFORM_URL, value: "{{.TunnelURL}}"}
        - {name: CLUSTER_ID, value: "{{.ClusterID}}"}
        - {name: AGENT_ID, valueFrom: {fieldRef: {fieldPath: metadata.name}}}  # 双副本须不同
        - {name: TOKEN_FILE, value: "/etc/k8s-am/token/token"}
        - {name: HEARTBEAT_PING_INTERVAL, value: "20s"}   # §6.3：Nginx /tunnel 读超时须 > 此值
        - {name: BACKOFF_BASE, value: "1s"}
        - {name: BACKOFF_MAX, value: "30s"}
        # - {name: PLATFORM_INSECURE, value: "1"}         # 仅开发/自签证书
        # - {name: APISERVER_URL, value: "https://..."}   # 显式覆盖 in-cluster 探测
        volumeMounts:
        - {name: token, mountPath: /etc/k8s-am/token, readOnly: true}
        resources:
          requests: {cpu: 100m, memory: 128Mi}
          limits: {cpu: 500m, memory: 512Mi}
        securityContext:
          runAsNonRoot: true
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities: {drop: [ALL]}
      volumes:
      - name: token
        secret: {secretName: k8s-am-agent-token}
`))

// tunnelURL 平台对外地址 → Agent 拨号的隧道 WebSocket 入口（ws/wss + /tunnel）。
func tunnelURL(platformURL string) string {
	s := strings.TrimSpace(platformURL)
	s = strings.TrimSuffix(s, "/")
	switch {
	case strings.HasPrefix(s, "https://"):
		s = "wss://" + strings.TrimPrefix(s, "https://")
	case strings.HasPrefix(s, "http://"):
		s = "ws://" + strings.TrimPrefix(s, "http://")
	}
	return s + "/tunnel"
}

// AgentInstallYAML 校验 Agent Token 并渲染一键安装清单。
// 仅接受当前有效的长期 Token（注册 Token 是一次性交换凭证，不能直接装 Agent）；
// Token 归属集群即安装目标，调用方无需（也无权）另行指定。
func (s *Service) AgentInstallYAML(ctx context.Context, params *AgentInstallParams) (*modules.AgentInstallPreview, *errcode.Response) {
	if params == nil || params.Token == "" || params.ImageRepo == "" || params.PlatformURL == "" {
		return nil, errcode.ErrInvalidParam
	}
	clusterID, kind, err := s.LookupToken(ctx, params.Token)
	if err != nil || kind != modules.TokenKindLongterm {
		return nil, errcode.ErrTokenNotFound
	}

	image := strings.TrimSuffix(params.ImageRepo, "/") + ":" + params.ImageTag
	tunnel := tunnelURL(params.PlatformURL)
	data := struct {
		ClusterID string
		Image     string
		TunnelURL string
		Token     string
	}{
		ClusterID: clusterID,
		Image:     image,
		TunnelURL: tunnel,
		Token:     params.Token,
	}
	var sb strings.Builder
	if err := agentInstallTpl.Execute(&sb, data); err != nil {
		return nil, errcode.ServerInternal(err)
	}
	return &modules.AgentInstallPreview{
		ClusterID:    clusterID,
		Image:        image,
		TunnelURL:    tunnel,
		ApplyCommand: "kubectl apply -f \"" + strings.TrimSuffix(params.PlatformURL, "/") + "/agent-install.yaml?token=" + params.Token + "\"",
		YAML:         sb.String(),
	}, nil
}
