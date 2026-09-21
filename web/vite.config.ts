import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [
    vue(),
    tailwindcss(),
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    proxy: {
      // SSE endpoints necesitan timeout=0 y sin buffering para que el stream fluya.
      '/api/v1/sync/events': {
        target: 'http://localhost:8088',
        changeOrigin: true,
        timeout: 0,
        proxyTimeout: 0,
      },
      '/api/v1/worker/events': {
        target: 'http://localhost:8088',
        changeOrigin: true,
        timeout: 0,
        proxyTimeout: 0,
      },
      '/api': {
        target: 'http://localhost:8088',
        changeOrigin: true,
      },
    },
  },
})
