import { execFileSync } from 'node:child_process'
import { expect, test, type Page } from '@playwright/test'

/**
 * US-AD27 AC4 + AC5 — the cost ledger explorer.
 *
 * The rows are seeded straight into Postgres rather than produced by running a
 * worker. That is deliberate: the screen's job is to READ the ledger correctly —
 * filter it, total it, and say so when a range is empty — and a real run would
 * make the assertions depend on a model call, a price row, and a dispatcher tick
 * to test a table. The seed writes the same shape `CreateLedgerEntry` writes.
 *
 * The `runs` row is not decoration: `agent_id` lives on `runs`, not on
 * `ledger_entries`, so the Agent column only has a value because of the join. A
 * seed that skipped it would pass while the join was broken.
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

/** A ULID-shaped id: the schema constrains these columns to 26 characters. */
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
      email: `e2e-ledger-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`,
      password: PASSWORD,
      name,
      org_name: 'Ledger Co',
    },
  })
  const body = await res.text()
  expect(res.status(), body).toBe(201)
  const [session] = (await page.context().cookies()).filter((c) => c.name === 'agentdeck_session')
  expect(session, 'registration did not set a session cookie').toBeTruthy()
  await page.context().addCookies([{ ...session, secure: false }])
  const parsed = JSON.parse(body) as { workspace_id: string; user_id: string }
  return { orgID: parsed.workspace_id, userID: parsed.user_id }
}

interface SeedOptions {
  /** Days back from now, so a date filter has something to include and exclude. */
  daysAgo: number
  model: string
  /** The `agents.name` the row joins to; this is what the Agent column shows. */
  agentName: string
  costMicros: number
  tokensIn: number
  tokensOut: number
  /** Minutes back from now; lets a test stack rows in a known order. */
  minutesAgo?: number
}

/**
 * One agent, one task, one run and one ledger entry, wired to each other.
 *
 * The `agents` row is created via psql rather than the API because the API's
 * agent form wants a provider and a model catalogue; the ledger only needs the
 * `runs.agent_id` foreign key to resolve.
 */
function seedEntry(session: Session, options: SeedOptions): void {
  const { orgID, userID } = session
  const projectID = ulid()
  const boardID = ulid()
  const agentID = ulid()
  const taskID = ulid()
  const runID = ulid()
  const agentName = options.agentName

  // A project and a board, because `agents.project_id` and `tasks.board_id` are
  // both NOT NULL foreign keys. They are scaffolding for the ledger row, not
  // something the screen under test reads.
  psql(
    `INSERT INTO projects (id, org_id, slug, name) ` +
      `VALUES ('${projectID}', '${esc(orgID)}', 'ledger-${projectID.slice(-6).toLowerCase()}', 'Ledger project');`,
  )
  psql(
    `INSERT INTO boards (id, org_id, project_id, slug, name) ` +
      `VALUES ('${boardID}', '${esc(orgID)}', '${projectID}', 'ledger-${boardID.slice(-6).toLowerCase()}', 'Ledger board');`,
  )
  psql(
    `INSERT INTO agents (id, org_id, project_id, name, provider, model) ` +
      `VALUES ('${agentID}', '${esc(orgID)}', '${projectID}', '${esc(agentName)}', 'openai', '${esc(options.model)}');`,
  )
  psql(
    `INSERT INTO tasks (id, org_id, board_id, title, status, priority, created_by) ` +
      `VALUES ('${taskID}', '${esc(orgID)}', '${boardID}', 'Ledger seed', 'done', 2, '${esc(userID)}');`,
  )
  psql(
    `INSERT INTO runs (id, org_id, task_id, agent_id, status) ` +
      `VALUES ('${runID}', '${esc(orgID)}', '${taskID}', '${agentID}', 'ended');`,
  )
  psql(
    `INSERT INTO ledger_entries (org_id, run_id, task_id, provider, model, kind, ` +
      `tokens_in, tokens_out, cache_read_tokens, cache_write_tokens, reasoning_tokens, ` +
      `cost_micros, price_version, price_source, pricing_model, created_at) ` +
      `VALUES ('${esc(orgID)}', '${runID}', '${taskID}', 'openai', '${esc(options.model)}', 'llm', ` +
      `${options.tokensIn}, ${options.tokensOut}, 0, 0, 0, ` +
      `${options.costMicros}, 4, 'catalog', '${esc(options.model)}', ` +
      `now() - interval '${options.daysAgo} days' - interval '${options.minutesAgo ?? 0} minutes');`,
  )
}

