import { expect, test, type Page } from '@playwright/test'
import { createServer, type Server } from 'node:http'
import { AddressInfo } from 'node:net'

/**
 * Screen 47-providers — the credential registry (US-AD109).
 *
 * This spec is the UI gate for phase 4: layout, not business rules. The
 * registry's rules are already proven in Go (`cmd/api/providers_test.go`,
 * `internal/providerreg/postgres_test.go`), so nothing here re-tests them.
 * What the Go suite cannot see is whether the screen renders the design.
 *
 * Asserted here and nowhere else:
 *
 *  1. The density the design pins: 32px header row, 28px provider row.
 *  2. The 9 columns, in the design's order — a table that drops a column still
 *     looks right.
 *  3. AC2: the credential cell is bullets, never a value. This is the one
 *     assertion that must survive any future refactor of the cell.
 *  4. AC9: the default badge appears on exactly one row, and the provider the
 *     workspace just registered is the one carrying it.
 *  5. The row action buttons are labelled. Two bare icons per row is what the
 *     design draws; unlabelled they are unusable by screen reader.
 *
 * The account is registered per run, same as `members.spec.ts`, so the suite is
 * re-runnable against a database that is never truncated.
 */
const API = '/api/v1'
const PASSWORD = 'correct-horse-battery-staple'
const ULID = /[0-9A-HJKMNP-TV-Z]{26}/

/**
 * Registers a workspace and one provider through the API, then lands on the
 * screen. The provider is created via the API rather than the dialog because
 * this spec is about how the list renders; the dialog has its own spec.
 *
 * `base_url` is a loopback address on purpose: DECISIONS 6A.F allows the exact
 * strings `localhost`/`127.0.0.1` for an operator's own endpoint, so this is a
 * real supported case and it needs no reachable host to register.
 */
