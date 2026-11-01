package dao

import (
	"context"
	"fmt"

	"github.com/TuF3i/kamc/internal/platform/modules"
)

// SetupAudit 建审计事件表（幂等）
func (d *Dao) SetupAudit(ctx context.Context) error {
	if err := d.db.WithContext(ctx).AutoMigrate(&modules.AuditEvent{}); err != nil {
		return fmt.Errorf("dao -> migrate audit_event: %w", err)
	}
	return nil
}

// InsertBatch 批量写入审计事件（实现 audit.Sink 接口）
func (d *Dao) InsertBatch(ctx context.Context, events []*modules.AuditEvent) error {
	if len(events) == 0 {
		return nil
	}
	if err := d.db.WithContext(ctx).Create(&events).Error; err != nil {
		return fmt.Errorf("dao -> insert audit batch(%d): %w", len(events), err)
	}
	return nil
}

// AuditEventRow 查询结果行
type AuditEventRow = modules.AuditEvent

// ListAuditEvents 管理面审计查询（按时间倒序，可按用户/集群过滤）
func (d *Dao) ListAuditEvents(ctx context.Context, user, cluster string, limit int) ([]modules.AuditEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	q := d.db.WithContext(ctx).Model(&modules.AuditEvent{})
	if user != "" {
		q = q.Where("user = ?", user)
	}
	if cluster != "" {
		q = q.Where("cluster = ?", cluster)
	}
	var list []modules.AuditEvent
	if err := q.Order("id DESC").Limit(limit).Find(&list).Error; err != nil {
		return nil, fmt.Errorf("dao -> list audit: %w", err)
	}
	return list, nil
}
