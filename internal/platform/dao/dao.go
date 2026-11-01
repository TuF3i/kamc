// Package dao 集群注册的数据库操作层：只做 gorm 读写与建表，
// 唯一性/存在性等业务判断一律在 service 层完成（错误处理模式 A）。
package dao

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/TuF3i/kamc/internal/platform/modules"
)

// Dao 集群注册数据访问对象（只包 db，不持其他状态）
type Dao struct {
	db *gorm.DB
}

// NewDao 构造 Dao
func NewDao(db *gorm.DB) *Dao {
	return &Dao{db: db}
}

// SetupCluster 建集群表（幂等，CreateRuntime 调用）
func (d *Dao) SetupCluster(ctx context.Context) error {
	if err := d.db.WithContext(ctx).AutoMigrate(&modules.Cluster{}); err != nil {
		return fmt.Errorf("dao -> migrate cluster: %w", err)
	}
	return nil
}

// Ping 数据库连通性检查（readyz 用）
func (d *Dao) Ping(ctx context.Context) error {
	sqlDB, err := d.db.DB()
	if err != nil {
		return fmt.Errorf("dao -> unwrap sql db: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		return fmt.Errorf("dao -> ping: %w", err)
	}
	return nil
}

// CreateCluster 新增集群行
func (d *Dao) CreateCluster(ctx context.Context, c *modules.Cluster) error {
	if err := d.db.WithContext(ctx).Create(c).Error; err != nil {
		return fmt.Errorf("dao -> create cluster %s: %w", c.ID, err)
	}
	return nil
}

// GetCluster 按 ID 取集群（未找到返回 gorm.ErrRecordNotFound，由 service 翻译）
func (d *Dao) GetCluster(ctx context.Context, id string) (*modules.Cluster, error) {
	var c modules.Cluster
	if err := d.db.WithContext(ctx).First(&c, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("dao -> get cluster %s: %w", id, err)
	}
	return &c, nil
}

// ListClusters 全量集群（按创建时间升序）
func (d *Dao) ListClusters(ctx context.Context) ([]modules.Cluster, error) {
	var list []modules.Cluster
	if err := d.db.WithContext(ctx).Order("created_at").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("dao -> list clusters: %w", err)
	}
	return list, nil
}

// UpdateClusterStatus 更新集群接入状态（Agent 接入/离线回调）
func (d *Dao) UpdateClusterStatus(ctx context.Context, id string, status modules.ClusterStatus) error {
	res := d.db.WithContext(ctx).Model(&modules.Cluster{}).
		Where("id = ?", id).Update("status", status)
	if res.Error != nil {
		return fmt.Errorf("dao -> update cluster status %s: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("dao -> update cluster status %s: %w", id, gorm.ErrRecordNotFound)
	}
	return nil
}

// DeleteCluster 按 ID 删除，返回删除行数（0 即不存在）
func (d *Dao) DeleteCluster(ctx context.Context, id string) (int64, error) {
	res := d.db.WithContext(ctx).Delete(&modules.Cluster{}, "id = ?", id)
	if res.Error != nil {
		return 0, fmt.Errorf("dao -> delete cluster %s: %w", id, res.Error)
	}
	return res.RowsAffected, nil
}
