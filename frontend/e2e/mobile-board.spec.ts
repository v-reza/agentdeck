import { expect, test, type Page } from '@playwright/test'

/**
 * US-AD60 — the mobile board.
 *
 * AC1 and AC3 are a pair: the same URL has to render an accordion below 768px and
 * lanes at or above it. That is why the viewport is the fixture here and not a
 * separate `/m/` route — the design entry names `/m/boards/:id`, but the story
 * says "pada viewport < 768px kolom board berubah menjadi accordion", which is a
 * responsive rule, and a second route would leave the real board still broken on a
 * phone.
 *
 * AC2 is the interesting one and it is NOT a navigation test: a rotation is a
 * resize. `page.setViewportSize` while the page stays mounted is exactly the
 * event AC2 describes, so the assertion is "the open column is still open after
 * the viewport changes and comes back" — no reload, no route change.
 */

const API = '/api/v1'
const PASSWORD = 'Sup3rSecret!2026'
const MOBILE = { width: 390, height: 844 }
const DESKTOP = { width: 1280, height: 900 }

async function signUp(page: Page, name: string): Promise<string> {
  const res = await page.request.post(`${API}/auth/register`, {
    data: {
      email: `e2e-mobile-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`,
      password: PASSWORD,
      name,
    },
  })
  const body = await res.text()
  expect(res.status(), body).toBe(201)
  const [session] = (await page.context().cookies()).filter((c) => c.name === 'agentdeck_session')
  await page.context().addCookies([{ ...session, secure: false }])
  return (JSON.parse(body) as { workspace_id: string }).workspace_id
}

