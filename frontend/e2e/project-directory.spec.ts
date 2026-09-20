import { expect, test, type Page } from '@playwright/test'

/**
 * Screen 11-project-list — the project directory (US-AD08 AC4, US-AD91 AC4).
 *
 * Two things are asserted here that behaviour tests never caught:
 *
 *  1. The `New project` CTA is the design's **filled accent** button with a plus
 *     icon. It shipped as a white outline button because the `Button` primitive
 *     defaults to `variant="secondary"` and the call site passed only `size`.
 *     A colour is as testable as a size, so the hex is pinned here.
 *  2. The create form opens in a **modal**, not inline in the toolbar. The
 *     design has no dialog for it; the treatment comes from DESIGN.md's `modal`
 *     token. Asserting the dialog role + backdrop + Escape means a future
 *     refactor back to an inline form fails loudly.
 *
 * The account is registered per run (same pattern as `dashboard.spec.ts`) rather
 * than read from the seeded demo workspace: a fresh ULID keeps the suite
 * re-runnable against a database that is never truncated, and it does not depend
 * on a fixture having been run first.
 */
const API = '/api/v1'
const PASSWORD = 'correct-horse-battery-staple'
const ULID = /[0-9A-HJKMNP-TV-Z]{26}/

async function signUp(page: Page): Promise<string> {
  const email = `e2e-projdir-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
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
    { api: API, credentials: { email, password: PASSWORD, name: 'E2E Operator', org_name: 'E2E Fleet' } },
  )
  expect(result.status, result.body).toBe(201)
  return (JSON.parse(result.body) as { workspace_id: string }).workspace_id
}

test.describe('project directory — design match', () => {
  let orgID: string

  test.beforeEach(async ({ page }) => {
    orgID = await signUp(page)
    await page.goto(`/app/${orgID}/projects`)
    await expect(page).toHaveURL(new RegExp(`/app/${ULID.source}/projects$`))
  })

  test('the New project CTA is the accent-filled button with a plus icon', async ({ page }) => {
    const cta = page.getByRole('button', { name: /new project/i }).first()
    await expect(cta).toBeVisible()

    const style = await cta.evaluate((el) => {
      const s = getComputedStyle(el)
      return {
        background: s.backgroundColor,
        color: s.color,
        height: Math.round(el.getBoundingClientRect().height),
        radius: s.borderRadius,
      }
    })

    // design 11-project-list: bg-[#0d7a70] text-white h-8 rounded-[6px]
    expect(style.background).toBe('rgb(13, 122, 112)')
    expect(style.color).toBe('rgb(255, 255, 255)')
    expect(style.height).toBe(32)
    expect(style.radius).toBe('6px')

    // the plus glyph the design puts before the label
    await expect(cta.locator('svg')).toHaveCount(1)
  })

  test('clicking New project opens a modal, not an inline form', async ({ page }) => {
    // nothing inline before the click
    await expect(page.getByRole('dialog')).toHaveCount(0)

    await page
      .getByRole('button', { name: /new project/i })
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

    // the form lives inside the dialog, and focus moved into it
    const name = dialog.locator('input[name="name"]')
    await expect(name).toBeVisible()
    await expect(name).toBeFocused()
  })

  test('the modal closes on Escape and returns focus to the trigger', async ({ page }) => {
    const trigger = page.getByRole('button', { name: /new project/i }).first()
    await trigger.click()
    await expect(page.getByRole('dialog')).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await expect(trigger).toBeFocused()
  })

  test('the modal locks background scroll and closes from the backdrop', async ({ page }) => {
    await page
      .getByRole('button', { name: /new project/i })
      .first()
      .click()
    await expect(page.getByRole('dialog')).toBeVisible()

    expect(await page.evaluate(() => document.body.style.overflow)).toBe('hidden')

    // the backdrop is the full-bleed layer behind the panel
    await page.getByTestId('modal-backdrop').click({ position: { x: 5, y: 5 } })
    await expect(page.getByRole('dialog')).toHaveCount(0)
    expect(await page.evaluate(() => document.body.style.overflow)).not.toBe('hidden')
  })

  test('the slug derives from the name but stays editable', async ({ page }) => {
    await page
      .getByRole('button', { name: /new project/i })
      .first()
      .click()

    const name = page.locator('input[name="name"]')
    const slug = page.locator('input[name="slug"]')

    await name.fill('Control Plane')
    await expect(slug).toHaveValue('control-plane')

    await slug.fill('custom-slug')
    await name.fill('Control Plane Two')
    await expect(slug).toHaveValue('custom-slug')
  })

  test('creating a project through the modal persists it — US-AD08 AC1', async ({ page }) => {
    await page
      .getByRole('button', { name: /new project/i })
      .first()
      .click()
    await page.getByLabel('Name').fill('Modal Control Plane')
    await page.getByRole('button', { name: 'Create' }).click()

    // the modal closes on success and the row arrives from the server
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await expect(page.getByRole('row', { name: /Modal Control Plane/ })).toBeVisible({ timeout: 15_000 })
  })

  test('a duplicate slug is rejected inline with the server message — US-AD08 AC2', async ({ page }) => {
    // Create the slug once through the UI, then try the same one again. AC2
    // requires 409; the form must surface the server's own text in the dialog
    // instead of closing on a success that never happened.
    await page
      .getByRole('button', { name: /new project/i })
      .first()
      .click()
    await page.getByLabel('Name').fill('Slug Owner')
    await page.getByLabel('Slug').fill('slug-owner')
    await page.getByRole('button', { name: 'Create' }).click()
    await expect(page.getByRole('row', { name: /Slug Owner/ })).toBeVisible({ timeout: 15_000 })

    await page
      .getByRole('button', { name: /new project/i })
      .first()
      .click()
    await page.getByLabel('Name').fill('Slug Copy')
    await page.locator('input[name="slug"]').fill('slug-owner')
    await page.getByRole('button', { name: 'Create' }).click()

    // the dialog stays open and the conflict is reported in place
    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()
    await expect(dialog.getByText(/already taken/i)).toBeVisible({ timeout: 15_000 })
    // and no second row was created
    await expect(page.getByRole('row', { name: /Slug Copy/ })).toHaveCount(0)
  })
})

/**
 * US-AD08 AC3 — only owner and admin may create a project.
 *
 * The CTA is asserted absent for a member, and the API is asserted to answer 403
 * for the same role, so the UI hiding and the server gate are pinned together:
 * hiding the control alone would be a UI-level lie, and a 403 alone would leave
 * a dead button on screen. The member joins the owner's workspace through the
 * real invite endpoint rather than a seeded fixture.
 */
test.describe('project directory — US-AD08 AC3 (owner/admin only)', () => {
  test('a member sees no New project CTA and the API answers 403', async ({ page, browser }) => {
    const orgID = await signUp(page)

    const memberEmail = `e2e-projmember-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
    const memberContext = await browser.newContext()
    const memberPage = await memberContext.newPage()

    // The member registers in their own context, which leaves a session cookie
    // there. Registration is done through the page (not `request`) so the cookie
    // lands in the browser context the navigation will use, and the page is on
    // an origin first — a relative `/api/v1/...` cannot be fetched from
    // `about:blank`.
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
      {
        api: API,
        credentials: { email: memberEmail, password: PASSWORD, name: 'E2E Member', org_name: 'Member Solo' },
      },
    )
    expect(registered.status, registered.body).toBe(201)

    // The owner invites them as `member` in the workspace created above. The
    // invite runs from the owner's page so it carries the owner's session — the
    // roster endpoint is admin-only and would answer 401 without it.
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

    await memberPage.goto(`/app/${orgID}/projects`)
    await expect(memberPage).toHaveURL(new RegExp(`/app/${ULID.source}/projects$`))

    // the directory renders for them — they are a member, not an outsider
    await expect(memberPage.getByText(/Project Directory/i).first()).toBeVisible({ timeout: 15_000 })
    await expect(memberPage.getByRole('button', { name: /new project/i })).toHaveCount(0)

    // and the gate is real: the same role calling the API directly gets 403
    const denied = await memberPage.evaluate(
      async ({ api, orgID }) => {
        const response = await fetch(`${api}/projects`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-Org-ID': orgID },
          credentials: 'include',
          body: JSON.stringify({ name: 'Sneaky', slug: 'sneaky-member-project' }),
        })
        return { status: response.status, body: await response.text() }
      },
      { api: API, orgID },
    )
    expect(denied.status, denied.body).toBe(403)

    await memberContext.close()
  })
})
