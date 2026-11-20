package avatarstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// pngHeader 1x1 PNG（用于内容类型探测，不校验完整图片）。
var pngHeader = []byte{
	0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
}

func TestLocalStorePutGetDelete(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocal(dir)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	ctx := context.Background()
	const uid = "11111111-1111-1111-1111-111111111111"

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

	info, err := os.Stat(filepath.Join(dir, uid))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file mode: want 0600, got %o", perm)
	}

	if err := store.Delete(ctx, uid); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, err := store.Get(ctx, uid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete: want ErrNotFound, got %v", err)
	}
	// 再删一次幂等
	if err := store.Delete(ctx, uid); err != nil {
		t.Fatalf("Delete missing: %v", err)
	}
}

func TestLocalStoreRejectsInvalidUID(t *testing.T) {
	store, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	for _, uid := range []string{"", "../etc/passwd", "a/b", "中文"} {
		if err := store.Put(context.Background(), uid, "image/png", pngHeader); !errors.Is(err, ErrInvalidUID) {
			t.Fatalf("Put(%q): want ErrInvalidUID, got %v", uid, err)
		}
		if _, _, err := store.Get(context.Background(), uid); !errors.Is(err, ErrInvalidUID) {
			t.Fatalf("Get(%q): want ErrInvalidUID, got %v", uid, err)
		}
	}
}
