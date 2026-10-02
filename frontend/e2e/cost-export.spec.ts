import { execFileSync } from 'node:child_process'
import { expect, test, type Page } from '@playwright/test'

/**
 * US-AD56 — the cost CSV export.
 *
 * The file is the artifact, so the spec downloads it and reads it back. Asserting
 * "the button was clicked" would pass on an export that wrote an empty file.
 *
 * AC1's column list is checked by header AND by position, because a CSV whose
 * columns are all present but in a different order is a different file to every
 * consumer that indexes by column.
 *
 * The rows are seeded into `ledger_entries` the same way `CreateLedgerEntry`
 * writes them, so the API under test is the real one. The `runs` row is not
 * decoration: `agent_name` only has a value because of the join, so a seed that
 * skipped it would let a broken join pass.
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
      email: `e2e-export-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`,
      password: PASSWORD,
      name,
      org_name: 'Export Co',
    },
  })
  const body = await res.text()
  expect(res.status(), body).toBe(201)
  const [cookie] = (await page.context().cookies()).filter((c) => c.name === 'agentdeck_session')
  await page.context().addCookies([{ ...cookie, secure: false }])
  const parsed = JSON.parse(body) as { workspace_id: string; user_id: string }
  return { orgID: parsed.workspace_id, userID: parsed.user_id }
}

interface SeedOptions {
  daysAgo: number
  model: string
  agentName: string
  costMicros: number
  tokensIn: number
  tokensOut: number
}

function seedEntry(session: Session, options: SeedOptions): { taskID: string; runID: string } {
  const { orgID, userID } = session
  const projectID = ulid()
  const boardID = ulid()
  const agentID = ulid()
  const taskID = ulid()
  const runID = ulid()

  psql(
    `INSERT INTO projects (id, org_id, slug, name) VALUES ('${projectID}', '${esc(orgID)}', 'exp-${projectID.slice(-6).toLowerCase()}', 'Export project');`,
  )
  psql(
    `INSERT INTO boards (id, org_id, project_id, slug, name) VALUES ('${boardID}', '${esc(orgID)}', '${projectID}', 'exp-${boardID.slice(-6).toLowerCase()}', 'Export board');`,
  )
  psql(
    `INSERT INTO agents (id, org_id, project_id, name, provider, model) VALUES ('${agentID}', '${esc(orgID)}', '${projectID}', '${esc(options.agentName)}', 'openai', '${esc(options.model)}');`,
  )
  psql(
    `INSERT INTO tasks (id, org_id, board_id, title, status, priority, created_by) VALUES ('${taskID}', '${esc(orgID)}', '${boardID}', 'Export seed', 'done', 2, '${esc(userID)}');`,
  )
  psql(
    `INSERT INTO runs (id, org_id, task_id, agent_id, status) VALUES ('${runID}', '${esc(orgID)}', '${taskID}', '${agentID}', 'ended');`,
  )
  psql(
    `INSERT INTO ledger_entries (org_id, run_id, task_id, provider, model, kind, tokens_in, tokens_out, cache_read_tokens, cache_write_tokens, reasoning_tokens, cost_micros, price_version, price_source, pricing_model, created_at) ` +
      `VALUES ('${esc(orgID)}', '${runID}', '${taskID}', 'openai', '${esc(options.model)}', 'llm', ${options.tokensIn}, ${options.tokensOut}, 0, 0, 0, ${options.costMicros}, 4, 'catalog', '${esc(options.model)}', now() - interval '${options.daysAgo} days');`,
  )
  return { taskID, runID }
}

function cleanup(orgID: string): void {
  psql(`DELETE FROM ledger_entries WHERE org_id = '${esc(orgID)}';`)
  psql(`DELETE FROM runs WHERE org_id = '${esc(orgID)}';`)
  psql(`DELETE FROM tasks WHERE org_id = '${esc(orgID)}';`)
  psql(`DELETE FROM agents WHERE org_id = '${esc(orgID)}';`)
  psql(`DELETE FROM boards WHERE org_id = '${esc(orgID)}';`)
  psql(`DELETE FROM projects WHERE org_id = '${esc(orgID)}';`)
}

/** The dialog's 7-day preset, so the seed can be placed inside and outside it. */
function daysAgoDate(days: number): string {
  const d = new Date()
  d.setDate(d.getDate() - days)
  return d.toISOString().slice(0, 10)
}

