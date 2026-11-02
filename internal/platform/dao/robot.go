package dao

import (
	"context"
	"fmt"

	"github.com/TuF3i/kamc/internal/platform/modules"
)

// SetupRobot 建机器人表（幂等，CreateRuntime 调用）
func (d *Dao) SetupRobot(ctx context.Context) error {
	if err := d.db.WithContext(ctx).AutoMigrate(&modules.Robot{}); err != nil {
		return fmt.Errorf("dao -> migrate robot: %w", err)
	}
	return nil
}

// CreateRobot 新增机器人行
func (d *Dao) CreateRobot(ctx context.Context, r *modules.Robot) error {
	if err := d.db.WithContext(ctx).Create(r).Error; err != nil {
		return fmt.Errorf("dao -> create robot %s: %w", r.Name, err)
	}
	return nil
}

// GetRobot 按 ID 取机器人（未找到返回 gorm.ErrRecordNotFound）
func (d *Dao) GetRobot(ctx context.Context, id string) (*modules.Robot, error) {
	var r modules.Robot
	if err := d.db.WithContext(ctx).First(&r, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("dao -> get robot %s: %w", id, err)
	}
	return &r, nil
}

// GetRobotByName 按名称取机器人（用于唯一性检查）
func (d *Dao) GetRobotByName(ctx context.Context, name string) (*modules.Robot, error) {
	var r modules.Robot
	if err := d.db.WithContext(ctx).First(&r, "name = ?", name).Error; err != nil {
		return nil, fmt.Errorf("dao -> get robot by name %s: %w", name, err)
	}
	return &r, nil
}

// ListRobots 全量机器人（按创建时间倒序）
func (d *Dao) ListRobots(ctx context.Context) ([]modules.Robot, error) {
	var list []modules.Robot
	if err := d.db.WithContext(ctx).Order("created_at DESC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("dao -> list robots: %w", err)
	}
	return list, nil
}

// DeleteRobot 按 ID 删除，返回删除行数（0 即不存在）
func (d *Dao) DeleteRobot(ctx context.Context, id string) (int64, error) {
	res := d.db.WithContext(ctx).Delete(&modules.Robot{}, "id = ?", id)
	if res.Error != nil {
		return 0, fmt.Errorf("dao -> delete robot %s: %w", id, res.Error)
	}
	return res.RowsAffected, nil
}
