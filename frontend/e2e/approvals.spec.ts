import { expect, test, type Page } from '@playwright/test'

/**
 * Screen 35-approval-inbox — US-AD37, the human approval gate.
 *
 * The inbox is driven from a stubbed `GET /approvals`, and the reason is worth
 * writing down: the API has **no** approvals module. `cmd/api` registers 62
 * routes — tasks, projects, orgs, providers, boards, auth, agents, agent-skills
 * — and not one of them is `/approvals`. `internal/store/queries/queries.sql`
 * has no approval query either, and no spec in this directory mentioned the
 * screen before this file. So the real endpoint cannot be seeded, and the
 * screen has to be exercised against a controlled response.
 *
 * What that buys is still real: the rendering rules (AC2's remaining time, the
 * preview shown as stored), the AC4 role gate, and the empty state are all
 * asserted on computed output. What it does NOT prove is that the wire shape
 * matches the server — there is no server to match. When the approvals module
 * lands, the stub should be replaced with a real seeded approval; until then
 * this file is the only thing holding the screen's behaviour in place.
 */

const API = '/api/v1'
const PASSWORD = 'Sup3rSecret!23'

async function signUp(page: Page, name: string): Promise<string> {
  const email = `e2e-approvals-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
  const response = await page.request.post(`${API}/auth/register`, {
    data: { email, password: PASSWORD, name },
  })
  const body = await response.text()
  expect(response.status(), body).toBe(201)
  return (JSON.parse(body) as { workspace_id: string }).workspace_id
}

async function signIn(page: Page, orgID: string) {
  await page.goto(`/app/${orgID}/projects`)
  await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible()
}

/** One pending approval as `GET /approvals` would return it. */
function approval(overrides: Record<string, unknown> = {}) {
  return {
    id: '01JBAPPROVAL00000000000001',
    org_id: 'org',
    task_id: '01JBTASK000000000000000001',
    run_id: '01JBRUN0000000000000000001',
    requested_by: 'agent-infra',
    decided_by: '',
    decision: 'pending',
    gate_mode: 'require',
    reason: 'Deletes 3 stale replicas in the staging namespace.',
    preview_json: '{"action":"drop_stale_replicas","count":3}',
    expires_at: new Date(Date.now() + 30 * 60_000).toISOString(),
    decided_at: '',
    created_at: new Date().toISOString(),
    ...overrides,
  }
}

/** Serves `list` from `GET /approvals`, everything else passes through. */
async function stubApprovals(page: Page, list: unknown[]) {
  await page.route(`**${API}/approvals`, (route) =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(list) }),
  )
}

test.describe('approval inbox — US-AD37', () => {
  let orgID: string

  test.beforeEach(async ({ page }) => {
    orgID = await signUp(page, 'E2E Approval Owner')
    await signIn(page, orgID)
  })

  // AC2: preview, remaining time, and both decision buttons on every card.
  test('AC2 — a pending approval shows its preview, its remaining time, and both buttons', async ({ page }) => {
    await stubApprovals(page, [approval()])
    await page.goto(`/app/${orgID}/approvals`)

    const card = page.getByText('Deletes 3 stale replicas in the staging namespace.')
    await expect(card).toBeVisible({ timeout: 15_000 })

    // The preview is the design's "Preview Aksi" card, and the payload inside it
    // is rendered as stored: the mockup's card header names the gated action,
    // and re-formatting the body would hide the difference between what the
    // agent sent and what the operator approves.
    const preview = page.getByTestId('approval-preview')
    await expect(preview).toBeVisible()
    await expect(preview).toContainText(/preview aksi berisiko/i)
    // The header label is derived from the payload's own `action` key.
    await expect(preview).toContainText('action: drop_stale_replicas')
    // Verbatim means byte-for-byte: the fixture is single-line JSON, so a
    // pretty-printer would introduce a newline right after the opening brace.
    const raw = await preview.locator('pre').textContent()
    expect(raw).toBe('{"action":"drop_stale_replicas","count":3}')

    // AC2's "waktu tersisa" — a deadline counting down, not a creation stamp.
    const remaining = page.getByTestId('approval-remaining')
    await expect(remaining).toBeVisible()
    await expect(remaining).toContainText(/Remaining: \d+m \d+s/)
    // Exactly 30 minutes out sits on the warning threshold, so the tone here is
    // amber (`--color-warning` #b45309). The calm tone is asserted in the tick
    // test below, which starts at 90 minutes.
    await expect(remaining).toHaveCSS('color', 'rgb(180, 83, 9)')

    await expect(page.getByRole('button', { name: /approve & run/i })).toBeVisible()
    await expect(page.getByRole('button', { name: /^reject$/i })).toBeVisible()
  })

  test('AC2 — the countdown is derived from expires_at, and an expired gate says so', async ({ page }) => {
    // Two gates, one past its deadline: the screen must tell them apart. This is
    // the assertion that fails if the chip ever goes back to rendering
    // `created_at`, because a creation stamp cannot express "expired".
    await stubApprovals(page, [
      approval({
        id: '01JBAPPROVAL00000000000002',
        reason: 'Gate that already ran out.',
        expires_at: new Date(Date.now() - 60_000).toISOString(),
      }),
    ])
    await page.goto(`/app/${orgID}/approvals`)

    const remaining = page.getByTestId('approval-remaining')
    await expect(remaining).toBeVisible({ timeout: 15_000 })
    await expect(remaining).toContainText('Expired')
    // `--color-danger` (#b91c1c), the semantic token — not the `status-failed`
    // red, which is a task-state colour and a different value.
    await expect(remaining).toHaveCSS('color', 'rgb(185, 28, 28)')
  })

  test('AC2 — the countdown ticks down while the screen is open', async ({ page }) => {
    await stubApprovals(page, [approval({ expires_at: new Date(Date.now() + 90 * 60_000).toISOString() })])
    await page.goto(`/app/${orgID}/approvals`)

    const remaining = page.getByTestId('approval-remaining')
    await expect(remaining).toContainText('Remaining: 1h 29m', { timeout: 15_000 })
    // An hour and a half out is the calm tone, not the warning one.
    await expect(remaining).toHaveCSS('color', 'rgb(13, 122, 112)')
    // A minute later it must read lower without a reload. Waiting a real minute
    // is too slow for a test, so the clock is moved instead — the component
    // re-derives from `expires_at` on each tick, so shifting the system clock
    // is equivalent to time passing.
    await page.evaluate(() => {
      const realNow = Date.now
      Date.now = () => realNow() + 120_000
    })
    await expect(remaining).toContainText('Remaining: 1h 27m', { timeout: 5_000 })
  })

  // AC1/AC2: a decided approval is not in the queue.
  test('AC1 — only pending approvals are listed', async ({ page }) => {
    await stubApprovals(page, [
      approval({ reason: 'Still waiting.' }),
      approval({
        id: '01JBAPPROVAL00000000000003',
        reason: 'Already approved.',
        decision: 'approved',
        decided_by: 'alice',
      }),
    ])
    await page.goto(`/app/${orgID}/approvals`)

    await expect(page.getByText('Still waiting.')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByText('Already approved.')).toBeHidden()
    await expect(page.getByTestId('approval-remaining')).toHaveCount(1)
  })

  test('AC1 — an empty queue explains itself instead of showing a blank page', async ({ page }) => {
    await stubApprovals(page, [])
    await page.goto(`/app/${orgID}/approvals`)

    await expect(page.getByText('Nothing waiting on you')).toBeVisible({ timeout: 15_000 })
  })

  // AC4: a viewer monitors the queue but does not get the decision buttons.
  test('AC4 — the decision buttons belong to owner/admin only', async ({ page }) => {
    await stubApprovals(page, [approval()])
    await page.goto(`/app/${orgID}/approvals`)

    // The registrant is the workspace owner, so the controls are there...
    await expect(page.getByRole('button', { name: /approve & run/i })).toBeVisible({ timeout: 15_000 })

    // ...and the role is read from `GET /auth/me` (the dashboard layout seeds
    // the session slice from it), so downgrading it there must take them away.
    // This stubs the identity response rather than inviting a real viewer: the
    // invite flow is covered by members.spec.ts, and what is under test here is
    // only what the inbox does with the role it is handed.
    await page.route(`**${API}/auth/me`, async (route) => {
      const response = await route.fetch()
      const me = (await response.json()) as { workspaces: { id: string; role: string }[] }
      for (const workspace of me.workspaces) workspace.role = 'viewer'
      await route.fulfill({ response, json: me })
    })
    await page.reload()

    await expect(page.getByText(/needs the owner or admin role/i)).toBeVisible({ timeout: 15_000 })
    await expect(page.getByRole('button', { name: /approve & run/i })).toHaveCount(0)
    await expect(page.getByRole('button', { name: /^reject$/i })).toHaveCount(0)
    // The queue itself stays visible — AC4 says viewers can monitor it.
    await expect(page.getByText('Deletes 3 stale replicas in the staging namespace.')).toBeVisible()
  })
})
