import { expect, test, type Page } from '@playwright/test'

/**
 * US-AD19 — the board's dependency graph.
 *
 * AC1 asks for two things on a card that has dependencies: a badge with the
 * count, and a thin line to the parent. Both come from ONE request per board
 * (`GET /boards/{id}/dependencies`), and both are asserted structurally — never
 * against a translated string, since the default locale is `id`.
 *
 * The three things worth guarding here, and why:
 *
 *  1. The count is per CHILD, not the board's edge total. A board with two edges
 *     into one task and one edge into another has to show 2 and 1, and a graph
 *     that renders the whole edge set on every card would pass the simplest
 *     fixture and fail here.
 *  2. A line is drawn only between two cards that are both on screen. An edge
 *     whose parent lives on another board is real (the server reports it) but has
 *     no card to attach to, so it is named on the child instead of invented as a
 *     floating node.
 *  3. `Terkunci` reads the task's own `block_kind`. It is NOT derived from having
 *     parents: a task whose parents are all `done` has edges and is not blocked.
 *
 * Plumbing: the session cookie is `Secure` while the dev servers run on plain
 * HTTP, and Playwright's `page.request` does NOT send a Secure cookie to
 * 127.0.0.1 — it answers 401 for every authenticated call. So only the register
 * call (which has to happen before a page exists) uses `page.request`; everything
 * else is an in-page `fetch`, which shares the browser's cookie handling. Same
 * shape as `column-editor.spec.ts`.
 */

const API = '/api/v1'
const PASSWORD = 'Sup3rSecret!2026'

