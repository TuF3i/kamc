// Package catalog 通过 Agent 隧道读取目标集群的 Kubernetes discovery 信息，
// 为策略编辑器提供 GVR + 可用动词 + namespace 列表。
package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
	"github.com/TuF3i/kamc/internal/platform/proxy"
)

// ClusterReader 只依赖集群存在性查询，避免 catalog 与 service 包耦合。
type ClusterReader interface {
	GetCluster(ctx context.Context, id string) (*modules.ClusterDTO, *errcode.Response)
}

// Resource 一条 GVR（subresource 为空表示主资源）。
type Resource struct {
	Group       string   `json:"group"`
	Version     string   `json:"version"`
	Resource    string   `json:"resource"`
	Subresource string   `json:"subresource,omitempty"`
	GVR         string   `json:"gvr"`
	Kind        string   `json:"kind"`
	Namespaced  bool     `json:"namespaced"`
	Verbs       []string `json:"verbs"`
}

// Catalog 一个集群的资源目录。
type Catalog struct {
	ClusterID           string     `json:"cluster_id"`
	Namespaces          []string   `json:"namespaces"`
	Resources           []Resource `json:"resources"`
	Partial             bool       `json:"partial"`
	FailedGroupVersions []string   `json:"failed_group_versions,omitempty"`
	Cached              bool       `json:"cached"`
	DiscoveredAt        time.Time  `json:"discovered_at"`
}

type cacheEntry struct {
	catalog *Catalog
	expires time.Time
}

// Service 资源目录服务（带 5 分钟内存缓存）。
type Service struct {
	clusters ClusterReader
	pool     *proxy.Pool

	mu    sync.Mutex
	cache map[string]cacheEntry
}

func New(clusters ClusterReader, pool *proxy.Pool) *Service {
	return &Service{clusters: clusters, pool: pool, cache: make(map[string]cacheEntry)}
}

// List 返回集群资源目录；refresh=true 绕过缓存。
func (s *Service) List(ctx context.Context, clusterID string, refresh bool) (*Catalog, *errcode.Response) {
	clusterID = strings.TrimSpace(clusterID)
	if clusterID == "" {
		return nil, errcode.ErrInvalidParam
	}
	if _, rsp := s.clusters.GetCluster(ctx, clusterID); rsp != nil {
		return nil, rsp
	}
	if s.pool == nil {
		return nil, errcode.ErrClusterUnreachable
	}

	now := time.Now()
	s.mu.Lock()
	if entry, ok := s.cache[clusterID]; ok && !refresh && now.Before(entry.expires) {
		cached := *entry.catalog
		cached.Cached = true
		s.mu.Unlock()
		return &cached, nil
	}
	s.mu.Unlock()

	catalog, rsp := s.discover(ctx, clusterID)
	if rsp != nil {
		return nil, rsp
	}
	s.mu.Lock()
	s.cache[clusterID] = cacheEntry{catalog: catalog, expires: time.Now().Add(5 * time.Minute)}
	s.mu.Unlock()
	return catalog, nil
}

