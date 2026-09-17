import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 构建产物直出到 Go 内嵌目录（omnibot 模式的 backend 变体）：
// worldd 以 //go:embed 打包 backend/web/dist，单二进制自包含。
export default defineConfig({
  plugins: [vue()],
  build: {
    outDir: '../../backend/web/dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
