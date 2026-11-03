import { fileURLToPath, URL } from 'node:url';
import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';

// 开发代理目标：KAMC_API 环境变量优先，默认平台本地监听地址。
// 平台无 CORS，浏览器必须与 API 同源访问，开发期由 Vite 反代实现。
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const target = process.env.KAMC_API || env.KAMC_API || 'http://127.0.0.1:8080';

  return {
    plugins: [react()],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    server: {
      port: 5173,
      host: '0.0.0.0',
      proxy: {
        '/api': { target, changeOrigin: true },
        '/auth': { target, changeOrigin: true },
        '/avatars': { target, changeOrigin: true },
        '/proxy': { target, changeOrigin: true, ws: true },
        '/.well-known': { target, changeOrigin: true },
        '/healthz': { target, changeOrigin: true },
        '/readyz': { target, changeOrigin: true },
        '/version': { target, changeOrigin: true },
      },
    },
    build: {
      // 生产构建产物统一落在仓库根 data/web（后端 AM_WEB_DIR 默认读取该目录）
      outDir: '../data/web',
      emptyOutDir: true,
      chunkSizeWarningLimit: 1600,
    },
  };
});
