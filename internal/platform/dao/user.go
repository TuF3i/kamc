package dao

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/TuF3i/kamc/internal/platform/modules"
)

// ── 用户 ──

// SetupUser 建平台用户表（幂等）
func (d *Dao) SetupUser(ctx context.Context) error {
	if err := d.db.WithContext(ctx).AutoMigrate(&modules.PlatformUser{}); err != nil {
		return fmt.Errorf("dao -> migrate platform_user: %w", err)
	}
	return nil
}

// UpsertUser 按 username 覆盖写入（登录自动同步的落点）；
// 新建时生成 uid 由 service 层负责——这里只保证 upsert 语义。
func (d *Dao) UpsertUser(ctx context.Context, u *modules.PlatformUser) error {
	if err := d.db.WithContext(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "username"}}, DoUpdates: clause.AssignmentColumns([]string{
			"nickname", "avatar_url", "groups", "source", "email", "metadata", "last_login",
		})}).
		Create(u).Error; err != nil {
		return fmt.Errorf("dao -> upsert user %s: %w", u.Username, err)
	}
	return nil
}

// GetUser 按 username 取用户（未找到返回 gorm.ErrRecordNotFound）
func (d *Dao) GetUser(ctx context.Context, username string) (*modules.PlatformUser, error) {
	var u modules.PlatformUser
	if err := d.db.WithContext(ctx).First(&u, "username = ?", username).Error; err != nil {
		return nil, fmt.Errorf("dao -> get user %s: %w", username, err)
	}
	return &u, nil
}

// ListUsers 全量用户（按创建时间升序）
func (d *Dao) ListUsers(ctx context.Context) ([]modules.PlatformUser, error) {
	var list []modules.PlatformUser
	if err := d.db.WithContext(ctx).Order("created_at").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("dao -> list users: %w", err)
	}
	return list, nil
}

// UpdateUser 管理面编辑用户资料（仅更新可编辑列）
func (d *Dao) UpdateUser(ctx context.Context, username string, cols map[string]any) error {
	res := d.db.WithContext(ctx).Model(&modules.PlatformUser{}).
		Where("username = ?", username).Updates(cols)
	if res.Error != nil {
		return fmt.Errorf("dao -> update user %s: %w", username, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("dao -> update user %s: %w", username, gorm.ErrRecordNotFound)
	}
	return nil
}

// DeleteUser 按 username 删除，返回删除行数
func (d *Dao) DeleteUser(ctx context.Context, username string) (int64, error) {
	res := d.db.WithContext(ctx).Delete(&modules.PlatformUser{}, "username = ?", username)
	if res.Error != nil {
		return 0, fmt.Errorf("dao -> delete user %s: %w", username, res.Error)
	}
	return res.RowsAffected, nil
}

// ── 组 ──

// SetupGroup 建平台组表（幂等）
func (d *Dao) SetupGroup(ctx context.Context) error {
	if err := d.db.WithContext(ctx).AutoMigrate(&modules.PlatformGroup{}); err != nil {
		return fmt.Errorf("dao -> migrate platform_group: %w", err)
	}
	return nil
}

// UpsertGroup 按 name 覆盖写入（登录同步与手动创建共用）
func (d *Dao) UpsertGroup(ctx context.Context, g *modules.PlatformGroup) error {
	if err := d.db.WithContext(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "name"}}, DoUpdates: clause.AssignmentColumns([]string{"desc", "source"})}).
		Create(g).Error; err != nil {
		return fmt.Errorf("dao -> upsert group %s: %w", g.Name, err)
	}
	return nil
}

// GetGroup 按 name 取组（未找到返回 gorm.ErrRecordNotFound）
func (d *Dao) GetGroup(ctx context.Context, name string) (*modules.PlatformGroup, error) {
	var g modules.PlatformGroup
	if err := d.db.WithContext(ctx).First(&g, "name = ?", name).Error; err != nil {
		return nil, fmt.Errorf("dao -> get group %s: %w", name, err)
	}
	return &g, nil
}

// UpdateGroup 更新组元信息（desc 等）
func (d *Dao) UpdateGroup(ctx context.Context, name string, cols map[string]any) error {
	res := d.db.WithContext(ctx).Model(&modules.PlatformGroup{}).Where("name = ?", name).Updates(cols)
	if res.Error != nil {
		return fmt.Errorf("dao -> update group %s: %w", name, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("dao -> update group %s: %w", name, gorm.ErrRecordNotFound)
	}
	return nil
}

// ListGroups 全量组（按名称升序）
func (d *Dao) ListGroups(ctx context.Context) ([]modules.PlatformGroup, error) {
	var list []modules.PlatformGroup
	if err := d.db.WithContext(ctx).Order("name").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("dao -> list groups: %w", err)
	}
	return list, nil
}

// DeleteGroup 按 name 删除，返回删除行数
func (d *Dao) DeleteGroup(ctx context.Context, name string) (int64, error) {
	res := d.db.WithContext(ctx).Delete(&modules.PlatformGroup{}, "name = ?", name)
	if res.Error != nil {
		return 0, fmt.Errorf("dao -> delete group %s: %w", name, res.Error)
	}
	return res.RowsAffected, nil
}
