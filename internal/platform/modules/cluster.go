// Package modules 平台模块的纯数据模型：gorm 模型 / DTO / 常量，不含业务逻辑，
// 不 import 同模块其他包（叶包约定）。
package modules

import "time"

// ClusterStatus 集群接入状态
type ClusterStatus string

const (
	ClusterStatusPending ClusterStatus = "pending" // 已注册，Agent 尚未接入
	ClusterStatusOnline  ClusterStatus = "online"  // 至少一条隧道在线
	ClusterStatusOffline ClusterStatus = "offline" // 隧道全部离线
)

// Cluster 集群注册表：平台纳管的每个目标集群一行（Casbin dom 维度）。
type Cluster struct {
	ID        string        `gorm:"primaryKey;size:64" json:"id"` // 集群 ID（Casbin dom）
	Name      string        `gorm:"size:128;not null" json:"name"`
	Desc      string        `gorm:"size:512" json:"desc"`
	Status    ClusterStatus `gorm:"size:16;not null;default:pending" json:"status"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// CreateClusterRequest POST /api/v1/clusters 请求体
type CreateClusterRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// ClusterDTO 对外输出的集群信息（未来敏感字段在此收敛，不直接出 gorm 模型）
type ClusterDTO struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Desc      string        `json:"desc"`
	Status    ClusterStatus `json:"status"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// ToDTO 转对外 DTO（哈希/密钥等敏感字段一旦出现，置空必须发生在 service 层、本转换之前）
func (c *Cluster) ToDTO() *ClusterDTO {
	return &ClusterDTO{
		ID:        c.ID,
		Name:      c.Name,
		Desc:      c.Desc,
		Status:    c.Status,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

// ClusterVersionInfo 目标集群 kube-apiserver /version 信息（经 Agent 隧道读取）
type ClusterVersionInfo struct {
	Major      string `json:"major"`
	Minor      string `json:"minor"`
	GitVersion string `json:"git_version"`
	GitCommit  string `json:"git_commit"`
	BuildDate  string `json:"build_date"`
	GoVersion  string `json:"go_version"`
	Compiler   string `json:"compiler"`
	Platform   string `json:"platform"`
}

// ClusterInfo 集群概要信息（kubectl cluster-info 语义）：版本 + 控制平面/DNS
// 地址。地址字段读不到时留空（前端显示 —），不视为失败。
type ClusterInfo struct {
	Version      *ClusterVersionInfo `json:"version"`
	ControlPlane string              `json:"control_plane,omitempty"` // default/kubernetes Endpoints（https://ip:port）
	CoreDNS      string              `json:"coredns,omitempty"`       // kube-system/kube-dns Service（https://clusterIP[:port]）
	Nodes        []ClusterNodeInfo   `json:"nodes,omitempty"`         // 集群节点列表（读取失败为空，不视为失败）
	FetchedAt    time.Time           `json:"fetched_at"`
}

// ClusterNodeInfo 一个集群节点的概要（kubectl get nodes -o wide 语义的字段子集）
type ClusterNodeInfo struct {
	Name           string   `json:"name"`
	Roles          []string `json:"roles,omitempty"` // node-role.kubernetes.io/* 标签（如 control-plane/worker）
	Ready          bool     `json:"ready"`           // Ready 条件 == True
	Unschedulable  bool     `json:"unschedulable,omitempty"`
	InternalIP     string   `json:"internal_ip,omitempty"`
	KubeletVersion string   `json:"kubelet_version,omitempty"`
	OSImage        string   `json:"os_image,omitempty"`
	Architecture   string   `json:"architecture,omitempty"`
	Runtime        string   `json:"runtime,omitempty"` // 容器运行时（containerd://…）
	CPU            string   `json:"cpu,omitempty"`     // capacity.cpu（如 4 / 3800m）
	Memory         string   `json:"memory,omitempty"`  // capacity.memory（已转 Gi 展示）
}

// AgentInstallPreview Agent 一键安装预览：YAML 亦可经 /agent-install.yaml?token=
// 被 kubectl 直接拉取（Token 即拉取凭证，与清单内嵌的 Agent Token 同源）
type AgentInstallPreview struct {
	ClusterID    string `json:"cluster_id"`
	Image        string `json:"image"`
	TunnelURL    string `json:"tunnel_url"`
	ApplyCommand string `json:"apply_command"`
	YAML         string `json:"yaml"`
}

// AgentInstallPreviewRequest POST /api/v1/clusters/{id}/agent-install 请求体
type AgentInstallPreviewRequest struct {
	Token string `json:"token"`
}
