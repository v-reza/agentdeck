import { expect, test, type Page } from '@playwright/test'
import { chooseOption, optionValues, typeCustom } from './combobox'

/**
 * Screens 26-agent-form (the register modal, US-AD96) and 27-agent-provider-key
 * (the 420px credential panel, US-AD86).
 *
 * This file exists because both screens were rebuilt from a plain page form
 * into a modal plus a panel, and the things that regress in that move are the
 * ones no type-check catches: a label that wraps once the panel narrows, a
 * choice list that quietly becomes hardcoded, a dialog that stops returning
 * focus, and a credential that is accepted by the UI and never stored.
 *
 * Layout is measured with `getBoundingClientRect` through the DOM, never read
 * off a screenshot: a full-page screenshot at this density is not reliable
 * enough to say whether a label wrapped.
 *
 * Auth plumbing mirrors `agents.spec.ts` — the session cookie is `Secure` while
 * the dev server runs on plain HTTP, so only registration uses `page.request`;
 * everything else is an in-page `fetch` that shares the browser's cookies.
 */
const API = '/api/v1'
const PASSWORD = 'correct-horse-battery-staple'

async function signUp(page: Page, name = 'E2E Form Operator'): Promise<string> {
  const email = `e2e-form-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
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
  method: 'GET' | 'POST' | 'PUT' | 'DELETE',
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

/** Opens the registry with the register modal already open. */
async function openRegisterModal(page: Page, orgID: string) {
  await page.goto(`/app/${orgID}/projects`)
  await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible()
  await page.goto(`/app/${orgID}/agents`)
  const trigger = page.getByRole('button', { name: /daftarkan agent/i })
  await expect(trigger).toBeVisible()
  await trigger.click()
  const dialog = page.getByRole('dialog')
  await expect(dialog).toBeVisible()
  return { trigger, dialog }
}

test.describe('register agent modal (26-agent-form, US-AD96)', () => {
  let orgID: string

  test.beforeEach(async ({ page }) => {
    orgID = await signUp(page)
  })

  /**
   * US-AD96 AC6: no label may wrap to a second line inside the modal. Measured,
   * not eyeballed — the panel is 420px and a two-line label is a 4px change
   * that a screenshot cannot be trusted to show.
   */
  test('AC6 — every field label fits on one line inside the modal', async ({ page }) => {
    const { dialog } = await openRegisterModal(page, orgID)

    const labels = dialog.locator('form label > span:first-child, form > section > div > span:first-child')
    const count = await labels.count()
    expect(count, 'the form renders labels to measure').toBeGreaterThan(5)

    const wrapped: string[] = []
    for (let index = 0; index < count; index += 1) {
      const label = labels.nth(index)
      const box = await label.boundingBox()
      const text = (await label.innerText()).trim()
      if (!box || !text) continue
      // A single line at 10-11px with 1.2 line-height is under 18px. Anything
      // taller is a wrapped label, and the exact height does not matter.
      if (box.height > 18) wrapped.push(`${text} (${Math.round(box.height)}px)`)
    }
    expect(wrapped, 'labels that wrapped to a second line').toEqual([])
  })

  /** The dialog contract: role, aria-modal, a title binding, and a scroll lock. */
  test('the modal is a labelled dialog that locks background scroll', async ({ page }) => {
    const { dialog } = await openRegisterModal(page, orgID)

    await expect(dialog).toHaveAttribute('aria-modal', 'true')
    const titleID = await dialog.getAttribute('aria-labelledby')
    expect(titleID).toBeTruthy()
    await expect(page.locator(`#${titleID}`)).toHaveText(/daftarkan agent/i)

    expect(await page.evaluate(() => document.body.style.overflow)).toBe('hidden')
  })

  /**
   * US-AD96 AC1, as re-decided: the register form is BYO-only (DECISIONS 6A.F),
   * so there is exactly one provider and the model list comes from the
   * operator's own endpoint through the stateless probe — not from our price
   * table. Asserting the catalog is *absent* is the point: if someone wires the
   * priced list back in, this fails.
   */
  test('AC1 — the register form is BYO-only and offers no priced catalog', async ({ page }) => {
    const { dialog } = await openRegisterModal(page, orgID)

    const providers = await optionValues(dialog, /^provider$/i)
    expect(providers).toEqual(['openai_compatible'])

    // No probe has run yet, so the model list is empty rather than pre-filled
    // from the catalog.
    expect(await optionValues(dialog, /^model$/i)).toEqual([])

    // The endpoint field is always rendered now, not only for the BYO provider:
    // it is the only provider there is.
    await expect(dialog.getByLabel(/url dasar provider|provider base url/i)).toBeVisible()

    // The probe refuses to fire without both inputs, and says which are missing.
    await dialog.getByRole('button', { name: /tarik daftar model/i }).click()
    await expect(dialog.getByText(/isi endpoint dan api key dulu/i)).toBeVisible()
  })

  /** US-AD96 AC8 — skills are the org's library, and an empty library says so. */
  test('AC8 — skills are the org library, not free text', async ({ page }) => {
    const { dialog } = await openRegisterModal(page, orgID)

    const skills = await api<{ slug: string }[]>(page, orgID, 'GET', '/agent-skills')
    expect(skills.status, skills.text).toBe(200)

    const checkboxes = dialog.locator('input[name="skills"]')
    await expect(checkboxes).toHaveCount(skills.data.length)
    // There is no text input for skills anywhere in the form.
    await expect(dialog.locator('input[name="skills"][type="text"]')).toHaveCount(0)
  })

  /** US-AD96 AC7 — the nine primitives, closed. */
  test('AC7 — tools are the nine closed primitives', async ({ page }) => {
    const { dialog } = await openRegisterModal(page, orgID)

    const tools = await dialog
      .locator('input[name="tools"]')
      .evaluateAll((inputs) => inputs.map((input) => (input as HTMLInputElement).value))
    expect(tools).toEqual([
      'read_file',
      'write_file',
      'edit_file',
      'list_dir',
      'search_files',
      'bash',
      'sql_query',
      'http_fetch',
      'git',
    ])
  })

  /**
   * The off-catalog refusal is gone from this form on purpose. With no catalog
   * in the flow there is nothing to check a model against, and a BYO model name
   * is by definition not in our table — refusing it would make every BYO
   * registration fail. The server still gates provider and model (US-AD67), so
   * this pins that the client no longer refuses and the write really goes out.
   */
  test('AC1 — a model our table does not price is accepted, not refused', async ({ page }) => {
    const { dialog } = await openRegisterModal(page, orgID)
    const name = `agent-byo-${Date.now()}`

    await dialog.getByLabel(/nama/i).fill(name)
    await chooseOption(dialog, /^provider$/i, 'openai_compatible')
    await dialog.getByLabel(/url dasar provider|provider base url/i).fill('http://localhost:11434/v1')
    await typeCustom(dialog, /^model$/i, 'some-model-we-do-not-price')

    let posted = 0
    page.on('request', (request) => {
      if (request.method() === 'POST' && request.url().endsWith('/agents')) posted += 1
    })

    await page.getByRole('button', { name: /^simpan$/i }).click()

    const projects = await api<{ id: string }[]>(page, orgID, 'GET', '/projects')
    const list = await api<{ name: string; model: string }[]>(
      page,
      orgID,
      'GET',
      `/projects/${projects.data[0].id}/agents`,
    )
    await expect.poll(() => list.data.find((a) => a.name === name)?.model ?? null).toBe('some-model-we-do-not-price')
    expect(posted, 'the write reached the API').toBeGreaterThan(0)
  })

  /** US-AD96 AC2/AC3 — a key typed in the form is really stored. */
  test('AC2/AC3 — a credential typed in the form is encrypted and stored', async ({ page }) => {
    const { dialog } = await openRegisterModal(page, orgID)
    const name = `agent-keyed-${Date.now()}`

    await dialog.getByLabel(/nama/i).fill(name)
    await chooseOption(dialog, /^provider$/i, 'openai_compatible')
    await dialog.getByLabel(/url dasar provider|provider base url/i).fill('http://localhost:11434/v1')
    await typeCustom(dialog, /^model$/i, 'probe-model-1')
    await dialog.getByLabel(/api key provider/i).fill('sk-e2e-abcdefghijklmnop1234')
    await page.getByRole('button', { name: /^simpan$/i }).click()

    const projects = await api<{ id: string }[]>(page, orgID, 'GET', '/projects')
    const projectID = projects.data[0].id

    await expect
      .poll(async () => {
        const list = await api<{ id: string; name: string; has_provider_key: boolean }[]>(
          page,
          orgID,
          'GET',
          `/projects/${projectID}/agents`,
        )
        return list.data.find((agent) => agent.name === name)?.has_provider_key
      })
      .toBe(true)

    // AC2: the stored value never comes back. A read of the row carries the
    // boolean and no key material at all.
    const list = await api<{ name: string }[]>(page, orgID, 'GET', `/projects/${projectID}/agents`)
    expect(JSON.stringify(list.data)).not.toContain('sk-e2e-abcdefghijklmnop1234')
  })

  /** US-AD20 AC5 / US-AD96 AC3 — no key is a valid registration, not a failure. */
  test('AC3 — registering without a credential stays valid and not ready', async ({ page }) => {
    const { dialog } = await openRegisterModal(page, orgID)
    const name = `agent-keyless-${Date.now()}`

    await dialog.getByLabel(/nama/i).fill(name)
    await chooseOption(dialog, /^provider$/i, 'openai_compatible')
    await dialog.getByLabel(/url dasar provider|provider base url/i).fill('http://localhost:11434/v1')
    await typeCustom(dialog, /^model$/i, 'probe-model-1')
    await page.getByRole('button', { name: /^simpan$/i }).click()

    const projects = await api<{ id: string }[]>(page, orgID, 'GET', '/projects')
    const list = await api<{ name: string; has_provider_key: boolean }[]>(
      page,
      orgID,
      'GET',
      `/projects/${projects.data[0].id}/agents`,
    )
    const row = list.data.find((agent) => agent.name === name)
    expect(row, 'the agent was registered without a credential').toBeTruthy()
    expect(row?.has_provider_key).toBe(false)
  })

  /** Escape closes the modal and focus goes back to the button that opened it. */
  test('Escape closes the modal and returns focus to the trigger', async ({ page }) => {
    const { trigger, dialog } = await openRegisterModal(page, orgID)

    await page.keyboard.press('Escape')
    await expect(dialog).toBeHidden()
    await expect(trigger).toBeFocused()
    expect(await page.evaluate(() => document.body.style.overflow)).not.toBe('hidden')
  })
})

