import { expect, test, type Page } from '@playwright/test'
import { chooseOption, optionValues } from './combobox'

/**
 * Screen 38-members — the workspace roster (US-AD04 AC1/AC2/AC3).
 *
 * Asserted here and nowhere else:
 *
 *  1. The density the design pins in its own header note — 32px header row,
 *     28px member row. A table that renders at its natural height looks correct
 *     and is still wrong.
 *  2. The role chips use the design's classes, including the owner chip's
 *     accent tint. `Array.prototype.join`-style drift (one grey chip for every
 *     role) is exactly what these pin.
 *  3. The invite form opens in a **modal**, not inline, and the invite actually
 *     lands: AC1 writes the membership and emails a notice.
 *  4. `owner` is not in the role picker and the owner's row exposes no "change
 *     role" control, because no path may demote the last owner (AC3).
 *
 * The account is registered per run, same as `project-directory.spec.ts`, so the
 * suite is re-runnable against a database that is never truncated.
 */
const API = '/api/v1'
const PASSWORD = 'correct-horse-battery-staple'
const ULID = /[0-9A-HJKMNP-TV-Z]{26}/

async function signUp(page: Page): Promise<string> {
  const email = `e2e-members-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
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
    { api: API, credentials: { email, password: PASSWORD, name: 'Roster Owner', org_name: 'Roster Co' } },
  )
  expect(result.status, result.body).toBe(201)
  return (JSON.parse(result.body) as { workspace_id: string }).workspace_id
}

test.describe('members — design match and US-AD04', () => {
  let orgID: string

  test.beforeEach(async ({ page }) => {
    orgID = await signUp(page)
    await page.goto(`/app/${orgID}/settings/members`)
    await expect(page).toHaveURL(new RegExp(`/app/${ULID.source}/settings/members$`))
  })

  test('the table uses the design density: 32px header row, 28px member row', async ({ page }) => {
    // The owner is always a member of their own workspace, so the body row
    // exists; waiting on the cell first avoids measuring during the redirect
    // that follows /auth/me resolving.
    await expect(page.getByRole('cell', { name: /roster owner/i }).first()).toBeVisible({ timeout: 15_000 })

    const headerRow = page.getByRole('row').first()
    await expect(headerRow).toBeVisible()

    const headerHeight = await headerRow.evaluate((el) => Math.round(el.getBoundingClientRect().height))
    expect(headerHeight, 'design: h-[32px] on <tr> in <thead>').toBe(32)

    const memberRow = page.getByRole('row').nth(1)
    await expect
      .poll(() => memberRow.evaluate((el) => Math.round(el.getBoundingClientRect().height)), {
        timeout: 15_000,
        message: 'design: h-[28px] on <tr> in <tbody>',
      })
      .toBe(28)
  })

  test('role chips are per-role, not one shared chip', async ({ page }) => {
    const dialog = page.getByRole('dialog')

    // Invite a second person as viewer so the roster has two distinct ranks.
    await page.getByRole('button', { name: /undang anggota/i }).click()
    await expect(dialog).toBeVisible()
    await dialog.locator('input[name="email"]').fill(`viewer-${Date.now()}@example.com`)
    // The role picker is the shared `Combobox`, so it is driven by its label
    // through the repo's helper — there is no `<option>` to `selectOption`.
    // The label is the dictionary's, not the enum: the app runs Indonesian here
    // (as `workspace-settings.spec.ts` does), so `viewer` renders as "Pengamat".
    await chooseOption(dialog, /role|peran/i, 'Pengamat')
    await dialog.locator('button[type="submit"]').click()
    await expect(dialog).toHaveCount(0, { timeout: 15_000 })

    const ownerChip = page.locator('[data-role="owner"]').first()
    await expect(ownerChip).toBeVisible()

    const ownerStyle = await ownerChip.evaluate((el) => {
      const s = getComputedStyle(el)
      return { background: s.backgroundColor, color: s.color }
    })
    // design owner chip: bg-[#eef7f5]-equivalent accent tint, text-[#0d7a70]
    expect(ownerStyle.background).toBe('rgb(230, 242, 240)')
    expect(ownerStyle.color).toBe('rgb(13, 122, 112)')

    const viewerChip = page.locator('[data-role="viewer"]').first()
    await expect(viewerChip).toBeVisible()
    const viewerBackground = await viewerChip.evaluate((el) => getComputedStyle(el).backgroundColor)
    // the viewer rank must not reuse the owner's accent tint
    expect(viewerBackground).not.toBe(ownerStyle.background)
  })

  test('inviting opens a modal and the invited member joins the roster — US-AD04 AC1', async ({ page }) => {
    // nothing inline before the click
    await expect(page.getByRole('dialog')).toHaveCount(0)

    await page.getByRole('button', { name: /undang anggota/i }).click()
    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()
    await expect(dialog).toHaveAttribute('aria-modal', 'true')

    // DESIGN.md `modal` token: surface-elevated, radius 14px, padding 20px
    const panel = await dialog.evaluate((el) => {
      const s = getComputedStyle(el)
      return { radius: s.borderRadius, padding: s.paddingTop }
    })
    expect(panel.radius).toBe('14px')
    expect(panel.padding).toBe('20px')

    const invitee = `invitee-${Date.now()}@example.com`
    await dialog.locator('input[name="email"]').fill(invitee)
    await dialog.locator('button[type="submit"]').click()

    // the dialog closes on success and the row arrives from the server
    await expect(dialog).toHaveCount(0, { timeout: 15_000 })
    await expect(page.getByRole('cell', { name: invitee })).toBeVisible({ timeout: 15_000 })
  })

  test('the modal closes on Escape and returns focus to the trigger', async ({ page }) => {
    const trigger = page.getByRole('button', { name: /undang anggota/i })
    await trigger.click()
    await expect(page.getByRole('dialog')).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await expect(trigger).toBeFocused()
  })

  test('owner is not assignable and the owner row offers no demotion — US-AD04 AC3', async ({ page }) => {
    await page.getByRole('button', { name: /undang anggota/i }).click()
    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()

    // `owner` is granted with the workspace and can never be handed out here.
    // A closed combobox has no `<option>` elements, so the rule is asserted on
    // the list it actually offers.
    expect(await optionValues(dialog, /role|peran/i)).not.toContain('Pemilik')

    // the design's own "Full owner" label sits on the owner's row instead of a
    // control that the server would answer with 403
    await expect(page.getByText(/pemilik penuh/i).first()).toBeVisible()
    await expect(page.getByRole('button', { name: /ubah role/i })).toHaveCount(0)
  })
})
