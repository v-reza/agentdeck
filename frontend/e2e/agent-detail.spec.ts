import { expect, test, type Page } from '@playwright/test'

/**
 * Screen 28-agent-detail — the one-agent configuration page.
 *
 * This file exists because the registry's jargon guard never opened this page.
 * A raw `needsKey` enum reached the badge in production while every gate stayed
 * green: the guard in `agents.spec.ts` only walked the list screen. The tests
 * here walk the detail screen and pin the two things that regressed, plus the
 * archive toggle the screen was rebuilt for.
 *
 * Auth plumbing mirrors `agents.spec.ts`: registration returns a workspace id,
 * and every authenticated call goes through the browser's own cookies because
 * the session cookie is `Secure` while the dev servers run on plain HTTP.
 */
const API = '/api/v1'
const PASSWORD = 'correct-horse-battery-staple'

async function signUp(page: Page, name = 'E2E Detail Operator'): Promise<string> {
  const email = `e2e-detail-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
  const response = await page.request.post(`${API}/auth/register`, {
    data: { email, password: PASSWORD, name },
  })
  const body = await response.text()
  expect(response.status(), body).toBe(201)
  return (JSON.parse(body) as { workspace_id: string }).workspace_id
}

async function api<T>(
  page: Page,
  orgID: string,
  method: 'GET' | 'POST' | 'PATCH' | 'DELETE',
  path: string,
  body?: unknown,
): Promise<{ status: number; data: T; text: string }> {
  return page.evaluate(
    async ({ api, orgID, method, path, body }) => {
      const response = await fetch(`${api}${path}`, {
        method,
        headers: body ? { 'Content-Type': 'application/json', 'X-Org-ID': orgID } : { 'X-Org-ID': orgID },
        credentials: 'include',
        body: body ? JSON.stringify(body) : undefined,
      })
      const text = await response.text()
      let data: unknown = null
      try {
        data = text ? JSON.parse(text) : null
      } catch {
        data = null
      }
      return { status: response.status, data: data as never, text }
    },
    { api: API, orgID, method, path, body },
  )
}

async function signIn(page: Page, orgID: string) {
  await page.goto(`/app/${orgID}/projects`)
  await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible()
}

const AGENT = {
  name: 'agent-detail-e2e',
  provider: 'openai',
  model: 'gpt-4o',
  reasoning_effort: 'medium',
  max_runtime_seconds: 1800,
  retry_policy: 'transient_only',
  max_attempts: 3,
  tools: ['git'],
  skills: [],
}

/** Seeds one agent through the real API and opens its detail page. */
async function openDetail(page: Page, orgID: string, overrides: Record<string, unknown> = {}) {
  const projects = await api<{ id: string }[]>(page, orgID, 'GET', '/projects')
  expect(projects.status, 'listing projects').toBe(200)
  const projectID = projects.data[0].id

  const created = await api<{ id: string }>(page, orgID, 'POST', `/projects/${projectID}/agents`, {
    ...AGENT,
    ...overrides,
  })
  expect(created.status, created.text).toBe(201)
  const agentID = created.data.id

  await page.goto(`/app/${orgID}/agents/${agentID}`)
  await expect(page.getByTestId('agent-status-pill')).toBeVisible()
  return { projectID, agentID }
}

test.describe('agent detail (28-agent-detail)', () => {
  let orgID: string

  test.beforeEach(async ({ page }) => {
    orgID = await signUp(page)
    await signIn(page, orgID)
  })

  /**
   * The regression this file was written for: `agentState()` returns the enum
   * `needsKey`, and printing it put that literal in the badge. An agent with no
   * credential renders "NEEDS CREDENTIAL" (or its Indonesian equivalent), never
   * the camelCase identifier.
   */
  test('never prints the lifecycle enum, only its dictionary label', async ({ page }) => {
    await openDetail(page, orgID)

    const body = (await page.locator('#root').innerText()).replace(/\s+/g, ' ')
    for (const leaked of ['needsKey', 'NEEDSKEY', 'has_provider_key', 'archived_at']) {
      expect(body, `raw identifier leaked into the UI: ${leaked}`).not.toContain(leaked)
    }
    // The label itself must be present, so the assertion above cannot pass by
    // rendering nothing at all.
    expect(body).toMatch(/NEEDS CREDENTIAL|BUTUH KREDENSIAL/i)
  })

  /** The same jargon rule the registry page enforces, applied to this screen. */
  test('the page never renders story, AC, milestone, or status-code jargon', async ({ page }) => {
    await openDetail(page, orgID)

    const body = (await page.locator('#root').innerText()).replace(/\s+/g, ' ')
    for (const pattern of [
      /US-AD\d+/, // story ids
      /\bAC\d\b/, // acceptance-criterion numbers
      /HTTP \d{3}/, // status codes used as copy
      // Milestone phases, anchored to whitespace: a bare `\bM[1-6]\b` also
      // matches real model names in the dropdown ("MiniMax-M2.1", "MiniMax-M3"),
      // which is a false positive, not a leak.
      /(?:^|\s)M[1-6](?=\s|$)/,
      /B2C|B2B/, // segment jargon
      /internal\/pricing/, // module path from the mock
    ]) {
      expect(body, `jargon leaked into the UI: ${pattern}`).not.toMatch(pattern)
    }
  })

  /** Every cost figure is an estimate, and the screen says so (US-AD108 AC1). */
  test('every rate carries the estimate wording', async ({ page }) => {
    await openDetail(page, orgID)

    const body = await page.locator('#root').innerText()
    const rates = body.match(/\$[\d.,]+/g) ?? []
    expect(rates.length, 'the model has catalog rates to render').toBeGreaterThan(0)
    expect(body).toMatch(/estimate|estimasi/i)
  })

  test('archiving the agent answers 204 and the page re-renders as archived', async ({ page }) => {
    await openDetail(page, orgID)

    const archive = page.getByRole('button', { name: /arsipkan|archive/i })
    await expect(archive).toBeEnabled()
    await archive.click()

    // The pill flips to the archived wording once the PATCH lands and the tag
    // invalidation refetches the row.
    await expect(page.getByTestId('agent-status-pill')).toContainText(/DIARSIP|ARCHIVED/i)
    await expect(page.getByRole('button', { name: /batal arsip|unarchive/i })).toBeVisible()
  })

  test('a saved edit round-trips through PATCH and survives a reload', async ({ page }) => {
    const { agentID } = await openDetail(page, orgID)

    // `max_attempts` is one of the fields this screen actually edits; there is
    // no name field here (the registry owns creation, this page owns the spec).
    const attempts = page.locator('#agent-attempts')
    await attempts.fill('5')
    await page.getByRole('button', { name: /simpan|save/i }).click()

    await expect
      .poll(async () => {
        const read = await api<{ max_attempts: number }>(page, orgID, 'GET', `/agents/${agentID}`)
        return read.data.max_attempts
      })
      .toBe(5)

    await page.reload()
    await expect(page.locator('#agent-attempts')).toHaveValue('5')
  })
})