async function api<T>(
  page: Page,
  orgID: string,
  method: 'GET' | 'POST',
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

/** A board with one task in `backlog` and one in `ready`, so two lanes have work. */
async function seedBoard(page: Page, orgID: string) {
  const project = await api<{ id: string }>(page, orgID, 'POST', '/projects', {
    name: 'Mobile',
    slug: `mobile-${Date.now()}`,
  })
  expect(project.status, project.text).toBe(201)
  const board = await api<{ id: string }>(page, orgID, 'POST', `/projects/${project.data.id}/boards`, {
    name: 'Mobile Board',
    slug: `mobile-b-${Date.now()}`,
  })
  expect(board.status, board.text).toBe(201)

  const backlog = await api(page, orgID, 'POST', `/boards/${board.data.id}/tasks`, {
    title: 'Backlog on a phone',
    status: 'backlog',
  })
  expect(backlog.status, backlog.text).toBe(201)
  const ready = await api(page, orgID, 'POST', `/boards/${board.data.id}/tasks`, {
    title: 'Ready on a phone',
    status: 'ready',
  })
  expect(ready.status, ready.text).toBe(201)
  return board.data.id
}

test.describe('mobile board — US-AD60', () => {
  test('AC1 — below 768px the board is a vertical accordion, one column open, no horizontal scroll', async ({
    page,
  }) => {
    await page.setViewportSize(MOBILE)
    const orgID = await signUp(page, 'Mobile Owner')
    await page.goto('/login')
    const boardID = await seedBoard(page, orgID)

    await page.goto(`/app/${orgID}/boards/${boardID}`)

    const accordion = page.getByTestId('mobile-board')
    await expect(accordion).toBeVisible({ timeout: 15_000 })
    // Five columns, as accordion sections rather than five lanes.
    await expect(accordion.locator('section')).toHaveCount(5)

    // AC1: selecting a column shows that column's tasks. `backlog` is where the
    // seed put work, and it is the first lane with tasks, so it starts open.
    await expect(page.getByTestId('mobile-toggle-backlog')).toHaveAttribute('aria-expanded', 'true')
    await expect(page.getByText('Backlog on a phone')).toBeVisible()
    // …and the others are closed, so their tasks are not on screen.
    await expect(page.getByTestId('mobile-toggle-ready')).toHaveAttribute('aria-expanded', 'false')
    await expect(page.getByText('Ready on a phone')).toBeHidden()

    await page.getByTestId('mobile-toggle-ready').click()
    await expect(page.getByTestId('mobile-toggle-ready')).toHaveAttribute('aria-expanded', 'true')
    await expect(page.getByTestId('mobile-toggle-backlog')).toHaveAttribute('aria-expanded', 'false')
    await expect(page.getByText('Ready on a phone')).toBeVisible()
    await expect(page.getByText('Backlog on a phone')).toBeHidden()

    // "tanpa horizontal scroll" — the point of the story. Measured, not assumed.
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
    expect(overflow, 'the board must not scroll sideways on a phone').toBeLessThanOrEqual(0)
  })

  // The shell, not the board: below the breakpoint the side chrome comes off the
  // screen and navigation moves to the bottom bar. This is the assertion that makes
  // AC1's "no horizontal scroll" meaningful — the desktop shell is 532px of rail,
  // sidebar and cost rail, so with it on screen at 390px the board is not narrow,
  // it is covered.
  test('AC1 — below 768px the shell drops its side chrome and shows the bottom bar', async ({ page }) => {
    await page.setViewportSize(MOBILE)
    const orgID = await signUp(page, 'Shell Owner')
    await page.goto('/login')
    const boardID = await seedBoard(page, orgID)
    await page.goto(`/app/${orgID}/boards/${boardID}`)
    await expect(page.getByTestId('mobile-board')).toBeVisible({ timeout: 15_000 })

    await expect(page.getByTestId('mobile-tab-bar')).toBeVisible()
    await expect(page.getByRole('navigation', { name: 'Workspace' })).toHaveCount(0)
    await expect(page.getByRole('navigation', { name: 'Primary' })).toHaveCount(0)
    await expect(page.getByRole('complementary', { name: 'Cost and usage' })).toHaveCount(0)

    // …and the bottom bar actually navigates. Matched loosely because the label is
    // translated ('Board' in the default `id` locale, 'Boards' in `en`) and pinning
    // one of them is how four earlier phases broke their own e2e on a dictionary
    // change. `data-testid` on the bar, a name pattern on the link.
    await page.getByTestId('mobile-tab-bar').getByRole('link', { name: /board/i }).first().click()
    await expect(page).toHaveURL(new RegExp(`/app/${orgID}/boards`))
  })

  test('AC3 — at or above 768px the same URL renders the lanes, not the accordion', async ({ page }) => {
    const orgID = await signUp(page, 'Desktop Owner')
    await page.goto('/login')
    const boardID = await seedBoard(page, orgID)

    await page.setViewportSize(DESKTOP)
    await page.goto(`/app/${orgID}/boards/${boardID}`)

    // The desktop board: five lanes side by side, and no accordion at all.
    await expect(page.getByTestId('mobile-board')).toHaveCount(0)
    await expect(page.getByText('Backlog on a phone')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByText('Ready on a phone')).toBeVisible()
    // The chrome comes back with the width.
    await expect(page.getByTestId('mobile-tab-bar')).toHaveCount(0)
    await expect(page.getByRole('navigation', { name: 'Workspace' })).toBeVisible()
  })

  test('AC2 — the open column survives a rotation, with no reload and no navigation', async ({ page }) => {
    await page.setViewportSize(MOBILE)
    const orgID = await signUp(page, 'Rotate Owner')
    await page.goto('/login')
    const boardID = await seedBoard(page, orgID)

    await page.goto(`/app/${orgID}/boards/${boardID}`)
    await expect(page.getByTestId('mobile-board')).toBeVisible({ timeout: 15_000 })

    // Choose a column that is NOT the default one, or this test would pass on an
    // implementation that simply always opens `backlog`.
    await page.getByTestId('mobile-toggle-review').click()
    await expect(page.getByTestId('mobile-toggle-review')).toHaveAttribute('aria-expanded', 'true')

    // Rotate to landscape — 667x375 (iPhone SE), which is still UNDER the 768px
    // breakpoint, so the accordion stays and AC2 is the rule in play.
    //
    // This is deliberately not a modern phone's 844x390: that is WIDER than the
    // breakpoint, so rotating into it is AC3's case (desktop lanes), not AC2's,
    // and asserting the accordion there would be asserting the wrong contract.
    // An earlier draft used 844x390 and passed or failed depending on whether the
    // assertion ran before or after the media query listener re-rendered.
    await page.setViewportSize({ width: 667, height: 375 })
    await expect(page.getByTestId('mobile-board')).toBeVisible()
    await expect(page.getByTestId('mobile-toggle-review')).toHaveAttribute('aria-expanded', 'true')

    // Rotate back to portrait.
    await page.setViewportSize(MOBILE)
    await expect(page.getByTestId('mobile-toggle-review')).toHaveAttribute('aria-expanded', 'true')

    // AC2 + AC3 together: a trip to a desktop width renders the lanes, and coming
    // back keeps the same column open. That is the part a local `useState` inside
    // an unmounted accordion could not do.
    await page.setViewportSize(DESKTOP)
    await expect(page.getByTestId('mobile-board')).toHaveCount(0)
    await page.setViewportSize(MOBILE)
    await expect(page.getByTestId('mobile-toggle-review')).toHaveAttribute('aria-expanded', 'true')
  })
})
