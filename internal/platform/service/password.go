package service

// 本地密码：个人中心修改密码 + 内置账号密码登录的库内哈希校验。
//
// 语义约定：
//   - PlatformUser.PasswordHash 非空时，登录以库内哈希为准（环境变量密码失效）；
//   - 哈希为空且用户是内置管理员时，由 handler 用环境变量密码核对
//     （service 不持有 AM_PASSWORD_LOGIN_PASSWORD）；
//   - SSO/LDAP 账号的密码由 IdP 管理，本层一律拒绝；
//   - 登录同步 UpsertUser 的冲突更新列不含 password_hash，密码不会被冲掉。

import (
	"context"
	"crypto/subtle"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

// passwordMinLen / passwordMaxLen 新密码长度约束（字节；上限同时满足 bcrypt 72 字节限制）。
const (
	passwordMinLen = 8
	passwordMaxLen = 72
)

// VerifyPassword 校验用户的当前密码（修改密码入口的第一步）。
// 返回值：
//   - verified：库内哈希比对通过；
//   - builtinFallback：用户未设置库内密码且是内置管理员——由调用方用
//     环境变量密码核对（成功后调用 SetPasswordHash 落库）。
//     其余情形通过 rsp 返回业务错误（密码错误/外部账号/未设置密码）。
func (s *Service) VerifyPassword(ctx context.Context, username, password string) (verified, builtinFallback bool, rsp *errcode.Response) {
	username = strings.TrimSpace(username)
	u, err := s.d.GetUser(ctx, username)
	switch {
	case isNotFound(err):
		// 正常登录链路都会落用户记录；防御性兜底：内置管理员允许走环境密码核对
		if username == s.adminUsername() {
			return false, true, nil
		}
		return false, false, errcode.ErrPasswordWrong
	case err != nil:
		return false, false, errcode.ServerInternal(err)
	}
	if u.Source != modules.UserSourceLocal {
		return false, false, errcode.ErrPasswordExternal
	}
	if u.PasswordHash == "" {
		if username == s.adminUsername() {
			return false, true, nil
		}
		return false, false, errcode.ErrPasswordNotSet
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return false, false, errcode.ErrPasswordWrong
	}
	return true, false, nil
}

// VerifyBuiltinPassword 内置管理员环境变量密码核对（恒定时间比较），
// handler 在 VerifyPassword 返回 builtinFallback 时调用。
func VerifyBuiltinPassword(username, envUser, envPass, password string) bool {
	userOK := subtle.ConstantTimeCompare([]byte(strings.TrimSpace(username)), []byte(envUser)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(password), []byte(envPass)) == 1
	return userOK && passOK
}

// HashNewPassword 校验新密码长度并计算 bcrypt 哈希（先验证后吊销：
// 让“新密码不合法”在用户仍处于登录态时失败，避免白白踢下线）。
func HashNewPassword(newPassword string) (string, *errcode.Response) {
	if len(newPassword) < passwordMinLen || len(newPassword) > passwordMaxLen {
		return "", errcode.ErrInvalidParam
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return "", errcode.ServerInternal(err)
	}
	return string(hash), nil
}

// SetPasswordHash 落库用户本地密码哈希（HashNewPassword 的产物）。
// 用户记录不存在时（防御）创建最小本地用户。
func (s *Service) SetPasswordHash(ctx context.Context, username, hash string) *errcode.Response {
	username = strings.TrimSpace(username)
	if username == "" || hash == "" {
		return errcode.ErrInvalidParam
	}
	u, gerr := s.d.GetUser(ctx, username)
	switch {
	case isNotFound(gerr):
		uid, uerr := newUID()
		if uerr != nil {
			return errcode.ServerInternal(uerr)
		}
		now := nowUTC()
		if serr := s.d.UpsertUser(ctx, &modules.PlatformUser{
			UID: uid, Username: username, Source: modules.UserSourceLocal,
			Metadata: "{}", PasswordHash: hash, LastLogin: &now,
		}); serr != nil {
			return errcode.ServerInternal(serr)
		}
	case gerr != nil:
		return errcode.ServerInternal(gerr)
	default:
		if u.Source != modules.UserSourceLocal {
			return errcode.ErrPasswordExternal
		}
		if uerr := s.d.UpdateUser(ctx, username, map[string]any{"password_hash": hash}); uerr != nil {
			return errcode.ServerInternal(uerr)
		}
	}
	return nil
}

// AuthenticateLocal 密码登录的库内哈希校验。返回值：
//   - matched：哈希存在且与给定密码匹配（命中即签发）；
//   - hasHash：用户存在库内哈希（未匹配时调用方应直接判 401——
//     本地账号以库内密码为准，不再回退内置环境变量密码）。
func (s *Service) AuthenticateLocal(ctx context.Context, username, password string) (matched, hasHash bool) {
	if username == "" || password == "" {
		return false, false
	}
	u, err := s.d.GetUser(ctx, strings.TrimSpace(username))
	if err != nil || u.PasswordHash == "" {
		return false, false
	}
	return bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) == nil, true
}
