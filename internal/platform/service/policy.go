package service

import (
	"context"
	"strings"

	"github.com/TuF3i/kamc/internal/platform/errcode"
)

// maxPolicyFieldLen 策略字段长度上限（防滥用；DB 列宽一致）
const maxPolicyFieldLen = 256

// validatePolicyFields 通用字段校验（非空 + 长度）
func validatePolicyFields(fields ...string) *errcode.Response {
	for _, f := range fields {
		if strings.TrimSpace(f) == "" || len(f) > maxPolicyFieldLen {
			return errcode.ErrInvalidParam
		}
	}
	return nil
}

// AddPolicy 增加授权策略（p, sub, dom, obj, act）
func (s *Service) AddPolicy(_ context.Context, sub, dom, obj, act string) *errcode.Response {
	if rsp := validatePolicyFields(sub, dom, obj, act); rsp != nil {
		return rsp
	}
	if _, err := s.authz.AddPolicy(sub, dom, obj, act); err != nil {
		return errcode.ServerInternal(err)
	}
	return nil
}

// RemovePolicy 删除授权策略（不存在返回业务码）
func (s *Service) RemovePolicy(_ context.Context, sub, dom, obj, act string) *errcode.Response {
	if rsp := validatePolicyFields(sub, dom, obj, act); rsp != nil {
		return rsp
	}
	removed, err := s.authz.RemovePolicy(sub, dom, obj, act)
	if err != nil {
		return errcode.ServerInternal(err)
	}
	if !removed {
		return errcode.ErrPolicyNotFound
	}
	return nil
}

// ListPolicies 全量策略行
func (s *Service) ListPolicies() ([][]string, *errcode.Response) {
	rules, err := s.authz.ListPolicies()
	if err != nil {
		return nil, errcode.ServerInternal(err)
	}
	return rules, nil
}

// AddBinding 增加 用户-角色-集群 绑定（g, user, role, dom）
func (s *Service) AddBinding(_ context.Context, user, role, dom string) *errcode.Response {
	if rsp := validatePolicyFields(user, role, dom); rsp != nil {
		return rsp
	}
	if _, err := s.authz.AddBinding(user, role, dom); err != nil {
		return errcode.ServerInternal(err)
	}
	return nil
}

// RemoveBinding 删除绑定（不存在返回业务码）
func (s *Service) RemoveBinding(_ context.Context, user, role, dom string) *errcode.Response {
	if rsp := validatePolicyFields(user, role, dom); rsp != nil {
		return rsp
	}
	removed, err := s.authz.RemoveBinding(user, role, dom)
	if err != nil {
		return errcode.ServerInternal(err)
	}
	if !removed {
		return errcode.ErrPolicyNotFound
	}
	return nil
}

// ListBindings 全量绑定
func (s *Service) ListBindings() ([][]string, *errcode.Response) {
	rules, err := s.authz.ListBindings()
	if err != nil {
		return nil, errcode.ServerInternal(err)
	}
	return rules, nil
}

// Enforce 数据面鉴权判定（/proxy 专用；fail-closed）
func (s *Service) Enforce(sub, dom, obj, act string) (bool, error) {
	return s.authz.Enforce(sub, dom, obj, act)
}
