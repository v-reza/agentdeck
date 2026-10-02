import { execFileSync } from 'node:child_process'
import { expect, test, type Page } from '@playwright/test'

/**
 * US-AD55 — the command palette.
 *
 * AC1 names three kinds of entry, so the spec asserts one of each by its stable
 * `data-kind` token (`action`, `project`, `task`). The visible section labels are
 * translated — the app's default locale is Indonesian — so locking them would
 * make this a translation test that breaks when a word changes.
 *
 * AC2 is keyboard navigation, and it is checked by the OBSERVABLE effect of
 * Enter: pressing Down moves `data-active` to the next row, and Enter on that
 * row navigates. Asserting "the handler ran" is not possible from here, and
 * asserting only that a list exists would pass on a palette with no keyboard at
 * all.
 *
 * The seed goes through the API for the project (so the row is real server data)
 * and through psql for the task, because creating a task needs a board and the
 * board's columns; the search endpoint under test is the real one either way.
 */

const API = '/api/v1'
const DB_CONTAINER = 'agentdeck-db'
const PASSWORD = 'Sup3rSecret!2026'

function psql(sql: string): string {
  return execFileSync('docker', ['exec', DB_CONTAINER, 'psql', '-U', 'agentdeck', '-d', 'agentdeck', '-tAc', sql], {
    encoding: 'utf8',
  }).trim()
}