async function seedWorkspaceWithProvider(page: Page): Promise<string> {
  const email = `e2e-providers-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
  await page.goto('/login')
  const result = await page.evaluate(
    async ({ api, credentials }) => {
      const registered = await fetch(`${api}/auth/register`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify(credentials),
      })
      const body = await registered.text()
      if (registered.status !== 201) return { status: registered.status, body, orgID: '' }
      const { workspace_id } = JSON.parse(body) as { workspace_id: string }
      const created = await fetch(`${api}/providers`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-Org-ID': workspace_id },
        credentials: 'include',
        body: JSON.stringify({
          name: 'Local Ollama',
          protocol: 'openai_compatible',
          base_url: 'http://127.0.0.1:11434/v1',
          api_key: 'sk-test-not-a-real-key',
        }),
      })
      return { status: created.status, body: await created.text(), orgID: workspace_id }
    },
    { api: API, credentials: { email, password: PASSWORD, name: 'Registry Owner', org_name: 'Registry Co' } },
  )
  expect(result.status, result.body).toBe(201)
  return result.orgID
}

test.describe('providers — reachable from the shell', () => {
  /**
   * The bug this exists to catch: the screen was built, routed and styled, and
   * nothing linked to it. The rail's gear reaches `settings/workspace` and the
   * sidebar rendered no settings navigation at all, so seven settings routes
   * (members, providers, api-keys, webhooks, profile, workspace) had no way in.
   *
   * Every other test in this file navigates straight to the URL, which is
   * exactly why none of them noticed. This one starts at the rail and clicks,
   * the way an operator arrives.
   */
  test('the rail and sidebar reach the registry without a typed URL', async ({ page }) => {
    const orgID = await seedWorkspaceWithProvider(page)

    // Start where the operator starts: the gear in the rail.
    await page.goto(`/app/${orgID}/boards`)
    await page.getByRole('link', { name: 'Settings' }).click()
    await expect(page).toHaveURL(new RegExp(`/app/${ULID.source}/settings/`))

    // The sidebar is the only navigation into the settings routes.
    const link = page.getByRole('link', { name: /provider llm/i })
    await expect(link).toBeVisible({ timeout: 15_000 })
    await link.click()

    await expect(page).toHaveURL(new RegExp(`/app/${ULID.source}/settings/providers$`))
    await expect(page.getByRole('cell', { name: /local ollama/i }).first()).toBeVisible({ timeout: 15_000 })
  })

  test('every settings link in the sidebar resolves to a real screen', async ({ page }) => {
    const orgID = await seedWorkspaceWithProvider(page)
    await page.goto(`/app/${orgID}/settings/providers`)

    // The shell mounts asynchronously — the sidebar is not in the first paint.
    // Counting before it appears reads 0 and looks like a missing nav; wait for
    // one link first, then count.
    await expect(page.getByRole('link', { name: /provider llm/i })).toBeVisible({ timeout: 15_000 })

    // A link that 404s is worse than an item that is not rendered, so this
    // walks the sidebar's settings links and proves each one lands somewhere.
    const hrefs = await page
      .locator('nav a[href*="/settings/"]')
      .evaluateAll((nodes) => nodes.map((n) => (n as HTMLAnchorElement).getAttribute('href') ?? ''))
    expect(hrefs.length, 'the sidebar must render the settings groups').toBeGreaterThan(3)

    for (const href of hrefs) {
      const response = await page.request.get(href)
      // SPA fallback serves index.html for a client route, so a real check is
      // that the screen renders rather than that the status is 200.
      expect(response.status(), href).toBeLessThan(400)
      await page.goto(href)
      await expect(page.getByRole('main').or(page.locator('header')).first(), href).toBeVisible({ timeout: 15_000 })
    }
  })
})

test.describe('providers — the write surface and the status bullet', () => {
  let orgID: string

  test.beforeEach(async ({ page }) => {
    orgID = await seedWorkspaceWithProvider(page)
    await page.goto(`/app/${orgID}/settings/providers`)
    await expect(page.getByRole('cell', { name: /local ollama/i }).first()).toBeVisible({ timeout: 15_000 })
  })

  /**
   * The design draws a 420px drawer pinned to the right edge, and that is what
   * create/edit must be. A centred 440px dialog is the same fields in the wrong
   * place: the operator asked for the panel, and the repo already had the
   * geometry (`Modal placement="right" size="panel"`, the credential panel's).
   */
  test('create opens the 420px right-anchored panel, not a centred dialog', async ({ page }) => {
    await page.getByRole('button', { name: /tambah provider/i }).click()

    const panel = page.getByRole('dialog')
    await expect(panel).toBeVisible()

    const box = await panel.boundingBox()
    expect(box, 'the panel must be laid out').not.toBeNull()
    expect(Math.round(box!.width), 'design: w-[420px]').toBe(420)
    expect(Math.round(box!.height), 'a right panel is full height').toBe(page.viewportSize()!.height)
    // Pinned right: its right edge is the viewport's right edge, and its left
    // edge is not at the viewport's left.
    expect(Math.round(box!.x + box!.width)).toBe(page.viewportSize()!.width)
    expect(Math.round(box!.x)).toBeGreaterThan(page.viewportSize()!.width / 2)

    // The design's footer is the write surface: save, and (on edit) delete.
    await expect(page.getByRole('button', { name: /simpan provider/i })).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(panel).toBeHidden()
  })

  test('edit opens the same panel, carrying the provider and a delete action', async ({ page }) => {
    await page
      .getByRole('button', { name: /ubah provider/i })
      .first()
      .click()

    const panel = page.getByRole('dialog')
    await expect(panel).toBeVisible()
    const box = await panel.boundingBox()
    expect(Math.round(box!.width), 'edit uses the same 420px panel').toBe(420)

    await expect(panel.getByLabel(/nama provider/i)).toHaveValue('Local Ollama')
    await expect(page.getByRole('button', { name: /hapus provider/i })).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(panel).toBeHidden()
  })

  /**
   * The design puts a status bullet beside the provider name: accent when the
   * credential is proven, neutral when it is not.
   *
   * "The bullet exists" is not the assertion — "the bullet turns accent after a
   * passing verify" is. The verify runs against a local stub that answers 200 on
   * /chat/completions, so the test proves the colour is driven by
   * `last_verified_at` rather than by a hardcoded class.
   *
   * The stub listens on every interface because the caller is the Go API, not
   * this test: in this repo's dev setup the API runs in a container, so the
   * address it can reach is `host.docker.internal`, while a host-run API reaches
   * the same stub on `127.0.0.1`. Both are guard-allowed exact strings
   * (DECISIONS 6A.F), and both are tried — the test skips only when neither
   * works, which means the API could not reach the stub at all and the colour
   * question was never asked.
   */
  test('AC3: the name bullet is neutral until a passing verify turns it accent', async ({ page }) => {
    // The stub answers both calls the registry makes, and they are different:
    // model discovery reads `data[].id`, while the credential probe only needs a
    // 2xx on /chat/completions. Answering one shape to both requests makes the
    // fetch fail to parse and the test skip for the wrong reason.
    const stub: Server = createServer((req, res) => {
      res.writeHead(200, { 'Content-Type': 'application/json' })
      if (req.method === 'GET') {
        res.end(JSON.stringify({ object: 'list', data: [{ id: 'stub-model', object: 'model' }] }))
        return
      }
      res.end(JSON.stringify({ choices: [{ message: { content: 'pong' } }] }))
    })
    await new Promise<void>((resolve) => stub.listen(0, '0.0.0.0', resolve))
    const port = (stub.address() as AddressInfo).port
    try {
      // A provider pointing at the stub, with the model list already fetched so
      // verify has a model to probe with. The reachable address is discovered by
      // trying them, not by inspecting the API's deployment.
      //
      // The name is unique per run: `providers_org_name_key` makes a repeat a 409,
      // and a test that leaves its row behind would fail the next run with a
      // name collision rather than with the thing it is checking.
      const attempt = async (host: string, name: string) => {
        const baseURL = `http://${host}:${port}/v1`
        return page.evaluate(
          async ({ api, org, baseURL, name }) => {
            const headers = { 'Content-Type': 'application/json', 'X-Org-ID': org }
            const response = await fetch(`${api}/providers`, {
              method: 'POST',
              headers,
              credentials: 'include',
              body: JSON.stringify({
                name,
                protocol: 'openai_compatible',
                base_url: baseURL,
                api_key: '«redacted:sk-…»',
              }),
            })
            // Read as text and parse defensively: a 409 answers with a plain-text
            // body, and calling .json() on it throws inside the page, which
            // surfaces as an opaque `page.evaluate: SyntaxError` instead of the
            // real status.
            const created = await response.text()
            if (!response.ok) return { created: response.status, models: 0, id: '', detail: created }
            const provider = JSON.parse(created) as { id: string }
            const models = await fetch(`${api}/providers/${provider.id}/models`, {
              method: 'POST',
              headers,
              credentials: 'include',
            })
            return { created: response.status, models: models.status, id: provider.id, detail: '' }
          },
          { api: API, org: orgID, baseURL, name },
        )
      }

      const name = `Stub Gateway ${Date.now()}`
      let reachable = await attempt('host.docker.internal', name)
      if (reachable.models !== 200) {
        // The container address failed, so try the one a host-run API uses. The
        // name is suffixed so the retry does not collide with the first attempt.
        reachable = await attempt('127.0.0.1', `${name} b`)
      }
      test.skip(
        reachable.models !== 200,
        `the api could not reach the stub on either address (last: HTTP ${reachable.models} ${reachable.detail}) — it must share a network with the host for this test to ask its question`,
      )
      expect(reachable.created, 'provider create').toBe(201)

      await page.reload()
      const row = page.getByRole('row').filter({ hasText: name })
      await expect(row).toBeVisible({ timeout: 15_000 })

      const bullet = row.locator('span[data-verified]')
      await expect(bullet).toHaveAttribute('data-verified', 'false')

      await row.getByRole('button', { name: new RegExp(`uji — ${name}`, 'i') }).click()
      // The row is invalidated by the mutation, so the attribute flipping is the
      // observable the verify produced.
      await expect(bullet).toHaveAttribute('data-verified', 'true', { timeout: 15_000 })

      // Colour, not just the attribute: the two states must be different
      // backgrounds, or "accent when verified" is an attribute nothing renders.
      const verifiedColor = await bullet.evaluate((el) => getComputedStyle(el).backgroundColor)
      const other = page.getByRole('row').filter({ hasText: 'Local Ollama' }).locator('span[data-verified]')
      const unverifiedColor = await other.evaluate((el) => getComputedStyle(el).backgroundColor)
      expect(verifiedColor).not.toBe(unverifiedColor)
    } finally {
      await new Promise<void>((resolve) => stub.close(() => resolve()))
    }
  })
})

