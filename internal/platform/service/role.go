package service

import (
	"context"
	"errors"
	"sort"
	"strings"

	"gorm.io/gorm"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

const maxRoleNameLen = 256

// ListRoles 角色列表：roles 表显式角色 + 现有策略/绑定中出现的角色（合并计数）。
func (s *Service) ListRoles(ctx context.Context) ([]*modules.RoleDTO, *errcode.Response) {
	dbRoles, err := s.d.ListRoles(ctx)
	if err != nil {
		return nil, errcode.ServerInternal(err)
	}
	policies, err := s.authz.ListPolicies()
	if err != nil {
		return nil, errcode.ServerInternal(err)
	}
	bindings, err := s.authz.ListBindings()
	if err != nil {
		return nil, errcode.ServerInternal(err)
	}

	byName := make(map[string]*modules.RoleDTO)
	roleSet := make(map[string]bool)
	ensure := func(name string) *modules.RoleDTO {
		dto, ok := byName[name]
		if !ok {
			dto = &modules.RoleDTO{Name: name}
			byName[name] = dto
		}
		return dto
	}

	for i := range dbRoles {
		r := dbRoles[i]
		roleSet[r.Name] = true
		dto := ensure(r.Name)
		dto.Desc = r.Desc
		dto.Managed = true
		created := r.CreatedAt
		dto.CreatedAt = &created
	}
	for _, b := range bindings {
		if len(b) > 1 && strings.TrimSpace(b[1]) != "" {
			roleSet[b[1]] = true
			ensure(b[1]).BindingCount++
		}
	}
	for _, p := range policies {
		if len(p) == 0 {
			continue
		}
		sub := strings.TrimSpace(p[0])
		// 只把角色主体计入角色列表；直接授予用户名的策略留给策略页展示
		if sub == "" || (!roleSet[sub] && !strings.HasPrefix(sub, "role:")) {
			continue
		}
		ensure(sub).PolicyCount++
	}

	list := make([]*modules.RoleDTO, 0, len(byName))
	for _, dto := range byName {
		list = append(list, dto)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list, nil
}

// CreateRole 显式创建角色（已存在返回 ErrRoleExists；派生角色可补建为受管角色）。
func (s *Service) CreateRole(ctx context.Context, req *modules.CreateRoleRequest) (*modules.RoleDTO, *errcode.Response) {
	if req == nil {
		return nil, errcode.ErrInvalidParam
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > maxRoleNameLen {
		return nil, errcode.ErrInvalidParam
	}
	if _, err := s.d.GetRole(ctx, name); err == nil {
		return nil, errcode.ErrRoleExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.ServerInternal(err)
	}
	r := &modules.Role{Name: name, Desc: strings.TrimSpace(req.Desc)}
	if err := s.d.CreateRole(ctx, r); err != nil {
		return nil, errcode.ServerInternal(err)
	}
	created := r.CreatedAt
	return &modules.RoleDTO{
		Name:      r.Name,
		Desc:      r.Desc,
		Managed:   true,
		CreatedAt: &created,
	}, nil
}

// DeleteRole 删除显式角色：仍被策略或绑定引用时拒绝（先清理引用再删除）。
func (s *Service) DeleteRole(ctx context.Context, name string) *errcode.Response {
	name = strings.TrimSpace(name)
	if name == "" {
		return errcode.ErrInvalidParam
	}
	if _, err := s.d.GetRole(ctx, name); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.ErrRoleNotFound
		}
		return errcode.ServerInternal(err)
	}
	policies, err := s.authz.ListPolicies()
	if err != nil {
		return errcode.ServerInternal(err)
	}
	bindings, err := s.authz.ListBindings()
	if err != nil {
		return errcode.ServerInternal(err)
	}
	for _, p := range policies {
		if len(p) > 0 && p[0] == name {
			return errcode.ErrRoleInUse
		}
	}
	for _, b := range bindings {
		if len(b) > 1 && b[1] == name {
			return errcode.ErrRoleInUse
		}
	}
	rows, err := s.d.DeleteRole(ctx, name)
	if err != nil {
		return errcode.ServerInternal(err)
	}
	if rows == 0 {
		return errcode.ErrRoleNotFound
	}
	return nil
}
