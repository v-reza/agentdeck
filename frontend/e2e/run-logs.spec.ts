import { expect, test, type Page } from '@playwright/test'

/**
 * US-AD26 (Must, M3) + US-AD94 (Must, M2) -- tab Logs: timeline step per run.
 *
 * Kenapa test ini ada:
 *
 *  1. `GET /tasks/{id}/runs` dan `GET /runs/{id}/steps` sudah hidup sejak run
 *     lifecycle mendarat, dan NOL komponen memanggilnya. Jadi tab Logs tidak
 *     menunggu endpoint, dia menunggu klien yang belum ditulis.
 *  2. `steps` adalah tabel append-only dengan `seq` unik per run, dan
 *     `steps_run_seq_key` ada justru supaya urutan tidak ambigu. Test ini
 *     memeriksa urutannya DI LAYAR, bukan di API -- kalau komponennya
 *     mengurutkan sendiri, `ORDER BY seq` di SQL tidak lagi yang menentukan.
 *  3. US-AD94 AC3 minta step yang belum selesai menampilkan durasi sebagai
 *     "berjalan", bukan 0 yang menyesatkan. AC itu satu-satunya bagian yang bisa
 *     salah tanpa error apa pun, jadi diuji lewat step berstatus `running` yang
 *     `ended_at`-nya kosong.
 *  4. US-AD26 AC3 minta payload kosong menampilkan panel kosong BERTANDA, bukan
 *     error. Step tanpa payload adalah keadaan normal saat run baru mulai.
 *
 * Akun didaftarkan per run, jadi suite tetap bisa diulang di database yang tidak
 * pernah dipangkas.
 */
const API = '/api/v1'
const PASSWORD = 'correct-horse-battery-staple'

