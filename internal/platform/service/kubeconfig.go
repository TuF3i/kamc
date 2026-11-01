package service

import (
	"context"
	"fmt"
	"strings"

	clientcmd "k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/TuF3i/kamc/internal/platform/errcode"
)

// kubeconfig 模式（§4.4）：static 嵌入调用者自己的平台 JWT（过期需重新下载）；
// exec 用 k8s-am-login 插件按需换取（推荐）。
const (
	KubeconfigModeStatic = "static"
	KubeconfigModeExec   = "exec"
)

// KubeconfigParams 生成参数（全部由 handler 从请求与装配配置组装）
type KubeconfigParams struct {
	ClusterID    string
	Mode         string // static / exec（空默认 exec）
	ExternalURL  string // 平台对外地址（https://access-manager.example.com）
	CallerToken  string // 调用者自己的平台 JWT（static 模式嵌入；仅自服务）
	CallerSub    string // 调用者身份（user 名）
	CAData       []byte // 平台 CA（可选；公网 CA 证书链可省略）
	ExecIssuer   string // exec 插件参数：OIDC issuer（未配置 OIDC 时留空）
	ExecClientID string // exec 插件参数：OIDC client id
}

// GenerateKubeconfig 生成标准 kubeconfig YAML（§4.4）：
// server 指向平台 /proxy/{clusterID}，数据面鉴权在请求时由 Casbin 执行——
// kubeconfig 本身只承载身份，不含授权。
func (s *Service) GenerateKubeconfig(_ context.Context, p KubeconfigParams) ([]byte, *errcode.Response) {
	if _, rsp := s.getCluster(context.Background(), p.ClusterID); rsp != nil {
		return nil, rsp
	}
	switch p.Mode {
	case KubeconfigModeStatic:
		if p.CallerToken == "" {
			return nil, errcode.ErrInvalidParam
		}
	case KubeconfigModeExec, "":
		p.Mode = KubeconfigModeExec
	default:
		return nil, errcode.ErrInvalidParam
	}
	if p.ExternalURL == "" || p.CallerSub == "" {
		return nil, errcode.ErrInvalidParam
	}
	p.ExternalURL = strings.TrimRight(p.ExternalURL, "/")

	cfg := clientcmdapi.NewConfig()
	cfg.Clusters[p.ClusterID] = &clientcmdapi.Cluster{
		Server:                   p.ExternalURL + "/proxy/" + p.ClusterID,
		CertificateAuthorityData: p.CAData,
	}

	authInfo := &clientcmdapi.AuthInfo{}
	if p.Mode == KubeconfigModeStatic {
		authInfo.Token = p.CallerToken
	} else {
		args := []string{
			"--server=" + p.ExternalURL,
			"--cluster-id=" + p.ClusterID,
		}
		if p.ExecIssuer != "" {
			args = append(args, "--oidc-issuer="+p.ExecIssuer)
		}
		if p.ExecClientID != "" {
			args = append(args, "--oidc-client-id="+p.ExecClientID)
		}
		authInfo.Exec = &clientcmdapi.ExecConfig{
			APIVersion:      "client.authentication.k8s.io/v1",
			Command:         "k8s-am-login",
			Args:            args,
			InteractiveMode: clientcmdapi.IfAvailableExecInteractiveMode,
		}
	}
	cfg.AuthInfos[p.CallerSub] = authInfo
	cfg.Contexts[p.ClusterID] = &clientcmdapi.Context{Cluster: p.ClusterID, AuthInfo: p.CallerSub}
	cfg.CurrentContext = p.ClusterID

	data, err := clientcmd.Write(*cfg)
	if err != nil {
		return nil, errcode.ServerInternal(fmt.Errorf("service -> write kubeconfig: %w", err))
	}

	// 自校验：产物必须能被 client-go 标准解析（防结构漂移）
	if _, err := clientcmd.Load(data); err != nil {
		return nil, errcode.ServerInternal(fmt.Errorf("service -> validate kubeconfig: %w", err))
	}
	return data, nil
}
