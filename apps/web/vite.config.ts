import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

const apiUpstream = process.env.LICE_API_UPSTREAM ?? 'http://localhost:8080'

export default defineConfig({
  plugins: [react()],
  build: {
    sourcemap: false,
  },
  server: {
    proxy: {
      '/api': {
        target: apiUpstream,
        changeOrigin: false,
      },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./vitest.setup.ts'],
    css: true,
    restoreMocks: true,
  },
})
