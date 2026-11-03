// Package webstatic 运行期静态前端托管：把磁盘上的前端构建产物
// （默认 web/dist）映射为后端路由，实现前后端同一进程启动。
//
// 与 go:embed 方案不同，前端产物不进入二进制——发布时前端与后端一起部署：
//
//	web/dist/…（Vite 构建产物）
//	bin/kamc-platform（从 AM_WEB_DIR 读取上述目录）
//
// SPA 路由规则：存在的静态文件直出（assets/ 长缓存、index.html 不缓存）；
// 未知 GET/HEAD 路径回退 index.html；/api /auth /proxy 等后端命名空间
// 不参与回退，避免 API 客户端拿到 HTML。
package webstatic

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Handler 以 dir 为根提供 SPA 静态资源；dir 不存在或无 index.html 时返回 nil。
func Handler(dir string) http.Handler {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	indexPath := filepath.Join(abs, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		return nil
	}
	root := http.Dir(abs)
	fileServer := http.FileServer(root)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// 后端命名空间保持原有语义（未注册即 404，不回退 SPA）
		if isBackendPath(r.URL.Path) {
			http.NotFound(w, r)
			return
		}

		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" || p == "." {
			p = "index.html"
		}
		// 目录穿越防护：清理后仍含 .. 的一律拒绝
		if strings.Contains(p, "..") {
			http.NotFound(w, r)
			return
		}
		if f, err := root.Open(p); err == nil {
			_ = f.Close()
			if strings.HasPrefix(p, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// 缺失的构建产物不回退（避免 JS 请求拿到 HTML）
		if strings.HasPrefix(p, "assets/") {
			http.NotFound(w, r)
			return
		}

		// SPA fallback：返回 index.html（不缓存，发版立即生效）
		index, err := fs.ReadFile(os.DirFS(abs), "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}

// isBackendPath 后端已注册/保留的路径前缀（不参与 SPA 回退）
func isBackendPath(p string) bool {
	for _, prefix := range []string{
		"/api/", "/auth/", "/proxy/", "/tunnel", "/avatars/",
		"/healthz", "/readyz", "/version", "/metrics",
		"/.well-known/", "/agent-install.yaml",
	} {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}
