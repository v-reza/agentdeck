import { expect, test, type Page } from '@playwright/test'

/**
 * US-AD54 — the dense table view: AC1's seven columns and AC2's multi-sort.
 *
 * Both ACs are asserted against rows the test itself seeds, through the API, so
 * the expected order is known before the click. A sort test that reads whatever
 * the fixture happens to contain proves nothing.
 *
 * Accounts are registered per run (same pattern as `board-toolbar.spec.ts`), so
 * the suite can be re-run against a database that is never pruned.
 */
const API = '/api/v1'
const PASSWORD = 'correct-horse-battery-staple'

async function signUp(page: Page): Promise<string> {
  const email = `e2e-table-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
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
    { api: API, credentials: { email, password: PASSWORD, name: 'Table Owner', org_name: 'Table Co' } },
  )
  expect(result.status, result.body).toBe(201)
  return (JSON.parse(result.body) as { workspace_id: string }).workspace_id
}

interface Seeded {
  boardID: string
  /** Titles in the order the table should show them, once sorted by title asc. */
  alphabetical: string[]
}

/**
 * A board with three tasks whose titles are deliberately out of alphabetical
 * order, created in the order C, A, B — so "sorted by title" and "insertion
 * order" cannot be confused for one another.
 */
async function seed(page: Page, orgID: string): Promise<Seeded> {
  return page.evaluate(
    async ({ api, org, titles }) => {
      const post = async (url: string, body: unknown) => {
        const res = await fetch(`${api}${url}`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-Org-ID': org },
          credentials: 'include',
          body: JSON.stringify(body),
        })
        return (await res.json()) as { id: string }
      }
      const project = await post('/projects', { name: 'Table', slug: 'table' })
      const board = await post(`/projects/${project.id}/boards`, { name: 'Table Board', slug: 'table' })
      for (const title of titles) {
        await post(`/boards/${board.id}/tasks`, { title, status: 'backlog' })
      }
      return { boardID: board.id, alphabetical: [...titles].sort((a, b) => a.localeCompare(b)) }
    },
    { api: API, org: orgID, titles: ['Gamma sorting row', 'Alpha sorting row', 'Beta sorting row'] },
  )
}

test.describe('the table view (US-AD54)', () => {
  let orgID: string
  let seeded: Seeded

  test.beforeEach(async ({ page }) => {
    orgID = await signUp(page)
    seeded = await seed(page, orgID)
    await page.goto(`/app/${orgID}/boards/${seeded.boardID}/table`)
    await expect(page.getByTestId('table-sort-title')).toBeVisible()
  })

  test('AC1 — the seven required columns are rendered, in order', async ({ page }) => {
    // AC1: "32px/baris, kolom: ID, Title, Status, Priority, Agent, Cost, Created."
    // Asserted as an ordered list of header names, not as "some headers exist":
    // the table used to carry a Tokens column instead of ID/Agent/Created.
    const headers = await page
      .locator('thead th')
      .evaluateAll((cells) => cells.map((c) => c.textContent?.trim().toLowerCase() ?? ''))
    expect(headers).toHaveLength(7)
    expect(headers[0]).toMatch(/^(id)$/)
    expect(headers[1]).toMatch(/^(title|judul)$/)
    expect(headers[2]).toMatch(/^(status)$/)
    expect(headers[3]).toMatch(/^(priority|prioritas)$/)
    expect(headers[4]).toMatch(/^(agent)$/)
    expect(headers[5]).toMatch(/^(cost|biaya)$/)
    expect(headers[6]).toMatch(/^(created|dibuat)$/)

    // The rows are dense: AC1's 32px is a layout contract, so it is measured —
    // and measured the way the design builds it. The row carries `h-8` plus a
    // 1px bottom border, and `border-box` (Tailwind's default) puts the border
    // OUTSIDE the 32px, so the painted box is 34. Asserting 34 would be
    // asserting Chrome's box model, not the design; what matters is that the
    // 32px height is declared and that the row is not taller than that plus its
    // border. The bug this guards is a squeezed Title column wrapping the row to
    // 55px, which is what a missing `overflow-x-auto` produced.
    const rowHeight = await page
      .locator('tbody tr')
      .first()
      .evaluate((row) => Math.round(row.getBoundingClientRect().height))
    expect(rowHeight).toBeLessThanOrEqual(34)

    // The real anti-regression signal is the one that caught the bug: seven
    // fixed-width columns need more room than the content pane has (44 rail +
    // 224 sidebar + 264 cost rail out of 1280), so the table must keep its
    // min-width and scroll. Without that the Title column is squeezed, every
    // cell wraps, and the row grows to 55px. Measuring the table's own width
    // asserts the cause; measuring only the row height would let a taller row
    // through on a different viewport.
    const tableWidth = await page.locator('table').evaluate((el) => Math.round(el.getBoundingClientRect().width))
    expect(tableWidth).toBeGreaterThanOrEqual(900)

    // And it SCROLLS rather than overflowing the pane, which is what the design
    // draws. Two separate things, and a mutation run proved they are not the same
    // one: `min-w` is what stops the squeeze, the wrapper is what makes the
    // overflow reachable. Removing either alone leaves the other assertion green,
    // so both are asserted.
    const scrolls = await page.locator('table').evaluate((el) => getComputedStyle(el.parentElement!).overflowX)
    expect(scrolls).toBe('auto')

    // Seven cells per row: AC1's column count, per row and not just in the head.
    await expect(page.locator('tbody tr').first().locator('td')).toHaveCount(7)

    // The ID column carries the task id, and the created column a real date.
    const firstRow = page.locator('tbody tr').first()
    await expect(firstRow.locator('td').first()).toHaveText(/…?[0-9A-HJKMNP-TV-Z]{8}/)
    await expect(firstRow.locator('td').nth(6)).toHaveText(/\d/)
  })

  test('AC2 — clicking a header sorts ascending, clicking again flips to descending', async ({ page }) => {
    const titles = () => page.locator('tbody tr td:nth-child(2)').allTextContents()

    // The unsorted order is the SERVER's — `listTasks` has no ORDER BY this test
    // may rely on, so it is read rather than assumed. The sort is what has to be
    // provable, and it is compared against a permutation of the same rows: if the
    // table did nothing, the two lists would be equal and the assertion would
    // fail on that instead.
    const unsorted = await titles()
    expect([...unsorted].sort()).toEqual([...seeded.alphabetical])

    await page.getByTestId('table-sort-title').click()
    expect(await titles()).toEqual(seeded.alphabetical)

    // The header reports its state, so the flip is observable without guessing.
    await expect(page.locator('thead th').nth(1)).toHaveAttribute('aria-sort', 'ascending')
    await page.getByTestId('table-sort-title').click()
    expect(await titles()).toEqual([...seeded.alphabetical].reverse())
    await expect(page.locator('thead th').nth(1)).toHaveAttribute('aria-sort', 'descending')
  })

  test('AC2 — a second column composes with the first, and the bar shows the chain', async ({ page }) => {
    const before = await page.locator('tbody tr td:nth-child(2)').allTextContents()
    await page.getByTestId('table-sort-title').click()
    await page.getByTestId('table-sort-priority').click()

    // Two chips, in click order: the design's indicator, with its 1 and 2.
    const chips = page.getByTestId('table-sort-chip')
    await expect(chips).toHaveCount(2)
    await expect(chips.nth(0)).toHaveAttribute('data-sort-key', 'title')
    await expect(chips.nth(0)).toHaveAttribute('data-sort-order', '1')
    await expect(chips.nth(1)).toHaveAttribute('data-sort-key', 'priority')
    await expect(chips.nth(1)).toHaveAttribute('data-sort-order', '2')

    // The chain is a chain, not a replacement: the primary key still governs.
    const titles = await page.locator('tbody tr td:nth-child(2)').allTextContents()
    expect(titles).toEqual(seeded.alphabetical)

    // Reset clears the whole chain, and the bar goes with it. The rows return to
    // the server's order, which is the order read before any sort was applied.
    await page.getByTestId('table-sort-reset').click()
    await expect(page.getByTestId('table-sort-chip')).toHaveCount(0)
    expect(await page.locator('tbody tr td:nth-child(2)').allTextContents()).toEqual(before)
  })
})
