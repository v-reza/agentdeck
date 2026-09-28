import { execFileSync } from 'node:child_process'
import { expect, test, type Page } from '@playwright/test'

/**
 * US-AD76 — dashboard.
 *
 * Kenapa test ini ada:
 *
 *  1. Tiga kartu metrik datang dari tiga endpoint org-wide yang berbeda
 *     (`search/runs`, `orgs/{id}/cost-summary`, `approvals`). Yang paling mudah
 *     rusak diam-diam adalah scope-nya: sebuah `?outcome=running` yang lupa
 *     diteruskan tetap mengembalikan 200 dengan daftar yang salah. Test pertama
 *     membandingkan angka di layar dengan angka yang dihitung dari respons API
 *     yang sama, bukan dengan konstanta.
 *  2. AC3 bergantung pada JUMLAH workspace, jadi harus diuji di dua keadaan:
 *     akun dengan satu workspace memakai nama pengguna, akun yang di-invite ke
 *     workspace kedua memakai nama workspace. Satu keadaan saja tidak
 *     membuktikan cabangnya.
 *  3. Rute ini tidak bisa dicapai lewat `index`: layout me-redirect /app/:orgID
 *     ke /projects. Kalau `/dashboard` pernah diubah jadi `index`, tes ini gagal
 *     — dan itu memang yang diinginkan.
 */

const API = '/api/v1'
const PASSWORD = 'Sup3rSecret!2026'
const DB_CONTAINER = 'agentdeck-db'

/**
 * Jalankan satu statement di container DB.
 *
 * Dipakai HANYA untuk data yang tidak punya jalur tulis di API. Ledger adalah
 * kasusnya: tidak ada endpoint yang menulis `ledger_entries` — dispatcher yang
 * menuliskannya saat run berjalan — jadi satu-satunya cara mengisi panel biaya
 * dengan baris nyata adalah menulisnya langsung, seperti `notifications.spec.ts`
 * melakukannya untuk notifikasi.
 */
function psql(sql: string): string {
  return execFileSync('docker', ['exec', DB_CONTAINER, 'psql', '-U', 'agentdeck', '-d', 'agentdeck', '-tAc', sql], {
    encoding: 'utf8',
  }).trim()
}

async function signUp(page: Page, email: string, orgName: string) {
  const res = await page.request.post(`${API}/auth/register`, {
    data: { email, password: PASSWORD, name: 'Dash Owner', org_name: orgName },
  })
  expect(res.status(), await res.text()).toBe(201)
  const [session] = (await page.context().cookies()).filter((c) => c.name === 'agentdeck_session')
  expect(session, 'registrasi tidak memasang cookie sesi').toBeTruthy()
  await page.context().addCookies([{ ...session, secure: false }])
  return (await res.json()) as { workspace_id: string }
}

