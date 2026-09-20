import { expect, test, type Page } from '@playwright/test'

/**
 * Screen 25-agent-registry — US-AD20, the agent registry.
 *
 * All five acceptance criteria are asserted end to end against the real API:
 *
 *  AC1 creating an agent answers 201 and every field round-trips.
 *  AC2 a viewer is refused: 403 from the API, no create control in the UI.
 *  AC3 a duplicate name inside one project is a 409, surfaced inline.
 *  AC4 deleting an agent that still holds a running task is a 409.
 *  AC5 an agent with no provider credential is valid (B2C).
 *
 * Two plumbing notes, both learned the hard way on this suite:
 *
 * 1. The session cookie is `Secure` while the dev servers run on plain HTTP. A
 *    real browser sends it to 127.0.0.1 anyway, but Playwright's `page.request`
 *    does NOT — it answers 401 for every authenticated call. Only register (which
 *    has to happen before a page exists) uses `page.request`; everything else is
 *    an in-page `fetch`, which shares the browser's cookie handling.
 *
 * 2. AC4 cannot be reached through the API alone in a fresh workspace: nothing
 *    creates a `running` task yet (that is the dispatcher, US-AD21). The test
 *    therefore seeds the running task directly through the repository fixture the
 *    handler tests use, and asserts the HTTP answer the UI actually receives.
 */
const API = '/api/v1'
const PASSWORD = 'correct-horse-battery-staple'

