import { execFileSync } from 'node:child_process'
import { expect, test, type APIRequestContext, type Page } from '@playwright/test'

/**
 * US-AD95 — the audit log viewer.
 *
 * The rows are seeded straight into Postgres. The screen's job is to READ
 * `audit_log` correctly — paginate it by cursor, filter it by actor/action/date,
 * and show before/after — and driving a real mutation would make every assertion
 * depend on which endpoint happened to write the row. The seed writes the shape
 * `RecordAudit` writes, and the API under test is the real one.
 *
 * AC4 is why the second case uses a SECOND browser context. "member and viewer get
 * a 403 at the page level" cannot be observed by an owner; a member session has to
 * be created and asked. The member is invited through `POST /orgs/{id}/members`,
 * so the role comes from the real membership path rather than from a hand-written
 * `memberships` row.
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

interface Session {
  orgID: string
  userID: string
  email: string
}

async function signUp(page: Page, name: string): Promise<Session> {
  const email = `e2e-audit-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
  const res = await page.request.post(`${API}/auth/register`, {
    data: { email, password: PASSWORD, name, org_name: 'Audit Co' },
  })
  const body = await res.text()
  expect(res.status(), body).toBe(201)
  const [cookie] = (await page.context().cookies()).filter((c) => c.name === 'agentdeck_session')
  expect(cookie, 'registration did not set a session cookie').toBeTruthy()
  await page.context().addCookies([{ ...cookie, secure: false }])
  const parsed = JSON.parse(body) as { workspace_id: string; user_id: string }
  return { orgID: parsed.workspace_id, userID: parsed.user_id, email }
}

/**
 * Invite an existing user into the workspace at a given role.
 *
 * The invitee must already exist: the endpoint takes an email and adds a
 * membership, so a member context has to register first and then be invited.
 * The cookie is stripped off the request context afterwards — the invite is a
 * request the OWNER makes, and a request context carries cookies.
 */
async function invite(page: Page, orgID: string, email: string, role: string): Promise<void> {
  const res = await page.request.post(`${API}/orgs/${orgID}/members`, { data: { email, role } })
  const body = await res.text()
  expect(res.status(), body).toBeLessThan(300)
  await page.request.storageState({ path: undefined })
  const [cookie] = await page.context().cookies()
  await page.context().clearCookies()
  await page.context().addCookies([cookie])
}

/**
 * Register a user who is NOT the page's user.
 *
 * This goes through a bare APIRequestContext, not `page.request`. A page's
 * request context shares the page's cookie jar, so registering here would
 * replace the owner's session cookie with the newcomer's and every later
 * assertion would run as the wrong user — which is exactly what the first
 * version of this file did.
 */
async function registerBare(context: APIRequestContext, email: string): Promise<string> {
  const res = await context.post(`${API}/auth/register`, {
    data: { email, password: PASSWORD, name: 'Audit Member', org_name: 'Throwaway Co' },
  })
  expect(res.status(), await res.text()).toBe(201)
  const parsed = JSON.parse(await res.text()) as { user_id: string }
  return parsed.user_id
}

interface SeedOptions {
  actorUserID?: string
  actorAgentID?: string
  action: string
  targetType: string
  targetID: string
  before?: string
  after?: string
  ip?: string
  /** Minutes back from now, so the cursor order and the date filter are known. */
  minutesAgo?: number
}

function seedAudit(session: Session, options: SeedOptions): void {
  const before = options.before ? `'${esc(options.before)}'::jsonb` : 'NULL'
  const after = options.after ? `'${esc(options.after)}'::jsonb` : 'NULL'
  const agent = options.actorAgentID ? `'${esc(options.actorAgentID)}'` : 'NULL'
  const user = options.actorUserID ? `'${esc(options.actorUserID)}'` : 'NULL'
  psql(
    `INSERT INTO audit_log (org_id, actor_user_id, actor_agent_id, action, target_type, target_id, before_json, after_json, ip, created_at) ` +
      `VALUES ('${esc(session.orgID)}', ${user}, ${agent}, '${esc(options.action)}', ` +
      `'${esc(options.targetType)}', '${esc(options.targetID)}', ${before}, ${after}, '${esc(options.ip ?? '10.0.0.9')}', ` +
      `now() - interval '${options.minutesAgo ?? 0} minutes');`,
  )
}

