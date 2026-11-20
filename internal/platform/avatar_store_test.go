package platform

import (
	"context"
	"strings"
	"testing"

	"github.com/TuF3i/kamc/internal/platform/avatarstore"
)

func TestBuildAvatarStoreLocalDefault(t *testing.T) {
	dir := t.TempDir()
	store, err := buildAvatarStore(context.Background(), Config{
		AvatarBackend: "auto",
		AvatarDir:     dir,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, ok := store.(*avatarstore.Local); !ok {
		t.Fatalf("want local store, got %T", store)
	}

	// 显式 local 时即使配置了 endpoint 也走本地
	store, err = buildAvatarStore(context.Background(), Config{
		AvatarBackend: "local",
		AvatarDir:     dir,
		AvatarMinIO:   AvatarMinIOConfig{Endpoint: "minio:9000"},
	})
	if err != nil {
		t.Fatalf("explicit local: %v", err)
	}
	if _, ok := store.(*avatarstore.Local); !ok {
		t.Fatalf("want local store, got %T", store)
	}
}

func TestBuildAvatarStoreMinIOValidation(t *testing.T) {
	// auto + endpoint 选择 MinIO；缺少凭据必须启动失败而不是静默回退本地
	_, err := buildAvatarStore(context.Background(), Config{
		AvatarBackend: "auto",
		AvatarDir:     t.TempDir(),
		AvatarMinIO:   AvatarMinIOConfig{Endpoint: "minio:9000", Bucket: "kamc-avatars"},
	})
	if err == nil || !strings.Contains(err.Error(), "access key") {
		t.Fatalf("want access key error, got %v", err)
	}

	_, err = buildAvatarStore(context.Background(), Config{AvatarBackend: "unknown"})
	if err == nil || !strings.Contains(err.Error(), "unknown avatar backend") {
		t.Fatalf("want unknown backend error, got %v", err)
	}
}

func TestLoadConfigAvatarMinIO(t *testing.T) {
	t.Setenv("AM_AVATAR_BACKEND", "minio")
	t.Setenv("AM_AVATAR_MINIO_ENDPOINT", "minio:9000")
	t.Setenv("AM_AVATAR_MINIO_ACCESS_KEY", "ak")
	t.Setenv("AM_AVATAR_MINIO_SECRET_KEY", "sk")
	t.Setenv("AM_AVATAR_MINIO_BUCKET", "avatars")
	t.Setenv("AM_AVATAR_MINIO_USE_SSL", "1")
	t.Setenv("AM_AVATAR_MINIO_REGION", "us-east-1")
	t.Setenv("AM_AVATAR_MINIO_PREFIX", "/uploads/")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.AvatarBackend != "minio" {
		t.Fatalf("backend: %q", cfg.AvatarBackend)
	}
	if cfg.AvatarMinIO.Endpoint != "minio:9000" || cfg.AvatarMinIO.AccessKey != "ak" ||
		cfg.AvatarMinIO.SecretKey != "sk" || cfg.AvatarMinIO.Bucket != "avatars" ||
		!cfg.AvatarMinIO.UseSSL || cfg.AvatarMinIO.Region != "us-east-1" ||
		cfg.AvatarMinIO.Prefix != "/uploads/" {
		t.Fatalf("minio config: %+v", cfg.AvatarMinIO)
	}
}
