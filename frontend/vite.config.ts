/// <reference types="vitest/config" />
import react from '@vitejs/plugin-react'
import { defineConfig, loadEnv } from 'vite'

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  // Go API (backend/cmd/api). В docker compose он опубликован на 8080.
  const api = env.VITE_API_URL || 'http://localhost:8080'
  return {
    plugins: [react()],
    server: {
      proxy: {
        // SSE /api/chat тоже идёт через прокси: Vite не буферизует поток.
        '/api': { target: api, changeOrigin: true },
      },
    },
    test: {
      environment: 'node',
    },
  }
})
