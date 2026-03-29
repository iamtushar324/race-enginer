import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 8092,
    proxy: {
      '/ws': { target: 'http://localhost:8081', ws: true },
      '/api': 'http://localhost:8081',
      '/health': 'http://localhost:8081',
    },
  },
})
