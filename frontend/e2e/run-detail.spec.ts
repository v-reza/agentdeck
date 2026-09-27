import { expect, test, type Page } from '@playwright/test'

/**
 * US-AD41 — halaman detail run (`/runs/:id`).
 *
 * Kenapa test ini ada:
 *
 *  1. `GET /runs/{id}` dan `GET /runs/{id}/summary` sudah hidup sejak lama dan
 *     **tidak punya pemanggil**. Test ini yang membuat klaim itu berhenti jadi
 *     klaim.
 *  2. AC1 minta enam fakta (status, outcome, durasi, biaya, token, attempt).
 *     Yang paling mudah hilang diam-diam adalah **durasi**, karena satu-satunya
 *     sumbernya adalah `/summary` — field itu tidak ada di baris `runs`. Kalau
 *     endpoint itu tidak dipanggil, durasinya jadi "—" dan tidak ada yang gagal.
 *  3. AC3/AC4 minta 404, **bukan halaman kosong**. "Tidak ada" dan "kosong" harus
 *     terbaca berbeda; kalau tidak, operator menunggu run yang sudah hilang.
 *  4. Tab Ledger membaca `GET /runs/{id}/ledger`, satu klien yang sudah ada sejak
 *     lama dan belum pernah dipakai layar mana pun.
 */
const API = '/api/v1'
const PASSWORD = 'correct-horse-battery-staple'

