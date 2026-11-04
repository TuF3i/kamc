package service

import (
	"context"
	"time"
	"unicode/utf8"

	"github.com/TuF3i/kamc/internal/platform/dao"
	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

// ListAudit 审计查询（管理面）：组合过滤条件，时间倒序。
// Summary=true 时不返回报文正文，仅提供长度（概览/列表用）。
func (s *Service) ListAudit(ctx context.Context, filter dao.AuditFilter) ([]modules.AuditEvent, *errcode.Response) {
	list, err := s.d.ListAuditEvents(ctx, filter)
	if err != nil {
		return nil, errcode.ServerInternal(err)
	}
	if !filter.Summary {
		for i := range list {
			list[i].RequestSize = int64(utf8.RuneCountInString(list[i].RequestBody))
			list[i].ResponseSize = int64(utf8.RuneCountInString(list[i].ResponseBody))
		}
	}
	return list, nil
}

// GetAudit 单条审计详情（含原始请求/响应报文）。
func (s *Service) GetAudit(ctx context.Context, id int64) (*modules.AuditEvent, *errcode.Response) {
	event, err := s.d.GetAuditEvent(ctx, id)
	if err != nil {
		return nil, errcode.ServerInternal(err)
	}
	event.RequestSize = int64(utf8.RuneCountInString(event.RequestBody))
	event.ResponseSize = int64(utf8.RuneCountInString(event.ResponseBody))
	return event, nil
}

// DeleteAuditBefore 清理早于 cutoff 的审计事件（保留期任务）。
func (s *Service) DeleteAuditBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	return s.d.DeleteAuditBefore(ctx, cutoff)
}

var _ = dao.NewDao // 编译期引用（dao 类型经 s.d 使用）
