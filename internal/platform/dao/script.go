package dao

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/TuF3i/kamc/internal/platform/modules"
)

// SetupAuthScript 建绑定脚本表（幂等）
func (d *Dao) SetupAuthScript(ctx context.Context) error {
	if err := d.db.WithContext(ctx).AutoMigrate(&modules.AuthScript{}); err != nil {
		return fmt.Errorf("dao -> migrate auth_script: %w", err)
	}
	return nil
}

// UpsertAuthScript 按 name 覆盖写入
func (d *Dao) UpsertAuthScript(ctx context.Context, s *modules.AuthScript) error {
	if err := d.db.WithContext(ctx).
		Clauses(clause.OnConflict{UpdateAll: true}).
		Create(s).Error; err != nil {
		return fmt.Errorf("dao -> upsert auth script %s: %w", s.Name, err)
	}
	return nil
}

// GetAuthScript 按 name 取脚本（未找到返回 gorm.ErrRecordNotFound）
func (d *Dao) GetAuthScript(ctx context.Context, name string) (*modules.AuthScript, error) {
	var s modules.AuthScript
	if err := d.db.WithContext(ctx).First(&s, "name = ?", name).Error; err != nil {
		return nil, fmt.Errorf("dao -> get auth script %s: %w", name, err)
	}
	return &s, nil
}

// ListAuthScripts 脚本列表（enabledOnly=true 仅启用的，按 name 排序——执行顺序）
func (d *Dao) ListAuthScripts(ctx context.Context, enabledOnly bool) ([]modules.AuthScript, error) {
	q := d.db.WithContext(ctx).Model(&modules.AuthScript{})
	if enabledOnly {
		q = q.Where("enabled = ?", true)
	}
	var list []modules.AuthScript
	if err := q.Order("name").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("dao -> list auth scripts: %w", err)
	}
	return list, nil
}

// DeleteAuthScript 删除脚本，返回删除行数
func (d *Dao) DeleteAuthScript(ctx context.Context, name string) (int64, error) {
	res := d.db.WithContext(ctx).Where("name = ?", name).Delete(&modules.AuthScript{})
	if res.Error != nil {
		return 0, fmt.Errorf("dao -> delete auth script %s: %w", name, res.Error)
	}
	return res.RowsAffected, nil
}

// GetAuthScript 未找到错误判定（错误链经 %w 包装，errors.Is 可穿透）
func IsAuthScriptNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
