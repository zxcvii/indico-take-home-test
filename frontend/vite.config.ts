import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// The Go backend sends no CORS headers, so in development API calls are proxied
// through the Vite dev server (same-origin from the browser's perspective).
const backendUrl = process.env.BACKEND_URL ?? 'http://localhost:8080'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': { target: backendUrl, changeOrigin: true },
      '/health': { target: backendUrl, changeOrigin: true },
    },
  },
})
