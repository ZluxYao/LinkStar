import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  base: '/',
  plugins: [react(), tailwindcss()],
  server: {
    host: '0.0.0.0',
    port: 3009,
    strictPort: true,
    proxy: {
      '/api': 'http://localhost:3333',
      '/data': 'http://localhost:3333',
    },
  },
})
