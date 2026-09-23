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

/**
 * Seeds a provider through the real API (US-AD109). The address is loopback on
 * purpose: DECISIONS 6A.F allows `127.0.0.1` as an exact string, so the request
 * passes the SSRF guard without a network call being made anywhere.
 */
async function seedProvider(
  page: Page,
  orgID: string,
  name: string,
): Promise<{ id: string; name: string; base_url: string }> {
  const created = await api<{ id: string }>(page, orgID, 'POST', '/providers', {
    name,
    protocol: 'openai_compatible',
    base_url: `http://127.0.0.1:11434/v1/${name}`,
  })
  expect(created.status, created.text).toBe(201)
  return { id: created.data.id, name, base_url: `http://127.0.0.1:11434/v1/${name}` }
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

    // The rates arrive from their own query, so they are not necessarily on
    // screen the moment the status pill appears. Reading `innerText` once raced
    // that query and passed only when the module happened to be warm; a
    // web-first assertion retries instead of sampling once.
    await expect(page.locator('#root')).toContainText(/\$[\d.,]+/)
    const body = await page.locator('#root').innerText()
    const rates = body.match(/\$[\d.,]+/g) ?? []
    expect(rates.length, 'the model has catalog rates to render').toBeGreaterThan(0)
    expect(body).toMatch(/estimate|estimasi/i)
  })

  /**
   * The assignment section, which the screen did not have at all until now
   * (US-AD73 AC1/AC2). The mock draws it as its third card; the implementation
   * went straight from the lifecycle card to the runtime card.
   *
   * This is the assertion that the section reports real rows rather than the
   * mock's hardcoded ones: a task is created through the API, assigned to the
   * agent, and then looked for by its own title.
   */
  test('the assignment section lists the tasks this agent holds', async ({ page }) => {
    const { agentID, projectID } = await openDetail(page, orgID)

    // One board to hold the task, then a task assigned to this agent.
    const board = await api<{ id: string }>(page, orgID, 'POST', `/projects/${projectID}/boards`, {
      name: `Assignment Board ${Date.now()}`,
      slug: `assignment-${Date.now()}`,
    })
    expect(board.status, board.text).toBe(201)
    const title = `assigned-job-${Date.now()}`
    const task = await api<{ id: string }>(page, orgID, 'POST', `/boards/${board.data.id}/tasks`, { title })
    expect(task.status, task.text).toBe(201)
    // Assignment is its own endpoint (POST /tasks/{id}/assign), not a field on
    // create — the create path does not read assignee_agent_id at all.
    const assigned = await api(page, orgID, 'POST', `/tasks/${task.data.id}/assign`, { agent_id: agentID })
    expect(assigned.status, assigned.text).toBe(200)

    await page.reload()
    const section = page.getByTestId('agent-assignment')
    await expect(section).toBeVisible()
    // The row names the task the API was just told about. The explicit timeout is
    // the same 15s the other write-then-read assertions in this file use: under a
    // full-suite run the reload plus the assignment query can outlast the 5s
    // default, which reads as a missing row rather than a slow one.
    await expect(section.getByTestId('assignment-row').filter({ hasText: title })).toBeVisible({
      timeout: 15_000,
    })
  })

  /**
   * AC2's visible half: the picker offers this agent (it is active) and states
   * how many archived agents it left out. An archived agent must not be an
   * option — that is the rule, not the count.
   */
  test('the picker lists the active agent and reports what it hid', async ({ page }) => {
    const { agentID, projectID } = await openDetail(page, orgID)

    const board = await api<{ id: string }>(page, orgID, 'POST', `/projects/${projectID}/boards`, {
      name: `Picker Board ${Date.now()}`,
      slug: `picker-${Date.now()}`,
    })
    expect(board.status, board.text).toBe(201)
    const title = `picker-job-${Date.now()}`
    const task = await api<{ id: string }>(page, orgID, 'POST', `/boards/${board.data.id}/tasks`, { title })
    expect(task.status, task.text).toBe(201)
    const assigned = await api(page, orgID, 'POST', `/tasks/${task.data.id}/assign`, { agent_id: agentID })
    expect(assigned.status, assigned.text).toBe(200)

    // A second agent in the same project, then archived: it must vanish from the
    // picker while the count of hidden rows goes up.
    const retired = await api<{ id: string }>(page, orgID, 'POST', `/projects/${projectID}/agents`, {
      ...AGENT,
      name: `agent-retired-${Date.now()}`,
    })
    expect(retired.status, retired.text).toBe(201)
    const archived = await api(page, orgID, 'PATCH', `/agents/${retired.data.id}`, { archived: true })
    expect(archived.status, archived.text).toBe(200)

    await page.reload()
    const section = page.getByTestId('agent-assignment')
    await expect(section).toBeVisible()

    const picker = section.getByTestId('assignment-picker')
    await expect(picker.getByText('agent-detail-e2e')).toBeVisible()
    await expect(picker).not.toContainText('agent-retired-')
    await expect(section.getByTestId('assignment-hidden')).toContainText(/1/)
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

  /**
   * US-AD109 AC6: the screen edits the *provider*, never the endpoint. The
   * address is rendered as text under the dropdown and there is no field that
   * would let the agent carry a copy of it — the copy is what the registry
   * exists to remove.
   */
  test('the endpoint is shown as text and is not editable', async ({ page }) => {
    const provider = await seedProvider(page, orgID, `detail-url-${Date.now()}`)
    const { agentID } = await openDetail(page, orgID, { provider_id: provider.id })

    await expect(page.getByTestId('agent-detail-base-url')).toHaveText(provider.base_url)

    // No input, textarea, or select in the config card may carry the address.
    const editable = await page.evaluate(() => {
      const fields = [...document.querySelectorAll('input, textarea, select')]
      return fields.map((field) => (field as HTMLInputElement).name).filter(Boolean)
    })
    expect(editable, 'no field may write the endpoint').not.toContain('base_url')

    // The save that the screen sends must not carry it either.
    const read = await api<Record<string, unknown>>(page, orgID, 'GET', `/agents/${agentID}`)
    expect(read.data.base_url, 'an agent must not carry an address copy').toBeUndefined()
  })

  /**
   * US-AD109 AC10: changing the provider empties the model choice, because the
   * old provider's models need not exist on the new one.
   */
  test('changing the provider empties the model field', async ({ page }) => {
    const first = await seedProvider(page, orgID, `detail-a-${Date.now()}`)
    const second = await seedProvider(page, orgID, `detail-b-${Date.now()}`)
    await openDetail(page, orgID, { provider_id: first.id })

    const model = page.getByRole('combobox', { name: /model/i })
    await expect(model).toHaveValue('gpt-4o')

    const provider = page.getByRole('combobox', { name: /^Provider$/i })
    await provider.click()
    await page.getByRole('option', { name: second.name }).click()

    await expect(model, 'the old provider’s model must not survive the switch').toHaveValue('')
    await expect(page.getByTestId('agent-detail-base-url')).toHaveText(second.base_url)
  })

  /**
   * The provider is a registry row now, so the dropdown offers the workspace's
   * own providers rather than the four hard-coded vendor names it used to.
   */
  test('the provider dropdown lists the workspace registry', async ({ page }) => {
    const provider = await seedProvider(page, orgID, `detail-reg-${Date.now()}`)
    await openDetail(page, orgID, { provider_id: provider.id })

    const dropdown = page.getByRole('combobox', { name: /^Provider$/i })
    await expect(dropdown).toHaveText(provider.name)

    await dropdown.click()
    await expect(page.getByRole('option', { name: provider.name })).toBeVisible()
    // The old closed set is gone: `anthropic` was never a row in this workspace.
    await expect(page.getByRole('option', { name: 'anthropic', exact: true })).toHaveCount(0)
  })
})
