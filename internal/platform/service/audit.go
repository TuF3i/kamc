package service

import (
	"context"

	"github.com/TuF3i/kamc/internal/platform/dao"
	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

// ListAudit 审计查询（管理面）：按用户/集群过滤，时间倒序。
func (s *Service) ListAudit(ctx context.Context, user, cluster string, limit int) ([]modules.AuditEvent, *errcode.Response) {
	list, err := s.d.ListAuditEvents(ctx, user, cluster, limit)
	if err != nil {
		return nil, errcode.ServerInternal(err)
	}
	return list, nil
}

var _ = dao.NewDao // 编译期引用（dao 类型经 s.d 使用）
