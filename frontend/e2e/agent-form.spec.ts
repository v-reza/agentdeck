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

/**
 * Registers a workspace provider through the real API (US-AD109), so the
 * register form has a registry to offer. The form cannot invent one: the whole
 * point of phase 5 is that the endpoint and the credential come from here.
 */
async function seedProvider(page: Page, orgID: string, name: string): Promise<{ id: string; base_url: string }> {
  // The in-page fetch needs a document origin, and this helper runs before the
  // caller navigates. Landing on the app first is what gives it one.
  if (new URL(page.url() || 'about:blank').origin === 'null') {
    await page.goto(`/app/${orgID}/projects`)
  }
  const created = await api<{ id: string; base_url: string }>(page, orgID, 'POST', '/providers', {
    name,
    protocol: 'openai_compatible',
    base_url: `http://localhost:11434/v1/${name}`,
    api_key: '«redacted:sk-…»',
  })
  expect(created.status, created.text).toBe(201)
  // The model list is deliberately left empty. It normally arrives from the
  // upstream probe (AC7), and a provider with no list yet is the state a
  // just-registered one is really in — the server skips the allowlist check
  // until a list exists, which is what lets the form accept a typed model.
  return created.data
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
   * US-AD109 phase 5: the form offers the workspace registry, not a protocol
   * name and a hand-typed endpoint. Asserting the endpoint is *not* an input is
   * the point — an agent that carries its own copy of the base URL is what the
   * registry exists to remove (AC6).
   */
  test('the form offers the workspace registry and no endpoint field', async ({ page }) => {
    const provider = await seedProvider(page, orgID, 'registry-gateway')
    const { dialog } = await openRegisterModal(page, orgID)

    // The control shows the provider's name, not its id: an operator picks a
    // name, and the id is what the request carries. `optionValues` reads the
    // popup's text, so this is the name.
    const providers = await optionValues(dialog, /^provider$/i)
    expect(providers).toEqual(['registry-gateway'])

    // The endpoint is shown, read-only, under the provider.
    await expect(dialog.getByTestId('agent-form-base-url')).toHaveText(provider.base_url)

    // There is no way to type an endpoint anywhere in this form.
    await expect(dialog.locator('input[name="baseUrl"]')).toHaveCount(0)
    await expect(dialog.getByLabel(/url dasar provider|provider base url/i)).toHaveCount(0)

    // The hand-run probe is gone too: the model list comes from the provider.
    await expect(dialog.getByRole('button', { name: /tarik daftar model/i })).toHaveCount(0)
  })

  /**
   * AC9: the default provider is preselected, so the common case is one click.
   * The first provider a workspace creates is the default by construction.
   */
  test('the workspace default provider is preselected', async ({ page }) => {
    await seedProvider(page, orgID, 'default-gateway')
    const { dialog } = await openRegisterModal(page, orgID)

    const control = dialog.getByRole('combobox', { name: /^provider$/i })
    await expect(control).toHaveText(/default-gateway/)
  })

  /**
   * AC10: switching provider clears the model, because the old provider's model
   * need not exist on the new one.
   */
  test('switching provider clears the chosen model', async ({ page }) => {
    await seedProvider(page, orgID, 'gateway-one')
    await seedProvider(page, orgID, 'gateway-two')
    const { dialog } = await openRegisterModal(page, orgID)

    await typeCustom(dialog, /^model$/i, 'model-on-the-first-provider')
    // The model field accepts a value outside its list (`allowCustom`), so it is
    // an <input> and the assertion is on its value, not its text.
    const modelControl = dialog.getByRole('combobox', { name: /^model$/i })
    await expect(modelControl).toHaveValue('model-on-the-first-provider')

    await chooseOption(dialog, /^provider$/i, 'gateway-two')
    await expect(modelControl).toHaveValue('')
  })

  /**
   * AC6: the agent is registered against the provider, and the row carries the
   * provider's id rather than a copy of its endpoint.
   */
  test('a registered agent points at its provider', async ({ page }) => {
    const provider = await seedProvider(page, orgID, 'agent-gateway')
    const { dialog } = await openRegisterModal(page, orgID)
    const name = `agent-registry-${Date.now()}`

    await dialog.getByLabel(/nama/i).fill(name)
    await chooseOption(dialog, /^provider$/i, 'agent-gateway')
    await typeCustom(dialog, /^model$/i, 'registry-model')
    await page.getByRole('button', { name: /^simpan$/i }).click()

    const projects = await api<{ id: string }[]>(page, orgID, 'GET', '/projects')
    const projectID = projects.data[0].id

    // The list is re-read inside the poll: one read captured before the write
    // lands asserts against a snapshot that can only ever be stale.
    await expect
      .poll(async () => {
        const list = await api<{ name: string; provider_id?: string }[]>(
          page,
          orgID,
          'GET',
          `/projects/${projectID}/agents`,
        )
        const row = list.data.find((agent) => agent.name === name)
        // A missing row and a row with no provider are different failures;
        // collapsing both to null would report the wrong one.
        return row ? (row.provider_id ?? 'no-provider') : 'agent-not-registered'
      })
      .toBe(provider.id)
  })

  /**
   * US-AD109 AC6 versus US-AD86: with a provider selected the credential block
   * is replaced by a note, because the provider owns the key. This pins the
   * replacement — a form that kept collecting a per-agent key beside a provider
   * would be a second writer for one fact.
   */
  test('a provider replaces the per-agent credential field', async ({ page }) => {
    await seedProvider(page, orgID, 'owned-gateway')
    const { dialog } = await openRegisterModal(page, orgID)

    // The first provider of a workspace is its default (AC9), so one is
    // preselected and the credential field is gone.
    await expect(dialog.getByLabel(/api key provider/i)).toHaveCount(0)
    await expect(dialog.getByText(/dipegang provider|held by the provider/i)).toBeVisible()
  })

  /**
   * A workspace with no provider cannot register an agent at all — measured
   * against a fresh workspace, the POST the old form sent answers 400 "invalid
   * input", and the server derives both `provider` and `base_url` from a
   * provider the workspace does not have. So the section states that and points
   * at the page that fixes it, instead of drawing a credential field the server
   * would never receive.
   */
  test('a workspace with no provider is told to add one, and gets no credential field', async ({ page }) => {
    const { dialog } = await openRegisterModal(page, orgID)

    await expect(dialog.getByLabel(/api key provider/i)).toHaveCount(0)
    await expect(dialog.getByText(/belum punya provider|no provider yet/i)).toBeVisible()
    await expect(dialog.getByRole('link', { name: /tambah provider|add a provider/i })).toBeVisible()
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
