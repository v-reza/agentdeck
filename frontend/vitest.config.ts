import { defineConfig } from 'vitest/config'
import { fileURLToPath } from 'node:url'

/**
 * Unit-test config. Separate from vite.config.ts so the app build never carries
 * the test runner's settings, and so `vitest` does not have to parse the React
 * Compiler plugin for every unit test.
 */
export default defineConfig({
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./vitest.setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
  },
})
