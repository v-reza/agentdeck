import { expect, test, type Page } from '@playwright/test'

/**
 * US-AD63 / US-AD64 / US-AD65 — the board's loading, empty and failed states.
 *
 * The e2e suite is the right place for the first three: they are about what the
 * screen does while the API is slow, empty or broken, and the only way to know
 * that is to make the real API behave that way. `page.route` replays the API's
 * answer, so the assertions are about the UI's handling of the answer, not about a
 * fixture the test invented.
 *
 * US-AD65's boundary is covered by `src/components/ui/error-boundary.test.tsx`
 * instead, and deliberately: a render error is not reachable from outside the
 * page, so driving one from Playwright would mean testing a mock of the thing.
 *
 * The failure mode worth guarding hardest is US-AD64 AC3: an empty list over a
 * FAILED request. RTK Query answers a failed query with `data === undefined`, so
 * a board that only checks `tasks.length === 0` renders "create your first task"
 * over a 500 — and an operator who believes it creates a duplicate of work that
 * already exists. The failed case is asserted BEFORE the empty one below, because
 * the two look identical in a fixture where nothing was ever created.
 */

const API = '/api/v1'
const PASSWORD = 'Sup3rSecret!2026'

async function signUp(page: Page, email: string, name: string): Promise<string> {
  const response = await page.request.post(`${API}/auth/register`, {
    data: { email, password: PASSWORD, name },
  })
  const body = await response.text()
  expect(response.status(), body).toBe(201)

  // The session cookie is issued `Secure`, and Playwright's request context
  // refuses to send one over the plain-http test origin — every later
  // `page.request` call answers 401. Re-adding the same value with `secure: false`
  // (the origin really is plain http) leaves one jar the browser and the request
  // context share. Same trick as `board-live.spec.ts`, which needs it to act as a
  // second client.
  const [session] = (await page.context().cookies()).filter((c) => c.name === 'agentdeck_session')
  expect(session, 'registration did not set a session cookie').toBeTruthy()
  await page.context().addCookies([{ ...session, secure: false }])

  return (JSON.parse(body) as { workspace_id: string }).workspace_id
}

/** A write from "another client", so the board's own query is invalidated. */
async function createTaskFromOtherClient(page: Page, orgID: string, boardID: string, title: string) {
  const res = await page.request.post(`${API}/boards/${boardID}/tasks`, {
    headers: { 'X-Org-ID': orgID },
    data: { title, status: 'backlog' },
  })
  expect(res.status(), await res.text()).toBe(201)
}

/** Authenticated call from inside the app origin, using the browser's cookies. */
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
        data = JSON.parse(text)
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
  await expect(page).toHaveURL(/\/app\/[^/]+\/projects$/)
}

async function createBoard(page: Page, orgID: string, label: string) {
  const stamp = Date.now()
  const project = await api<{ id: string }>(page, orgID, 'POST', '/projects', {
    name: `${label} Project`,
    slug: `${label.replace(/[^a-z]/g, '')}-${stamp}`,
  })
  expect(project.status, project.text).toBe(201)
  const board = await api<{ id: string }>(page, orgID, 'POST', `/projects/${project.data.id}/boards`, {
    name: `${label} Board`,
    slug: `${label.replace(/[^a-z]/g, '')}-b-${stamp}`,
  })
  expect(board.status, board.text).toBe(201)
  return board.data.id
}

