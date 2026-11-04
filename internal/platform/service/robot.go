package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

const maxRobotNameLen = 128

// ListRobots 机器人列表（不含 Token 明文）。
func (s *Service) ListRobots(ctx context.Context) ([]*modules.RobotDTO, *errcode.Response) {
	list, err := s.d.ListRobots(ctx)
	if err != nil {
		return nil, errcode.ServerInternal(err)
	}
	now := time.Now()
	dtos := make([]*modules.RobotDTO, 0, len(list))
	for i := range list {
		dtos = append(dtos, list[i].ToDTO(now))
	}
	return dtos, nil
}

// GetRobot 查询单个机器人（DTO）。
func (s *Service) GetRobot(ctx context.Context, id string) (*modules.RobotDTO, *errcode.Response) {
	r, rsp := s.GetRobotModel(ctx, id)
	if rsp != nil {
		return nil, rsp
	}
	return r.ToDTO(time.Now()), nil
}

// GetRobotModel 查询机器人原始模型（handler 下载 kubeconfig / 吊销时使用）。
func (s *Service) GetRobotModel(ctx context.Context, id string) (*modules.Robot, *errcode.Response) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errcode.ErrInvalidParam
	}
	r, err := s.d.GetRobot(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.ErrRobotNotFound
		}
		return nil, errcode.ServerInternal(err)
	}
	return r, nil
}

// CreateRobot 创建机器人身份；权限由管理员通过 user:<robot sub> 策略单独授予。
// Token 的 JTI/签发/到期时间由 handler 先生成后传入。
func (s *Service) CreateRobot(ctx context.Context, r *modules.Robot) (*modules.RobotDTO, *errcode.Response) {
	if r == nil {
		return nil, errcode.ErrInvalidParam
	}
	r.Name = strings.TrimSpace(r.Name)
	r.ClusterID = strings.TrimSpace(r.ClusterID)
	r.Sub = strings.TrimSpace(r.Sub)
	if r.Name == "" || len(r.Name) > maxRobotNameLen || r.ClusterID == "" ||
		r.Sub == "" || r.JTI == "" || r.ExpiresAt.IsZero() {
		return nil, errcode.ErrInvalidParam
	}
	if _, rsp := s.getCluster(ctx, r.ClusterID); rsp != nil {
		return nil, rsp
	}
	if _, err := s.d.GetRobotByName(ctx, r.Name); err == nil {
		return nil, errcode.ErrRobotExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.ServerInternal(err)
	}

	if err := s.d.CreateRobot(ctx, r); err != nil {
		return nil, errcode.ServerInternal(err)
	}
	return r.ToDTO(time.Now()), nil
}

// DeleteRobot 删除机器人，并清理 robot:<id> 的全部直接策略。
func (s *Service) DeleteRobot(ctx context.Context, id string) (*modules.Robot, *errcode.Response) {
	r, rsp := s.GetRobotModel(ctx, id)
	if rsp != nil {
		return nil, rsp
	}
	if _, rsp := s.RemovePoliciesBySubject(ctx, r.Sub); rsp != nil {
		return nil, rsp
	}
	rows, err := s.d.DeleteRobot(ctx, r.ID)
	if err != nil {
		return nil, errcode.ServerInternal(err)
	}
	if rows == 0 {
		return nil, errcode.ErrRobotNotFound
	}
	return r, nil
}
