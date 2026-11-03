// Package webui 嵌入前端构建产物（web/dist），使平台后端单二进制同时
// 提供管理前端与 API。
//
// 构建方式：`make release` 会先构建前端，再把 web/dist 复制到本包 dist/
// 目录后编译（见 Makefile）；开发阶段不复制时仅嵌入 .gitkeep，
// Available() 返回 false，平台退化为纯 API 服务（前端由 Vite 提供）。
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var embedded embed.FS

// Available 是否嵌入了可用的前端产物（存在 index.html）
func Available() bool {
	_, err := fs.Stat(embedded, "dist/index.html")
	return err == nil
}

// Handler 返回 SPA 静态资源处理器：
//   - 存在的静态文件直接返回（assets/ 带强缓存）；
//   - 其余 GET/HEAD 路径回退 index.html（前端路由）；
//   - /api /auth /proxy 等后端命名空间不 fallback，避免把 HTML 返给 API 客户端。
func Handler() http.Handler {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// 后端命名空间保持 JSON/纯文本语义（未注册即 404，不回退 SPA）
		if isBackendPath(r.URL.Path) {
			http.NotFound(w, r)
			return
		}

		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" || p == "." {
			p = "index.html"
		}
		if f, err := sub.Open(p); err == nil {
			_ = f.Close()
			if strings.HasPrefix(p, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// 缺失的构建产物不回退（避免 JS 请求拿到 HTML 导致难以排查的语法错误）
		if strings.HasPrefix(p, "assets/") {
			http.NotFound(w, r)
			return
		}

		// SPA fallback：返回 index.html（不缓存，保证发版后立即生效）
		index, err := fs.ReadFile(sub, "index.html")
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
