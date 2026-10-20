/// <reference types="vitest/config" />
import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig, loadEnv, type Plugin, type UserConfig } from 'vite'

/** The Go server `vite dev` proxies to. */
const GO_SERVER = 'http://localhost:8123'

/** The runtime settings block the Go server puts in the page it serves
 *  (`internal/static`). */
const CONFIG_BLOCK = /<script type="application\/json" id="uno-config">.*?<\/script>/s

/**
 * Copies the Go server's settings block into the page `vite dev` serves, so
 * the SPA reads the same block in dev as in production. Without the Go server
 * there is no block to copy, and the page says to start it.
 */
function goConfigBlock(): Plugin {
  return {
    name: 'uno-go-config-block',
    async transformIndexHtml(html) {
      const page = await fetch(`${GO_SERVER}/`)
        .then((res) => res.text())
        .catch(() => '')
      const block = CONFIG_BLOCK.exec(page)?.[0]
      if (!block) {
        return `<!doctype html><title>Uno</title><p>Uno's Go server isn't answering at ${GO_SERVER}. Start it (<code>go run ./cmd/uno</code> from the repo root), then reload.</p>`
      }
      return html.replace('</head>', `${block}\n</head>`)
    },
  }
}

/** The Latin files of the two fonts every page draws text in (`src/index.css`). */
const PRELOADED_FONTS = /^assets\/(figtree-latin-wght|archivo-latin-wdth)-normal-[\w-]+\.woff2$/

/**
 * Preloads the built page's two Latin font files, so the browser fetches them
 * alongside the stylesheet rather than once the stylesheet has been parsed.
 */
function preloadFonts(): Plugin {
  let base = '/'
  return {
    name: 'uno-preload-fonts',
    apply: 'build',
    configResolved(resolved) {
      base = resolved.base
    },
    transformIndexHtml: {
      order: 'post',
      handler(_html, ctx) {
        return Object.keys(ctx.bundle ?? {})
          .filter((file) => PRELOADED_FONTS.test(file))
          .map((file) => ({
            tag: 'link',
            attrs: { rel: 'preload', as: 'font', type: 'font/woff2', crossorigin: true, href: base + file },
            injectTo: 'head' as const,
          }))
      },
    },
  }
}

// https://vite.dev/config/
//
// The dev server alone reads DEV_AUTH_BYPASS_TOKEN from the repo-root .env
// the Go server reads, so the login page's bypass button sends the token the
// server expects. Keyed on `command`, not `mode`: `vite build --mode
// development` must not inline the token into a bundle, and the settings
// block comes from the Go server serving the build, not the build itself.
export default defineConfig(({ command, mode }) => {
  if (command !== 'serve' || mode === 'test') {
    return config
  }
  const env = loadEnv(mode, path.resolve(import.meta.dirname, '..'), '')
  return {
    ...config,
    plugins: [...(config.plugins ?? []), goConfigBlock()],
    define: {
      'import.meta.env.VITE_DEV_AUTH_BYPASS_TOKEN': JSON.stringify(env.DEV_AUTH_BYPASS_TOKEN ?? ''),
    },
  }
})

const config: UserConfig = {
  plugins: [react(), tailwindcss(), preloadFonts()],
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, './src'),
    },
  },
  server: {
    proxy: {
      '/api': GO_SERVER,
      '/u': GO_SERVER,
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
