package dao

import (
	"context"
	"fmt"
	"time"

	"github.com/TuF3i/kamc/internal/platform/modules"
)

// SetupAudit 建审计事件表（幂等）。历史行（无 kind）按集群事件归类。
func (d *Dao) SetupAudit(ctx context.Context) error {
	if err := d.db.WithContext(ctx).AutoMigrate(&modules.AuditEvent{}); err != nil {
		return fmt.Errorf("dao -> migrate audit_event: %w", err)
	}
	if err := d.db.WithContext(ctx).Exec(
		"UPDATE audit_events SET kind = ? WHERE kind IS NULL OR kind = ''", modules.AuditKindCluster,
	).Error; err != nil {
		return fmt.Errorf("dao -> backfill audit kind: %w", err)
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

// AuditFilter 审计查询条件。
type AuditFilter struct {
	Kind    string
	User    string
	Cluster string
	Event   string
	From    *time.Time
	To      *time.Time
	Allowed *bool
	Limit   int
	// Summary=true 不返回请求/响应报文正文，仅返回长度（列表/概览用）
	Summary bool
}

// auditSummaryColumns 列表摘要列：不含 request_body/response_body，附加长度别名。
const auditSummaryColumns = `id, kind, event, timestamp, "user" AS user, groups, cluster,
	method, path, object, verb, allowed, reason, source_ip, forwarded_for, user_agent,
	geo_country, geo_region, geo_city, status_code, latency_ms,
	length(request_body) AS request_size, length(response_body) AS response_size`

// ListAuditEvents 管理面审计查询（按时间倒序，支持类别/用户/集群/事件/时间/结果过滤）
func (d *Dao) ListAuditEvents(ctx context.Context, f AuditFilter) ([]modules.AuditEvent, error) {
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	q := d.db.WithContext(ctx).Model(&modules.AuditEvent{})
	if f.Kind == string(modules.AuditKindPlatform) || f.Kind == string(modules.AuditKindCluster) {
		q = q.Where("kind = ?", f.Kind)
	}
	if f.User != "" {
		q = q.Where("\"user\" = ?", f.User)
	}
	if f.Cluster != "" {
		q = q.Where("cluster = ?", f.Cluster)
	}
	if f.Event != "" {
		q = q.Where("event = ?", f.Event)
	}
	if f.From != nil {
		q = q.Where("timestamp >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("timestamp <= ?", *f.To)
	}
	if f.Allowed != nil {
		q = q.Where("allowed = ?", *f.Allowed)
	}
	if f.Summary {
		q = q.Select(auditSummaryColumns)
	}
	var list []modules.AuditEvent
	if err := q.Order("id DESC").Limit(limit).Find(&list).Error; err != nil {
		return nil, fmt.Errorf("dao -> list audit: %w", err)
	}
	return list, nil
}

// GetAuditEvent 单条审计事件（详情接口，含完整报文）。
func (d *Dao) GetAuditEvent(ctx context.Context, id int64) (*modules.AuditEvent, error) {
	var event modules.AuditEvent
	if err := d.db.WithContext(ctx).First(&event, id).Error; err != nil {
		return nil, fmt.Errorf("dao -> get audit %d: %w", id, err)
	}
	return &event, nil
}

// DeleteAuditBefore 删除早于 cutoff 的审计事件，返回删除行数。
func (d *Dao) DeleteAuditBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	res := d.db.WithContext(ctx).Where("timestamp < ?", cutoff).Delete(&modules.AuditEvent{})
	if res.Error != nil {
		return 0, fmt.Errorf("dao -> delete audit before %s: %w", cutoff.Format(time.RFC3339), res.Error)
	}
	return res.RowsAffected, nil
}
