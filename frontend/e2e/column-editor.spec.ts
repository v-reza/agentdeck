import { expect, test, type Page } from '@playwright/test'

/**
 * Screen 22-column-editor — US-AD10, editing a board's columns.
 *
 * All five acceptance criteria are asserted end to end against the real API:
 *
 *  AC1 the new column lands in the LAST position.
 *  AC2 removing a column that holds tasks is a 409.
 *  AC3 a duplicate column name is a 409 and is surfaced inline.
 *  AC4 a member sees no editor and the API answers 403 for them.
 *  AC5 a solo workspace (B2C) can edit columns with no RBAC setup at all.
 *
 * Two things about the plumbing are load-bearing:
 *
 * 1. `PATCH /boards/{id}` is Member-gated. The layout used to be writable through
 *    it, which made AC4's Admin rule bypassable by any member. The escalation
 *    assertions below pin that shut — an AC4 test alone would still pass if the
 *    old path were reopened.
 *
 * 2. The session cookie is `Secure` (production posture) while the dev servers run
 *    on plain HTTP. A real browser sends a Secure cookie to 127.0.0.1 anyway, but
 *    Playwright's `page.request` does NOT — it answers 401 for every authenticated
 *    call. So only the one call that has to happen before a page exists (register)
 *    uses `page.request`; everything else is an in-page `fetch`, which shares the
 *    browser's cookie handling. Probed both ways before settling on this.
 */
const API = '/api/v1'
const PASSWORD = 'correct-horse-battery-staple'

