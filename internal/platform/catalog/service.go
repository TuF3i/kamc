// Package catalog 通过 Agent 隧道读取目标集群的 Kubernetes discovery 信息，
// 为策略编辑器提供 GVR + 可用动词 + namespace 列表。
package catalog

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

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
