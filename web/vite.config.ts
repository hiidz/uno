/// <reference types="vitest/config" />
import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig, loadEnv, type UserConfig } from 'vite'

const requiredEnv = ['VITE_NUVIO_BASE_URL', 'VITE_NUVIO_PUBLISHABLE_KEY']

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  if (mode !== 'test') {
    const env = loadEnv(mode, import.meta.dirname, 'VITE_')
    const missing = requiredEnv.filter((name) => !env[name])
    if (missing.length > 0) {
      throw new Error(`missing ${missing.join(', ')}; see web/.env.example`)
    }
  }
  return config
})

const config: UserConfig = {
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, './src'),
    },
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8123',
      '/u': 'http://localhost:8123',
    },
  },
  // Tests run in node; a file that renders or needs `window` opts into jsdom
  // with a `// @vitest-environment jsdom` comment on its first line.
  test: {
    setupFiles: ['./src/test/setup.ts'],
    coverage: {
      provider: 'v8',
      include: ['src/**/*.{ts,tsx}'],
      exclude: ['src/**/*.test.{ts,tsx}', 'src/test/**'],
    },
  },
}