test.describe('board states (US-AD63, US-AD64)', () => {
  test('AC63.1/AC63.2 — the skeleton is the board’s shape and only the first load shows it', async ({ page }) => {
    const orgID = await signUp(page, `states-${Date.now()}@example.com`, 'States Co')
    await signIn(page, orgID)
    const boardID = await createBoard(page, orgID, 'states')

    // Hold the task list open so the first-load state is observable. The route
    // still hits the real API; it just does not answer until released.
    let release: (() => void) | undefined
    const gate = new Promise<void>((resolve) => {
      release = resolve
    })
    await page.route(`**${API}/boards/${boardID}/tasks*`, async (route) => {
      await gate
      await route.continue()
    })

    await page.goto(`/app/${orgID}/boards/${boardID}`)

    const skeleton = page.getByTestId('board-skeleton')
    await expect(skeleton).toBeVisible({ timeout: 15_000 })

    // Five lanes, three cards each (AC1), and the pulse the story names.
    await expect(skeleton.getByTestId('skeleton-card')).toHaveCount(15)
    const pulsed = await skeleton
      .getByTestId('skeleton-card')
      .first()
      .locator('div')
      .first()
      .evaluate((el) => {
        return getComputedStyle(el).animationName
      })
    expect(pulsed).not.toBe('none')

    // The board renders for real once the request lands.
    release?.()
    await expect(page.getByText('Belum ada task')).toBeVisible({ timeout: 15_000 })
    await expect(skeleton).toHaveCount(0)

    // AC2: a background refresh must NOT bring the skeleton back. This drives the
    // REAL path rather than a hand-rolled fetch: the toolbar's SSE subscription is
    // open on this page, so a task created by another client invalidates the very
    // query the board reads and RTK refetches it.
    await expect(page.getByText(/^(Live|Langsung)$/)).toBeVisible({ timeout: 15_000 })

    // The refetch is held open deliberately. Without this the refetch lands in
    // milliseconds and the assertion below samples only its AFTER state, where the
    // skeleton is gone either way — a mutant that shows the skeleton on every
    // `isFetching` survives that. The delay is what makes the in-flight window
    // observable, and in-flight is the only moment the two implementations differ.
    await page.route(`**${API}/boards/${boardID}/tasks*`, async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 2500))
      await route.continue()
    })

    await createTaskFromOtherClient(page, orgID, boardID, 'Arrived during a background refresh')

    // Mid-refetch: the operator keeps the board they already had. `isLoading` is
    // false here and `isFetching` is true — the exact distinction the story draws.
    await page.waitForTimeout(800)
    await expect(page.getByTestId('board-skeleton')).toHaveCount(0)
    await expect(page.getByTestId('board-empty')).toBeVisible()

    // And the background refresh still lands.
    await expect(page.getByText('Arrived during a background refresh')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('board-skeleton')).toHaveCount(0)
  })

  test('AC64.1 — an empty board invites a first task, and a filtered board says something else', async ({ page }) => {
    const orgID = await signUp(page, `empty-${Date.now()}@example.com`, 'Empty Co')
    await signIn(page, orgID)
    const boardID = await createBoard(page, orgID, 'empty')

    await page.goto(`/app/${orgID}/boards/${boardID}`)
    await expect(page.getByTestId('board-empty')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('board-empty-filtered')).toHaveCount(0)

    // AC2: the CTA opens the create form. Asserting the modal rather than a URL
    // change, because the form is a modal driven by redux, not a route.
    await page.getByTestId('board-create-first').click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // AC1: with a filter active, the SAME empty board must not offer "create your
    // first task" — the board is not empty, the filter is.
    await page.keyboard.press('Escape')
    await page.getByTestId('board-search').fill('zzz-no-such-task')
    await expect(page.getByTestId('board-empty-filtered')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('board-empty')).toHaveCount(0)
    await expect(page.getByTestId('board-create-first')).toHaveCount(0)
  })

  test('AC64.3/AC63.3 — a failed load is an error state with a retry, never the empty state', async ({ page }) => {
    const orgID = await signUp(page, `fail-${Date.now()}@example.com`, 'Fail Co')
    await signIn(page, orgID)
    const boardID = await createBoard(page, orgID, 'fail')

    let failing = true
    await page.route(`**${API}/boards/${boardID}/tasks*`, async (route) => {
      if (failing) {
        await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: 'boom' }) })
        return
      }
      await route.continue()
    })

    await page.goto(`/app/${orgID}/boards/${boardID}`)

    await expect(page.getByTestId('board-error')).toBeVisible({ timeout: 15_000 })
    // The lie this test exists to prevent.
    await expect(page.getByTestId('board-empty')).toHaveCount(0)
    await expect(page.getByTestId('board-create-first')).toHaveCount(0)

    // The retry is a real refetch, not a reload: the same route is asked again
    // and this time it is allowed through.
    failing = false
    await page.getByTestId('board-retry').click()
    await expect(page.getByTestId('board-error')).toHaveCount(0)
    await expect(page.getByTestId('board-empty')).toBeVisible({ timeout: 15_000 })
  })
})