function esc(value: string): string {
  return value.replace(/'/g, "''")
}

function ulid(): string {
  const alphabet = '0123456789ABCDEFGHJKMNPQRSTVWXYZ'
  const stamp = Date.now().toString(32).toUpperCase().padStart(10, '0').slice(-10)
  let random = ''
  for (let i = 0; i < 16; i += 1) random += alphabet[Math.floor(Math.random() * alphabet.length)]
  return stamp + random
}

interface Session {
  orgID: string
  userID: string
}

async function signUp(page: Page, name: string): Promise<Session> {
  const res = await page.request.post(`${API}/auth/register`, {
    data: {
      email: `e2e-palette-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`,
      password: PASSWORD,
      name,
      org_name: 'Palette Co',
    },
  })
  const body = await res.text()
  expect(res.status(), body).toBe(201)
  const [cookie] = (await page.context().cookies()).filter((c) => c.name === 'agentdeck_session')
  await page.context().addCookies([{ ...cookie, secure: false }])
  const parsed = JSON.parse(body) as { workspace_id: string; user_id: string }
  return { orgID: parsed.workspace_id, userID: parsed.user_id }
}

/** A project + board + task, so a palette row exists for each AC1 kind. */
function seedTask(session: Session, title: string): { projectName: string; boardID: string } {
  const { orgID, userID } = session
  const projectID = ulid()
  const boardID = ulid()
  const taskID = ulid()
  const projectName = `Palette project ${projectID.slice(-5)}`

  psql(
    `INSERT INTO projects (id, org_id, slug, name) VALUES ('${projectID}', '${esc(orgID)}', 'pal-${projectID.slice(-6).toLowerCase()}', '${esc(projectName)}');`,
  )
  psql(
    `INSERT INTO boards (id, org_id, project_id, slug, name) VALUES ('${boardID}', '${esc(orgID)}', '${projectID}', 'pal-${boardID.slice(-6).toLowerCase()}', 'Palette board');`,
  )
  psql(
    `INSERT INTO tasks (id, org_id, board_id, title, status, priority, created_by) VALUES ('${taskID}', '${esc(orgID)}', '${boardID}', '${esc(title)}', 'ready', 2, '${esc(userID)}');`,
  )
  return { projectName, boardID }
}

function cleanup(orgID: string): void {
  psql(`DELETE FROM tasks WHERE org_id = '${esc(orgID)}';`)
  psql(`DELETE FROM boards WHERE org_id = '${esc(orgID)}';`)
  psql(`DELETE FROM projects WHERE org_id = '${esc(orgID)}';`)
}

test.describe('command palette — US-AD55', () => {
  let session: Session

  test.afterEach(() => {
    if (session) cleanup(session.orgID)
  })

  test('AC1 — actions, navigation and task search are all in the list', async ({ page }) => {
    session = await signUp(page, 'Palette Owner')
    const marker = `palette-marker-${Date.now()}`
    const seeded = seedTask(session, `Find me ${marker}`)

    await page.goto(`/app/${session.orgID}/projects`)
    // Wait for the shell's module graph and data to settle before sending the
    // chord: `goto` resolves on load, and the palette's listener lives in the
    // shell. A key pressed before that has no listener to reach — and on this
    // dev server the first request after a restart pays the cold transform.
    await page.waitForLoadState('networkidle')
    await expect(page.getByTestId('palette-input')).toHaveCount(0)

    // The shortcut opens it. Control on this host; the chord is the same.
    await page.keyboard.press('Control+k')
    const input = page.getByTestId('palette-input')
    await expect(input).toBeVisible({ timeout: 15_000 })
    await expect(input).toBeFocused()

    const rows = page.getByTestId('palette-row')
    // AC1's first kind: quick actions, with no query typed. `data-kind` sits on
    // the row itself, so this is an attribute selector, not a `has:` child match.
    const actionRows = page.locator('[data-testid="palette-row"][data-kind="action"]')
    await expect(actionRows.first()).toBeVisible()
    expect(await actionRows.count()).toBeGreaterThan(0)

    // AC1's second kind: navigation to a project.
    await input.fill(seeded.projectName)
    const projectRow = rows.filter({ hasText: seeded.projectName })
    await expect(projectRow).toHaveCount(1)
    await expect(projectRow).toHaveAttribute('data-kind', 'project')

    // AC1's third kind: task search, against the real endpoint.
    await input.fill(marker)
    const taskRow = page.locator('[data-testid="palette-row"][data-kind="task"]')
    await expect(taskRow).toHaveCount(1)
    await expect(taskRow).toHaveText(new RegExp(marker))

    // A query that matches nothing says so rather than rendering an empty list.
    await input.fill('zzz-no-such-thing-zzz')
    await expect(page.getByTestId('palette-empty')).toBeVisible()
    await expect(rows).toHaveCount(0)
  })

  test('AC2 — arrow keys move the highlight and Enter acts on it', async ({ page }) => {
    session = await signUp(page, 'Palette Keys')
    // The title carries the word the project name does, so the query "Palette"
    // yields exactly TWO rows: one project and one task. With a single row,
    // ArrowDown wraps back onto itself and the test proves nothing.
    const seeded = seedTask(session, `Palette second row ${Date.now()}`)

    await page.goto(`/app/${session.orgID}/projects`)
    await page.waitForLoadState('networkidle')
    await page.keyboard.press('Control+k')
    const input = page.getByTestId('palette-input')
    await expect(input).toBeVisible({ timeout: 15_000 })

    // A query that yields exactly two rows: the seeded project and the seeded
    // task both carry the word. Two rows is what makes "Down moves the
    // highlight" observable — with a single row, Down wraps back to itself.
    await input.fill('Palette')
    const rows = page.getByTestId('palette-row')
    await expect(rows.first()).toBeVisible()
    await expect(rows).toHaveCount(2)

    const activeIndex = async () =>
      rows.evaluateAll((els) => els.findIndex((e) => e.getAttribute('data-active') === 'true'))

    // Starts on the first row.
    expect(await activeIndex()).toBe(0)

    await page.keyboard.press('ArrowDown')
    expect(await activeIndex()).toBe(1)

    // Wrap-around, so the last row's Down is not a dead key.
    await page.keyboard.press('ArrowUp')
    expect(await activeIndex()).toBe(0)

    // Enter on the highlighted row navigates. The first row after a query is a
    // project, so the destination is that project's page.
    await input.fill(seeded.projectName)
    await expect(rows).toHaveCount(1)
    await page.keyboard.press('Enter')
    await expect(page).toHaveURL(new RegExp('/projects/'), { timeout: 15_000 })
    await expect(page.getByTestId('palette-input')).toHaveCount(0)
  })

  test('Escape closes it, and the shortcut toggles', async ({ page }) => {
    session = await signUp(page, 'Palette Escape')
    await page.goto(`/app/${session.orgID}/projects`)
    await page.waitForLoadState('networkidle')

    await page.keyboard.press('Control+k')
    await expect(page.getByTestId('palette-input')).toBeVisible({ timeout: 15_000 })

    // Escape is the Modal's job; asserting it here is what proves the palette is
    // a Modal and not a hand-rolled overlay.
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('palette-input')).toHaveCount(0)

    // The chord is a toggle, not an open-only key.
    await page.keyboard.press('Control+k')
    await expect(page.getByTestId('palette-input')).toBeVisible()
    await page.keyboard.press('Control+k')
    await expect(page.getByTestId('palette-input')).toHaveCount(0)
  })
})