async function signUp(page: Page): Promise<string> {
  const email = `e2e-run-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
  const res = await page.request.post(`${API}/auth/register`, {
    data: { email, password: PASSWORD, name: 'Run Owner', org_name: 'Run Co' },
  })
  expect(res.status(), await res.text()).toBe(201)
  const [session] = (await page.context().cookies()).filter((c) => c.name === 'agentdeck_session')
  expect(session, 'registration did not set a session cookie').toBeTruthy()
  await page.context().addCookies([{ ...session, secure: false }])
  return ((await res.json()) as { workspace_id: string }).workspace_id
}

async function createBoard(page: Page, orgID: string) {
  const projectRes = await page.request.post(`${API}/projects`, { data: { name: 'Runs', slug: 'runs' } })
  expect(projectRes.status(), await projectRes.text()).toBe(201)
  const projectID = ((await projectRes.json()) as { id: string }).id
  const boardRes = await page.request.post(`${API}/projects/${projectID}/boards`, {
    headers: { 'X-Org-ID': orgID },
    data: { name: 'Runs', slug: 'runs' },
  })
  expect(boardRes.status(), await boardRes.text()).toBe(201)
  return { projectID, boardID: ((await boardRes.json()) as { id: string }).id }
}

/** Task ter-claim: itu yang membuat satu run nyata, lengkap dengan id-nya. */
async function seedRun(page: Page, orgID: string, projectID: string, boardID: string, title: string) {
  const agentRes = await page.request.post(`${API}/projects/${projectID}/agents`, {
    headers: { 'X-Org-ID': orgID },
    data: { name: `runner-${Date.now()}`, provider: 'openai_compatible', model: 'gpt-4o' },
  })
  expect(agentRes.status(), await agentRes.text()).toBe(201)
  const agentID = ((await agentRes.json()) as { id: string }).id

  const taskRes = await page.request.post(`${API}/boards/${boardID}/tasks`, {
    headers: { 'X-Org-ID': orgID },
    data: { title, status: 'backlog' },
  })
  expect(taskRes.status(), await taskRes.text()).toBe(201)
  const taskID = ((await taskRes.json()) as { id: string }).id

  for (const [path, data] of [
    [`tasks/${taskID}/move`, { from: 'backlog', to: 'ready' }],
    [`tasks/${taskID}/assign`, { agent_id: agentID }],
  ] as Array<[string, unknown]>) {
    const res = await page.request.post(`${API}/${path}`, { headers: { 'X-Org-ID': orgID }, data })
    expect(res.status(), `${path}: ${await res.text()}`).toBeLessThan(300)
  }

  const claimRes = await page.request.post(`${API}/tasks/${taskID}/claim`, { headers: { 'X-Org-ID': orgID } })
  expect(claimRes.status(), await claimRes.text()).toBeLessThan(300)
  const runID = ((await claimRes.json()) as { id: string }).id
  return { taskID, runID }
}

/** Satu step nyata di atas run, supaya tab Steps tidak kosong. */
async function seedStep(page: Page, orgID: string, runID: string, seq: number, name: string) {
  const res = await page.request.post(`${API}/runs/${runID}/steps`, {
    headers: { 'X-Org-ID': orgID },
    data: {
      seq,
      kind: 'llm',
      name,
      status: 'succeeded',
      // Field-nya `payload` (lihat `stepRequest`), bukan `payload_json`: itu nama
      // KOLOM, dan handler memakai nama request. Mengirim `payload_json` dijawab
      // 201 dengan payload null — sukses, tapi tanpa data, yang justru bentuk
      // gagal paling mahal untuk ditemukan.
      payload: { tool: 'read_file', nested: { path: '/var/agentdeck/config.go' } },
    },
  })
  expect(res.status(), await res.text()).toBeLessThan(300)
}

test.describe('the run detail screen', () => {
  let orgID: string
  let projectID: string
  let boardID: string

  test.beforeEach(async ({ page }) => {
    orgID = await signUp(page)
    ;({ projectID, boardID } = await createBoard(page, orgID))
  })

  test('summarises the run and lists its steps', async ({ page }) => {
    const { runID } = await seedRun(page, orgID, projectID, boardID, 'Jalankan migrasi')
    await seedStep(page, orgID, runID, 1, 'baca konfigurasi')

    await page.goto(`/app/${orgID}/runs/${runID}`)

    // AC1: attempt ada di header, dan statusnya adalah status run ini.
    await expect(page.getByText(/Attempt|Percobaan/).first()).toBeVisible({ timeout: 30_000 })
    await expect(page.getByText('#1')).toBeVisible()

    // Durasi hanya datang dari `/summary`. Kalau endpoint itu tidak dipanggil,
    // yang tampil "—" dan tidak ada yang gagal — karena itu diperiksa di sini
    // bahwa nilainya BUKAN "—" untuk run yang sudah punya started_at.
    const duration = page.locator('dl dd').first()
    await expect(duration).not.toHaveText('—')

    // Tab Steps memuat step yang benar-benar disemai, read lewat `/runs/{id}/steps`.
    await expect(page.getByText('baca konfigurasi')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByText('read_file')).toBeVisible()

    // Tab Ledger: run ini belum punya baris ledger, dan itu harus MENGATAKAN
    // sesuatu — bukan panel kosong yang tidak bisa dibedakan dari gagal muat.
    await page.getByRole('tab', { name: /^(Ledger)$/ }).click()
    await expect(page.getByText(/No ledger rows|Belum ada baris ledger/)).toBeVisible({ timeout: 30_000 })
  })

  test('is reachable from the run rows in the task drawer', async ({ page }) => {
    // Sebuah layar tanpa pintu masuk adalah rute yang tidak bisa dicapai siapa
    // pun. Baris run di tab Logs sudah jadi tempat operator melihat attempt mana
    // yang gagal, jadi itu pintunya — dan dites dari UI, bukan dari URL langsung.
    const { taskID, runID } = await seedRun(page, orgID, projectID, boardID, 'Punya run')
    await seedStep(page, orgID, runID, 1, 'langkah pertama')

    await page.goto(`/app/${orgID}/boards/${boardID}`)
    const card = page.getByText('Punya run').first()
    await expect(card).toBeVisible({ timeout: 30_000 })
    await card.click()

    const drawer = page.getByRole('complementary', { name: 'Task Detail Drawer' })
    await drawer.getByRole('tab', { name: /^Logs?$/ }).click()

    const link = drawer.getByRole('link', { name: new RegExp(runID) })
    await expect(link).toBeVisible({ timeout: 30_000 })
    await link.click()

    await expect(page).toHaveURL(new RegExp(`/runs/${runID}$`))
    await expect(page.getByText('langkah pertama')).toBeVisible({ timeout: 30_000 })
    expect(taskID).toBeTruthy()
  })

  test('a run that is not in this organisation says so instead of rendering empty', async ({ page }) => {
    // AC3/AC4: server menjawab 404 untuk run yang tidak ada DAN untuk run org
    // lain — sengaja tak terbedakan supaya endpoint-nya tidak bisa dipakai
    // memancing id milik tenant lain. Layarnya harus menyebut itu, bukan
    // menampilkan run kosong.
    await page.goto(`/app/${orgID}/runs/01NONEXISTENTRUN000000000000`)

    await expect(page.getByText(/does not exist|tidak ada/)).toBeVisible({ timeout: 30_000 })
  })
})