// ServerVersion 读取目标集群 kube-apiserver 的 /version（经 Agent 隧道，
// 集群详情「集群详细信息」卡片展示用）。不缓存——版本低频读取且需反映实时可达性。
func (s *Service) ServerVersion(ctx context.Context, clusterID string) (*modules.ClusterVersionInfo, *errcode.Response) {
	clusterID = strings.TrimSpace(clusterID)
	if clusterID == "" {
		return nil, errcode.ErrInvalidParam
	}
	if _, rsp := s.clusters.GetCluster(ctx, clusterID); rsp != nil {
		return nil, rsp
	}
	if s.pool == nil {
		return nil, errcode.ErrClusterUnreachable
	}
	transport, err := s.pool.Transport(clusterID)
	if err != nil {
		return nil, errcode.ErrClusterUnreachable
	}
	base := s.pool.Location(clusterID)
	if base == nil {
		return nil, errcode.ErrClusterUnreachable
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.JoinPath("version").String(), nil)
	if err != nil {
		return nil, errcode.ServerInternal(fmt.Errorf("catalog -> version request: %w", err))
	}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errcode.ErrClusterUnreachable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errcode.ErrClusterUnreachable
	}

	var raw struct {
		Major      string `json:"major"`
		Minor      string `json:"minor"`
		GitVersion string `json:"gitVersion"`
		GitCommit  string `json:"gitCommit"`
		BuildDate  string `json:"buildDate"`
		GoVersion  string `json:"goVersion"`
		Compiler   string `json:"compiler"`
		Platform   string `json:"platform"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, errcode.ServerInternal(fmt.Errorf("catalog -> decode version: %w", err))
	}
	return &modules.ClusterVersionInfo{
		Major:      raw.Major,
		Minor:      raw.Minor,
		GitVersion: raw.GitVersion,
		GitCommit:  raw.GitCommit,
		BuildDate:  raw.BuildDate,
		GoVersion:  raw.GoVersion,
		Compiler:   raw.Compiler,
		Platform:   raw.Platform,
	}, nil
}

// Info 集群概要（kubectl cluster-info 语义）：版本 + 控制平面/DNS 地址。
// 地址读取失败只降级（字段留空），不影响整体返回。
func (s *Service) Info(ctx context.Context, clusterID string) (*modules.ClusterInfo, *errcode.Response) {
	version, rsp := s.ServerVersion(ctx, clusterID)
	if rsp != nil {
		return nil, rsp
	}
	info := &modules.ClusterInfo{Version: version, FetchedAt: time.Now()}

	transport, err := s.pool.Transport(clusterID)
	if err != nil {
		return info, nil
	}
	base := s.pool.Location(clusterID)
	if base == nil {
		return info, nil
	}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	// 控制平面地址：default/kubernetes Endpoints 的 subsets 地址（同 kubectl cluster-info）
	if ep, err := client.Get(base.JoinPath("api/v1/namespaces/default/endpoints/kubernetes").String()); err == nil {
		var body struct {
			Subsets []struct {
				Addresses []struct {
					IP string `json:"ip"`
				} `json:"addresses"`
				Ports []struct {
					Port int    `json:"port"`
					Name string `json:"name"`
				} `json:"ports"`
			} `json:"subsets"`
		}
		if json.NewDecoder(ep.Body).Decode(&body) == nil && len(body.Subsets) > 0 &&
			len(body.Subsets[0].Addresses) > 0 {
			port := 443
			if len(body.Subsets[0].Ports) > 0 && body.Subsets[0].Ports[0].Port != 0 {
				port = body.Subsets[0].Ports[0].Port
			}
			info.ControlPlane = "https://" + body.Subsets[0].Addresses[0].IP + ":" + strconv.Itoa(port)
		}
		ep.Body.Close()
	}

	// CoreDNS 地址：kube-system/kube-dns 的 Service ClusterIP（coredns/kube-dns 同义）
	if svc, err := client.Get(base.JoinPath("api/v1/namespaces/kube-system/services/kube-dns").String()); err == nil {
		var body struct {
			Spec struct {
				ClusterIP string `json:"clusterIP"`
				Ports     []struct {
					Port     int    `json:"port"`
					Protocol string `json:"protocol"`
					Name     string `json:"name"`
				} `json:"ports"`
			} `json:"spec"`
		}
		if json.NewDecoder(svc.Body).Decode(&body) == nil && body.Spec.ClusterIP != "" &&
			body.Spec.ClusterIP != "None" {
			dnsPort := 53
			for _, p := range body.Spec.Ports {
				if p.Protocol == "UDP" && p.Port != 0 {
					dnsPort = p.Port
					break
				}
			}
			info.CoreDNS = body.Spec.ClusterIP + ":" + strconv.Itoa(dnsPort)
		}
		svc.Body.Close()
	}

	// 节点列表（kubectl get nodes -o wide 子集）；失败仅降级为空列表
	info.Nodes = s.listNodes(ctx, clusterID)

	return info, nil
}

// listNodes 经隧道列出集群节点概要（失败返回空——节点展示是增强信息，不阻塞整体）
func (s *Service) listNodes(ctx context.Context, clusterID string) []modules.ClusterNodeInfo {
	transport, err := s.pool.Transport(clusterID)
	if err != nil {
		return nil
	}
	base := s.pool.Location(clusterID)
	if base == nil {
		return nil
	}
	cfg := &rest.Config{
		Host:      base.String(),
		Transport: transport,
		Timeout:   15 * time.Second,
	}
	kc, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil
	}
	listCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	nodeList, err := kc.CoreV1().Nodes().List(listCtx, metav1.ListOptions{})
	if err != nil {
		return nil
	}

	nodes := make([]modules.ClusterNodeInfo, 0, len(nodeList.Items))
	for _, n := range nodeList.Items {
		node := modules.ClusterNodeInfo{
			Name:          n.Name,
			Unschedulable: n.Spec.Unschedulable,
		}
		// 角色：node-role.kubernetes.io/<role> 标签（缺省视为 worker）
		for label := range n.Labels {
			if role, ok := strings.CutPrefix(label, "node-role.kubernetes.io/"); ok && role != "" {
				node.Roles = append(node.Roles, role)
			}
		}
		sort.Strings(node.Roles)
		if len(node.Roles) == 0 {
			node.Roles = []string{"worker"}
		}
		// Ready 条件
		for _, c := range n.Status.Conditions {
			if c.Type == v1.NodeReady {
				node.Ready = c.Status == v1.ConditionTrue
				break
			}
		}
		// 地址：优先 InternalIP
		for _, a := range n.Status.Addresses {
			if a.Type == v1.NodeInternalIP {
				node.InternalIP = a.Address
				break
			}
		}
		node.KubeletVersion = n.Status.NodeInfo.KubeletVersion
		node.OSImage = n.Status.NodeInfo.OSImage
		node.Architecture = n.Status.NodeInfo.Architecture
		node.Runtime = n.Status.NodeInfo.ContainerRuntimeVersion
		// 容量：CPU 原样；memory 向上取整转 Gi（8589934592 → 8 Gi）
		node.CPU = n.Status.Capacity.Cpu().String()
		if mem := n.Status.Capacity.Memory(); !mem.IsZero() {
			gi := mem.Value() / (1 << 30)
			if mem.Value()%(1<<30) > 0 {
				gi++
			}
			node.Memory = fmt.Sprintf("%d Gi", gi)
		}
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	return nodes
}

func (s *Service) discover(ctx context.Context, clusterID string) (*Catalog, *errcode.Response) {
	transport, err := s.pool.Transport(clusterID)
	if err != nil {
		return nil, errcode.ErrClusterUnreachable
	}
	base := s.pool.Location(clusterID)
	if base == nil {
		return nil, errcode.ErrClusterUnreachable
	}

	cfg := &rest.Config{
		Host:      base.String(), // 隧道内为明文 HTTP，TLS 在 Agent→apiserver 段
		Transport: transport,
		Timeout:   15 * time.Second,
		QPS:       50,
		Burst:     100,
	}
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, errcode.ServerInternal(fmt.Errorf("catalog -> discovery client: %w", err))
	}

	discoveryCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	lists, discoveryErr := dc.ServerPreferredResources()
	partial := false
	failed := make([]string, 0)
	if discoveryErr != nil {
		var groupErr *discovery.ErrGroupDiscoveryFailed
		if errors.As(discoveryErr, &groupErr) {
			partial = true
			for gv := range groupErr.Groups {
				failed = append(failed, gv.String())
			}
		} else {
			return nil, errcode.ErrDiscoveryFailed
		}
	}

	resources := make([]Resource, 0)
	for _, list := range lists {
		if list == nil {
			continue
		}
		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil {
			continue
		}
		for _, apiResource := range list.APIResources {
			if apiResource.Name == "" {
				continue
			}
			parent, subresource, _ := strings.Cut(apiResource.Name, "/")
			verbs := make([]string, 0, len(apiResource.Verbs))
			for _, v := range apiResource.Verbs {
				verbs = append(verbs, string(v))
			}
			sort.Strings(verbs)
			resources = append(resources, Resource{
				Group:       gv.Group,
				Version:     gv.Version,
				Resource:    parent,
				Subresource: subresource,
				GVR:         gv.String() + "/" + parent,
				Kind:        apiResource.Kind,
				Namespaced:  apiResource.Namespaced,
				Verbs:       verbs,
			})
		}
	}
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].Group != resources[j].Group {
			return resources[i].Group < resources[j].Group
		}
		if resources[i].Version != resources[j].Version {
			return resources[i].Version < resources[j].Version
		}
		if resources[i].Resource != resources[j].Resource {
			return resources[i].Resource < resources[j].Resource
		}
		return resources[i].Subresource < resources[j].Subresource
	})

	kc, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, errcode.ServerInternal(fmt.Errorf("catalog -> kube client: %w", err))
	}
	nsList, err := kc.CoreV1().Namespaces().List(discoveryCtx, metav1.ListOptions{Limit: 500})
	if err != nil {
		return nil, errcode.ErrDiscoveryFailed
	}
	namespaces := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		namespaces = append(namespaces, ns.Name)
	}
	sort.Strings(namespaces)

	return &Catalog{
		ClusterID:           clusterID,
		Namespaces:          namespaces,
		Resources:           resources,
		Partial:             partial,
		FailedGroupVersions: failed,
		Cached:              false,
		DiscoveredAt:        time.Now(),
	}, nil
}