function cleanup(orgID: string): void {
  psql(`DELETE FROM audit_log WHERE org_id = '${esc(orgID)}';`)
}

test.describe('audit log — US-AD95', () => {
  let session: Session

  test.afterEach(() => {
    if (session) cleanup(session.orgID)
  })

  test('AC1 — every documented column, including before and after', async ({ page }) => {
    session = await signUp(page, 'Audit Owner')
    seedAudit(session, {
      actorUserID: session.userID,
      action: 'workspace.rename',
      targetType: 'org',
      targetID: session.orgID,
      before: '{"name":"Old name"}',
      after: '{"name":"New name"}',
      minutesAgo: 20,
    })
    // An agent actor: the column exists for it, so a row is seeded with only that
    // side filled. Rendering an empty actor here would hide the distinction.
    seedAudit(session, {
      actorAgentID: 'agent-runner',
      action: 'task.created',
      targetType: 'task',
      targetID: 'task-abc',
      after: '{"title":"Ship it"}',
      minutesAgo: 2,
    })

    await page.goto(`/app/${session.orgID}/audit`)
    const table = page.getByTestId('audit-table')
    await expect(table).toBeVisible({ timeout: 15_000 })

    // Asserted by testid, not by header text: the default locale is Indonesian,
    // so locking English copy would turn this into a translation test.
    for (const column of ['time', 'actor', 'action', 'target', 'diff']) {
      await expect(table.getByTestId(`audit-col-${column}`)).toBeVisible()
    }

    const rows = table.getByTestId('audit-row')
    await expect(rows).toHaveCount(2)

    // Newest first, and the action name is the stored one — not a prettified
    // label. A screen that translated `workspace.rename` into prose would make
    // the audit trail unusable for grep.
    await expect(rows.nth(0).getByTestId('audit-action')).toHaveText('task.created')
    await expect(rows.nth(1).getByTestId('audit-action')).toHaveText('workspace.rename')

    // The two actor kinds are distinguishable. `audit_log` has a column for
    // each and exactly one is set, so a row with only `actor_agent_id` must
    // render as an agent — a screen that fell back to `actor_user_id` would show
    // "unknown actor" here and this is the only place that would notice.
    await expect(rows.nth(1).getByTestId('audit-actor')).toHaveAttribute('data-actor', session.userID)
    await expect(rows.nth(0).getByTestId('audit-actor')).toHaveAttribute('data-actor', 'agent-runner')
    await expect(rows.nth(0).getByTestId('audit-actor')).toContainText('agent-runner')

    // AC1's before/after, both on screen.
    await expect(table.getByText('Old name')).toBeVisible()
    await expect(table.getByText('New name')).toBeVisible()
  })

  test('AC2 — actor, action and date filters narrow the table', async ({ page, request }) => {
    session = await signUp(page, 'Audit Filter')
    // `request` is its own isolated cookie jar, so registering here cannot
    // touch the owner's session on `page`.
    const otherID = await registerBare(request, `e2e-audit-other-${Date.now()}@example.com`)

    seedAudit(session, {
      actorUserID: session.userID,
      action: 'workspace.rename',
      targetType: 'org',
      targetID: 'org-1',
      minutesAgo: 1,
    })
    seedAudit(session, {
      actorUserID: otherID,
      action: 'api_key.created',
      targetType: 'api_key',
      targetID: 'key-1',
      minutesAgo: 2,
    })
    // 40 days old, so the date filter has something to exclude.
    seedAudit(session, {
      actorUserID: session.userID,
      action: 'member.invited',
      targetType: 'member',
      targetID: 'member-1',
      minutesAgo: 60 * 24 * 40,
    })

    await page.goto(`/app/${session.orgID}/audit`)
    await expect(page.getByTestId('audit-table')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('audit-row')).toHaveCount(3)

    // The action filter is a free-text field because the API takes an exact
    // action name and the catalogue of names is not exposed anywhere.
    await page.getByTestId('audit-action-input').fill('api_key.created')
    await expect(page.getByTestId('audit-row')).toHaveCount(1)
    await expect(page.getByTestId('audit-row').getByTestId('audit-action')).toHaveText('api_key.created')

    await page.getByTestId('audit-action-input').fill('')
    await expect(page.getByTestId('audit-row')).toHaveCount(3)

    // AC2's date range. `from` alone excludes the 40-day-old row; the API parses
    // RFC3339, and the input is a date, so the screen has to widen it to a range.
    const recent = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString().slice(0, 10)
    await page.getByTestId('audit-from').fill(recent)
    await expect(page.getByTestId('audit-row')).toHaveCount(2)

    // And the actor filter narrows to one of those two.
    await page.getByTestId('audit-actor-input').fill(session.userID)
    await expect(page.getByTestId('audit-row')).toHaveCount(1)
    await expect(page.getByTestId('audit-row').getByTestId('audit-action')).toHaveText('workspace.rename')
  })

  test('AC3 — an empty range says so instead of rendering an empty table', async ({ page }) => {
    session = await signUp(page, 'Audit Empty')
    seedAudit(session, {
      actorUserID: session.userID,
      action: 'workspace.rename',
      targetType: 'org',
      targetID: 'org-1',
      minutesAgo: 1,
    })

    await page.goto(`/app/${session.orgID}/audit`)
    await expect(page.getByTestId('audit-table')).toBeVisible({ timeout: 15_000 })

    // A range nothing falls into. The empty state is its own element, so a table
    // that merely rendered zero rows would not satisfy this.
    await page.getByTestId('audit-from').fill('2020-01-01')
    await page.getByTestId('audit-to').fill('2020-01-31')
    await expect(page.getByTestId('audit-empty')).toBeVisible()
    await expect(page.getByTestId('audit-row')).toHaveCount(0)
  })

  test('AC4 — a member is refused at the page, not only at the endpoint', async ({ page, browser, request }) => {
    session = await signUp(page, 'Audit Gate')
    seedAudit(session, {
      actorUserID: session.userID,
      action: 'workspace.rename',
      targetType: 'org',
      targetID: 'org-1',
      minutesAgo: 1,
    })

    const memberEmail = `e2e-audit-member-${Date.now()}@example.com`
    await registerBare(request, memberEmail)
    await invite(page, session.orgID, memberEmail, 'member')

    // The member signs in with their own session, in their own context.
    const memberContext = await browser.newContext()
    const memberPage = await memberContext.newPage()
    try {
      const res = await memberPage.request.post(`${API}/auth/login`, {
        data: { email: memberEmail, password: PASSWORD },
      })
      expect(res.status(), await res.text()).toBe(200)
      const [cookie] = (await memberPage.context().cookies()).filter((c) => c.name === 'agentdeck_session')
      await memberPage.context().addCookies([{ ...cookie, secure: false }])

      await memberPage.goto(`/app/${session.orgID}/audit`)

      // AC4's server half, with the tenant named EXPLICITLY.
      //
      // The member holds two workspaces — the throwaway one registration made
      // them owner of, and the invite — so an unqualified call resolves against
      // whichever the shell has selected, and right after `goto` that is a race
      // with `setActiveOrg`. Naming the workspace removes the race and tests the
      // rule that matters: a member is refused on THIS workspace.
      const api = await memberPage.evaluate(async (orgID) => {
        const response = await fetch('/api/v1/audit-log', {
          credentials: 'include',
          headers: { 'X-Org-ID': orgID },
        })
        return response.status
      }, session.orgID)
      expect(api, 'the audit endpoint must refuse a member').toBe(403)

      // And so does the page, without rendering the table.
      await expect(memberPage.getByTestId('audit-forbidden')).toBeVisible({ timeout: 15_000 })
      await expect(memberPage.getByTestId('audit-table')).toHaveCount(0)
    } finally {
      await memberContext.close()
    }

    // The owner still sees it: the gate is on the role, not on the workspace.
    await page.goto(`/app/${session.orgID}/audit`)
    await expect(page.getByTestId('audit-table')).toBeVisible({ timeout: 15_000 })
  })
})
