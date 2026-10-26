import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// In dev the Go server runs on :8080 with REFRACT_AUTH=none.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
      '/healthz': 'http://localhost:8080',
    },
  },
})
