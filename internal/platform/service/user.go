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
	for i := range id.Groups {
		id.Groups[i] = strings.TrimSpace(id.Groups[i])
	}
	// 内置管理员固定组（去重——handler 侧可能已带）
	if id.Username == s.adminUsername() && id.Source == modules.UserSourceLocal {
		if !containsGroup(id.Groups, s.adminGroupName()) {
			id.Groups = append(id.Groups, s.adminGroupName())
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

	// 平台管理员组：成员所在任一平台组被标记 is_admin，登录身份补上管理组
	// 声明（RequireAdmin 依据）。只补 JWT 组，不写入用户的真实组集合。
	if s.hasAdminGroup(ctx, id.Groups) && !containsGroup(id.Groups, s.adminGroupName()) {
		id.Groups = append(id.Groups, s.adminGroupName())
	}
	return nil
}

// CreateUserRequest 管理面手动创建本地用户
type CreateUserRequest struct {
	Username  string   `json:"username"`
	Nickname  string   `json:"nickname"`
	Email     string   `json:"email"`
	AvatarURL string   `json:"avatar_url"`
	Groups    []string `json:"groups"`
}

// CreateUser 手动创建本地用户（source=local；SSO/LDAP 同名用户下次登录会覆盖来源）。
func (s *Service) CreateUser(ctx context.Context, req *CreateUserRequest) (*modules.UserDTO, *errcode.Response) {
	if req == nil {
		return nil, errcode.ErrInvalidParam
	}
	username := strings.TrimSpace(req.Username)
	if username == "" || len(username) > 128 {
		return nil, errcode.ErrInvalidParam
	}
	if _, err := s.d.GetUser(ctx, username); err == nil {
		return nil, errcode.ErrUserExists
	} else if !isNotFound(err) {
		return nil, errcode.ServerInternal(err)
	}
	uid, err := newUID()
	if err != nil {
		return nil, errcode.ServerInternal(err)
	}
	now := nowUTC()
	u := &modules.PlatformUser{
		UID:       uid,
		Username:  username,
		Nickname:  strings.TrimSpace(req.Nickname),
		Email:     strings.TrimSpace(req.Email),
		AvatarURL: strings.TrimSpace(req.AvatarURL),
		Groups:    modulesJoinGroups(req.Groups),
		Source:    modules.UserSourceLocal,
		Metadata:  "{}",
		LastLogin: &now,
	}
	if err := s.d.UpsertUser(ctx, u); err != nil {
		return nil, errcode.ServerInternal(err)
	}
	// 组记录同步
	for _, g := range req.Groups {
		if g = strings.TrimSpace(g); g == "" {
			continue
		}
		if _, err := s.d.GetGroup(ctx, g); isNotFound(err) {
			_ = s.d.UpsertGroup(ctx, &modules.PlatformGroup{Name: g, Source: "local"})
		}
	}
	return u.ToDTO(), nil
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
			Name: g.Name, Desc: g.Desc, Source: g.Source, IsAdmin: g.IsAdmin, Members: ms, CreatedAt: g.CreatedAt,
		})
	}
	return dtos, nil
}

// CreateGroupRequest 手动建组请求
type CreateGroupRequest struct {
	Name    string `json:"name"`
	Desc    string `json:"desc"`
	IsAdmin bool   `json:"is_admin"` // 平台管理员组：成员登录后获得管理面权限
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
	g := &modules.PlatformGroup{Name: req.Name, Desc: strings.TrimSpace(req.Desc), Source: "local", IsAdmin: req.IsAdmin}
	if err := s.d.UpsertGroup(ctx, g); err != nil {
		return nil, errcode.ServerInternal(err)
	}
	return &modules.GroupDTO{Name: g.Name, Desc: g.Desc, Source: g.Source, IsAdmin: g.IsAdmin, Members: []string{}, CreatedAt: g.CreatedAt}, nil
}

// UpdateGroupRequest 组编辑（nil 字段不更新）
type UpdateGroupRequest struct {
	Name    *string `json:"name"`
	Desc    *string `json:"desc"`
	IsAdmin *bool   `json:"is_admin"`
}

