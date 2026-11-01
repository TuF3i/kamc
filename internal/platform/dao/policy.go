package dao

import (
	"context"
	"fmt"

	"github.com/TuF3i/kamc/internal/platform/modules"
)

// SetupPolicy 建 Casbin 策略表（幂等）
func (d *Dao) SetupPolicy(ctx context.Context) error {
	if err := d.db.WithContext(ctx).AutoMigrate(&modules.PolicyRule{}); err != nil {
		return fmt.Errorf("dao -> migrate policy_rule: %w", err)
	}
	return nil
}

// ListPolicyRules 全量策略行（casbin LoadPolicy 用）
func (d *Dao) ListPolicyRules(ctx context.Context) ([]modules.PolicyRule, error) {
	var list []modules.PolicyRule
	if err := d.db.WithContext(ctx).Order("id").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("dao -> list policy rules: %w", err)
	}
	return list, nil
}

// AddPolicyRule 新增策略行（重复插入由唯一性语义兜底：casbin 侧已去重，DB 层允许重复无害）
func (d *Dao) AddPolicyRule(ctx context.Context, r *modules.PolicyRule) error {
	if err := d.db.WithContext(ctx).Create(r).Error; err != nil {
		return fmt.Errorf("dao -> add policy rule: %w", err)
	}
	return nil
}

// RemovePolicyRule 删除与字段完全匹配的行，返回删除行数
func (d *Dao) RemovePolicyRule(ctx context.Context, r *modules.PolicyRule) (int64, error) {
	res := d.db.WithContext(ctx).
		Where("ptype = ? AND v0 = ? AND v1 = ? AND v2 = ? AND v3 = ? AND v4 = ? AND v5 = ?",
			r.Ptype, r.V0, r.V1, r.V2, r.V3, r.V4, r.V5).
		Delete(&modules.PolicyRule{})
	if res.Error != nil {
		return 0, fmt.Errorf("dao -> remove policy rule: %w", res.Error)
	}
	return res.RowsAffected, nil
}
