package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

// modulesJoinGroups / splitUserGroups 对 modules 包组串工具的本地别名
func modulesJoinGroups(groups []string) string {
	seen := make(map[string]struct{}, len(groups))
	out := make([]string, 0, len(groups))
	for _, g := range groups {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		if _, dup := seen[g]; dup {
			continue
		}
		seen[g] = struct{}{}
		out = append(out, g)
	}
	return strings.Join(out, ",")
}

func splitUserGroups(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// nowUTC 当前 UTC 时间（登录时间戳）
func nowUTC() time.Time { return time.Now().UTC() }

// Identity 登录身份（OIDC / LDAP / 密码登录统一形态），
// 登录挂载点据此同步平台用户与组。
type Identity struct {
	Username string // OIDC sub / LDAP uid / 内置账号名
	Email    string
	Nickname string // 显示名（OIDC preferred_username / LDAP cn，空取 Username）
	Avatar   string // 头像 URL（OIDC picture）
	Groups   []string
	Source   modules.UserSource
}

// SyncUserOnLogin 登录挂载点：upsert 平台用户（组集合同步）与组记录。
// 同步失败不阻断登录（记为系统错误日志），但用户记录缺失会体现在列表里。
func (s *Service) SyncUserOnLogin(ctx context.Context, id *Identity) *errcode.Response {
	if id == nil || strings.TrimSpace(id.Username) == "" {
		return nil
	}
	if id.Source == "" {
		id.Source = modules.UserSourceLocal
	}
	// 内置管理员固定组（去重——handler 侧可能已带）
	if id.Username == s.adminUsername() && id.Source == modules.UserSourceLocal {
		has := false
		for _, g := range id.Groups {
			if g == "am:admin" {
				has = true
				break
			}
		}
		if !has {
			id.Groups = append(id.Groups, "am:admin")
		}
	}

	u := &modules.PlatformUser{
		Username:  id.Username,
		Nickname:  id.Nickname,
		AvatarURL: id.Avatar,
		Groups:    modulesJoinGroups(id.Groups),
		Source:    id.Source,
		Email:     id.Email,
		Metadata:  "{}",
	}
	existing, err := s.d.GetUser(ctx, id.Username)
	switch {
	case err == nil:
		u.UID = existing.UID // username 不变，保留 uid
		if u.Nickname == "" {
			u.Nickname = existing.Nickname
		}
		if u.AvatarURL == "" {
			u.AvatarURL = existing.AvatarURL
		}
		now := nowUTC()
		u.LastLogin = &now
	case isNotFound(err):
		uid, err := newUID()
		if err != nil {
			return errcode.ServerInternal(err)
		}
		u.UID = uid
		now := nowUTC()
		u.LastLogin = &now
	default:
		return errcode.ServerInternal(err)
	}
	if err := s.d.UpsertUser(ctx, u); err != nil {
		return errcode.ServerInternal(err)
	}

	// 组记录同步（登录带来的组自动入组表，便于组页聚合展示）
	for _, g := range id.Groups {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		source := string(id.Source)
		if _, err := s.d.GetGroup(ctx, g); isNotFound(err) {
			if err := s.d.UpsertGroup(ctx, &modules.PlatformGroup{Name: g, Source: source}); err != nil {
				return errcode.ServerInternal(err)
			}
		}
	}
	return nil
}

// ListUsers 平台用户列表（管理面）
func (s *Service) ListUsers(ctx context.Context) ([]*modules.UserDTO, *errcode.Response) {
	list, err := s.d.ListUsers(ctx)
	if err != nil {
		return nil, errcode.ServerInternal(err)
	}
	dtos := make([]*modules.UserDTO, 0, len(list))
	for i := range list {
		dtos = append(dtos, list[i].ToDTO())
	}
	return dtos, nil
}

// UpdateUserRequest 管理面编辑用户（nil/空字段不更新）
type UpdateUserRequest struct {
	Nickname  *string  `json:"nickname"`
	Email     *string  `json:"email"`
	Groups    []string `json:"groups"` // 全量替换
	AvatarURL *string  `json:"avatar_url"`
}

// UpdateUser 管理面编辑用户资料
func (s *Service) UpdateUser(ctx context.Context, username string, req *UpdateUserRequest) (*modules.UserDTO, *errcode.Response) {
	if req == nil {
		return nil, errcode.ErrInvalidParam
	}
	if _, err := s.d.GetUser(ctx, username); err != nil {
		if isNotFound(err) {
			return nil, errcode.ErrUserNotFound
		}
		return nil, errcode.ServerInternal(err)
	}
	cols := map[string]any{}
	if req.Nickname != nil {
		cols["nickname"] = strings.TrimSpace(*req.Nickname)
	}
	if req.Email != nil {
		cols["email"] = strings.TrimSpace(*req.Email)
	}
	if req.AvatarURL != nil {
		cols["avatar_url"] = strings.TrimSpace(*req.AvatarURL)
	}
	if req.Groups != nil {
		cols["groups"] = modulesJoinGroups(req.Groups)
	}
	if len(cols) > 0 {
		if err := s.d.UpdateUser(ctx, username, cols); err != nil {
			return nil, errcode.ServerInternal(err)
		}
	}
	u, err := s.d.GetUser(ctx, username)
	if err != nil {
		return nil, errcode.ServerInternal(err)
	}
	return u.ToDTO(), nil
}

// DeleteUser 删除平台用户（不影响 JWT 吊销——登出/踢出走 /auth/revoke）
func (s *Service) DeleteUser(ctx context.Context, username string) *errcode.Response {
	rows, err := s.d.DeleteUser(ctx, username)
	if err != nil {
		return errcode.ServerInternal(err)
	}
	if rows == 0 {
		return errcode.ErrUserNotFound
	}
	return nil
}

// ListGroups 平台组列表（成员聚合自用户表）
func (s *Service) ListGroups(ctx context.Context) ([]*modules.GroupDTO, *errcode.Response) {
	groups, err := s.d.ListGroups(ctx)
	if err != nil {
		return nil, errcode.ServerInternal(err)
	}
	users, err := s.d.ListUsers(ctx)
	if err != nil {
		return nil, errcode.ServerInternal(err)
	}
	// 组名 → 成员用户名
	members := map[string][]string{}
	for _, u := range users {
		for _, g := range splitUserGroups(u.Groups) {
			members[g] = append(members[g], u.Username)
		}
	}
	dtos := make([]*modules.GroupDTO, 0, len(groups))
	for _, g := range groups {
		ms := members[g.Name]
		sort.Strings(ms)
		if ms == nil {
			ms = []string{}
		}
		dtos = append(dtos, &modules.GroupDTO{
			Name: g.Name, Desc: g.Desc, Source: g.Source, Members: ms, CreatedAt: g.CreatedAt,
		})
	}
	return dtos, nil
}

// CreateGroupRequest 手动建组请求
type CreateGroupRequest struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// CreateGroup 手动创建组（local 来源）
func (s *Service) CreateGroup(ctx context.Context, req *CreateGroupRequest) (*modules.GroupDTO, *errcode.Response) {
	if req == nil || strings.TrimSpace(req.Name) == "" || len(req.Name) > 128 {
		return nil, errcode.ErrInvalidParam
	}
	if _, err := s.d.GetGroup(ctx, req.Name); err == nil {
		return nil, errcode.ErrGroupExists
	} else if !isNotFound(err) {
		return nil, errcode.ServerInternal(err)
	}
	g := &modules.PlatformGroup{Name: req.Name, Desc: strings.TrimSpace(req.Desc), Source: "local"}
	if err := s.d.UpsertGroup(ctx, g); err != nil {
		return nil, errcode.ServerInternal(err)
	}
	return &modules.GroupDTO{Name: g.Name, Desc: g.Desc, Source: g.Source, Members: []string{}, CreatedAt: g.CreatedAt}, nil
}

// DeleteGroup 删除组（不清理用户 groups 字段——下次登录同步收敛；策略需另行清理）
func (s *Service) DeleteGroup(ctx context.Context, name string) *errcode.Response {
	rows, err := s.d.DeleteGroup(ctx, name)
	if err != nil {
		return errcode.ServerInternal(err)
	}
	if rows == 0 {
		return errcode.ErrGroupNotFound
	}
	return nil
}

// newUID 生成 uuid v4 形态的唯一 ID
func newUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("service -> uid rand: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	dst := make([]byte, 36)
	hex.Encode(dst, b[:4])
	dst[8] = '-'
	hex.Encode(dst[9:], b[4:6])
	dst[13] = '-'
	hex.Encode(dst[14:], b[6:8])
	dst[18] = '-'
	hex.Encode(dst[19:], b[8:10])
	dst[23] = '-'
	hex.Encode(dst[24:], b[10:])
	return string(dst), nil
}

// isNotFound gorm 未找到翻译（本包统一入口）
func isNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }

// adminUsername 内置管理员账号名（同步时固定补 admin 组）
func (s *Service) adminUsername() string { return "sysadmin" }
