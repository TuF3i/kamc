package avatarstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestMinIOIntegration MinIO 集成测试：TEST_MINIO_ENDPOINT 未设置时跳过。
// 需要 TEST_MINIO_ACCESS_KEY / TEST_MINIO_SECRET_KEY；bucket 默认
// kamc-avatars-test（不存在时由 NewMinIO 创建）。
func TestMinIOIntegration(t *testing.T) {
	endpoint := os.Getenv("TEST_MINIO_ENDPOINT")
	if endpoint == "" {
		t.Skip("TEST_MINIO_ENDPOINT not set")
	}
	accessKey := os.Getenv("TEST_MINIO_ACCESS_KEY")
	secretKey := os.Getenv("TEST_MINIO_SECRET_KEY")
	if accessKey == "" || secretKey == "" {
		t.Skip("TEST_MINIO_ACCESS_KEY/TEST_MINIO_SECRET_KEY not set")
	}
	bucket := os.Getenv("TEST_MINIO_BUCKET")
	if bucket == "" {
		bucket = "kamc-avatars-test"
	}

	ctx := context.Background()
	store, err := NewMinIO(ctx, MinIOConfig{
		Endpoint:  endpoint,
		AccessKey: accessKey,
		SecretKey: secretKey,
		Bucket:    bucket,
		Prefix:    "avatars/",
		UseSSL:    os.Getenv("TEST_MINIO_USE_SSL") == "1",
	})
	if err != nil {
		t.Fatalf("NewMinIO: %v", err)
	}

	uid := fmt.Sprintf("test-%d", time.Now().UnixNano())
	if err := store.Put(ctx, uid, "image/png", pngHeader); err != nil {
		t.Fatalf("Put: %v", err)
	}
	data, contentType, err := store.Get(ctx, uid)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(data) != string(pngHeader) {
		t.Fatalf("Get data mismatch: %d bytes", len(data))
	}
	if contentType != "image/png" {
		t.Fatalf("content type: want image/png, got %q", contentType)
	}

	if err := store.Delete(ctx, uid); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, err := store.Get(ctx, uid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete: want ErrNotFound, got %v", err)
	}
}
