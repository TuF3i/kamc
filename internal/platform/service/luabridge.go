package service

// luaauth.UserStore 适配：脚本桥接直接操作用户/组目录（不走管理面校验语义）。

import (
	"context"
	"errors"

	"github.com/TuF3i/kamc/internal/platform/modules"
)

// errInvalid 参数错误 sentinel（桥接内部用，不外露业务码）
func errInvalid(msg string) error { return errors.New("luabridge -> " + msg) }

// UpsertUserDirect 脚本桥接的用户 upsert（username 必填，缺 uid 自动补）
func (s *Service) UpsertUserDirect(ctx context.Context, u *modules.PlatformUser) error {
	if u == nil || u.Username == "" {
		return errInvalid("username required")
	}
	if u.UID == "" {
		if existing, err := s.d.GetUser(ctx, u.Username); err == nil {
			u.UID = existing.UID
		} else {
			uid, err := newUID()
			if err != nil {
				return err
			}
			u.UID = uid
		}
	}
	if u.Metadata == "" {
		u.Metadata = "{}"
	}
	if u.Source == "" {
		u.Source = modules.UserSourceLocal
	}
	return s.d.UpsertUser(ctx, u)
}

// UpsertGroupDirect 脚本桥接的组 upsert
func (s *Service) UpsertGroupDirect(ctx context.Context, name, desc, source string) error {
	if name == "" {
		return errInvalid("group name required")
	}
	if source == "" {
		source = "script"
	}
	return s.d.UpsertGroup(ctx, &modules.PlatformGroup{Name: name, Desc: desc, Source: source})
}

// ListUsersDirect 脚本桥接的用户列表
func (s *Service) ListUsersDirect(ctx context.Context) ([]modules.PlatformUser, error) {
	return s.d.ListUsers(ctx)
}

// ListGroupsDirect 脚本桥接的组列表
func (s *Service) ListGroupsDirect(ctx context.Context) ([]modules.PlatformGroup, error) {
	return s.d.ListGroups(ctx)
}