/** Daftar lewat API, lalu pasang cookie-nya ke browser DAN request context. */
async function signUp(page: Page): Promise<string> {
  const email = `e2e-logs-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
  const res = await page.request.post(`${API}/auth/register`, {
    data: { email, password: PASSWORD, name: 'Logs Owner', org_name: 'Logs Co' },
  })
  expect(res.status(), await res.text()).toBe(201)
  const [session] = (await page.context().cookies()).filter((c) => c.name === 'agentdeck_session')
  expect(session, 'registration did not set a session cookie').toBeTruthy()
  // Cookie sesi di-issue `Secure`, dan request context Playwright menolak
  // mengirim cookie Secure lewat HTTP. Dipasang ulang dengan Secure=false supaya
  // SATU jar cookie berlaku untuk browser dan request context sekaligus.
  await page.context().addCookies([{ ...session, secure: false }])
  await page.goto('/login')
  return ((await res.json()) as { workspace_id: string }).workspace_id
}

async function createBoard(page: Page, orgID: string) {
  const projectRes = await page.request.post(`${API}/projects`, {
    headers: { 'X-Org-ID': orgID },
    data: { name: 'Logs', slug: 'logs' },
  })
  expect(projectRes.status(), await projectRes.text()).toBe(201)
  const project = (await projectRes.json()) as { id: string }

  const boardRes = await page.request.post(`${API}/projects/${project.id}/boards`, {
    headers: { 'X-Org-ID': orgID },
    data: { name: 'Logs Board', slug: 'logs' },
  })
  expect(boardRes.status(), await boardRes.text()).toBe(201)
  return { projectID: project.id, boardID: ((await boardRes.json()) as { id: string }).id }
}

/**
 * Bikin task yang benar-benar PUNYA run: task harus `ready` dan punya assignee
 * sebelum bisa di-claim, dan `claim` inilah yang membuat baris `runs`.
 */
async function seedRun(page: Page, orgID: string, projectID: string, boardID: string) {
  const agentRes = await page.request.post(`${API}/projects/${projectID}/agents`, {
    headers: { 'X-Org-ID': orgID },
    data: { name: `logs-runner-${Date.now()}`, provider: 'openai_compatible', model: 'gpt-4o' },
  })
  expect(agentRes.status(), await agentRes.text()).toBe(201)
  const agent = (await agentRes.json()) as { id: string }

  const taskRes = await page.request.post(`${API}/boards/${boardID}/tasks`, {
    headers: { 'X-Org-ID': orgID },
    data: { title: 'Jalankan sesuatu', status: 'backlog' },
  })
  expect(taskRes.status(), await taskRes.text()).toBe(201)
  const task = (await taskRes.json()) as { id: string }

  await page.request.post(`${API}/tasks/${task.id}/move`, {
    headers: { 'X-Org-ID': orgID },
    data: { from: 'backlog', to: 'ready' },
  })
  await page.request.post(`${API}/tasks/${task.id}/assign`, {
    headers: { 'X-Org-ID': orgID },
    data: { agent_id: agent.id },
  })
  const claimRes = await page.request.post(`${API}/tasks/${task.id}/claim`, { headers: { 'X-Org-ID': orgID } })
  expect(claimRes.status(), await claimRes.text()).toBeLessThan(300)
  const runID = ((await claimRes.json()) as { id: string }).id

  return { taskID: task.id, runID }
}

/** Buka drawer task dan pindah ke tab Logs. */
async function openLogsTab(page: Page, orgID: string, boardID: string) {
  await page.goto(`/app/${orgID}/boards/${boardID}`)
  const card = page.getByText('Jalankan sesuatu').first()
  await expect(card).toBeVisible({ timeout: 30_000 })
  await card.click()

  const drawer = page.getByRole('complementary', { name: 'Task Detail Drawer' })
  await drawer.getByRole('tab', { name: /^Logs?$/ }).click()
  return drawer
}

test.describe('the logs tab', () => {
  let orgID: string
  let projectID: string
  let boardID: string

  test.beforeEach(async ({ page }) => {
    orgID = await signUp(page)
    ;({ projectID, boardID } = await createBoard(page, orgID))
  })

  test('renders each step in seq order with status, tokens and cost', async ({ page }) => {
    const { runID } = await seedRun(page, orgID, projectID, boardID)

    // Tiga step: dua selesai, satu masih jalan. Payload ditulis sebagai objek
    // dan dibaca kembali sebagai STRING -- kolomnya JSONB dan handler
    // mengembalikan teks yang ditulis Postgres, bukan objek yang di-encode ulang.
    const steps = [
      { seq: 1, kind: 'llm', name: 'plan', status: 'succeeded', payload: { prompt: 'rencanakan' } },
      { seq: 2, kind: 'tool', name: 'read_file', status: 'succeeded', payload: { path: 'a.go' } },
      { seq: 3, kind: 'llm', name: 'implement', status: 'running', payload: null },
      { seq: 4, kind: 'tool', name: 'write_file', status: 'failed', payload: null },
    ]
    for (const step of steps) {
      // Payload ditulis saat step DIBUKA, bukan saat ditutup: `finishRunStep`
      // membangun `board.Step{Status, CostMicros}` saja dan membuang field
      // payload dari body, jadi mengirimnya lewat PATCH adalah no-op yang tidak
      // berbunyi. Step `running` sengaja tidak difinish supaya `ended_at` kosong.
      const startRes = await page.request.post(`${API}/runs/${runID}/steps`, {
        headers: { 'X-Org-ID': orgID },
        data: { seq: step.seq, kind: step.kind, name: step.name, ...(step.payload ? { payload: step.payload } : {}) },
      })
      expect(startRes.status(), await startRes.text()).toBe(201)
      if (step.payload) {
        const finishRes = await page.request.patch(`${API}/runs/${runID}/steps/${step.seq}`, {
          headers: { 'X-Org-ID': orgID },
          data: { status: 'succeeded', tokens: { in: 120, out: 40 }, cost_micros: 25_000 },
        })
        expect(finishRes.status(), await finishRes.text()).toBeLessThan(300)
      }
      if (step.status === 'failed') {
        const failRes = await page.request.patch(`${API}/runs/${runID}/steps/${step.seq}`, {
          headers: { 'X-Org-ID': orgID },
          data: { status: 'failed', cost_micros: 1_500 },
        })
        expect(failRes.status(), await failRes.text()).toBeLessThan(300)
      }
    }

    const drawer = await openLogsTab(page, orgID, boardID)
    const panel = drawer.getByRole('tabpanel', { name: /^Logs?$/ })

    // Langganan step per run, jadi barisnya menunggu request kedua. Ditunggu
    // eksplisit supaya kegagalan berikutnya menunjuk pada yang benar-benar salah.
    await expect(panel.getByText('plan')).toBeVisible({ timeout: 15_000 })

    // Urutan: `seq` yang menentukan, bukan urutan kedatangan di cache.
    const names = await panel.locator('li button span.truncate').allInnerTexts()
    expect(names).toEqual(['plan', 'read_file', 'implement', 'write_file'])

    // AC26 AC1: status punya warna sendiri. Diperiksa lewat `data-status` pada
    // ikon plus kelas warna yang dipakai, bukan lewat selector kelas Tailwind
    // mentah: `svg.text-[var(--color-success)]` bukan selector CSS yang valid
    // dan `querySelectorAll` menolaknya.
    await expect(panel.locator('svg[data-status="succeeded"]')).toHaveCount(2)
    await expect(panel.locator('svg[data-status="failed"]')).toHaveCount(1)
    await expect(panel.locator('svg[data-status="running"]')).toHaveCount(1)

    // Warna mengikuti status, dan tokennya dari DESIGN.md -- bukan hex bebas.
    await expect(panel.locator('li', { hasText: 'plan' }).first().locator('svg[data-status="succeeded"]')).toHaveClass(
      /color-success/,
    )
    await expect(
      panel.locator('li', { hasText: 'write_file' }).first().locator('svg[data-status="failed"]'),
    ).toHaveClass(/color-danger/)

    // AC94 AC3: step yang belum selesai tidak boleh mengaku 0ms.
    const runningRow = panel.locator('li', { hasText: 'implement' }).first()
    await expect(runningRow.getByText(/^(running|berjalan)$/)).toBeVisible()

    // AC94 AC1: angka biaya dan token ada di baris, monospace tabular.
    const planRow = panel.locator('li', { hasText: 'plan' }).first()
    await expect(planRow).toContainText('160')
  })

  test('expanding a step shows its payload, and an empty one says so', async ({ page }) => {
    const { runID } = await seedRun(page, orgID, projectID, boardID)

    const withPayload = await page.request.post(`${API}/runs/${runID}/steps`, {
      headers: { 'X-Org-ID': orgID },
      data: {
        seq: 1,
        kind: 'llm',
        name: 'with-payload',
        payload: { completion: { text: 'selesai' }, tool: { name: 'read_file' } },
      },
    })
    expect(withPayload.status(), await withPayload.text()).toBe(201)
    await page.request.patch(`${API}/runs/${runID}/steps/1`, {
      headers: { 'X-Org-ID': orgID },
      data: { status: 'succeeded' },
    })

    const withoutPayload = await page.request.post(`${API}/runs/${runID}/steps`, {
      headers: { 'X-Org-ID': orgID },
      data: { seq: 2, kind: 'tool', name: 'no-payload' },
    })
    expect(withoutPayload.status(), await withoutPayload.text()).toBe(201)
    await page.request.patch(`${API}/runs/${runID}/steps/2`, {
      headers: { 'X-Org-ID': orgID },
      data: { status: 'succeeded' },
    })

    const drawer = await openLogsTab(page, orgID, boardID)
    const panel = drawer.getByRole('tabpanel', { name: /^Logs?$/ })

    // AC26 AC2: payload dibaca, dan ditampilkan sebagai JSON yang terbaca --
    // bukan string JSON mentah satu baris.
    await expect(panel.getByText('with-payload')).toBeVisible({ timeout: 15_000 })
    await panel.getByRole('button', { name: /with-payload/ }).click()
    const pre = panel.locator('pre')
    // Pembeda pretty-print vs JSON mentah adalah JUMLAH BARIS, dan itu tidak
    // kelihatan sampai payloadnya bersarang: kolom `payload_json` adalah JSONB,
    // jadi Postgres sudah mengembalikannya dengan spasi setelah titik dua
    // (`{"a": 1}`), persis seperti hasil stringify untuk objek datar. Dua asert
    // sebelumnya -- `toContainText('"completion"')` lalu versi berspasinya --
    // SAMA-SAMA cocok untuk payload mentah maupun yang di-pretty-print; keduanya
    // baru ketahuan sebagai asert kosong dari mutan yang SURVIVED.
    const rendered = await pre.innerText()
    expect(rendered.split('\n').length).toBeGreaterThan(1)
    await expect(pre).toContainText('"text": "selesai"')
    await expect(pre).toContainText('"name": "read_file"')

    // AC26 AC3: kosong = panel bertanda jelas, BUKAN error.
    await panel.getByRole('button', { name: /no-payload/ }).click()
    await expect(panel.getByText(/payload tersimpan|payload stored|No payload/i).last()).toBeVisible()
    await expect(panel.getByText(/tidak bisa dimuat|could not load/i)).toHaveCount(0)
  })

  test('a task that never ran says so instead of showing an empty timeline', async ({ page }) => {
    const taskRes = await page.request.post(`${API}/boards/${boardID}/tasks`, {
      headers: { 'X-Org-ID': orgID },
      data: { title: 'Belum pernah jalan', status: 'backlog' },
    })
    expect(taskRes.status(), await taskRes.text()).toBe(201)

    await page.goto(`/app/${orgID}/boards/${boardID}`)
    const card = page.getByText('Belum pernah jalan').first()
    await expect(card).toBeVisible({ timeout: 30_000 })
    await card.click()

    const drawer = page.getByRole('complementary', { name: 'Task Detail Drawer' })
    await drawer.getByRole('tab', { name: /^Logs?$/ }).click()

    // "belum pernah dijalankan" dan "gagal memuat" terlihat sama kecuali state
    // kosongnya nyata -- dan jawaban pertama yang menghentikan operator mencari.
    await expect(drawer.getByText(/belum pernah dijalankan|not been run/i)).toBeVisible()
  })
})