test.describe('cost CSV export — US-AD56', () => {
  let session: Session

  test.afterEach(() => {
    if (session) cleanup(session.orgID)
  })

  test('AC1 — the file carries the seven documented columns, in order', async ({ page }) => {
    session = await signUp(page, 'Export Owner')
    const seeded = seedEntry(session, {
      daysAgo: 0,
      model: 'gpt-4o',
      agentName: 'agent-backend',
      costMicros: 2840,
      tokensIn: 1420,
      tokensOut: 184,
    })

    await page.goto(`/app/${session.orgID}/cost/ledger`)
    await expect(page.getByTestId('ledger-export')).toBeVisible({ timeout: 15_000 })
    await page.getByTestId('ledger-export').click()

    // The dialog says what it is about to write before it writes it.
    await expect(page.getByTestId('cost-export-count')).toHaveText('1', { timeout: 15_000 })

    const [download] = await Promise.all([
      page.waitForEvent('download'),
      page.getByTestId('cost-export-download').click(),
    ])
    const stream = await download.createReadStream()
    let text = ''
    for await (const chunk of stream) text += chunk.toString('utf8')

    // AC1's list, in AC1's order. Run ID is on the list; the design's own
    // "Format Kolom" line omits it, and the AC wins.
    const lines = text
      .replace(/^\uFEFF/, '')
      .trim()
      .split('\r\n')
    expect(lines[0]).toBe('"time","task_id","run_id","agent","model","tokens_in","tokens_out","cost_micros"')
    expect(lines).toHaveLength(2)

    const cells = lines[1].split(',')
    expect(cells[1]).toBe(`"${seeded.taskID}"`)
    expect(cells[2]).toBe(`"${seeded.runID}"`)
    // The agent name is the JOIN's doing; a broken join writes an empty cell.
    expect(cells[3]).toBe('"agent-backend"')
    expect(cells[4]).toBe('"gpt-4o"')
    expect(cells[5]).toBe('"1420"')
    expect(cells[6]).toBe('"184"')
    // Cost is the integer micro-USD, never a rounded float: 2840 micros is
    // $0.00284, and writing `0.00` would lose the only precision the ledger has.
    expect(cells[7]).toBe('"2840"')
  })

  test('AC2 — the date range decides which rows are in the file', async ({ page }) => {
    session = await signUp(page, 'Export Range')
    const recent = seedEntry(session, {
      daysAgo: 0,
      model: 'gpt-4o',
      agentName: 'agent-recent',
      costMicros: 1000,
      tokensIn: 10,
      tokensOut: 20,
    })
    const old = seedEntry(session, {
      daysAgo: 40,
      model: 'deepseek-coder',
      agentName: 'agent-old',
      costMicros: 2000,
      tokensIn: 30,
      tokensOut: 40,
    })

    await page.goto(`/app/${session.orgID}/cost/ledger`)
    await expect(page.getByTestId('ledger-export')).toBeVisible({ timeout: 15_000 })
    await page.getByTestId('ledger-export').click()

    // The default preset is 7 days, so the 40-day-old row is already excluded —
    // and the count the dialog shows is the API's total for that range, not the
    // number of rows on the explorer's page.
    await expect(page.getByTestId('cost-export-count')).toHaveText('1', { timeout: 15_000 })

    const [download] = await Promise.all([
      page.waitForEvent('download'),
      page.getByTestId('cost-export-download').click(),
    ])
    const stream = await download.createReadStream()
    let text = ''
    for await (const chunk of stream) text += chunk.toString('utf8')
    expect(text).toContain(recent.taskID)
    expect(text).not.toContain(old.taskID)

    // Now widen the range to a custom window that includes both.
    await page.getByTestId('ledger-export').click()
    await page.getByTestId('cost-export-range-custom').click()
    await page.getByTestId('cost-export-from').fill(daysAgoDate(60))
    await page.getByTestId('cost-export-to').fill(daysAgoDate(0))
    await expect(page.getByTestId('cost-export-count')).toHaveText('2', { timeout: 15_000 })

    const [wide] = await Promise.all([page.waitForEvent('download'), page.getByTestId('cost-export-download').click()])
    const wideStream = await wide.createReadStream()
    let wideText = ''
    for await (const chunk of wideStream) wideText += chunk.toString('utf8')
    expect(wideText).toContain(recent.taskID)
    expect(wideText).toContain(old.taskID)
  })

  test('the empty range offers no download, rather than an empty file', async ({ page }) => {
    session = await signUp(page, 'Export Empty')

    await page.goto(`/app/${session.orgID}/cost/ledger`)
    await expect(page.getByTestId('ledger-export')).toBeVisible({ timeout: 15_000 })
    await page.getByTestId('ledger-export').click()

    await expect(page.getByTestId('cost-export-count')).toHaveText('0', { timeout: 15_000 })
    await expect(page.getByTestId('cost-export-download')).toBeDisabled()
  })
})
