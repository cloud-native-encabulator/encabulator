import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

const alias = {
  '@': fileURLToPath(new URL('./src/app', import.meta.url)),
  '@shared': fileURLToPath(new URL('./src/shared', import.meta.url)),
}

export default defineConfig({
  test: {
    projects: [
      {
        // BFF: role enforcement, topology assembly, SSE, upstream clients.
        resolve: { alias },
        test: {
          name: 'bff',
          environment: 'node',
          include: ['src/server/**/*.test.ts', 'src/shared/**/*.test.ts', 'test/**/*.test.ts'],
        },
      },
      {
        // SPA: stores, components, the particle engine, honesty guards.
        plugins: [vue()],
        resolve: { alias },
        test: {
          name: 'web',
          environment: 'happy-dom',
          include: ['src/app/**/*.test.ts'],
        },
      },
    ],
  },
})