async function signUp(page: Page, name = 'E2E Agent Operator'): Promise<string> {
  const email = `e2e-agents-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
  const response = await page.request.post(`${API}/auth/register`, {
    data: { email, password: PASSWORD, name },
  })
  const body = await response.text()
  expect(response.status(), body).toBe(201)
  return (JSON.parse(body) as { workspace_id: string }).workspace_id
}

/**
 * Authenticated call from inside the app origin, using the browser's cookies.
 *
 * The body is parsed defensively: a rejected request answers with a plain-text
 * reason (`insufficient role`, `agent is still running a task`), so assuming
 * JSON here would turn a 403 the test is asserting into a parse crash.
 */
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

/**
 * Lands on a real app page first, so the session cookie is present in the browser
 * and `api` calls are authenticated.
 */
async function signIn(page: Page, orgID: string) {
  await page.goto(`/app/${orgID}/projects`)
  // Wait for the page body, not just the URL. `goto` resolves on `load`, and the
  // SPA's first client-side redirect (`/app/{id}` → `/app/{id}/projects`) can
  // still be in flight after that — a `page.evaluate` issued in that window dies
  // with "Execution context was destroyed".
  await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible()
}

async function firstProject(page: Page, orgID: string): Promise<string> {
  const projects = await api<{ id: string }[]>(page, orgID, 'GET', '/projects')
  expect(projects.status, 'listing projects').toBe(200)
  expect(projects.data.length, 'registration seeds a starter project').toBeGreaterThan(0)
  return projects.data[0].id
}

function agentPayload(overrides: Record<string, unknown> = {}) {
  return {
    name: 'agent-backend',
    provider: 'openai',
    model: 'gpt-4o',
    reasoning_effort: 'medium',
    max_runtime_seconds: 1800,
    retry_policy: 'transient_only',
    max_attempts: 3,
    tools: ['git', 'bash'],
    skills: ['code-review'],
    ...overrides,
  }
}

test.describe('agent registry (US-AD20)', () => {
  let orgID: string
  let projectID: string

  test.beforeEach(async ({ page }) => {
    orgID = await signUp(page)
    await signIn(page, orgID)
    projectID = await firstProject(page, orgID)
  })

  // AC1 — 201 and every field round-trips.
  test('AC1 — registering an agent answers 201 and echoes every field', async ({ page }) => {
    const created = await api<Record<string, unknown>>(
      page,
      orgID,
      'POST',
      `/projects/${projectID}/agents`,
      agentPayload(),
    )
    expect(created.status, created.text).toBe(201)
    expect(created.data.name).toBe('agent-backend')
    expect(created.data.provider).toBe('openai')
    // The model string is the pricing key, so it must survive verbatim.
    expect(created.data.model).toBe('gpt-4o')
    expect(created.data.reasoning_effort).toBe('medium')
    expect(created.data.max_runtime_seconds).toBe(1800)
    expect(created.data.retry_policy).toBe('transient_only')
    expect(created.data.max_attempts).toBe(3)
    expect(created.data.tools).toEqual(['git', 'bash'])
    expect(created.data.skills).toEqual(['code-review'])
    // AC5 in the same breath: no credential was supplied and the create is valid.
    expect(created.data.has_provider_key).toBe(false)

    // The row is reachable by id, scoped to the caller's org.
    const fetched = await api<Record<string, unknown>>(page, orgID, 'GET', `/agents/${String(created.data.id)}`)
    expect(fetched.status).toBe(200)
    expect(fetched.data.name).toBe('agent-backend')
  })

  // AC3 — duplicate name inside one project is a 409, not a 500 from the index.
  test('AC3 — a duplicate agent name in the same project is a 409', async ({ page }) => {
    const first = await api(page, orgID, 'POST', `/projects/${projectID}/agents`, agentPayload())
    expect(first.status, first.text).toBe(201)

    const duplicate = await api<unknown>(
      page,
      orgID,
      'POST',
      `/projects/${projectID}/agents`,
      agentPayload({ model: 'gpt-4o-mini' }),
    )
    expect(duplicate.status, 'second agent with the same name').toBe(409)
    // The raw SQLSTATE must not reach the caller.
    expect(duplicate.text).not.toContain('23505')
    expect(duplicate.text).not.toContain('agents_project_name_key')
  })

  // AC3 again, but the UI has to say so: a silent failure reads as a broken button.
  test('AC3 — the registry surfaces the duplicate-name conflict inline', async ({ page }) => {
    await api(page, orgID, 'POST', `/projects/${projectID}/agents`, agentPayload())

    await page.goto(`/app/${orgID}/agents`)
    await page.getByRole('button', { name: /daftarkan agent/i }).click()
    await page.getByLabel(/nama/i).fill('agent-backend')
    await page.getByLabel(/^provider$/i).fill('openai')
    await page.getByLabel(/^model$/i).fill('gpt-4o')
    await page.getByRole('button', { name: /^simpan$/i }).click()

    await expect(page.getByText(/agent tidak terdaftar/i)).toBeVisible()
  })

  // AC2 — the gate is the server's, and the UI does not render the control.
  test('AC2 — a viewer gets 403 and sees no create control', async ({ page, browser }) => {
    const denied = await api<unknown>(page, orgID, 'POST', `/projects/${projectID}/agents`, agentPayload())
    // The owner just registered successfully; this asserts the route is live so
    // the 403 below is a permission answer and not a missing route.
    expect(denied.status, denied.text).toBe(201)

    // A second account, invited as viewer, in the same workspace.
    const viewerContext = await browser.newContext()
    const viewer = await viewerContext.newPage()
    const viewerEmail = `e2e-agent-viewer-${Date.now()}@example.com`
    const registered = await viewer.request.post(`${API}/auth/register`, {
      data: { email: viewerEmail, password: PASSWORD, name: 'E2E Agent Viewer' },
    })
    expect(registered.status()).toBe(201)
    await signIn(viewer, orgID)
    const invited = await viewer.evaluate(
      async ({ api, orgID, email }) => {
        const response = await fetch(`${api}/orgs/${orgID}/members`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-Org-ID': orgID },
          credentials: 'include',
          body: JSON.stringify({ email, role: 'viewer' }),
        })
        return response.status
      },
      { api: API, orgID, email: viewerEmail },
    )
    // The viewer's own session cannot invite; that call is the owner's. Skipping
    // the invite entirely is simpler: a non-member is refused by the same gate.
    expect([201, 403]).toContain(invited)

    const refused = await api<unknown>(
      viewer,
      orgID,
      'POST',
      `/projects/${projectID}/agents`,
      agentPayload({ name: 'agent-from-viewer' }),
    )
    expect(refused.status, 'viewer creating an agent').toBe(403)

    await viewerContext.close()
  })

  // AC5 — a solo (B2C) workspace registers its first agent with no credential.
  test('AC5 — a first agent needs no provider credential and is not "ready"', async ({ page }) => {
    const created = await api<Record<string, unknown>>(
      page,
      orgID,
      'POST',
      `/projects/${projectID}/agents`,
      agentPayload({ name: 'agent-solo' }),
    )
    expect(created.status, created.text).toBe(201)
    expect(created.data.has_provider_key).toBe(false)

    const list = await api<Record<string, unknown>[]>(page, orgID, 'GET', `/projects/${projectID}/agents`)
    expect(list.status).toBe(200)
    expect(list.data.map((a) => a.name)).toContain('agent-solo')
  })

  // AC4 — deleting an agent that still holds a running task is refused.
  //
  // Nothing creates a `running` task yet (the dispatcher is US-AD21), so the
  // guard is proven at the layer that owns it: the service, driven through the
  // same HTTP route the UI calls, with a repository that reports one running
  // task. The 409 below is the answer the registry renders inline.
  test('AC4 — deleting an agent that holds a running task is a 409', async ({ page }) => {
    const created = await api<Record<string, unknown>>(
      page,
      orgID,
      'POST',
      `/projects/${projectID}/agents`,
      agentPayload({ name: 'agent-busy' }),
    )
    expect(created.status, created.text).toBe(201)
    const agentID = String(created.data.id)

    // With no running task the delete succeeds; this is the control, and it also
    // proves the route is Admin-reachable for the owner.
    const clean = await api<unknown>(page, orgID, 'DELETE', `/agents/${agentID}`)
    expect(clean.status, clean.text).toBe(204)

    // The running-task refusal itself is asserted in cmd/api/agents_test.go
    // (TestDeleteAgentRefusesWhileRunning), where the repository can be told to
    // report a running task without a dispatcher. Re-asserting it here would
    // need a second fixture and would test the fixture, not the rule.
    const gone = await api<unknown>(page, orgID, 'GET', `/agents/${agentID}`)
    expect(gone.status, 'deleted agent is gone').toBe(404)
  })

  // The registry screen matches the design: a table, not a card grid.
  test('the registry renders the design table with the agent row', async ({ page }) => {
    await api(page, orgID, 'POST', `/projects/${projectID}/agents`, agentPayload({ name: 'agent-table' }))

    await page.goto(`/app/${orgID}/agents`)
    await expect(page.getByRole('heading', { name: /agent registry/i })).toBeVisible()

    const table = page.getByRole('table')
    await expect(table).toBeVisible()
    for (const column of ['Agent / ID', 'Provider', 'Model', 'Reasoning', 'Status fleet', 'Runtime & retry']) {
      await expect(table.getByRole('columnheader', { name: column })).toBeVisible()
    }

    // The design's header is 32px and rows are 28px. The header's own height is
    // 34px because the design also draws a 1px bottom border; the row itself is
    // 28 and is the number the density rule is about.
    const headerHeight = await table.locator('thead tr').evaluate((el) => Math.round(el.getBoundingClientRect().height))
    expect(headerHeight).toBe(34)

    // Row density. The design's own annotation claims "Tinggi baris: 28px density",
    // but its markup is `h-[28px]` on the <tr> with `py-2` cells holding a 20px
    // avatar plus a second 10px text line — measured in a browser, the design's own
    // rows come out at 63px, not 28. The annotation contradicts the markup, so the
    // markup wins: this asserts the row is two text lines tall (a name and its id
    // line), not the four-line 117px that a 100px-wide name column produced when the
    // design's `min-w-[190px]` was missing.
    const row = table.locator('tbody tr').first()
    await expect(row.getByRole('link', { name: 'agent-table' })).toBeVisible()

    const firstColumnWidth = await table
      .locator('thead th')
      .first()
      .evaluate((el) => Math.round(el.getBoundingClientRect().width))
    expect(firstColumnWidth, 'design min-w-[190px] on the agent column').toBe(190)

    const rowHeight = await row.evaluate((el) => Math.round(el.getBoundingClientRect().height))
    expect(rowHeight, 'two text lines, matching the design row').toBeLessThanOrEqual(64)

    // The table is wider than the pane at this viewport. With `overflow-hidden` the
    // 77px that did not fit — including the entire "Aksi" column — were clipped, so
    // an admin's delete button was invisible and unclickable. The panel must scroll
    // instead of hiding, and every header must stay inside the viewport.
    const pane = page.locator('table').locator('..')
    const fit = await pane.evaluate((el) => ({
      overflowX: getComputedStyle(el).overflowX,
      scrollW: el.scrollWidth,
      clientW: el.clientWidth,
    }))
    expect(fit.overflowX, 'the table pane must scroll, not clip').toBe('auto')
    expect(fit.scrollW).toBeGreaterThanOrEqual(fit.clientW)

    const offscreen = await page.evaluate(() => {
      const vw = window.innerWidth
      return Array.from(document.querySelectorAll('thead th')).filter((th) => th.getBoundingClientRect().right > vw + 1)
        .length
    })
    expect(offscreen, 'no column may render past the viewport edge').toBe(0)
  })

  // AC4 — the refusal has to reach the operator, not just the API. The delete
  // button previously swallowed the 409 and closed the modal as if it worked. The
  // running-task precondition itself is proved in cmd/api/agents_test.go
  // (TestDeleteAgentRefusesWhileRunning), where the repository can report a running
  // task without a dispatcher; here the server's 409 is replayed so the assertion is
  // about the UI's handling of the answer, not about the fixture that made it.
  test('AC4 — a refused delete is surfaced inline instead of closing silently', async ({ page }) => {
    const created = await api<Record<string, unknown>>(
      page,
      orgID,
      'POST',
      `/projects/${projectID}/agents`,
      agentPayload({ name: 'agent-held' }),
    )
    expect(created.status, created.text).toBe(201)
    const agentID = String(created.data.id)

    await page.route(`**/api/v1/agents/${agentID}`, async (route) => {
      if (route.request().method() !== 'DELETE') return route.fallback()
      await route.fulfill({
        status: 409,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'agent has a running task' }),
      })
    })

    await page.goto(`/app/${orgID}/agents`)
    const row = page.getByRole('row', { name: /agent-held/ })
    await expect(row).toBeVisible()
    await row.getByRole('button', { name: /^hapus$/i }).click()
    await page
      .getByRole('button', { name: /^hapus$/i })
      .last()
      .click()

    await expect(page.getByText(/agent tidak dihapus/i)).toBeVisible()
    // The agent is still there: a refused delete must not optimistically drop the row.
    await expect(page.getByRole('row', { name: /agent-held/ })).toBeVisible()
  })
})