test.describe('providers — design match and US-AD109', () => {
  let orgID: string

  test.beforeEach(async ({ page }) => {
    orgID = await seedWorkspaceWithProvider(page)
    await page.goto(`/app/${orgID}/settings/providers`)
    await expect(page).toHaveURL(new RegExp(`/app/${ULID.source}/settings/providers$`))
  })

  test('the table uses the design density: 32px header row, 28px provider row', async ({ page }) => {
    await expect(page.getByRole('cell', { name: /local ollama/i }).first()).toBeVisible({ timeout: 15_000 })

    const headerRow = page.getByRole('row').first()
    const headerHeight = await headerRow.evaluate((el) => Math.round(el.getBoundingClientRect().height))
    expect(headerHeight, 'design: h-[32px] on <tr> in <thead>').toBe(32)

    const providerRow = page.getByRole('row').nth(1)
    await expect
      .poll(() => providerRow.evaluate((el) => Math.round(el.getBoundingClientRect().height)), {
        timeout: 15_000,
        message: 'design: h-[28px] on <tr> in <tbody>',
      })
      .toBe(28)
  })

  test('the 9 columns render in the design order', async ({ page }) => {
    await expect(page.getByRole('cell', { name: /local ollama/i }).first()).toBeVisible({ timeout: 15_000 })

    const headers = await page.getByRole('row').first().getByRole('columnheader').allInnerTexts()
    // The design uppercases the header row in CSS (`uppercase`), so the rendered
    // text is upper case while the copy is not.
    expect(headers.map((h) => h.trim().toLowerCase())).toEqual([
      'nama',
      'protokol',
      'base url',
      'kredensial',
      'model',
      'verifikasi',
      'sinkronisasi model',
      'default',
      'aksi',
    ])
  })

  test('AC2: the credential cell is masked and carries no value', async ({ page }) => {
    const cell = page.getByRole('cell', { name: /terenkripsi/i }).first()
    await expect(cell).toBeVisible({ timeout: 15_000 })

    const text = (await cell.innerText()).trim()
    expect(text, 'the mask, then the label').toContain('••••')
    expect(text, 'the stored key must never be rendered').toContain('terenkripsi')
    expect(text, 'no plaintext credential in the cell').not.toContain('sk-test-not-a-real-key')
  })

  test('AC9: the default badge marks exactly one row', async ({ page }) => {
    await expect(page.getByRole('cell', { name: /local ollama/i }).first()).toBeVisible({ timeout: 15_000 })

    // The workspace has exactly one provider, and the first provider of a
    // workspace becomes its default — so the badge is on that row and nowhere
    // else. The column header is also named "Default", so the search is scoped
    // to the body: an unscoped `getByText` matches the header too and the count
    // is 2, which is what this test caught on its first run.
    const badges = page.getByRole('rowgroup').nth(1).getByText('Default', { exact: true })
    await expect(badges).toHaveCount(1)
  })

  test('AC3/AC7: the row exposes labelled test and fetch controls', async ({ page }) => {
    await expect(page.getByRole('cell', { name: /local ollama/i }).first()).toBeVisible({ timeout: 15_000 })

    await expect(page.getByRole('button', { name: 'Uji — Local Ollama' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Tarik model — Local Ollama' })).toBeVisible()
  })

  test('AC8: a viewer sees the list without any write control', async ({ page, browser }) => {
    // A second account joins the first workspace as a viewer. Done through the
    // API because there is no invite-acceptance UI in scope here.
    const viewerEmail = `e2e-provider-viewer-${Date.now()}@example.com`
    const invited = await page.evaluate(
      async ({ api, org, email }) => {
        const response = await fetch(`${api}/orgs/${org}/members`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-Org-ID': org },
          credentials: 'include',
          body: JSON.stringify({ email, role: 'viewer' }),
        })
        return { status: response.status, body: await response.text() }
      },
      { api: API, org: orgID, email: viewerEmail },
    )
    expect(invited.status, invited.body).toBe(201)

    const context = await browser.newContext()
    const viewerPage = await context.newPage()
    await viewerPage.goto('/login')
    const registered = await viewerPage.evaluate(
      async ({ api, credentials }) => {
        const response = await fetch(`${api}/auth/register`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'include',
          body: JSON.stringify(credentials),
        })
        return { status: response.status, body: await response.text() }
      },
      { api: API, credentials: { email: viewerEmail, password: PASSWORD, name: 'Registry Viewer' } },
    )
    // The account already exists as a member, so registration is expected to be
    // refused; what matters is that the membership lets this session in.
    expect([201, 409], registered.body).toContain(registered.status)

    await viewerPage.goto(`/app/${orgID}/settings/providers`)
    await expect(viewerPage.getByRole('cell', { name: /local ollama/i }).first()).toBeVisible({ timeout: 15_000 })
    await expect(viewerPage.getByRole('button', { name: /tambah provider/i })).toHaveCount(0)
    await expect(viewerPage.getByRole('button', { name: /uji —/i })).toHaveCount(0)
    await context.close()
  })
})
