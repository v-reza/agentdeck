import { expect, test, type Page } from '@playwright/test'

/**
 * Screen 10-board-list — creating a board (US-AD09) and the empty state that
 * has to offer one (US-AD91 AC3).
 *
 * Three things are asserted that behaviour tests never caught:
 *
 *  1. The `New board` CTA is the design's **filled accent** button (`h-8`,
 *     radius 6, `text-[12px]`, plus glyph). `Button` defaults to
 *     `variant="secondary"`, so a call site that passes only `size` ships a
 *     white outline button where the design has a teal one — the same defect
 *     that already happened once on the project directory.
 *  2. US-AD09 AC4 holds end to end: a freshly registered operator can create a
 *     board **without creating a project first**, because registration seeds
 *     one. The test never calls the project endpoint — if the seed were removed,
 *     this would fail rather than quietly pass.
 *  3. US-AD09 AC3 holds on the server, not just in the UI: a `member` sees no
 *     CTA and the same request answers 403. Hiding the control alone would be a
 *     UI-level lie.
 *
 * The account is registered per run (same pattern as `dashboard.spec.ts`) rather
 * than read from the seeded demo workspace, so the suite stays re-runnable
 * against a database that is never truncated.
 */
const API = '/api/v1'
const PASSWORD = 'correct-horse-battery-staple'
const ULID = /[0-9A-HJKMNP-TV-Z]{26}/

