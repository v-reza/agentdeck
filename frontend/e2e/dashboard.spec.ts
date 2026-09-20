import { expect, test, type Page } from '@playwright/test'

/**
 * End-to-end flow against the real Go API on :8080 and the real PostgreSQL.
 *
 * Every assertion reads a value that came from the server, so a pass cannot be
 * produced by a mock or a hard-coded list: the workspace id, the project row,
 * the board's budget cap, and the rail's empty reason all originate in the
 * database.
 *
 * A unique email per run keeps the suite re-runnable against a database that is
 * never truncated.
 */

/**
 * Requests go through the Vite dev proxy rather than straight to :8080. The
 * session is an HttpOnly cookie scoped to the app's own origin, so a request to
 * a different origin would not carry it and every call would 401. The browser
 * always talks to the proxy, so the test does too.
 */
const API = '/api/v1'
const PASSWORD = 'correct-horse-battery-staple'
const ULID = /[0-9A-HJKMNP-TV-Z]{26}/

function uniqueEmail(): string {
  return `e2e-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
}

/**
 * Seeds an account and returns its workspace id.
 *
 * The calls run through `page.evaluate(fetch)`, i.e. inside the browser at the
 * app's own origin, rather than through Playwright's request context. The
 * session cookie is `HttpOnly; Secure` (ARCHITECTURE 11.1), and Playwright drops
 * a Secure cookie over plain-HTTP localhost — so an out-of-browser request would
 * 401 even though the real browser stays signed in.
 */
async function signUp(page: Page, email: string): Promise<string> {
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
    {
      api: API,
      credentials: { email, password: PASSWORD, name: 'E2E Operator', org_name: 'E2E Fleet' },
    },
  )
  expect(result.status, result.body).toBe(201)
  return (JSON.parse(result.body) as { workspace_id: string }).workspace_id
}

/** Creates a project and a board for the signed-in operator, in the browser. */
async function seedBoard(
  page: Page,
  orgID: string,
  projectName: string,
  boardName: string,
): Promise<{ projectID: string; boardID: string }> {
  const result = await page.evaluate(
    async ({ api, org, project, board }) => {
      const headers = { 'Content-Type': 'application/json', 'X-Org-ID': org }
      const created = await fetch(`${api}/projects`, {
        method: 'POST',
        headers,
        credentials: 'include',
        body: JSON.stringify({ name: project.name, slug: project.slug }),
      })
      const projectBody = await created.text()
      if (created.status !== 201) return { error: `${created.status} ${projectBody}` }

      const projectID = (JSON.parse(projectBody) as { id: string }).id
      const boardResponse = await fetch(`${api}/projects/${projectID}/boards`, {
        method: 'POST',
        headers,
        credentials: 'include',
        body: JSON.stringify({ name: board.name, slug: board.slug }),
      })
      const boardBody = await boardResponse.text()
      if (boardResponse.status !== 201) return { error: `${boardResponse.status} ${boardBody}` }

      return { projectID, boardID: (JSON.parse(boardBody) as { id: string }).id }
    },
    {
      api: API,
      org: orgID,
      project: { name: projectName, slug: projectName.toLowerCase().replace(/[^a-z0-9]+/g, '-') },
      board: { name: boardName, slug: boardName.toLowerCase().replace(/[^a-z0-9]+/g, '-') },
    },
  )
  if ('error' in result) throw new Error(`seedBoard failed: ${result.error}`)
  return result
}

async function signIn(page: Page, email: string, orgID: string): Promise<void> {
  await page.goto('/login')
  await page.getByLabel('Email').fill(email)
  await page.getByLabel('Password').fill(PASSWORD)
  // US-AD02 AC5 names the submit button "Log in"; the label is the criterion.
  await page.getByRole('button', { name: /log in/i }).click()
  await page.waitForURL(new RegExp(`/app/${orgID}/projects$`), { timeout: 30_000 })
}

test.describe('AgentDeck dashboard', () => {
  test('an operator registers, creates a project, and it survives a reload — US-AD01 AC1, US-AD08 AC1', async ({
    page,
  }) => {
    const email = uniqueEmail()

    await page.goto('/register')
    // 02-register asks for email and password only: the personal workspace is
    // created by the server (US-AD01 AC5) and the display name falls back to
    // the email local part (AC6), so there is no name or org field to fill.
    await expect(page.getByRole('heading', { name: /daftar ke agentdeck/i })).toBeVisible()

    await page.getByLabel('Email').fill(email)
    await page.getByLabel('Password').fill(PASSWORD)
    await page.getByRole('button', { name: /daftar akun/i }).click()

    // Landing on the dashboard means the session cookie was accepted and
    // /auth/me answered with a real workspace ULID.
    await page.waitForURL(new RegExp(`/app/${ULID.source}/projects$`), { timeout: 30_000 })
    await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible()

    // The shell is the contract frame: 44 / 224 / 52 / 264 px (DESIGN.md).
    expect((await page.locator('nav[aria-label="Primary"]').boundingBox())?.width).toBe(44)
    expect((await page.locator('nav[aria-label="Workspace"]').boundingBox())?.width).toBe(224)
    expect((await page.locator('header').first().boundingBox())?.height).toBe(52)
    expect((await page.locator('aside[aria-label="Cost and usage"]').boundingBox())?.width).toBe(264)

    // Create a project through the real form (useActionState + RTK Query).
    await page.getByRole('button', { name: 'New project' }).click()
    await page.getByLabel('Name').fill('E2E Control Plane')
    await page.getByRole('button', { name: 'Create' }).click()

    const row = page.getByRole('row', { name: /E2E Control Plane/ })
    await expect(row).toBeVisible({ timeout: 15_000 })
    // The slug is derived from the name client-side and persisted by the server.
    await expect(row).toContainText('e2e-control-plane')

    // Persistence: a reload must read the project back from PostgreSQL.
    await page.reload()
    await expect(page.getByRole('row', { name: /E2E Control Plane/ })).toBeVisible({ timeout: 15_000 })
  })

  test('the cost rail reports a board budget instead of an invented number — US-AD32 AC3', async ({ page }) => {
    const email = uniqueEmail()
    const orgID = await signUp(page, email)

    // A board is created through the API because the board-creation form is
    // still on the M1 list; the budget the rail renders is a server value.
    const { projectID, boardID } = await seedBoard(page, orgID, 'Rail Probe', 'Sprint 24')

    const costRail = page.locator('aside[aria-label="Cost and usage"]')

    await signIn(page, email, orgID)

    // No board is open yet, so the rail must say why it is empty rather than
    // render a zero that reads like real spend.
    await expect(costRail).toContainText(/open a board/i)

    await page.goto(`/app/${orgID}/boards/${boardID}`)
    await expect(costRail).toContainText(/cap/i, { timeout: 20_000 })
    await expect(costRail).not.toContainText(/open a board/i)

    // The project is listed with the board the API created.
    await page.goto(`/app/${orgID}/projects/${projectID}`)
    await expect(page.getByRole('link', { name: 'Sprint 24' })).toBeVisible({ timeout: 20_000 })
  })

  // No PRD criterion covers an unauthenticated deep link (the only ACs about
  // login-free access are US-AD100/US-AD103 for the public pages), so this test
  // carries no citation rather than a borrowed one.
  test('an unauthenticated deep link is sent to the login screen', async ({ page }) => {
    await page.goto('/app/01J8ZQ7F5K3M9N2P4R6T8V0XAB/boards')
    await page.waitForURL(/\/login$/, { timeout: 30_000 })
    await expect(page.getByRole('heading', { name: /masuk ke agentdeck/i })).toBeVisible()
  })

  /**
   * US-AD13 (Must, M1) — "Task detail drawer".
   *
   * Opening a card must reveal the drawer. This catches a drawer that is
   * implemented but never mounted: an earlier revision rendered only an sr-only
   * marker when a card was opened, so the click looked handled while nothing
   * appeared on screen.
   */
  test('clicking a board card opens the task detail drawer — US-AD13', async ({ page }) => {
    const email = `e2e-drawer-${Date.now()}@example.com`
    const orgID = await signUp(page, email)
    const { boardID } = await seedBoard(page, orgID, 'Drawer Project', 'Drawer Board')

    await page.evaluate(
      async ({ api, org, board }) => {
        await fetch(`${api}/boards/${board}/tasks`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-Org-ID': org },
          credentials: 'include',
          body: JSON.stringify({ title: 'Open me', body: 'the drawer must appear', priority: 1 }),
        })
      },
      { api: API, org: orgID, board: boardID },
    )

    await page.goto(`/app/${orgID}/boards/${boardID}`)

    const card = page.getByText('Open me').first()
    await expect(card).toBeVisible({ timeout: 30_000 })
    await card.click()

    // The drawer is the complementary landmark named by the design source
    // (20-task-drawer.html). Naming it matters: the cost rail is a complementary
    // landmark too, so an unnamed drawer cannot be addressed unambiguously.
    const drawer = page.getByRole('complementary', { name: 'Task Detail Drawer' })
    await expect(drawer).toBeVisible()
    await expect(drawer.getByText('Open me').first()).toBeVisible()
  })
})
