// Package service 集群管理业务层：组合 Dao，导出业务 API。
//
// 错误处理模式 B：返回 (data, rsp) 元组，业务错误为 errcode 包级 var 原样上抛，
// 系统错误包装为 errcode.ServerInternal(err)；dao 的 gorm 错误在本层翻译为业务码。
package service

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/TuF3i/kamc/internal/platform/authz"
	"github.com/TuF3i/kamc/internal/platform/dao"
	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

// maxClusterIDLen 集群 ID 上限（与 gorm 列宽一致）
const maxClusterIDLen = 64

// Service 集群管理业务
type Service struct {
	d     *dao.Dao
	authz *authz.Enforcer
}

// NewService 构造 Service（组合 Dao 与鉴权组件）
func NewService(d *dao.Dao, authz *authz.Enforcer) *Service {
	return &Service{d: d, authz: authz}
}

// Ping 数据库连通性（readyz）
func (s *Service) Ping(ctx context.Context) error {
	return s.d.Ping(ctx)
}

// Release 释放持有的组件（鉴权 watcher 等；幂等）
func (s *Service) Release() {
	s.authz.Release()
}

// CreateCluster 注册集群：参数校验 → 唯一性检查 → 落库（默认 pending，Agent 接入后转 online）
func (s *Service) CreateCluster(ctx context.Context, req *modules.CreateClusterRequest) (*modules.ClusterDTO, *errcode.Response) {
	if req == nil {
		return nil, errcode.ErrInvalidParam
	}
	id, name := strings.TrimSpace(req.ID), strings.TrimSpace(req.Name)
	if id == "" || name == "" || len(id) > maxClusterIDLen {
		return nil, errcode.ErrInvalidParam
	}

	// 唯一性：以 NotFound 作为"可创建"的判定信号，其余错误原样上抛
	if _, rsp := s.getCluster(ctx, id); rsp == nil {
		return nil, errcode.ErrClusterExists
	} else if !errors.Is(rsp, errcode.ErrClusterNotFound) {
		return nil, rsp
	}

	c := &modules.Cluster{ID: id, Name: name, Desc: strings.TrimSpace(req.Desc), Status: modules.ClusterStatusPending}
	if err := s.d.CreateCluster(ctx, c); err != nil {
		return nil, errcode.ServerInternal(err)
	}
	return c.ToDTO(), nil
}

// GetCluster 查询单个集群
func (s *Service) GetCluster(ctx context.Context, id string) (*modules.ClusterDTO, *errcode.Response) {
	c, rsp := s.getCluster(ctx, id)
	if rsp != nil {
		return nil, rsp
	}
	return c.ToDTO(), nil
}

// ListClusters 全量集群
func (s *Service) ListClusters(ctx context.Context) ([]*modules.ClusterDTO, *errcode.Response) {
	list, err := s.d.ListClusters(ctx)
	if err != nil {
		return nil, errcode.ServerInternal(err)
	}
	dtos := make([]*modules.ClusterDTO, 0, len(list))
	for i := range list {
		dtos = append(dtos, list[i].ToDTO())
	}
	return dtos, nil
}

// SetClusterStatus 更新集群接入状态（隧道接入→online；全部离线→offline）。
// 注：仅本进程视角——多副本下集群可能在其他副本有隧道，跨副本状态收敛
// 由注册表驱动（路线图）；当前以本地隧道为准，保守判定避免误标。
func (s *Service) SetClusterStatus(ctx context.Context, id string, status modules.ClusterStatus) (*modules.ClusterDTO, *errcode.Response) {
	switch status {
	case modules.ClusterStatusPending, modules.ClusterStatusOnline, modules.ClusterStatusOffline:
	default:
		return nil, errcode.ErrInvalidParam
	}
	if err := s.d.UpdateClusterStatus(ctx, id, status); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.ErrClusterNotFound
		}
		return nil, errcode.ServerInternal(err)
	}
	c, rsp := s.getCluster(ctx, id)
	if rsp != nil {
		return nil, rsp
	}
	return c.ToDTO(), nil
}

// DeleteCluster 注销集群：不存在返回业务码，成功返回 nil rsp
func (s *Service) DeleteCluster(ctx context.Context, id string) *errcode.Response {
	rows, err := s.d.DeleteCluster(ctx, id)
	if err != nil {
		return errcode.ServerInternal(err)
	}
	if rows == 0 {
		return errcode.ErrClusterNotFound
	}
	return nil
}

// getCluster 内部查询：gorm 未找到翻译为 ErrClusterNotFound（模式 A → B 的翻译点）
func (s *Service) getCluster(ctx context.Context, id string) (*modules.Cluster, *errcode.Response) {
	c, err := s.d.GetCluster(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.ErrClusterNotFound
		}
		return nil, errcode.ServerInternal(err)
	}
	return c, nil
}
