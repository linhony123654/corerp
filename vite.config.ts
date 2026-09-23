import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  base: './',
  build: {
    target: 'es2022',
    cssTarget: 'chrome120',
    assetsInlineLimit: 0,
    chunkSizeWarningLimit: 900
  },
  server: {
    host: '0.0.0.0',
    port: 5173
  }
})
