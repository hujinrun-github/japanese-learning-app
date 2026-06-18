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
    port: 35174,
    allowedHosts: ['tylerhu-1.tail5cec87.ts.net'],
    proxy: {
      '/api/admin': 'http://localhost:30082',
      '/audio': 'http://localhost:30081',
    },
  },
  build: {
    outDir: 'dist',
  },
})
