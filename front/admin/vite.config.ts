import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src'),
    },
  },
  server: {
    port: 5174,
    proxy: {
      '/api/admin': 'http://localhost:8082',
      '/audio': 'http://localhost:8081',
    },
  },
  build: {
    outDir: 'dist',
  },
})
