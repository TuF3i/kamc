package avatarstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// maxGetSize 读取对象的安全上限（上传上限 2MB，留一倍余量）。
const maxGetSize = 4 << 20

// MinIOConfig MinIO / S3 兼容对象存储配置。
type MinIOConfig struct {
	Endpoint  string // host:port，如 minio:9000
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
	Prefix    string // 对象前缀，如 avatars/
	UseSSL    bool
}

// MinIO S3 兼容对象存储。
type MinIO struct {
	client *minio.Client
	bucket string
	prefix string
}

// NewMinIO 创建 MinIO 存储：校验配置、确保 bucket 存在（不存在则创建）。
func NewMinIO(ctx context.Context, cfg MinIOConfig) (*MinIO, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	bucket := strings.TrimSpace(cfg.Bucket)
	if endpoint == "" {
		return nil, errors.New("avatarstore -> minio: empty endpoint")
	}
	if bucket == "" {
		return nil, errors.New("avatarstore -> minio: empty bucket")
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, errors.New("avatarstore -> minio: access key/secret key required")
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: strings.TrimSpace(cfg.Region),
	})
	if err != nil {
		return nil, fmt.Errorf("avatarstore -> minio client: %w", err)
	}

	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("avatarstore -> minio bucket check: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: strings.TrimSpace(cfg.Region)}); err != nil {
			return nil, fmt.Errorf("avatarstore -> minio create bucket %s: %w", bucket, err)
		}
	}

	prefix := strings.Trim(strings.TrimSpace(cfg.Prefix), "/")
	if prefix != "" {
		prefix += "/"
	}
	return &MinIO{client: client, bucket: bucket, prefix: prefix}, nil
}

func (s *MinIO) key(uid string) (string, error) {
	if !validUID(uid) {
		return "", ErrInvalidUID
	}
	return s.prefix + uid, nil
}

// Put 覆盖写入对象（Content-Type 随对象元数据保存）。
func (s *MinIO) Put(ctx context.Context, uid, contentType string, data []byte) error {
	key, err := s.key(uid)
	if err != nil {
		return err
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err = s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("avatarstore -> minio put %s: %w", key, err)
	}
	return nil
}

// Get 读取对象；NoSuchKey/NoSuchObject 返回 ErrNotFound。
func (s *MinIO) Get(ctx context.Context, uid string) ([]byte, string, error) {
	key, err := s.key(uid)
	if err != nil {
		return nil, "", err
	}
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("avatarstore -> minio get %s: %w", key, err)
	}
	defer obj.Close()

	info, err := obj.Stat()
	if err != nil {
		if isNotFound(err) {
			return nil, "", ErrNotFound
		}
		return nil, "", fmt.Errorf("avatarstore -> minio stat %s: %w", key, err)
	}
	data, err := io.ReadAll(io.LimitReader(obj, maxGetSize))
	if err != nil {
		return nil, "", fmt.Errorf("avatarstore -> minio read %s: %w", key, err)
	}
	contentType := info.ContentType
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	return data, contentType, nil
}

// Delete 删除对象；S3 语义下对象不存在也返回成功。
func (s *MinIO) Delete(ctx context.Context, uid string) error {
	key, err := s.key(uid)
	if err != nil {
		return err
	}
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("avatarstore -> minio delete %s: %w", key, err)
	}
	return nil
}

// isNotFound 判断 MinIO 错误码是否为对象不存在。
func isNotFound(err error) bool {
	switch minio.ToErrorResponse(err).Code {
	case "NoSuchKey", "NoSuchObject", "NoSuchBucket":
		return true
	default:
		return false
	}
}
