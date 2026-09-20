import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { fileURLToPath } from 'node:url'

/** Where the API lives: the host port by default, the service name in Docker. */
const proxyTarget = process.env.VITE_API_PROXY_TARGET ?? 'http://127.0.0.1:8080'

/**
 * ARCHITECTURE 18.2: Vite + Tailwind v4 + React Compiler.
 *
 * The React Compiler is enabled because the contract makes it the reason this
 * app carries no hand-written `useMemo`/`useCallback`: memoization is the
 * compiler's job. The config is ESM (`"type": "module"`), so the source alias is
 * derived from `import.meta.url` — `__dirname` does not exist in this module.
 */
export default defineConfig({
  plugins: [react({ compiler: true }), tailwindcss()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    // Host dev points at the local API; inside the compose stack the API is a
    // sibling container, so the target comes from the environment there.
    proxy: {
      '/api': proxyTarget,
      '/healthz': proxyTarget,
      '/livez': proxyTarget,
      '/readyz': proxyTarget,
    },
    watch: {
      // The source tree is a Windows bind mount, and inotify events do not
      // propagate across that boundary into the container. Without polling,
      // Vite keeps serving the module it cached at startup: edits are on disk
      // and the browser still receives the previous version. That shipped three
      // separate times as a "my fix does nothing" bug — a CSS import that never
      // reached the DOM, a listener change that appeared inert, and a copy
      // change the tests could not see — each one only fixed by restarting the
      // container. Polling costs CPU and buys a dev server that tells the truth.
      usePolling: true,
      interval: 300,
    },
  },
})
