package dao

import (
	"context"
	"fmt"

	"github.com/TuF3i/kamc/internal/platform/modules"
)

// SetupRole 建角色表（幂等，CreateRuntime 调用）
func (d *Dao) SetupRole(ctx context.Context) error {
	if err := d.db.WithContext(ctx).AutoMigrate(&modules.Role{}); err != nil {
		return fmt.Errorf("dao -> migrate role: %w", err)
	}
	return nil
}

// CreateRole 新增角色行
func (d *Dao) CreateRole(ctx context.Context, r *modules.Role) error {
	if err := d.db.WithContext(ctx).Create(r).Error; err != nil {
		return fmt.Errorf("dao -> create role %s: %w", r.Name, err)
	}
	return nil
}

// GetRole 按名称取角色（未找到返回 gorm.ErrRecordNotFound）
func (d *Dao) GetRole(ctx context.Context, name string) (*modules.Role, error) {
	var r modules.Role
	if err := d.db.WithContext(ctx).First(&r, "name = ?", name).Error; err != nil {
		return nil, fmt.Errorf("dao -> get role %s: %w", name, err)
	}
	return &r, nil
}

// ListRoles 全量角色（按名称升序）
func (d *Dao) ListRoles(ctx context.Context) ([]modules.Role, error) {
	var list []modules.Role
	if err := d.db.WithContext(ctx).Order("name").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("dao -> list roles: %w", err)
	}
	return list, nil
}

// DeleteRole 按名称删除，返回删除行数（0 即不存在）
func (d *Dao) DeleteRole(ctx context.Context, name string) (int64, error) {
	res := d.db.WithContext(ctx).Delete(&modules.Role{}, "name = ?", name)
	if res.Error != nil {
		return 0, fmt.Errorf("dao -> delete role %s: %w", name, res.Error)
	}
	return res.RowsAffected, nil
}
