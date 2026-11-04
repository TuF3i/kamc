package service

import (
	"context"

	"github.com/TuF3i/kamc/internal/platform/dao"
	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

// ListAudit 审计查询（管理面）：按类别（platform/cluster）+ 用户/集群/事件名过滤，时间倒序。
func (s *Service) ListAudit(ctx context.Context, kind, user, cluster, event string, limit int) ([]modules.AuditEvent, *errcode.Response) {
	list, err := s.d.ListAuditEvents(ctx, kind, user, cluster, event, limit)
	if err != nil {
		return nil, errcode.ServerInternal(err)
	}
	return list, nil
}

var _ = dao.NewDao // 编译期引用（dao 类型经 s.d 使用）