// UpdateGroup 编辑组：可改描述与名称（重命名会把用户 groups 字段中的旧名
// 全量替换为新名；引用该组的策略（org:<旧名>）不会自动迁移，需另行调整）。
func (s *Service) UpdateGroup(ctx context.Context, name string, req *UpdateGroupRequest) (*modules.GroupDTO, *errcode.Response) {
	if req == nil {
		return nil, errcode.ErrInvalidParam
	}
	g, err := s.d.GetGroup(ctx, name)
	if err != nil {
		if isNotFound(err) {
			return nil, errcode.ErrGroupNotFound
		}
		return nil, errcode.ServerInternal(err)
	}

	newName := name
	if req.Name != nil {
		if v := strings.TrimSpace(*req.Name); v != "" && v != name {
			if len(v) > 128 {
				return nil, errcode.ErrInvalidParam
			}
			if _, err := s.d.GetGroup(ctx, v); err == nil {
				return nil, errcode.ErrGroupExists
			} else if !isNotFound(err) {
				return nil, errcode.ServerInternal(err)
			}
			newName = v
		}
	}
	if req.Desc != nil || req.IsAdmin != nil {
		cols := map[string]any{}
		if req.Desc != nil {
			cols["desc"] = strings.TrimSpace(*req.Desc)
		}
		if req.IsAdmin != nil {
			cols["is_admin"] = *req.IsAdmin
		}
		if err := s.d.UpdateGroup(ctx, name, cols); err != nil {
			return nil, errcode.ServerInternal(err)
		}
	}

	if newName != name {
		desc := g.Desc
		if req.Desc != nil {
			desc = strings.TrimSpace(*req.Desc)
		}
		isAdmin := g.IsAdmin
		if req.IsAdmin != nil {
			isAdmin = *req.IsAdmin
		}
		if err := s.d.UpsertGroup(ctx, &modules.PlatformGroup{Name: newName, Desc: desc, Source: g.Source, IsAdmin: isAdmin}); err != nil {
			return nil, errcode.ServerInternal(err)
		}
		// 用户 groups 字段全量改名
		users, err := s.d.ListUsers(ctx)
		if err != nil {
			return nil, errcode.ServerInternal(err)
		}
		for _, u := range users {
			groups := splitUserGroups(u.Groups)
			changed := false
			for i, gu := range groups {
				if gu == name {
					groups[i] = newName
					changed = true
				}
			}
			if changed {
				if err := s.d.UpdateUser(ctx, u.Username, map[string]any{"groups": modulesJoinGroups(groups)}); err != nil {
					return nil, errcode.ServerInternal(err)
				}
			}
		}
		if _, err := s.d.DeleteGroup(ctx, name); err != nil && !isNotFound(err) {
			return nil, errcode.ServerInternal(err)
		}
	}

	groups, rsp := s.ListGroups(ctx)
	if rsp != nil {
		return nil, rsp
	}
	for _, dto := range groups {
		if dto.Name == newName {
			return dto, nil
		}
	}
	return nil, errcode.ErrGroupNotFound
}

// AddGroupMember 把用户加入组（更新用户 groups 字段）
func (s *Service) AddGroupMember(ctx context.Context, group, username string) *errcode.Response {
	group = strings.TrimSpace(group)
	username = strings.TrimSpace(username)
	if group == "" || username == "" {
		return errcode.ErrInvalidParam
	}
	if _, err := s.d.GetGroup(ctx, group); err != nil {
		if isNotFound(err) {
			return errcode.ErrGroupNotFound
		}
		return errcode.ServerInternal(err)
	}
	u, err := s.d.GetUser(ctx, username)
	if err != nil {
		if isNotFound(err) {
			return errcode.ErrUserNotFound
		}
		return errcode.ServerInternal(err)
	}
	groups := splitUserGroups(u.Groups)
	for _, g := range groups {
		if g == group {
			return nil // 已在组内
		}
	}
	groups = append(groups, group)
	if err := s.d.UpdateUser(ctx, username, map[string]any{"groups": modulesJoinGroups(groups)}); err != nil {
		return errcode.ServerInternal(err)
	}
	return nil
}

// RemoveGroupMember 把用户移出组
func (s *Service) RemoveGroupMember(ctx context.Context, group, username string) *errcode.Response {
	group = strings.TrimSpace(group)
	username = strings.TrimSpace(username)
	if group == "" || username == "" {
		return errcode.ErrInvalidParam
	}
	u, err := s.d.GetUser(ctx, username)
	if err != nil {
		if isNotFound(err) {
			return errcode.ErrUserNotFound
		}
		return errcode.ServerInternal(err)
	}
	groups := splitUserGroups(u.Groups)
	out := make([]string, 0, len(groups))
	for _, g := range groups {
		if g != group {
			out = append(out, g)
		}
	}
	if err := s.d.UpdateUser(ctx, username, map[string]any{"groups": modulesJoinGroups(out)}); err != nil {
		return errcode.ServerInternal(err)
	}
	return nil
}

// GetUser 单个用户（头像上传等管理面操作的前置校验）
func (s *Service) GetUser(ctx context.Context, username string) (*modules.UserDTO, *errcode.Response) {
	u, err := s.d.GetUser(ctx, username)
	if err != nil {
		if isNotFound(err) {
			return nil, errcode.ErrUserNotFound
		}
		return nil, errcode.ServerInternal(err)
	}
	return u.ToDTO(), nil
}

// SetAvatarURL 更新用户头像地址（头像上传后写入签名 URL）
func (s *Service) SetAvatarURL(ctx context.Context, username, url string) *errcode.Response {
	if _, err := s.d.GetUser(ctx, username); err != nil {
		if isNotFound(err) {
			return errcode.ErrUserNotFound
		}
		return errcode.ServerInternal(err)
	}
	if err := s.d.UpdateUser(ctx, username, map[string]any{"avatar_url": url}); err != nil {
		return errcode.ServerInternal(err)
	}
	return nil
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

// containsGroup 组集合是否已包含某组名
func containsGroup(groups []string, name string) bool {
	for _, g := range groups {
		if g == name {
			return true
		}
	}
	return false
}

// hasAdminGroup 给定组中是否存在标记为平台管理员的组（查询失败按非管理员处理）。
func (s *Service) hasAdminGroup(ctx context.Context, groups []string) bool {
	for _, g := range groups {
		if g == "" {
			continue
		}
		grp, err := s.d.GetGroup(ctx, g)
		if err == nil && grp.IsAdmin {
			return true
		}
	}
	return false
}