async function signUp(page: Page, name = 'E2E Operator'): Promise<string> {
  const email = `e2e-kanban-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
  await page.goto('/login')
  const result = await page.evaluate(
    async ({ api, credentials }) => {
      const registered = await fetch(`${api}/auth/register`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify(credentials),
      })
      return { status: registered.status, body: await registered.text() }
    },
    { api: API, credentials: { email, password: PASSWORD, name } },
  )
  expect(result.status, result.body).toBe(201)
  return (JSON.parse(result.body) as { workspace_id: string }).workspace_id
}

test.describe('board list — create board (US-AD09)', () => {
  let orgID: string

  test.beforeEach(async ({ page }) => {
    orgID = await signUp(page)
    await page.goto(`/app/${orgID}/boards`)
    await expect(page).toHaveURL(new RegExp(`/app/${ULID.source}/boards$`))
  })

  test('the New board CTA is the accent-filled button with a plus icon', async ({ page }) => {
    const cta = page.getByRole('button', { name: /board baru/i }).first()
    await expect(cta).toBeVisible()

    const style = await cta.evaluate((el) => {
      const s = getComputedStyle(el)
      return {
        background: s.backgroundColor,
        color: s.color,
        height: Math.round(el.getBoundingClientRect().height),
        radius: s.borderRadius,
        fontSize: s.fontSize,
      }
    })

    // design 10-board-list: bg-[#0d7a70] text-white h-8 rounded-[6px] text-[12px]
    expect(style.background).toBe('rgb(13, 122, 112)')
    expect(style.color).toBe('rgb(255, 255, 255)')
    expect(style.height).toBe(32)
    expect(style.radius).toBe('6px')
    expect(style.fontSize).toBe('12px')

    await expect(cta.locator('svg')).toHaveCount(1)
  })

  test('clicking New board opens a modal, not an inline form', async ({ page }) => {
    await expect(page.getByRole('dialog')).toHaveCount(0)

    await page
      .getByRole('button', { name: /board baru/i })
      .first()
      .click()

    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()
    await expect(dialog).toHaveAttribute('aria-modal', 'true')

    // DESIGN.md `modal` token: surface-elevated, radius 14px, padding 20px
    const panel = await dialog.evaluate((el) => {
      const s = getComputedStyle(el)
      return { background: s.backgroundColor, radius: s.borderRadius, padding: s.paddingTop }
    })
    expect(panel.background).toBe('rgb(255, 255, 255)')
    expect(panel.radius).toBe('14px')
    expect(panel.padding).toBe('20px')

    // Focus moved into the panel, and Escape closes it.
    await expect(dialog.locator(':focus')).toHaveCount(1)
    await page.keyboard.press('Escape')
    await expect(dialog).toHaveCount(0)
  })

  // US-AD09 AC4: no project was created by this test, so the board can only
  // land somewhere if registration seeded a project.
  test('a brand-new workspace can create a board without creating a project first', async ({ page }) => {
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await page
      .getByRole('button', { name: /board baru/i })
      .first()
      .click()

    const dialog = page.getByRole('dialog')
    // The project selector must already offer the seeded project.
    const projectSelect = dialog.getByLabel('Project')
    await expect(projectSelect).toBeVisible()
    await expect(projectSelect.locator('option')).toHaveCount(1)

    await dialog.getByLabel('Nama').fill('E2E Sprint')
    await page.getByRole('button', { name: /^Buat$/ }).click()

    await expect(dialog).toHaveCount(0)
    const row = page.getByRole('row', { name: /E2E Sprint/ })
    await expect(row).toBeVisible({ timeout: 15_000 })
    // The slug is derived from the name client-side and persisted by the server.
    await expect(row).toContainText('getting-started')
  })

  // US-AD09 AC1: the five default columns are what the server persists when the
  // form sends no `columns`, so the board page must render them.
  test('a created board opens with the five default columns', async ({ page }) => {
    await page
      .getByRole('button', { name: /board baru/i })
      .first()
      .click()
    const dialog = page.getByRole('dialog')
    await dialog.getByLabel('Nama').fill('Column Check')
    await page.getByRole('button', { name: /^Buat$/ }).click()

    await page
      .getByRole('row', { name: /Column Check/ })
      .getByRole('link', { name: /open/i })
      .click()
    await expect(page).toHaveURL(new RegExp(`/app/${ULID.source}/boards/${ULID.source}$`))

    for (const column of ['Backlog', 'Ready', 'Running', 'Review', 'Done']) {
      await expect(page.getByText(column, { exact: true }).first()).toBeVisible()
    }
  })

  // US-AD09 AC2: a duplicate board name inside one project is a 409, and the
  // modal must say so inline rather than silently closing.
  test('a duplicate board name is rejected with an inline error', async ({ page }) => {
    await page
      .getByRole('button', { name: /board baru/i })
      .first()
      .click()
    await page.getByRole('dialog').getByLabel('Nama').fill('Duplicate Board')
    await page.getByRole('button', { name: /^Buat$/ }).click()
    await expect(page.getByRole('row', { name: /Duplicate Board/ })).toBeVisible({ timeout: 15_000 })

    // Same name, same project → boards_project_name_key violation.
    await page
      .getByRole('button', { name: /board baru/i })
      .first()
      .click()
    const dialog = page.getByRole('dialog')
    await dialog.getByLabel('Nama').fill('Duplicate Board')
    await page.getByRole('button', { name: /^Buat$/ }).click()

    await expect(dialog).toBeVisible()
    await expect(dialog.getByText(/tidak dibuat/i)).toBeVisible()
  })
})

// US-AD09 AC3: the UI hiding and the server gate are pinned together, because
// either one alone is a lie — a hidden button over a 201 is insecure, and a 403
// behind a visible button is a dead control.
test.describe('board list — US-AD09 AC3 (owner/admin only)', () => {
  test('a member sees no New board CTA and the API answers 403', async ({ page, browser }) => {
    const orgID = await signUp(page)

    const memberEmail = `e2e-boardmember-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
    const memberContext = await browser.newContext()
    const memberPage = await memberContext.newPage()

    await memberPage.goto('/login')
    const registered = await memberPage.evaluate(
      async ({ api, credentials }) => {
        const response = await fetch(`${api}/auth/register`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'include',
          body: JSON.stringify(credentials),
        })
        return { status: response.status, body: await response.text() }
      },
      { api: API, credentials: { email: memberEmail, password: PASSWORD, name: 'E2E Board Member' } },
    )
    expect(registered.status, registered.body).toBe(201)

    const invited = await page.evaluate(
      async ({ api, orgID, email }) => {
        const response = await fetch(`${api}/orgs/${orgID}/members`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-Org-ID': orgID },
          credentials: 'include',
          body: JSON.stringify({ email, role: 'member' }),
        })
        return { status: response.status, body: await response.text() }
      },
      { api: API, orgID, email: memberEmail },
    )
    expect(invited.status, invited.body).toBe(201)

    // The owner's seeded project is what the member will try to add a board to.
    const projectID = await page.evaluate(
      async ({ api, orgID }) => {
        const response = await fetch(`${api}/projects`, {
          headers: { 'X-Org-ID': orgID },
          credentials: 'include',
        })
        const body = (await response.json()) as { id: string }[]
        return body[0]?.id ?? ''
      },
      { api: API, orgID },
    )
    expect(projectID, 'the seeded project should be readable by its owner').not.toBe('')

    await memberPage.goto(`/app/${orgID}/boards`)
    await expect(memberPage.getByRole('heading', { name: 'Boards' })).toBeVisible()

    // No CTA anywhere in the shell, including the empty-state row.
    await expect(memberPage.getByRole('button', { name: /board baru/i })).toHaveCount(0)
    await expect(memberPage.getByRole('button', { name: /buat board pertama/i })).toHaveCount(0)

    const denied = await memberPage.evaluate(
      async ({ api, orgID, projectID }) => {
        const response = await fetch(`${api}/projects/${projectID}/boards`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-Org-ID': orgID },
          credentials: 'include',
          body: JSON.stringify({ name: 'Member Board', slug: 'member-board' }),
        })
        return response.status
      },
      { api: API, orgID, projectID },
    )
    expect(denied).toBe(403)

    await memberContext.close()
  })
})