test.describe('provider credential panel (27-agent-provider-key, US-AD86)', () => {
  let orgID: string
  let agentID: string

  test.beforeEach(async ({ page }) => {
    orgID = await signUp(page)
    await page.goto(`/app/${orgID}/projects`)
    await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible()

    const projects = await api<{ id: string }[]>(page, orgID, 'GET', '/projects')
    const created = await api<{ id: string }>(page, orgID, 'POST', `/projects/${projects.data[0].id}/agents`, {
      name: 'agent-panel-e2e',
      provider: 'openai',
      model: 'gpt-4o',
      reasoning_effort: 'medium',
      max_runtime_seconds: 1800,
      retry_policy: 'transient_only',
      max_attempts: 3,
      tools: ['git'],
      skills: [],
    })
    expect(created.status, created.text).toBe(201)
    agentID = created.data.id
  })

  async function openPanel(page: Page) {
    await page.goto(`/app/${orgID}/agents/${agentID}`)
    const trigger = page.getByTestId('agent-key-panel-trigger')
    await expect(trigger).toBeVisible()
    await trigger.click()
    const panel = page.getByRole('dialog')
    await expect(panel).toBeVisible()
    return { trigger, panel }
  }

  /**
   * The one measurement the plan calls out by name: the panel is 420px wide,
   * anchored to the right edge, and full height. Read from the DOM.
   */
  test('the panel is 420px wide and pinned to the right edge', async ({ page }) => {
    const { panel } = await openPanel(page)

    const box = await panel.boundingBox()
    expect(box).not.toBeNull()
    expect(Math.round(box?.width ?? 0), 'panel width').toBe(420)

    const viewport = page.viewportSize()
    expect(viewport).not.toBeNull()
    expect(Math.round((box?.x ?? 0) + (box?.width ?? 0)), 'right edge').toBeGreaterThanOrEqual(
      (viewport?.width ?? 0) - 1,
    )
    expect(Math.round(box?.height ?? 0), 'panel height').toBe(viewport?.height)
  })

  test('the panel is a labelled modal dialog', async ({ page }) => {
    const { panel } = await openPanel(page)

    await expect(panel).toHaveAttribute('aria-modal', 'true')
    const titleID = await panel.getAttribute('aria-labelledby')
    expect(titleID).toBeTruthy()
    await expect(page.locator(`#${titleID}`)).toHaveText(/kredensial provider/i)
  })

  /** US-AD86 AC1/AC2/AC4 — the panel stores a key, and never shows it again. */
  test('AC1/AC2 — the panel stores the credential and only ever shows a mask', async ({ page }) => {
    const { panel } = await openPanel(page)
    const secret = 'sk-panel-abcdefghijklmnop9876'

    await expect(panel.getByTestId('provider-key-status')).toHaveText(/belum ada kredensial/i)
    await panel.getByLabel(/api key provider/i).fill(secret)
    await panel.getByRole('button', { name: /enkripsi & simpan/i }).click()

    // The mask the PUT answered with: first and last four, never the middle.
    await expect(panel.getByTestId('provider-key-masked')).toHaveText('sk-p...9876')
    await expect(panel.getByTestId('provider-key-status')).toHaveText(/kredensial tersimpan/i)
    // AC2: the full value is nowhere in the rendered page.
    expect(await page.locator('#root').innerText()).not.toContain(secret)

    const read = await api<{ has_provider_key: boolean }>(page, orgID, 'GET', `/agents/${agentID}`)
    expect(read.data.has_provider_key).toBe(true)
  })

  /** US-AD86 AC2 — revoking is real, and the panel reports the new state. */
  test('AC2 — revoking clears the stored credential', async ({ page }) => {
    await api(page, orgID, 'PUT', `/agents/${agentID}/provider-key`, { api_key: 'sk-revoke-me-1234567890' })
    // The panel seeds `hasKey` when it mounts, so the credential must already be
    // stored before the trigger is clicked — opening first would show the empty
    // state and hide the revoke control this test is about.
    const { panel } = await openPanel(page)

    await expect(panel.getByTestId('provider-key-status')).toHaveText(/kredensial tersimpan/i)
    await panel.getByRole('button', { name: /^cabut$/i }).click()
    await expect(panel.getByTestId('provider-key-status')).toHaveText(/belum ada kredensial/i)

    await expect
      .poll(async () => {
        const read = await api<{ has_provider_key: boolean }>(page, orgID, 'GET', `/agents/${agentID}`)
        return read.data.has_provider_key
      })
      .toBe(false)
  })

  /**
   * US-AD86 AC3 — the server's 400 for an unknown provider has to land inline in
   * the panel, not as a toast and not as a silent no-op.
   *
   * The 400 itself is not reachable from this screen: the endpoint validates the
   * *stored* agent's provider, and the only way to make that unknown is to
   * hand-edit the row (the Go test `TestPutProviderKeyRejectsUnknownProvider`
   * seeds exactly that and asserts the 400). What the UI owns is the surface, so
   * the route is fulfilled with the same 400 here and the panel's handling of it
   * is what gets asserted.
   */
  test('AC3 — a refused credential is reported inline in the panel', async ({ page }) => {
    await page.route(`**/agents/${agentID}/provider-key`, (route) =>
      route.fulfill({
        status: 400,
        contentType: 'text/plain',
        body: 'unknown provider: "totally-made-up-provider"',
      }),
    )

    const { panel } = await openPanel(page)
    await panel.getByLabel(/api key provider/i).fill('sk-whatever')
    await panel.getByRole('button', { name: /enkripsi & simpan/i }).click()

    const inline = panel.getByRole('alert')
    await expect(inline).toBeVisible()
    await expect(inline).toContainText(/kredensial tidak tersimpan/i)
    await expect(inline).toContainText('totally-made-up-provider')
  })

  /** Escape closes the panel and focus returns to the button that opened it. */
  test('Escape closes the panel and returns focus to the trigger', async ({ page }) => {
    const { trigger, panel } = await openPanel(page)

    await page.keyboard.press('Escape')
    await expect(panel).toBeHidden()
    await expect(trigger).toBeFocused()
  })
})
