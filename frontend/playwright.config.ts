import { defineConfig, devices } from '@playwright/test'

/**
 * E2E config. The suite drives the real Vite dev server against the real Go API
 * (started separately on :8080), so a pass means the browser talked to the
 * backend — not that a mock answered.
 *
 * `reuseExistingServer` keeps a developer's already-running dev server usable,
 * and CI starts a fresh one.
 *
 * `E2E_BASE_URL` (and `E2E_WS_URL`) override the target. It exists because some
 * suites need the dev server to proxy somewhere else: the artifact download URL
 * carries the storage host inside a signature, and only a server and a browser
 * on the same side of the container boundary agree on it. Additive — with the
 * variables unset every spec runs exactly where it ran before.
 */
const baseURL = process.env.E2E_BASE_URL ?? 'http://127.0.0.1:5173'

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: [['list']],
  timeout: 60_000,
  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    // The host is pinned: on this machine Vite otherwise binds IPv6 ::1 only,
    // while Playwright polls 127.0.0.1 — the server looks healthy in a terminal
    // and the suite still times out waiting for it.
    command: `npm run dev -- --host 127.0.0.1 --port ${new URL(baseURL).port} --strictPort`,
    url: baseURL,
    reuseExistingServer: true,
    timeout: 120_000,
  },
})
