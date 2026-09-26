import { expect, test, type Page } from '@playwright/test'

/**
 * Screen 18-kanban — toolbar board dan modal "Buat Task Baru" (US-AD11,
 * US-AD14, §6.2.16).
 *
 * Kenapa file ini ada, dengan angka sebelum perbaikan:
 *
 *  1. Kanban test memakai toolbar yang hanya hidup di TableView, dan
 *     `CreateTaskForm` tidak diimport siapa pun. Jadi dari UI tidak ada jalan
 *     membuat task sama sekali — satu-satunya tombol yang ada ("New task")
 *     berada di file mati. Test ini menekan tombolnya.
 *  2. Form-nya tiga field (title, priority) sementara design 21-task-create
 *     menggambar modal dengan deskripsi markdown dan assignee picker, dan
 *     US-AD11 AC1 menyebut `body` — yang backend-nya sudah terima sejak awal
 *     tapi tidak bisa diketik dari mana pun. Test ini mengetik body dan
 *     memeriksa hasilnya di server.
 *  3. Filter hidup di dua tempat: TableView menyaring di klien, sementara
 *     kontrak §6.2.16 menjanjikan `status, assignee, search` di server dengan
 *     status ✅ padahal handler-nya nol query param. Test ini memakai
 *     `request` langsung supaya yang diuji jawaban API, bukan tampilan.
 *
 * Akun didaftarkan per run (pola yang sama dengan `dashboard.spec.ts`), jadi
 * suite tetap bisa dijalankan ulang di database yang tidak pernah dipangkas.
 */
const API = '/api/v1'
const PASSWORD = 'correct-horse-battery-staple'
const ULID = /[0-9A-HJKMNP-TV-Z]{26}/

