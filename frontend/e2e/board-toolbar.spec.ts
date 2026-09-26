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
    await expect(dialog.getByLabel(/judul task|task title/i).first()).toBeVisible()
    await expect(dialog.getByLabel(/deskripsi|description/i)).toBeVisible()

    const title = `Toolbar task ${Date.now()}`
    await dialog
      .getByLabel(/judul task|task title/i)
      .first()
      .fill(title)
    await dialog.getByLabel(/deskripsi|description/i).fill('Body typed from the modal')
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
        return (await res.json()) as { title: string; body: string }[]
      },
      { api: API, board: boardID },
    )
    const written = stored.find((t) => t.title === title)
    expect(written, `task ${title} not in ${JSON.stringify(stored.map((t) => t.title))}`).toBeTruthy()
    expect(written!.body).toBe('Body typed from the modal')
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
