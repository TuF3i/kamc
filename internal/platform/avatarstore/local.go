package avatarstore

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// Local 本地目录存储：<dir>/<uid>，权限 0600。
type Local struct {
	dir string
}

// NewLocal 创建本地头像存储（目录不存在时创建）。
func NewLocal(dir string) (*Local, error) {
	if dir == "" {
		return nil, errors.New("avatarstore -> local: empty dir")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("avatarstore -> create dir: %w", err)
	}
	return &Local{dir: dir}, nil
}

func (s *Local) path(uid string) (string, error) {
	if !validUID(uid) {
		return "", ErrInvalidUID
	}
	return filepath.Join(s.dir, uid), nil
}

// Put 覆盖写入头像文件。
func (s *Local) Put(_ context.Context, uid, _ string, data []byte) error {
	path, err := s.path(uid)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("avatarstore -> write %s: %w", uid, err)
	}
	return nil
}

// Get 读取头像文件；不存在返回 ErrNotFound。
func (s *Local) Get(_ context.Context, uid string) ([]byte, string, error) {
	path, err := s.path(uid)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("avatarstore -> read %s: %w", uid, err)
	}
	return data, http.DetectContentType(data), nil
}

// Delete 删除头像文件；不存在视为成功。
func (s *Local) Delete(_ context.Context, uid string) error {
	path, err := s.path(uid)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("avatarstore -> delete %s: %w", uid, err)
	}
	return nil
}