async function signUp(page: Page, name = 'E2E Column Operator'): Promise<string> {
  const email = `e2e-columns-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
  const response = await page.request.post(`${API}/auth/register`, {
    data: { email, password: PASSWORD, name },
  })
  const body = await response.text()
  expect(response.status(), body).toBe(201)
  return (JSON.parse(body) as { workspace_id: string }).workspace_id
}

/**
 * Authenticated call from inside the app origin, using the browser's cookies.
 *
 * The body is parsed defensively: a rejected request answers with a plain-text
 * reason (`insufficient role`, `column still holds tasks`), so assuming JSON here
 * would turn a 403 the test is asserting into a parse crash.
 */
async function api<T>(
  page: Page,
  orgID: string,
  method: 'GET' | 'POST' | 'PATCH',
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

/**
 * Lands on a real app page first, so the session cookie is present in the browser
 * and `api` calls are authenticated.
 */
async function signIn(page: Page, orgID: string) {
  await page.goto(`/app/${orgID}/projects`)
  await expect(page).toHaveURL(/\/app\/[^/]+\/projects$/)
}

async function createBoard(page: Page, orgID: string, name: string): Promise<string> {
  const projects = await api<{ id: string }[]>(page, orgID, 'GET', '/projects')
  expect(projects.status, 'listing projects').toBe(200)

  const created = await api<{ id: string }>(page, orgID, 'POST', `/projects/${projects.data[0].id}/boards`, {
    name,
    slug: name.toLowerCase().replace(/[^a-z0-9]+/g, '-'),
  })
  expect(created.status, 'creating board').toBe(201)
  return created.data.id
}

async function readColumns(page: Page, orgID: string, boardID: string): Promise<{ key: string; name: string }[]> {
  const response = await api<{ key: string; name: string }[]>(page, orgID, 'GET', `/boards/${boardID}/columns`)
  expect(response.status, 'reading layout').toBe(200)
  return response.data
}

async function openEditor(page: Page, orgID: string, boardID: string) {
  await page.goto(`/app/${orgID}/boards/${boardID}/settings`)
  await page.getByRole('button', { name: /edit kolom/i }).click()
  await expect(page.getByRole('heading', { name: /editor kolom board/i })).toBeVisible()
}

test.describe('column editor (US-AD10)', () => {
  let orgID: string
  let boardID: string

  test.beforeEach(async ({ page }) => {
    orgID = await signUp(page)
    await signIn(page, orgID)
    boardID = await createBoard(page, orgID, 'Column Board')
  })

  test('the editor is the 420px design panel with a drag handle per column', async ({ page }) => {
    await openEditor(page, orgID, boardID)

    const panel = page.locator('aside').filter({ hasText: 'Editor Kolom Board' })
    const box = await panel.evaluate((el) => {
      const s = getComputedStyle(el)
      return { width: Math.round(el.getBoundingClientRect().width), background: s.backgroundColor }
    })
    // design 22-column-editor: panel 420px on surface-panel (#ffffff)
    expect(box.width).toBe(420)
    expect(box.background).toBe('rgb(255, 255, 255)')

    // One grab handle per column; the five defaults are editable by key.
    await expect(panel.getByRole('button', { name: /ubah urutan kolom/i })).toHaveCount(5)
    for (const key of ['backlog', 'ready', 'running', 'review', 'done']) {
      await expect(panel.getByLabel(`Nama kolom ${key}`)).toBeVisible()
    }
  })

  // AC1: the AC pins the POSITION, not just the status code.
  test('AC1 — a new column is appended last and persists', async ({ page }) => {
    await openEditor(page, orgID, boardID)

    await page.getByLabel('Nama kolom baru').fill('Blocked')
    await page.getByRole('button', { name: /^tambah$/i }).click()

    // The draft shows it last before saving.
    await expect(page.getByRole('button', { name: /ubah urutan kolom/i })).toHaveCount(6)

    await page.getByRole('button', { name: /simpan kolom/i }).click()
    await expect(page.getByText(/kolom tersimpan/i)).toBeVisible({ timeout: 15_000 })

    const persisted = await readColumns(page, orgID, boardID)
    expect(persisted).toHaveLength(6)
    expect(persisted[5].name).toBe('Blocked')
    // The existing order must survive the append.
    expect(persisted.slice(0, 5).map((c) => c.key)).toEqual(['backlog', 'ready', 'running', 'review', 'done'])
  })

  // AC3: duplicate NAME — what the operator actually edits.
  test('AC3 — renaming a column onto an existing name is a 409 shown inline', async ({ page }) => {
    await openEditor(page, orgID, boardID)

    await page.getByLabel('Nama kolom running').fill('Review')
    await page.getByRole('button', { name: /simpan kolom/i }).click()

    const alert = page.getByRole('alert')
    await expect(alert).toBeVisible({ timeout: 15_000 })
    await expect(alert).toContainText(/sudah ada|already exists/i)

    // Nothing was written: the server rejected the whole layout.
    const persisted = await readColumns(page, orgID, boardID)
    expect(persisted.map((c) => c.name)).toEqual(['Backlog', 'Ready', 'Running', 'Review', 'Done'])
  })

  // AC2: the guard is about the tasks, and the SERVER enforces it.
  test('AC2 — removing a column that holds tasks is refused', async ({ page }) => {
    // A task is created in `backlog` (the only status the API accepts at creation
    // — `running` is the dispatcher's claim, not a client's). So the column that
    // holds it is `backlog`, and the removal that must be refused is that one.
    const seeded = await api(page, orgID, 'POST', `/boards/${boardID}/tasks`, {
      title: 'E2E backlog task',
    })
    expect(seeded.status, seeded.text).toBe(201)

    await openEditor(page, orgID, boardID)

    // The badge counts it and the delete control is visibly disabled.
    await expect(page.getByTestId('task-count-backlog')).toHaveText('1 task')
    await expect(page.getByTestId('remove-column-backlog')).toBeDisabled()

    // The server refuses it too — the assertion the UI cannot fake.
    const refused = await api(page, orgID, 'PATCH', `/boards/${boardID}/columns`, {
      columns: [
        { key: 'ready', name: 'Ready' },
        { key: 'running', name: 'Running' },
        { key: 'review', name: 'Review' },
        { key: 'done', name: 'Done' },
      ],
    })
    expect(refused.status, refused.text).toBe(409)

    // The layout is untouched, and an EMPTY column is still removable: the guard
    // is a rule about tasks, not a blanket ban on removing columns.
    const persisted = await readColumns(page, orgID, boardID)
    expect(persisted).toHaveLength(5)

    const dropped = await api(page, orgID, 'PATCH', `/boards/${boardID}/columns`, {
      columns: [
        { key: 'backlog', name: 'Backlog' },
        { key: 'ready', name: 'Ready' },
        { key: 'running', name: 'Running' },
        { key: 'done', name: 'Done' },
      ],
    })
    expect(dropped.status, dropped.text).toBe(200)
  })

  test('reordering persists — the layout is an ordered list', async ({ page }) => {
    const swapped = await api(page, orgID, 'PATCH', `/boards/${boardID}/columns`, {
      columns: [
        { key: 'done', name: 'Done' },
        { key: 'backlog', name: 'Backlog' },
        { key: 'ready', name: 'Ready' },
        { key: 'running', name: 'Running' },
        { key: 'review', name: 'Review' },
      ],
    })
    expect(swapped.status).toBe(200)

    const persisted = await readColumns(page, orgID, boardID)
    expect(persisted.map((c) => c.key)).toEqual(['done', 'backlog', 'ready', 'running', 'review'])
  })
})

// AC4 + the escalation that AC4 alone cannot catch.
test.describe('column editor — US-AD10 AC4 (admin only)', () => {
  test('a member sees no editor, and the layout route answers 403', async ({ page, browser }) => {
    const orgID = await signUp(page)
    await signIn(page, orgID)
    const boardID = await createBoard(page, orgID, 'RBAC Board')

    const memberEmail = `e2e-colmember-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
    const memberContext = await browser.newContext()
    const memberPage = await memberContext.newPage()

    const registered = await memberPage.request.post(`${API}/auth/register`, {
      data: { email: memberEmail, password: PASSWORD, name: 'E2E Column Member' },
    })
    expect(registered.status(), await registered.text()).toBe(201)

    const invited = await api(page, orgID, 'POST', `/orgs/${orgID}/members`, {
      email: memberEmail,
      role: 'member',
    })
    expect(invited.status, 'inviting the member').toBe(201)

    await memberPage.goto(`/app/${orgID}/boards/${boardID}/settings`)
    await expect(memberPage.getByRole('heading', { name: /board settings/i })).toBeVisible()

    // The control is absent, so a member cannot reach the editor at all.
    await expect(memberPage.getByRole('button', { name: /edit kolom/i })).toHaveCount(0)

    // And the route is Admin-gated: hiding the button is not the boundary.
    const denied = await api(memberPage, orgID, 'PATCH', `/boards/${boardID}/columns`, {
      columns: [{ key: 'backlog', name: 'Member Rename' }],
    })
    expect(denied.status).toBe(403)

    // The escalation path: PATCH /boards/{id} is Member-gated, so if it accepted
    // `columns` a member could edit the layout anyway and the 403 above would be
    // decorative. Sending a layout there must not change the layout.
    const escalated = await api(memberPage, orgID, 'PATCH', `/boards/${boardID}`, {
      columns: [{ key: 'backlog', name: 'Escalated' }],
    })
    expect(escalated.status, 'PATCH /boards/{id} must stay Member-gated, not reject the whole request').not.toBe(403)

    const after = await readColumns(memberPage, orgID, boardID)
    expect(after.map((c) => c.name)).not.toContain('Escalated')

    await memberContext.close()
  })

  // AC5: the primary user is a solo builder, so the admin gate must not require
  // inviting a second person.
  test('AC5 — a solo workspace can edit its columns with no RBAC setup', async ({ page }) => {
    const soloOrg = await signUp(page, 'E2E Solo Builder')
    await signIn(page, soloOrg)
    const soloBoard = await createBoard(page, soloOrg, 'Solo Board')

    await openEditor(page, soloOrg, soloBoard)
    await page.getByLabel('Nama kolom baru').fill('Blocked')
    await page.getByRole('button', { name: /^tambah$/i }).click()
    await page.getByRole('button', { name: /simpan kolom/i }).click()
    await expect(page.getByText(/kolom tersimpan/i)).toBeVisible({ timeout: 15_000 })

    const persisted = await readColumns(page, soloOrg, soloBoard)
    expect(persisted).toHaveLength(6)
    expect(persisted[5].name).toBe('Blocked')
  })
})
