import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

/**
 * The SPA is served by the BFF from the same origin, so every API path in the
 * app is relative (`/bff/...`). No upstream host is ever baked into a build.
 * In `npm run dev:web` Vite proxies those paths to a locally running BFF.
 */
const BFF_DEV_TARGET = process.env.BFF_DEV_TARGET ?? 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src/app', import.meta.url)),
      '@shared': fileURLToPath(new URL('./src/shared', import.meta.url)),
    },
  },
  build: {
    outDir: 'dist/web',
    emptyOutDir: true,
    // Source maps are deliberately off: the production image ships no maps.
    sourcemap: false,
    chunkSizeWarningLimit: 900,
    // No manual chunking: the router lazy-loads every view, which already
    // splits d3 and the heavy pages out of the first paint.
  },
  server: {
    port: 5173,
    proxy: {
      '/bff': { target: BFF_DEV_TARGET, changeOrigin: true },
      '/healthz': { target: BFF_DEV_TARGET, changeOrigin: true },
      '/readyz': { target: BFF_DEV_TARGET, changeOrigin: true },
      '/metrics': { target: BFF_DEV_TARGET, changeOrigin: true },
    },
  },
})
