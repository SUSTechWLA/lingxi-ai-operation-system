import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  base: './',
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          'react-vendor': ['react', 'react-dom', 'react-icons'],
          'markdown-vendor': ['react-markdown'],
          'http-vendor': ['axios'],
        },
      },
    },
  },
  server: {
    port: 3000,
    proxy: {
      '/api/local': {
        target: 'http://127.0.0.1:18080',
        changeOrigin: true,
      },
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