async function signUp(page: Page, email: string, name: string): Promise<string> {
  const response = await page.request.post(`${API}/auth/register`, {
    data: { email, password: PASSWORD, name },
  })
  const body = await response.text()
  expect(response.status(), body).toBe(201)
  return (JSON.parse(body) as { workspace_id: string }).workspace_id
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

/** Land on a real app page, so the browser context holds the session cookie. */
async function signIn(page: Page, orgID: string) {
  await page.goto(`/app/${orgID}/projects`)
  await expect(page).toHaveURL(/\/app\/[^/]+\/projects$/)
}

async function createBoard(page: Page, orgID: string, label: string) {
  const org = await api<Array<{ id: string }>>(page, orgID, 'GET', '/projects')
  expect(org.status, org.text).toBe(200)

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
  return { projectID: project.data.id, boardID: board.data.id }
}

async function createTask(page: Page, orgID: string, boardID: string, title: string) {
  const res = await api<{ id: string }>(page, orgID, 'POST', `/boards/${boardID}/tasks`, { title })
  expect(res.status, res.text).toBe(201)
  return res.data.id
}

async function link(page: Page, orgID: string, childID: string, parentID: string) {
  const res = await api(page, orgID, 'POST', `/tasks/${childID}/links`, { parent_id: parentID })
  expect(res.status, res.text).toBe(201)
}

test.describe('dependency graph (US-AD19)', () => {
  test('AC1 — the badge counts a card’s own parents, and a line is drawn per on-board edge', async ({ page }) => {
    const orgID = await signUp(page, `dep-${Date.now()}@example.com`, 'Dep Co')
    await signIn(page, orgID)
    const { boardID } = await createBoard(page, orgID, 'dep')

    // Two parents into one child, and a second child with a single parent. The
    // asymmetric shape is the point: a graph that renders the board's edge TOTAL
    // on every card would pass a one-edge fixture and fail here.
    const parentA = await createTask(page, orgID, boardID, 'Parent A')
    const parentB = await createTask(page, orgID, boardID, 'Parent B')
    const childMany = await createTask(page, orgID, boardID, 'Child of both')
    const childOne = await createTask(page, orgID, boardID, 'Child of A')
    await link(page, orgID, childMany, parentA)
    await link(page, orgID, childMany, parentB)
    await link(page, orgID, childOne, parentA)

    // The API the screen reads, asserted directly: 3 edges, none invented.
    const deps = await api<{ edges: unknown[] }>(page, orgID, 'GET', `/boards/${boardID}/dependencies`)
    expect(deps.status, deps.text).toBe(200)
    expect(deps.data.edges).toHaveLength(3)

    await page.goto(`/app/${orgID}/boards/${boardID}/graph`)
    await expect(page.getByTestId('graph-card').first()).toBeVisible({ timeout: 15_000 })

    await expect(page.locator(`[data-task-id="${childMany}"]`).getByTestId('dep-badge')).toHaveText(/2/)
    await expect(page.locator(`[data-task-id="${childOne}"]`).getByTestId('dep-badge')).toHaveText(/1/)

    // The parent-less cards carry no badge at all — not a zero.
    await expect(page.locator(`[data-task-id="${parentA}"]`).getByTestId('dep-badge')).toHaveCount(0)
    await expect(page.locator(`[data-task-id="${parentB}"]`).getByTestId('dep-badge')).toHaveCount(0)

    // One line per edge whose both ends are on this board.
    await expect(page.getByTestId('graph-edge')).toHaveCount(3)
    await expect(page.locator(`[data-testid="graph-edge"][data-child="${childMany}"]`)).toHaveCount(2)
  })

  test('an edge whose parent is on another board is named, not drawn', async ({ page }) => {
    const orgID = await signUp(page, `depx-${Date.now()}@example.com`, 'Dep Cross Co')
    await signIn(page, orgID)

    const first = await createBoard(page, orgID, 'cross')
    const second = await createBoard(page, orgID, 'far')

    const child = await createTask(page, orgID, first.boardID, 'Waits on a far parent')
    const farParent = await createTask(page, orgID, second.boardID, 'Far parent task')
    await link(page, orgID, child, farParent)

    // The server reports it: scoping by the child's board is deliberate, because
    // the dispatcher will refuse to promote this task and hiding the reason would
    // make the board look stuck for no visible cause.
    const deps = await api<{ edges: Array<{ parent_title: string }> }>(
      page,
      orgID,
      'GET',
      `/boards/${first.boardID}/dependencies`,
    )
    expect(deps.data.edges).toHaveLength(1)
    expect(deps.data.edges[0].parent_title).toBe('Far parent task')

    await page.goto(`/app/${orgID}/boards/${first.boardID}/graph`)
    const card = page.locator(`[data-task-id="${child}"]`)
    await expect(card).toBeVisible({ timeout: 15_000 })
    await expect(card.getByTestId('dep-badge')).toHaveText(/1/)

    // No line: there is no card on this board to attach it to.
    await expect(page.getByTestId('graph-edge')).toHaveCount(0)
    // And it is named on the card rather than silently dropped.
    await expect(card.getByTestId('dep-offboard')).toContainText('Far parent task')
  })

  test('the locked marker follows block_kind, not the edge count', async ({ page }) => {
    const orgID = await signUp(page, `deplock-${Date.now()}@example.com`, 'Dep Lock Co')
    await signIn(page, orgID)
    const { projectID, boardID } = await createBoard(page, orgID, 'lock')

    const parent = await createTask(page, orgID, boardID, 'Blocker')
    const child = await createTask(page, orgID, boardID, 'Blocked child')
    await link(page, orgID, child, parent)

    // A task with a parent and NO block_kind: has a dependency, is not blocked.
    await page.goto(`/app/${orgID}/boards/${boardID}/graph`)
    const card = page.locator(`[data-task-id="${child}"]`)
    await expect(card).toBeVisible({ timeout: 15_000 })
    await expect(card.getByTestId('dep-badge')).toHaveText(/1/)
    await expect(card.getByTestId('dep-locked')).toHaveCount(0)

    // Now produce a real block_kind the only way the product does: a run that
    // fails with a blocking kind. `needs_input` is in the §10.2 taxonomy that
    // ends in `blocked` with its own block_kind.
    const agent = await api<{ id: string }>(page, orgID, 'POST', `/projects/${projectID}/agents`, {
      name: `dep-agent-${Date.now()}`,
      provider: 'openai_compatible',
      model: 'gpt-4o',
    })
    expect(agent.status, agent.text).toBe(201)

    for (const [path, body] of [
      [`/tasks/${child}/move`, { from: 'backlog', to: 'ready' }],
      [`/tasks/${child}/assign`, { agent_id: agent.data.id }],
    ] as Array<[string, unknown]>) {
      const res = await api(page, orgID, 'POST', path, body)
      expect(res.status, `${path}: ${res.text}`).toBeLessThan(300)
    }

    const claim = await api<{ id: string }>(page, orgID, 'POST', `/tasks/${child}/claim`)
    expect(claim.status, claim.text).toBe(201)

    const ended = await api(page, orgID, 'POST', `/runs/${claim.data.id}/end`, {
      outcome: 'failed',
      failure_kind: 'needs_input',
      error: 'needs a human',
    })
    expect(ended.status, ended.text).toBeLessThan(300)

    // The task really is blocked with a kind — the field the marker reads.
    const task = await api<{ status: string; block_kind: string }>(page, orgID, 'GET', `/tasks/${child}`)
    expect(task.data.status).toBe('blocked')
    expect(task.data.block_kind).toBe('needs_input')

    await page.reload()
    await expect(page.locator(`[data-task-id="${child}"]`).getByTestId('dep-locked')).toBeVisible({ timeout: 15_000 })
  })
})
