// Package avatarstore 头像对象存储抽象。
//
// 默认使用本地目录（AM_AVATAR_DIR）；配置 MinIO 时切换到 S3 兼容对象存储。
// 无论后端如何，对外 URL 始终是平台签名的 /avatars/{uid}?token=...，
// 存储地址与凭据不会下发给浏览器。
package avatarstore

import (
	"context"
	"errors"
)

// ErrNotFound 对象不存在（handler 映射为 404）。
var ErrNotFound = errors.New("avatar: object not found")

// ErrInvalidUID 非法的对象键（UID 必须是 UUID 风格的安全字符）。
var ErrInvalidUID = errors.New("avatar: invalid uid")

// Store 头像对象存储（实现：Local、MinIO）。
type Store interface {
	// Put 覆盖写入 uid 对应的头像对象。
	Put(ctx context.Context, uid, contentType string, data []byte) error
	// Get 读取头像；不存在返回 ErrNotFound。
	Get(ctx context.Context, uid string) (data []byte, contentType string, err error)
	// Delete 删除头像；对象不存在视为成功。
	Delete(ctx context.Context, uid string) error
}

// validUID 只允许字母、数字与连字符（平台 UID 为 UUID，足以覆盖）。
// 该检查同时防止本地路径穿越与对象键注入。
func validUID(uid string) bool {
	if uid == "" || len(uid) > 64 {
		return false
	}
	for _, r := range uid {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return false
		}
	}
	return true
}
