package dao

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/TuF3i/kamc/internal/platform/modules"
)

// SetupToken 建 Agent Token 表（幂等）
func (d *Dao) SetupToken(ctx context.Context) error {
	if err := d.db.WithContext(ctx).AutoMigrate(&modules.ClusterToken{}); err != nil {
		return fmt.Errorf("dao -> migrate cluster_token: %w", err)
	}
	return nil
}

// UpsertToken 按 (cluster_id, kind) 复合主键整体覆盖写入。
// OnConflict 必须挂在 Create 之前，否则 SQL 已执行子句不生效。
func (d *Dao) UpsertToken(ctx context.Context, t *modules.ClusterToken) error {
	if err := d.db.WithContext(ctx).
		Clauses(clause.OnConflict{UpdateAll: true}).
		Create(t).Error; err != nil {
		return fmt.Errorf("dao -> upsert token %s/%s: %w", t.ClusterID, t.Kind, err)
	}
	return nil
}

// GetToken 按 (cluster_id, kind) 取 Token（未找到返回 gorm.ErrRecordNotFound）
func (d *Dao) GetToken(ctx context.Context, clusterID string, kind modules.TokenKind) (*modules.ClusterToken, error) {
	var t modules.ClusterToken
	if err := d.db.WithContext(ctx).First(&t, "cluster_id = ? AND kind = ?", clusterID, kind).Error; err != nil {
		return nil, fmt.Errorf("dao -> get token %s/%s: %w", clusterID, kind, err)
	}
	return &t, nil
}

// FindTokenByHash 按哈希反查 Token：先查当前哈希，再查宽限期内的旧哈希。
// 返回值区分命中的是 current 还是 previous（宽限命中）。
func (d *Dao) FindTokenByHash(ctx context.Context, hash string) (*modules.ClusterToken, bool, error) {
	var t modules.ClusterToken
	err := d.db.WithContext(ctx).First(&t, "current_hash = ?", hash).Error
	if err == nil {
		return &t, false, nil
	}
	if !isNotFound(err) {
		return nil, false, fmt.Errorf("dao -> find token by current hash: %w", err)
	}

	err = d.db.WithContext(ctx).
		Where("previous_hash = ? AND grace_ends_at > ?", hash, time.Now()).
		First(&t).Error
	if err == nil {
		return &t, true, nil
	}
	if !isNotFound(err) {
		return nil, false, fmt.Errorf("dao -> find token by previous hash: %w", err)
	}
	return nil, false, nil
}

// ListTokens 某集群的全部 Token（按类型升序）
func (d *Dao) ListTokens(ctx context.Context, clusterID string) ([]modules.ClusterToken, error) {
	var list []modules.ClusterToken
	if err := d.db.WithContext(ctx).
		Where("cluster_id = ?", clusterID).Order("kind").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("dao -> list tokens %s: %w", clusterID, err)
	}
	return list, nil
}

// DeleteToken 注销 Token，返回删除行数
func (d *Dao) DeleteToken(ctx context.Context, clusterID string, kind modules.TokenKind) (int64, error) {
	res := d.db.WithContext(ctx).
		Where("cluster_id = ? AND kind = ?", clusterID, kind).
		Delete(&modules.ClusterToken{})
	if res.Error != nil {
		return 0, fmt.Errorf("dao -> delete token %s/%s: %w", clusterID, kind, res.Error)
	}
	return res.RowsAffected, nil
}

// isNotFound gorm 未找到判定（错误链经 %w 包装，errors.Is 可穿透）
func isNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
