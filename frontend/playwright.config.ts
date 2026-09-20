import { defineConfig, devices } from '@playwright/test'

/**
 * E2E config. The suite drives the real Vite dev server against the real Go API
 * (started separately on :8080), so a pass means the browser talked to the
 * backend — not that a mock answered.
 *
 * `reuseExistingServer` keeps a developer's already-running dev server usable,
 * and CI starts a fresh one.
 */
export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: [['list']],
  timeout: 60_000,
  use: {
    baseURL: 'http://127.0.0.1:5173',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    // The host is pinned: on this machine Vite otherwise binds IPv6 ::1 only,
    // while Playwright polls 127.0.0.1 — the server looks healthy in a terminal
    // and the suite still times out waiting for it.
    command: 'npm run dev -- --host 127.0.0.1 --port 5173 --strictPort',
    url: 'http://127.0.0.1:5173',
    reuseExistingServer: true,
    timeout: 120_000,
  },
})