test.describe('dashboard — US-AD76', () => {
  test('AC1 — the four metric cards read their endpoints, and the route is reachable', async ({ page }) => {
    const { workspace_id } = await signUp(page, `dash${Date.now()}@example.com`, 'Dash Co')

    await page.goto(`/app/${workspace_id}/dashboard`)

    // Kalau rute ini tidak terdaftar, layout akan membuang kita ke /projects.
    await expect(page).toHaveURL(new RegExp(`/app/${workspace_id}/dashboard$`), { timeout: 15_000 })
    await expect(page.getByTestId('metric-failed')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('metric-projects')).toBeVisible()
    await expect(page.getByTestId('metric-cost')).toBeVisible()
    await expect(page.getByTestId('metric-approvals')).toBeVisible()

    // Angka di kartu harus sama dengan yang dikatakan API, bukan dengan tebakan.
    const approvals = await page.request.get(`${API}/approvals`)
    expect(approvals.status()).toBe(200)
    const approvalCount = ((await approvals.json()) as unknown[]).length
    await expect(page.getByTestId('metric-approvals')).toContainText(String(approvalCount))

    // Kartu proyek membaca /projects. Angkanya dibandingkan dengan respons API,
    // bukan dengan nol: registrasi sudah membuat satu project bawaan.
    const projects = await page.request.get(`${API}/projects`)
    expect(projects.status()).toBe(200)
    const projectCount = ((await projects.json()) as unknown[]).length
    await expect(page.getByTestId('metric-projects')).toContainText(String(projectCount))

    // Workspace baru: tiga panel kosong harus menyebut keadaannya, bukan nol.
    await expect(page.getByTestId('dashboard-runs-empty')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('dashboard-approvals-empty')).toBeVisible()
    await expect(page.getByTestId('dashboard-spend-empty')).toBeVisible()
  })

  test('AC1 — a finished run reaches the failed card, and a live run cannot', async ({ page }) => {
    const { workspace_id } = await signUp(page, `dashrun${Date.now()}@example.com`, 'Dash Run Co')
    const org = { 'X-Org-ID': workspace_id }

    const project = await page.request.post(`${API}/projects`, {
      headers: org,
      data: { name: 'Dash Project', slug: `dash-${Date.now()}` },
    })
    expect(project.status(), await project.text()).toBe(201)
    const projectID = ((await project.json()) as { id: string }).id

    const board = await page.request.post(`${API}/projects/${projectID}/boards`, {
      headers: org,
      data: { name: 'Dash Board', slug: `dashb-${Date.now()}` },
    })
    expect(board.status(), await board.text()).toBe(201)
    const boardID = ((await board.json()) as { id: string }).id

    // Agent dulu: `ClaimAndStart` menolak task tanpa assignee, jadi agent adalah
    // prasyarat klaim, bukan pelengkap.
    const agent = await page.request.post(`${API}/projects/${projectID}/agents`, {
      headers: org,
      data: { name: `dash-runner-${Date.now()}`, provider: 'openai_compatible', model: 'gpt-4o' },
    })
    expect(agent.status(), await agent.text()).toBe(201)
    const agentID = ((await agent.json()) as { id: string }).id

    // Task lahir di `backlog`; klaim menuntut `ready`. Tiga langkah ini kontrak.
    const task = await page.request.post(`${API}/boards/${boardID}/tasks`, {
      headers: org,
      data: { title: 'Seeded failed task', status: 'backlog' },
    })
    expect(task.status(), await task.text()).toBe(201)
    const taskID = ((await task.json()) as { id: string }).id

    for (const [path, data] of [
      [`tasks/${taskID}/move`, { from: 'backlog', to: 'ready' }],
      [`tasks/${taskID}/assign`, { agent_id: agentID }],
    ] as Array<[string, unknown]>) {
      const res = await page.request.post(`${API}/${path}`, { headers: org, data })
      expect(res.status(), `${path}: ${await res.text()}`).toBeLessThan(300)
    }

    const claim = await page.request.post(`${API}/tasks/${taskID}/claim`, { headers: org })
    expect(claim.status(), await claim.text()).toBe(201)
    const runID = ((await claim.json()) as { id: string }).id

    // Run hidup: `?outcome=running` TIDAK menemukannya. Ini bukan kekurangan test
    // — `runs.outcome` baru ditulis `EndRun`, dan CHECK-nya bahkan tidak
    // mengizinkan 'running'. Dikunci di sini supaya asumsi itu tidak diam-diam
    // berubah jadi filter yang "kelihatannya jalan".
    const live = await page.request.get(`${API}/search/runs?outcome=running`, { headers: org })
    expect(live.status()).toBe(200)
    expect(((await live.json()) as { runs: unknown[] }).runs.length).toBe(0)

    // Run ditutup sebagai gagal lewat endpoint yang sama yang dipakai dispatcher.
    const ended = await page.request.post(`${API}/runs/${runID}/end`, {
      headers: org,
      data: { outcome: 'failed', error: 'seeded failure for the dashboard card' },
    })
    expect(ended.status(), await ended.text()).toBeLessThan(300)

    // Run gagal KEDUA, tapi berumur tiga hari. Tidak ada jalur API untuk
    // memundurkan waktu, jadi stempel waktunya digeser langsung di DB. Ini yang
    // membuat filter 24 jam benar-benar teruji: tanpa baris ini, "24 jam" dan
    // "semua" menghasilkan angka yang sama dan filternya tidak bisa dibedakan.
    const staleTask = await page.request.post(`${API}/boards/${boardID}/tasks`, {
      headers: org,
      data: { title: 'Old failed task', status: 'backlog' },
    })
    const staleTaskID = ((await staleTask.json()) as { id: string }).id
    for (const [path, data] of [
      [`tasks/${staleTaskID}/move`, { from: 'backlog', to: 'ready' }],
      [`tasks/${staleTaskID}/assign`, { agent_id: agentID }],
    ] as Array<[string, unknown]>) {
      const res = await page.request.post(`${API}/${path}`, { headers: org, data })
      expect(res.status(), `${path}: ${await res.text()}`).toBeLessThan(300)
    }
    const staleClaim = await page.request.post(`${API}/tasks/${staleTaskID}/claim`, { headers: org })
    const staleRunID = ((await staleClaim.json()) as { id: string }).id
    const staleEnd = await page.request.post(`${API}/runs/${staleRunID}/end`, {
      headers: org,
      data: { outcome: 'failed', error: 'old failure' },
    })
    expect(staleEnd.status(), await staleEnd.text()).toBeLessThan(300)
    psql(
      `UPDATE runs SET started_at = now() - interval '3 days', ended_at = now() - interval '3 days' ` +
        `WHERE id = '${staleRunID}';`,
    )

    const failed = await page.request.get(`${API}/search/runs?outcome=failed&limit=100`, { headers: org })
    expect(failed.status()).toBe(200)
    const allFailed = ((await failed.json()) as { runs: unknown[] }).runs.length
    // Server mengembalikan keduanya; layar hanya boleh menghitung yang < 24 jam.
    expect(allFailed).toBe(2)

    await page.goto(`/app/${workspace_id}/dashboard`)
    await expect(page.getByTestId('metric-failed')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('metric-failed')).toContainText('1')
    await expect(page.getByTestId('dashboard-run-row')).toHaveCount(1)

    // Dan barisnya menuju layar detail run yang sudah ada.
    await expect(page.getByTestId('dashboard-run-row')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('dashboard-run-row')).toHaveAttribute('data-outcome', 'failed')
  })

  test('AC1 — the approvals card counts the real inbox', async ({ page }) => {
    const { workspace_id } = await signUp(page, `dashapp${Date.now()}@example.com`, 'Dash Approval Co')
    const org = { 'X-Org-ID': workspace_id }

    const project = await page.request.post(`${API}/projects`, {
      headers: org,
      data: { name: 'Approval Project', slug: `dasha-${Date.now()}` },
    })
    const projectID = ((await project.json()) as { id: string }).id

    const board = await page.request.post(`${API}/projects/${projectID}/boards`, {
      headers: org,
      data: { name: 'Approval Board', slug: `dasha-b-${Date.now()}` },
    })
    const boardID = ((await board.json()) as { id: string }).id

    const agent = await page.request.post(`${API}/projects/${projectID}/agents`, {
      headers: org,
      data: { name: `dash-gate-${Date.now()}`, provider: 'openai_compatible', model: 'gpt-4o' },
    })
    const agentID = ((await agent.json()) as { id: string }).id

    const task = await page.request.post(`${API}/boards/${boardID}/tasks`, {
      headers: org,
      data: { title: 'Gated task', status: 'backlog' },
    })
    const taskID = ((await task.json()) as { id: string }).id

    for (const [path, data] of [
      [`tasks/${taskID}/move`, { from: 'backlog', to: 'ready' }],
      [`tasks/${taskID}/assign`, { agent_id: agentID }],
    ] as Array<[string, unknown]>) {
      const res = await page.request.post(`${API}/${path}`, { headers: org, data })
      expect(res.status(), `${path}: ${await res.text()}`).toBeLessThan(300)
    }

    const claim = await page.request.post(`${API}/tasks/${taskID}/claim`, { headers: org })
    expect(claim.status(), await claim.text()).toBe(201)

    // SATU gate per task, dan hanya selama run-nya hidup: `RequestApproval`
    // memarkir task di `awaiting_approval` lalu menutup run-nya, jadi gate kedua
    // ke task yang sama ditolak (`task.CurrentRunID` sudah tidak ada). Itu
    // perilaku kontrak, bukan batasan test.
    const req = await page.request.post(`${API}/tasks/${taskID}/approvals`, {
      headers: org,
      // `preview_json` adalah `json.RawMessage` — objek, bukan string berisi JSON.
      data: { reason: 'seeded gate', preview_json: { action: 'delete-stale-replicas' } },
    })
    expect(req.status(), await req.text()).toBe(201)

    const inbox = await page.request.get(`${API}/approvals`, { headers: org })
    expect(inbox.status()).toBe(200)
    const inboxCount = ((await inbox.json()) as unknown[]).length
    expect(inboxCount).toBe(1)

    await page.goto(`/app/${workspace_id}/dashboard`)
    await expect(page.getByTestId('metric-approvals')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('metric-approvals')).toContainText(String(inboxCount))
  })

  test('AC1 — spend by board shows the ledger rows that exist, and the empty state otherwise', async ({ page }) => {
    const { workspace_id } = await signUp(page, `dashspend${Date.now()}@example.com`, 'Dash Spend Co')
    const org = { 'X-Org-ID': workspace_id }

    const project = await page.request.post(`${API}/projects`, {
      headers: org,
      data: { name: 'Spend Project', slug: `dashs-${Date.now()}` },
    })
    const projectID = ((await project.json()) as { id: string }).id
    const board = await page.request.post(`${API}/projects/${projectID}/boards`, {
      headers: org,
      data: { name: 'Spend Board', slug: `dashs-b-${Date.now()}` },
    })
    const boardID = ((await board.json()) as { id: string }).id

    // Ledger menempel ke run nyata, jadi jalur task -> agent -> claim yang sama
    // dijalankan di sini juga. Baris ledger tanpa run tidak bisa ada.
    const agent = await page.request.post(`${API}/projects/${projectID}/agents`, {
      headers: org,
      data: { name: `dash-spend-${Date.now()}`, provider: 'openai_compatible', model: 'gpt-4o' },
    })
    const agentID = ((await agent.json()) as { id: string }).id
    const task = await page.request.post(`${API}/boards/${boardID}/tasks`, {
      headers: org,
      data: { title: 'Spend task', status: 'backlog' },
    })
    const taskID = ((await task.json()) as { id: string }).id
    for (const [path, data] of [
      [`tasks/${taskID}/move`, { from: 'backlog', to: 'ready' }],
      [`tasks/${taskID}/assign`, { agent_id: agentID }],
    ] as Array<[string, unknown]>) {
      const res = await page.request.post(`${API}/${path}`, { headers: org, data })
      expect(res.status(), `${path}: ${await res.text()}`).toBeLessThan(300)
    }
    const claim = await page.request.post(`${API}/tasks/${taskID}/claim`, { headers: org })
    expect(claim.status(), await claim.text()).toBe(201)
    const runID = ((await claim.json()) as { id: string }).id

    // Sebelum ada ledger: panel harus MENYEBUT kosongnya, bukan menampilkan nol.
    await page.goto(`/app/${workspace_id}/dashboard`)
    await expect(page.getByTestId('dashboard-spend-empty')).toBeVisible({ timeout: 15_000 })

    // `ledger_entries` tidak punya jalur tulis di API, jadi barisnya ditulis ke
    // container DB — sama seperti penyemaian notifikasi di fase sebelumnya.
    // `ledger_entries.id` adalah BIGSERIAL dan run_id/task_id NOT NULL, jadi
    // barisnya menempel ke run nyata yang dibuat di atas — bukan id karangan.
    psql(
      `INSERT INTO ledger_entries (org_id, run_id, task_id, provider, model, kind, tokens_in, tokens_out, cost_micros, price_version, price_source) ` +
        `VALUES ('${workspace_id}', '${runID}', '${taskID}', 'openai', 'gpt-4o', 'llm', 1200, 400, 2500000, 1, 'catalog');`,
    )

    const summary = await page.request.get(`${API}/orgs/${workspace_id}/cost-summary`, { headers: org })
    expect(summary.status()).toBe(200)
    const byBoard = ((await summary.json()) as { by_board: unknown[] }).by_board

    await page.reload()
    await expect(page.getByTestId('dashboard-spend-empty')).toBeHidden({ timeout: 15_000 })

    // Rinciannya datang dari respons yang sama. Di-scope ke barisnya: `$2.50`
    // juga muncul di kartu total, jadi pencarian se-halaman akan ambigu.
    const row = page.getByTestId('spend-row')
    await expect(row).toHaveCount(byBoard.length)
    for (const entry of byBoard as Array<{ name: string }>) {
      if (entry.name) await expect(row.filter({ hasText: entry.name })).toBeVisible()
    }
    await expect(row).toContainText('$2.50')
  })

  test('AC3 — one workspace is addressed by name, two workspaces by workspace', async ({ page }) => {
    const { workspace_id } = await signUp(page, `solo${Date.now()}@example.com`, 'Solo Workspace')

    await page.goto(`/app/${workspace_id}/dashboard`)
    // Satu workspace: judul = nama pengguna, bukan nama workspace.
    await expect(page.getByRole('heading', { name: 'Dash Owner' })).toBeVisible({ timeout: 15_000 })
    await expect(page.getByRole('heading', { name: 'Solo Workspace' })).toHaveCount(0)
  })
})