function cleanup(orgID: string): void {
  psql(`DELETE FROM ledger_entries WHERE org_id = '${esc(orgID)}';`)
  psql(`DELETE FROM runs WHERE org_id = '${esc(orgID)}';`)
  psql(`DELETE FROM tasks WHERE org_id = '${esc(orgID)}';`)
  psql(`DELETE FROM agents WHERE org_id = '${esc(orgID)}';`)
  psql(`DELETE FROM boards WHERE org_id = '${esc(orgID)}';`)
  psql(`DELETE FROM projects WHERE org_id = '${esc(orgID)}';`)
}

test.describe('ledger explorer — US-AD27', () => {
  let session: Session

  test.afterEach(() => {
    if (session) cleanup(session.orgID)
  })

  test('AC4 — the table carries every documented column, with tabular figures', async ({ page }) => {
    session = await signUp(page, 'Ledger Owner')
    const orgID = session.orgID
    seedEntry(session, {
      daysAgo: 0,
      model: 'gpt-4o',
      agentName: 'agent-backend',
      costMicros: 2840,
      tokensIn: 1420,
      tokensOut: 184,
    })
    seedEntry(session, {
      daysAgo: 0,
      model: 'deepseek-coder',
      agentName: 'agent-data',
      costMicros: 58100,
      tokensIn: 14880,
      tokensOut: 4320,
    })

    await page.goto(`/app/${orgID}/cost/ledger`)
    const table = page.getByTestId('ledger-table')
    await expect(table).toBeVisible({ timeout: 15_000 })

    // AC4's column list: time, agent, model, tokens in, tokens out, cache
    // read/write, cost, price version. Asserted by testid, not by label: the
    // app's default locale is Indonesian, so locking the English header text
    // would make this test a translation test that breaks when a word changes.
    for (const column of ['time', 'agent', 'model', 'tokens-in', 'tokens-out', 'cache', 'cost', 'price-version']) {
      await expect(table.getByTestId(`ledger-col-${column}`)).toBeVisible()
    }

    const rows = table.getByTestId('ledger-row')
    await expect(rows).toHaveCount(2)

    // The agent name is the JOIN's doing, not a column on ledger_entries — a
    // broken join renders '—' here and this is the only place that would notice.
    await expect(table.getByText('agent-backend')).toBeVisible()
    await expect(table.getByText('agent-data')).toBeVisible()
    await expect(table.getByText('deepseek-coder')).toBeVisible()

    // Cost is rendered from integer micro-USD, never a float, and the formatter's
    // two tiers are both on screen here: a sub-cent amount keeps the ledger's own
    // micro precision (2840 -> `$0.00284`) while a whole-cent amount uses the two
    // decimals the PRD pins (58100 -> `$0.06`). Asserting both is what stops a
    // future "simplify to two decimals" from silently hiding sub-cent spend.
    await expect(table.getByText('$0.00284')).toBeVisible()
    await expect(table.getByText('$0.06')).toBeVisible()

    // AC4 asks for monospace with TABULAR figures. The class is the contract:
    // without `tabular-nums` the digits are proportional and the column jitters.
    const numericCell = table.getByText('14880')
    await expect(numericCell).toBeVisible()
    await expect(numericCell).toHaveClass(/tabular-nums/)
  })

  test('AC4 — the totals describe the filter, not the visible page', async ({ page }) => {
    session = await signUp(page, 'Ledger Totals')
    const orgID = session.orgID
    // THREE rows against a page limit of 25 would not separate the two numbers,
    // so this test would pass even if the screen counted the visible rows. The
    // page limit is a constant, not an env knob, so the way to make the page
    // smaller than the filter is to shrink the page limit itself — see the
    // `ledger-paging` test, which is the one that actually pins the distinction.
    seedEntry(session, {
      daysAgo: 0,
      model: 'gpt-4o',
      agentName: 'agent-backend',
      costMicros: 1_000_000,
      tokensIn: 100,
      tokensOut: 10,
    })
    seedEntry(session, {
      daysAgo: 0,
      model: 'gpt-4o-mini',
      agentName: 'agent-infra',
      costMicros: 500_000,
      tokensIn: 200,
      tokensOut: 20,
    })

    await page.goto(`/app/${orgID}/cost/ledger`)
    await expect(page.getByTestId('ledger-table')).toBeVisible({ timeout: 15_000 })

    // $1.50 across the two rows, and the entry count is the FILTERED total.
    await expect(page.getByText('$1.50')).toBeVisible()
    // Same reason as the headers: the wording is locale-dependent, the numbers
    // are not.
    await expect(page.getByTestId('ledger-showing')).toHaveText(/2\D+2/)
  })

  test('AC4 — the row total and the page are different numbers once the filter outruns one page', async ({ page }) => {
    session = await signUp(page, 'Ledger Paging')
    const orgID = session.orgID
    // 30 rows, newest first. The screen asks for 25, so the page holds 25 of 30
    // and the summary must still say 30 — the number the four cards are built on.
    for (let index = 0; index < 30; index += 1) {
      seedEntry(session, {
        daysAgo: 0,
        model: 'gpt-4o',
        agentName: 'agent-backend',
        costMicros: 10_000,
        tokensIn: 100 + index,
        tokensOut: 10,
        minutesAgo: index,
      })
    }

    await page.goto(`/app/${orgID}/cost/ledger`)
    await expect(page.getByTestId('ledger-table')).toBeVisible({ timeout: 15_000 })

    // 25 rendered rows, 30 in the filter: two different numbers on purpose.
    await expect(page.getByTestId('ledger-row')).toHaveCount(25)
    await expect(page.getByTestId('ledger-showing')).toHaveText(/25\D+30/)

    // Every card reports the FILTER, not the page. 30 rows at $0.01 is $0.30,
    // and the entry card says 30 — not the 25 rows on screen.
    await expect(page.getByText('$0.30')).toBeVisible()
    await expect(page.getByTestId('ledger-card-entries')).toContainText('30')

    // Token totals come from the same window aggregate. Rows carry 100..129
    // tokens in, so the filter holds 3,435 (3.4k) while the page holds 2,800
    // (2.8k) — pinning the difference the way the entry count does.
    await expect(page.getByTestId('ledger-card-tokens-in')).toContainText('3.4k')

    // Paging reaches the rest.
    await page.getByTestId('ledger-next').click()
    await expect(page.getByTestId('ledger-row')).toHaveCount(5)
  })

  test('AC5 — an empty range gets an empty state that asks for a different range, not a bare table', async ({
    page,
  }) => {
    session = await signUp(page, 'Ledger Empty')
    const orgID = session.orgID
    seedEntry(session, {
      daysAgo: 30,
      model: 'gpt-4o',
      agentName: 'agent-backend',
      costMicros: 1000,
      tokensIn: 10,
      tokensOut: 1,
    })

    await page.goto(`/app/${orgID}/cost/ledger`)
    await expect(page.getByTestId('ledger-table')).toBeVisible({ timeout: 15_000 })

    // Ask for a range the seed is not in: the last two days.
    const today = new Date()
    const twoDaysAgo = new Date(today.getTime() - 2 * 24 * 60 * 60 * 1000)
    const iso = (d: Date) => d.toISOString().slice(0, 10)
    await page.getByTestId('ledger-from').fill(iso(twoDaysAgo))
    await page.getByTestId('ledger-to').fill(iso(today))

    // AC5: no table, an empty state, and it says what to do about it.
    await expect(page.getByTestId('ledger-empty')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('ledger-table')).toHaveCount(0)
    await expect(page.getByText(/widen the dates|lebarkan rentang/i)).toBeVisible()

    // Widening the range brings the row back — the filter is a filter, not a
    // one-way door.
    await page.getByTestId('ledger-from').fill(iso(new Date(today.getTime() - 60 * 24 * 60 * 60 * 1000)))
    await expect(page.getByTestId('ledger-table')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('ledger-row')).toHaveCount(1)
  })

  test('the date range is half-open on the end day, so a same-day range is not empty', async ({ page }) => {
    session = await signUp(page, 'Ledger Same Day')
    const orgID = session.orgID
    seedEntry(session, {
      daysAgo: 0,
      model: 'gpt-4o',
      agentName: 'agent-backend',
      costMicros: 4200,
      tokensIn: 40,
      tokensOut: 4,
    })

    await page.goto(`/app/${orgID}/cost/ledger`)
    await expect(page.getByTestId('ledger-table')).toBeVisible({ timeout: 15_000 })

    const today = new Date().toISOString().slice(0, 10)
    await page.getByTestId('ledger-from').fill(today)
    await page.getByTestId('ledger-to').fill(today)

    // Picking today-to-today means TODAY. If the handler sent `<=` on a bare
    // date it would bound at 00:00:00 and this row — created a moment ago —
    // would vanish, which reads as a bug in the ledger rather than a filter.
    await expect(page.getByTestId('ledger-row')).toHaveCount(1, { timeout: 15_000 })
  })
})
