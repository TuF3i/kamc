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

// CreateRobot 创建机器人并自动建立 user(robot sub) → role @ cluster 绑定。
// Token 的 JTI/签发/到期时间由 handler 先生成后传入；本方法只做持久化与绑定。
func (s *Service) CreateRobot(ctx context.Context, r *modules.Robot) (*modules.RobotDTO, *errcode.Response) {
	if r == nil {
		return nil, errcode.ErrInvalidParam
	}
	r.Name = strings.TrimSpace(r.Name)
	r.ClusterID = strings.TrimSpace(r.ClusterID)
	r.Role = strings.TrimSpace(r.Role)
	r.Sub = strings.TrimSpace(r.Sub)
	if r.Name == "" || len(r.Name) > maxRobotNameLen || r.ClusterID == "" || r.Role == "" ||
		r.Sub == "" || r.JTI == "" || r.ExpiresAt.IsZero() {
		return nil, errcode.ErrInvalidParam
	}
	if _, rsp := s.getCluster(ctx, r.ClusterID); rsp != nil {
		return nil, rsp
	}
	roles, rsp := s.ListRoles(ctx)
	if rsp != nil {
		return nil, rsp
	}
	roleFound := false
	for _, role := range roles {
		if role.Name == r.Role {
			roleFound = true
			break
		}
	}
	if !roleFound {
		return nil, errcode.ErrRoleNotFound
	}
	if _, err := s.d.GetRobotByName(ctx, r.Name); err == nil {
		return nil, errcode.ErrRobotExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.ServerInternal(err)
	}

	added, err := s.authz.AddBinding(r.Sub, r.Role, r.ClusterID)
	if err != nil {
		return nil, errcode.ServerInternal(err)
	}
	if err := s.d.CreateRobot(ctx, r); err != nil {
		if added {
			_, _ = s.authz.RemoveBinding(r.Sub, r.Role, r.ClusterID)
		}
		return nil, errcode.ServerInternal(err)
	}
	return r.ToDTO(time.Now()), nil
}

// DeleteRobot 删除机器人并解除角色绑定；返回被删除的模型供 handler 吊销 JWT。
func (s *Service) DeleteRobot(ctx context.Context, id string) (*modules.Robot, *errcode.Response) {
	r, rsp := s.GetRobotModel(ctx, id)
	if rsp != nil {
		return nil, rsp
	}
	if r.Role != "" {
		if _, err := s.authz.RemoveBinding(r.Sub, r.Role, r.ClusterID); err != nil {
			return nil, errcode.ServerInternal(err)
		}
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