async function signUp(page: Page): Promise<string> {
  const email = `e2e-toolbar-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
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
    { api: API, credentials: { email, password: PASSWORD, name: 'Toolbar Owner', org_name: 'Toolbar Co' } },
  )
  expect(result.status, result.body).toBe(201)
  return (JSON.parse(result.body) as { workspace_id: string }).workspace_id
}

test.describe('board toolbar and the create-task modal', () => {
  let orgID: string
  let boardID: string

  test.beforeEach(async ({ page }) => {
    orgID = await signUp(page)
    // A board is seeded by registration's project, but creating one here keeps
    // the test independent of that seed: only the slug rule is relied on.
    const created = await page.evaluate(
      async ({ api, body }) => {
        const projects = await fetch(`${api}/projects`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'include',
          body: JSON.stringify({ name: 'Toolbar', slug: 'toolbar' }),
        })
        const project = (await projects.json()) as { id: string }
        const boards = await fetch(`${api}/projects/${project.id}/boards`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-Org-ID': body },
          credentials: 'include',
          body: JSON.stringify({ name: 'Toolbar Board', slug: 'toolbar' }),
        })
        return (await boards.json()) as { id: string }
      },
      { api: API, body: orgID },
    )
    expect(created.id, JSON.stringify(created)).toBeTruthy()
    boardID = created.id
  })

  test('the board toolbar offers both views, search, filter and a task CTA', async ({ page }) => {
    await page.goto(`/app/${orgID}/boards/${boardID}`)
    await expect(page).toHaveURL(new RegExp(`/app/${ULID.source}/boards/${ULID.source}$`))

    // The toolbar is the design's: two view links, a search box, a status
    // filter and the accent CTA. Before this, the kanban had none of them.
    // The app defaults to Indonesian, so these are the ID strings. A regex per
    // label rather than an exact match keeps the test from breaking on the
    // locale switch, which is a different concern from the toolbar existing.
    await expect(page.getByRole('link', { name: /^(Board|Papan)$/ })).toBeVisible()
    await expect(page.getByRole('link', { name: /^(Table|Tabel)$/ })).toBeVisible()
    await expect(page.getByPlaceholder(/cari task|search tasks/i)).toBeVisible()
    // The filter is a `<details>` disclosure whose chips live inside it: the
    // element that carries the label is the `<summary>` the user clicks, and it
    // exposes no accessible name of its own (the label is a child text node),
    // so the control is matched on its text rather than on a role.
    await expect(page.locator('summary').filter({ hasText: /filter/i })).toBeVisible()
    // Opening it really reveals the per-status chips.
    await page
      .locator('summary')
      .filter({ hasText: /filter/i })
      .click()
    await expect(page.getByRole('button', { name: /^backlog$/i })).toBeVisible()
    await expect(page.getByRole('button', { name: /buat task|new task/i })).toBeVisible()
  })

  test('switching to the table view keeps the toolbar and the search box', async ({ page }) => {
    await page.goto(`/app/${orgID}/boards/${boardID}`)
    await page.getByRole('link', { name: /^(Table|Tabel)$/ }).click()
    await expect(page).toHaveURL(new RegExp(`/boards/${ULID.source}/table$`))
    await expect(page.getByPlaceholder(/cari task|search tasks/i)).toBeVisible()
    await expect(page.getByRole('button', { name: /buat task|new task/i })).toBeVisible()
  })

  test('the CTA opens the design modal, and creating a task writes title and body', async ({ page }) => {
    await page.goto(`/app/${orgID}/boards/${boardID}`)
    await page.getByRole('button', { name: /buat task|new task/i }).click()

    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()
    // The design's fields, not the old three-field inline form: a description
    // textarea (US-AD11 AC1's `body`) and the assignee picker.
    // Named by role rather than by label text: `Field` renders the hint inside
    // the label wrapper, so the title field's own accessible name contains the
    // word "deskripsi" and a label match resolves to two controls.
    await expect(dialog.getByRole('textbox', { name: /judul task|task title/i })).toBeVisible()
    await expect(dialog.getByRole('textbox', { name: /instruksi|instruction/i })).toBeVisible()

    // The design's four labelled priority levels. A free number field used to sit
    // here, which is how a board row ended up reading "7".
    await dialog.getByRole('combobox', { name: /prioritas|priority/i }).click()
    await expect(page.getByRole('option', { name: /p0 — blocker/i })).toBeVisible()
    await expect(page.getByRole('option', { name: /p3 — (low|rendah)/i })).toBeVisible()
    await page.getByRole('option', { name: /p1 — (high|tinggi)/i }).click()

    const title = `Toolbar task ${Date.now()}`
    await dialog.getByRole('textbox', { name: /judul task|task title/i }).fill(title)
    await dialog.getByRole('textbox', { name: /instruksi|instruction/i }).fill('Body typed from the modal')
    await dialog.getByRole('button', { name: /buat task|create task/i }).click()

    // The dialog closes and the task is on the board.
    await expect(dialog).toBeHidden({ timeout: 15_000 })
    await expect(page.getByText(title)).toBeVisible({ timeout: 15_000 })

    // `body` really reached the server — the field the inline form could not
    // send. Read through the API rather than the screen, because the screen
    // does not render the body in the column card.
    const stored = await page.evaluate(
      async ({ api, board }) => {
        const res = await fetch(`${api}/boards/${board}/tasks`, { credentials: 'include' })
        return (await res.json()) as { title: string; body: string; priority: number }[]
      },
      { api: API, board: boardID },
    )
    const written = stored.find((t) => t.title === title)
    expect(written, `task ${title} not in ${JSON.stringify(stored.map((t) => t.title))}`).toBeTruthy()
    expect(written!.body).toBe('Body typed from the modal')
    // The chosen level is what the API stores (`P1` is 1), not the default.
    expect(written!.priority).toBe(1)
  })

  test('the modal shows the design footer and states the initial status', async ({ page }) => {
    await page.goto(`/app/${orgID}/boards/${boardID}`)
    await page.getByRole('button', { name: /buat task|new task/i }).click()
    const dialog = page.getByRole('dialog')

    // The design's footer, which the old form did not have at all: the cancel
    // and submit pair lives below the form, and the target board is named there.
    await expect(dialog.getByRole('button', { name: /batal|cancel/i })).toBeVisible()
    await expect(dialog.getByRole('button', { name: /buat task|create task/i })).toBeVisible()
    await expect(dialog.getByText(/target:/i)).toBeVisible()

    // The initial status is stated, not offered: `ready` belongs to the
    // dispatcher (DECISIONS 3), so no control for it may be reachable here.
    await expect(dialog.getByText(/status awal|initial status/i)).toBeVisible()
    // Assignee and priority, and nothing else — a third combobox would be the
    // status list coming back.
    expect(await dialog.getByRole('combobox').count()).toBe(2)

    // The design's required marker after the required field's label.
    await expect(dialog.getByText('*')).toBeVisible()

    // The picker's unassigned row is AC5's whole point: creating a task without
    // an agent has to stay possible.
    await dialog.getByRole('combobox', { name: /assignee/i }).click()
    await expect(page.getByRole('option', { name: /tanpa agent|no agent/i })).toBeVisible()
  })

  test('the create-task card matches the design geometry', async ({ page }) => {
    await page.goto(`/app/${orgID}/boards/${boardID}`)
    await page.getByRole('button', { name: /buat task|new task/i }).click()
    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()

    // Screen 21-task-create: a 560px card at radius 14, a header band at 20x14
    // and a footer band at 20x12 filled with the page colour. Measuring the DOM
    // rather than eyeballing a screenshot — a full-page shot of a modal is
    // unreliable to read, and these four numbers are what the design states.
    const m = await dialog.evaluate((el) => {
      const px = (v: string) => Math.round(parseFloat(v))
      const card = getComputedStyle(el)
      const header = el.firstElementChild as HTMLElement
      const footer = el.lastElementChild as HTMLElement
      const buttons = [...el.querySelectorAll('button')]
      return {
        width: Math.round(el.getBoundingClientRect().width),
        radius: px(card.borderRadius),
        headerPad: getComputedStyle(header).padding,
        footerPad: getComputedStyle(footer).padding,
        footerBg: getComputedStyle(footer).backgroundColor,
        submitHeight: Math.round(buttons[buttons.length - 1].getBoundingClientRect().height),
      }
    })

    // `max-w-[560px]` at a 1440px viewport; 4px of slack absorbs the scrollbar.
    expect(m.width).toBeGreaterThanOrEqual(552)
    expect(m.width).toBeLessThanOrEqual(560)
    expect(m.radius).toBe(14)
    expect(m.headerPad).toBe('14px 20px')
    expect(m.footerPad).toBe('12px 20px')
    // `--color-surface-page`, i.e. the page tint the design fills the band with.
    expect(m.footerBg).toBe('rgb(246, 247, 246)')
    // DESIGN.md's button height, not the browser default.
    expect(m.submitHeight).toBe(32)
  })

  test('the list endpoint applies the filters the contract advertises', async ({ page }) => {
    const seeded = await page.evaluate(
      async ({ api, board, org }) => {
        const create = async (title: string, status: string) => {
          await fetch(`${api}/boards/${board}/tasks`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', 'X-Org-ID': org },
            credentials: 'include',
            body: JSON.stringify({ title, status }),
          })
        }
        await create('Alpha reclaim job', 'backlog')
        await create('Beta webhook retry', 'ready')
        const list = async (qs: string) => {
          const res = await fetch(`${api}/boards/${board}/tasks${qs}`, {
            headers: { 'X-Org-ID': org },
            credentials: 'include',
          })
          return ((await res.json()) as { title: string }[]).map((t) => t.title)
        }
        return { all: await list(''), ready: await list('?status=ready'), search: await list('?search=webhook') }
      },
      { api: API, board: boardID, org: orgID },
    )

    expect(seeded.all).toHaveLength(2)
    expect(seeded.ready).toEqual(['Beta webhook retry'])
    expect(seeded.search).toEqual(['Beta webhook retry'])
  })
})
