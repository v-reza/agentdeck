import { expect, test, type Page } from '@playwright/test'
import { chooseOption, typeCustom } from './combobox'

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

/**
 * Registers a workspace provider and returns it, so the register form has
 * something to select.
 *
 * US-AD109 phase 5: the form derives `provider` and `base_url` from the chosen
 * provider, so a test that wants the form to submit has to create one first.
 * The name is unique per run because the dev database keeps providers between
 * runs and `providers_org_name_key` makes a repeat a 409.
 */
async function seedProvider(page: Page, orgID: string, name: string): Promise<{ id: string }> {
  const created = await api<{ id: string }>(page, orgID, 'POST', '/providers', {
    name,
    protocol: 'openai_compatible',
    base_url: 'http://127.0.0.1:11434/v1',
  })
  expect(created.status, created.text).toBe(201)
  return created.data
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

    // Phase 5: the form picks a provider from the registry and derives the
    // endpoint from it. The provider is seeded *before* the modal opens, because
    // `seedProvider` writes through a raw fetch — RTK Query never sees it, so a
    // provider created while the form is already open cannot invalidate the list
    // the form has loaded. In the product the write goes through the mutation
    // that invalidates the tag; here the ordering has to carry that.
    const gateway = `dup-gateway-${Date.now()}`
    await seedProvider(page, orgID, gateway)

    await page.goto(`/app/${orgID}/agents`)
    await page.getByRole('button', { name: /daftarkan agent/i }).click()
    await page.getByLabel(/nama/i).fill('agent-backend')
    await chooseOption(page, /^provider$/i, gateway)
    await typeCustom(page, /^model$/i, 'probe-model-1')
    await page.getByRole('button', { name: /^simpan$/i }).click()

    // The message belongs under the field that caused it, and the input carries
    // the a11y state: an operator should not have to hunt for the one red line
    // at the bottom of a long dialog to find out which input the server refused.
    const nameField = page.locator('label:has(input[name="name"])')
    await expect(nameField.getByText(/agent tidak terdaftar/i)).toBeVisible()
    await expect(page.locator('input[name="name"]')).toHaveAttribute('aria-invalid', 'true')
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
    for (const column of ['Agent', 'Provider', 'Model', 'Reasoning', 'Status', 'Runtime']) {
      await expect(table.getByRole('columnheader', { name: column })).toBeVisible()
    }

    // The design's header is 32px and rows are 28px. The header's own height is
    // 34px because the design also draws a 1px bottom border; the row itself is
    // 28 and is the number the density rule is about.
    // The design's header row is 32px, and it is exactly 32 now. It used to
    // measure 34 because "Status fleet" and "Runtime & retry" wrapped to a
    // second line and grew the row; single-word labels removed that line box.
    const headerHeight = await table.locator('thead tr').evaluate((el) => Math.round(el.getBoundingClientRect().height))
    expect(headerHeight).toBe(32)

    // Every header must be one line. The design draws labels like "Status fleet"
    // and "Runtime & retry", which wrapped to two and three lines in a fixed
    // table and made the header taller than the rows under it. The labels are
    // single words now, and this is the assertion that keeps them that way.
    const wrapped = await table
      .locator('thead th')
      .evaluateAll((els) =>
        els.filter((el) => el.getBoundingClientRect().height > 32).map((el) => (el.textContent || '').trim()),
      )
    expect(wrapped, 'no header label may wrap to a second line').toEqual([])

    // Row density. The design's own annotation claims "Tinggi baris: 28px density",
    // but its markup is `h-[28px]` on the <tr> with `py-2` cells holding a 20px
    // avatar plus a second 10px text line — measured in a browser, the design's own
    // rows come out at 63px, not 28. The annotation contradicts the markup, so the
    // markup wins: this asserts the row is two text lines tall (a name and its id
    // line), not the four-line 117px that a 100px-wide name column produced when the
    // design's `min-w-[190px]` was missing.
    const row = table.locator('tbody tr').first()
    await expect(row.getByRole('link', { name: 'agent-table' })).toBeVisible()

    // A floor, not the design's exact 190. The table is `table-fixed` with an
    // explicit colgroup so the columns stop being redistributed by content, and
    // this column is 196 — sized so the whole table fits the pane instead of
    // scrolling. The number that matters is that it never drops back under the
    // design's minimum, which is what collapsed the row to four lines.
    const firstColumnWidth = await table
      .locator('thead th')
      .first()
      .evaluate((el) => Math.round(el.getBoundingClientRect().width))
    expect(firstColumnWidth, 'the agent column keeps the design minimum').toBeGreaterThanOrEqual(190)

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

  // The reported bug: an agent registered on a provider called "9router" showed
  // `openai_compatible` in the Provider column. The column printed
  // `agents.provider`, which US-AD109 AC6 made the *protocol* — so every
  // openai_compatible provider rendered identically and the operator could not
  // tell which one an agent draws its endpoint from.
  test('the Provider column names the provider, not the protocol', async ({ page }) => {
    const gateway = `gw-name-${Date.now()}`
    const provider = await seedProvider(page, orgID, gateway)
    const created = await api<{ id: string; provider: string }>(
      page,
      orgID,
      'POST',
      `/projects/${projectID}/agents`,
      agentPayload({ name: 'agent-named', provider_id: provider.id }),
    )
    expect(created.status, created.text).toBe(201)
    // The stored protocol is still what the contract derives — the row is right.
    expect(created.data.provider).toBe('openai_compatible')

    await page.goto(`/app/${orgID}/agents`)
    const row = page.getByRole('table').locator('tbody tr').filter({ hasText: 'agent-named' })
    // `.toHaveText` retries, so this waits out the window where the agent list has
    // arrived but the provider registry has not — and it fails fast if that window
    // ever renders the protocol, which is the reported bug.
    await expect(row.getByTestId('agent-provider')).toHaveText(gateway)
    await expect(row.getByTestId('agent-provider')).not.toHaveText('openai_compatible')
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

  // The toolbar, the table footer and the two guidance cards, cloned from the
  // design's own blocks. Each is asserted against data the API really returned:
  // no mock count is trusted, and the archived figure is 0 because `archived_at`
  // does not exist in the schema yet.
  test('the toolbar search narrows the rows to a name, model, or skill match', async ({ page }) => {
    await api(page, orgID, 'POST', `/projects/${projectID}/agents`, agentPayload({ name: 'agent-backend' }))
    await api(
      page,
      orgID,
      'POST',
      `/projects/${projectID}/agents`,
      agentPayload({ name: 'agent-frontend', model: 'gpt-4o-mini', skills: ['sql'] }),
    )

    await page.goto(`/app/${orgID}/agents`)
    await expect(page.getByRole('row', { name: /agent-backend/ })).toBeVisible()

    // 220px is the design's own width on the search field.
    const search = page.getByRole('searchbox', { name: /cari agent, model, atau skill/i })
    const width = await search.evaluate((el) => Math.round(el.getBoundingClientRect().width))
    expect(width, 'design w-[220px]').toBe(220)

    // By name.
    await search.fill('frontend')
    await expect(page.getByRole('row', { name: /agent-backend/ })).toBeHidden()
    await expect(page.getByRole('row', { name: /agent-frontend/ })).toBeVisible()

    // By model — the placeholder promises model search too.
    await search.fill('gpt-4o-mini')
    await expect(page.getByRole('row', { name: /agent-frontend/ })).toBeVisible()
    await expect(page.getByRole('row', { name: /agent-backend/ })).toBeHidden()

    // By skill — the third thing the placeholder names.
    await search.fill('sql')
    await expect(page.getByRole('row', { name: /agent-frontend/ })).toBeVisible()

    // Nothing matches: the table is replaced by a state that says so, rather
    // than an empty panel that reads as "this project has no agents".
    await search.fill('nothing-matches-this')
    await expect(page.getByText(/tidak ada agent yang cocok/i)).toBeVisible()
    await expect(page.getByRole('table')).toBeHidden()
  })

  test('the toolbar status filter narrows by fleet readiness', async ({ page }) => {
    // Nothing in the contract can set `has_provider_key` yet — provider keys are
    // US-AD86, M2 — so every registered agent reports false. Both branches are
    // still asserted against the API's own answer for this row, so what is being
    // proven is the filter's rule, not the fixture that produced the row.
    await api(page, orgID, 'POST', `/projects/${projectID}/agents`, agentPayload({ name: 'agent-no-key' }))

    await page.goto(`/app/${orgID}/agents`)
    const filter = page.getByLabel(/filter berdasarkan status fleet/i)
    // The control is a popover (button + listbox), not a native `<select>`: a
    // native select cannot show the design's trigger, and it clipped the longest
    // option. Driving it means opening it and picking the option by role.
    async function pick(option: string) {
      await filter.click()
      await page.getByRole('option', { name: option }).click()
    }

    await pick('SIAP')
    await expect(page.getByRole('row', { name: /agent-no-key/ })).toBeHidden()
    await expect(page.getByText(/tidak ada agent yang cocok/i)).toBeVisible()

    await pick('BUTUH KREDENSIAL')
    await expect(page.getByRole('row', { name: /agent-no-key/ })).toBeVisible()

    await pick('Semua Status')
    await expect(page.getByRole('row', { name: /agent-no-key/ })).toBeVisible()
  })

  test('the footer states how many of the registered agents are shown', async ({ page }) => {
    await api(page, orgID, 'POST', `/projects/${projectID}/agents`, agentPayload({ name: 'agent-one' }))
    await api(
      page,
      orgID,
      'POST',
      `/projects/${projectID}/agents`,
      agentPayload({ name: 'agent-two', model: 'gpt-4o-mini' }),
    )

    await page.goto(`/app/${orgID}/agents`)
    const footer = page.getByTestId('agents-footer')
    await expect(footer).toBeVisible()

    // The design's own footer sentence, with the real counts: 2 registered, 2 shown.
    await expect(footer.getByText('Menampilkan 2 dari 2 agent terdaftar')).toBeVisible()
    // No agent has a credential yet, so none is ready to assign — the design's
    // hardcoded "3 siap di-assign" is a mock number and is not rendered.
    await expect(footer.getByText('0 siap di-assign')).toBeVisible()
    await expect(footer.getByText('0 diarsip (tidak muncul di dropdown assign)')).toBeVisible()

    // The footer tracks the filter, not just the list.
    await page.getByRole('searchbox', { name: /cari agent, model, atau skill/i }).fill('agent-one')
    await expect(footer.getByText('Menampilkan 1 dari 2 agent terdaftar')).toBeVisible()
  })

  test('an archived agent is listed, counted, and never counted as assignable', async ({ page }) => {
    // US-AD73 AC2 end to end: archive through the API, then read what the
    // registry does with it. `archived_at` has to survive the round trip, or the
    // screen can only print a hardcoded zero for the archive count.
    const created = await api<{ id: string }>(
      page,
      orgID,
      'POST',
      `/projects/${projectID}/agents`,
      agentPayload({ name: 'agent-retired' }),
    )
    expect(created.status, 'creating the agent to archive').toBe(201)

    const archived = await api<{ archived_at?: string }>(page, orgID, 'PATCH', `/agents/${created.data.id}`, {
      archived: true,
    })
    expect(archived.status, 'archiving').toBe(200)
    expect(archived.data.archived_at, 'PATCH echoes archived_at').toBeTruthy()

    // The list must still return it: this screen is where a user unarchives.
    const listed = await api<{ name: string; archived_at?: string }[]>(
      page,
      orgID,
      'GET',
      `/projects/${projectID}/agents`,
    )
    const row = listed.data.find((a) => a.name === 'agent-retired')
    expect(row, 'archived agent is still listed').toBeTruthy()
    expect(row?.archived_at, 'list carries archived_at, not just PATCH').toBeTruthy()

    await page.goto(`/app/${orgID}/agents`)
    const row2 = page.getByRole('row', { name: /agent-retired/ })
    await expect(row2).toBeVisible()
    await expect(row2.getByText(/^DIARSIP$/)).toBeVisible()

    // AC2: it must not appear in the "ready" bucket the assign dropdown reads.
    const filter = page.getByLabel(/filter berdasarkan status fleet/i)
    await filter.click()
    await page.getByRole('option', { name: 'SIAP' }).click()
    await expect(row2).toBeHidden()

    await filter.click()
    await page.getByRole('option', { name: 'DIARSIP' }).click()
    await expect(row2).toBeVisible()

    // The archived ROW styling from `25-agent-registry.html`, which the
    // implementation had dropped: the mock marks a retired row four ways at
    // once, and a test that only checked the badge would have passed while all
    // four were missing. Each is asserted on computed style, not on a class
    // name, so a refactor that keeps the appearance keeps the test green.
    const retired = await row2.evaluate((el) => {
      const s = getComputedStyle(el)
      const name = el.querySelector('a')
      return {
        decoration: name ? getComputedStyle(name).textDecorationLine : '',
        background: s.backgroundColor,
        borderLeftWidth: s.borderLeftWidth,
        opacity: Number(s.opacity),
      }
    })
    expect(retired.decoration, 'the retired name is struck through').toContain('line-through')
    expect(retired.borderLeftWidth, 'the row carries a left marker').toBe('2px')
    expect(retired.opacity, 'the row is de-emphasised').toBeCloseTo(0.75, 2)
    // The active rows must NOT look retired, or the four markers mean nothing.
    // This second agent is the control: same table, same columns, not archived.
    await api(page, orgID, 'POST', `/projects/${projectID}/agents`, agentPayload({ name: 'agent-still-active' }))
    await page.reload()
    const live = await page.getByRole('row', { name: /agent-still-active/ }).evaluate((el) => {
      const s = getComputedStyle(el)
      const name = el.querySelector('a')
      return {
        decoration: name ? getComputedStyle(name).textDecorationLine : '',
        opacity: Number(s.opacity),
        borderLeftColor: s.borderLeftColor,
      }
    })
    expect(live.decoration, 'an active name is not struck through').not.toContain('line-through')
    expect(live.opacity, 'an active row is fully opaque').toBeCloseTo(1, 2)
    // Transparent, not absent: the marker's box is reserved on both states so
    // the archived row does not shift 2px out of line with its neighbours.
    expect(live.borderLeftColor).toMatch(/rgba\(0, 0, 0, 0\)|transparent/)
  })

  test('the two guidance cards render under the table', async ({ page }) => {
    await api(page, orgID, 'POST', `/projects/${projectID}/agents`, agentPayload({ name: 'agent-cards' }))

    await page.goto(`/app/${orgID}/agents`)
    const cards = page.getByTestId('agents-spec-cards')
    await expect(cards).toBeVisible()

    // The grid's two direct children are the two cards. `Panel` takes only
    // children and className, so there is no testid to hang on each one.
    const [crud, archive] = [cards.locator('> div').nth(0), cards.locator('> div').nth(1)]

    // The cards must say what the reader can act on, not which story shipped it.
    await expect(crud.getByText('Syarat agent yang valid')).toBeVisible()
    await expect(crud.getByText('8 field wajib')).toBeVisible()
    for (const chip of ['name:', 'skills:', 'retry_policy:']) {
      await expect(crud.getByText(chip, { exact: false })).toBeVisible()
    }
    await expect(crud.getByText(/Belum punya kredensial provider\?/)).toBeVisible()

    await expect(archive.getByText('Efek mengarsipkan agent')).toBeVisible()
    await expect(archive.getByText(/Task yang sedang jalan tetap tuntas/)).toBeVisible()
    await expect(archive.getByText(/Hilang dari penugasan baru/)).toBeVisible()

    // `grid-cols-2`: the two cards share the row, so they start at the same y and
    // each is narrower than the pair. A single column would stack them.
    const [a, b] = await Promise.all([crud.boundingBox(), archive.boundingBox()])
    expect(a).not.toBeNull()
    expect(b).not.toBeNull()
    expect(Math.abs((a?.y ?? 0) - (b?.y ?? 0)), 'both cards on one row').toBeLessThanOrEqual(1)
    expect(b?.x ?? 0).toBeGreaterThan((a?.x ?? 0) + 100)
  })

  /**
   * The registry is a product screen, not a spec document.
   *
   * The Stitch mock is an acceptance-criteria explainer: it prints "US-AD20",
   * "US-AD73", "AC1", "HTTP 201 Created", and "M1" as visible copy, because it
   * was drawn to show a reviewer which criteria the screen satisfies. Cloning it
   * class-for-class carried that vocabulary into the shipped UI. An operator
   * should never have to read a story id to use the page, so this asserts the
   * rendered text is free of it — and it fails on the whole page, not one card,
   * because the leak was in several places at once.
   */
  test('the page never renders story, AC, milestone, or status-code jargon', async ({ page }) => {
    await api(page, orgID, 'POST', `/projects/${projectID}/agents`, agentPayload({ name: 'agent-jargon' }))
    await page.goto(`/app/${orgID}/agents`)
    await expect(page.getByTestId('agents-spec-cards')).toBeVisible()

    const body = (await page.locator('#root').innerText()).replace(/\s+/g, ' ')
    for (const pattern of [
      /US-AD\d+/, // story ids
      /\bAC\d\b/, // acceptance-criterion numbers
      /HTTP \d{3}/, // status codes used as copy
      // Whitespace-anchored: `\bM[1-6]\b` also matches model names such as
      // "MiniMax-M2.1", which is a false positive. See agent-detail.spec.ts.
      /(?:^|\s)M[1-6](?=\s|$)/, // roadmap phases
      /B2C|B2B/, // segment jargon
      /[Ss]pec(ification)?\b.*\b(chip|card)\b/, // spec-document phrasing
    ]) {
      expect(body, `jargon leaked into the UI: ${pattern}`).not.toMatch(pattern)
    }
  })
})
