import { fileURLToPath, URL } from 'node:url';
import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';

// 开发代理：前端与平台同源（生产由 nginx 以相同规则反代，见 web/README.md）
const target = process.env.KAMC_API || 'http://127.0.0.1:8080';

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  css: {
    preprocessorOptions: {
      scss: {
        // 每个 scss 编译单元（含 SFC style 块）自动获得主题变量；
        // Element Plus 走预编译 CSS（dist/index.css），不受此注入影响
        additionalData: `@use "${fileURLToPath(new URL('./src/styles/variables.scss', import.meta.url))}" as *;\n`,
        silenceDeprecations: ['legacy-js-api'],
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': { target, changeOrigin: true },
      '/auth': { target, changeOrigin: true },
      '/proxy': { target, changeOrigin: true, ws: true },
      '/.well-known': { target, changeOrigin: true },
      '/healthz': { target, changeOrigin: true },
      '/readyz': { target, changeOrigin: true },
      '/version': { target, changeOrigin: true },
    },
  },
  build: {
    chunkSizeWarningLimit: 1500,
  },
});
