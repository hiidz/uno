/// <reference types="vitest/config" />
import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig, loadEnv, type UserConfig } from 'vite'

// https://vite.dev/config/
//
// The dev server alone reads DEV_AUTH_BYPASS_TOKEN from the repo-root .env
// the Go server reads, so the login page's bypass button sends the token the
// server expects. Keyed on `command`, not `mode`: `vite build --mode
// development` must not inline the token into a bundle.
export default defineConfig(({ command, mode }) => {
  if (command !== 'serve' || mode === 'test') {
    return config
  }
  const env = loadEnv(mode, path.resolve(import.meta.dirname, '..'), '')
  return {
    ...config,
    define: {
      'import.meta.env.VITE_DEV_AUTH_BYPASS_TOKEN': JSON.stringify(env.DEV_AUTH_BYPASS_TOKEN ?? ''),
    },
  }
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
      '/config.js': 'http://localhost:8123',
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
