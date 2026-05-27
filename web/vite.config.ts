import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// 出力は dist/（Goが embed.go で埋め込む）。base:'./' で相対パス参照。
// 開発時は `npm run dev` で /api を Go バックエンド(8080)へプロキシ。
export default defineConfig({
  plugins: [react()],
  base: './',
  build: { outDir: 'dist', emptyOutDir: true },
  server: {
    proxy: {
      '/api': { target: 'http://localhost:8080', changeOrigin: true },
    },
  },
})
