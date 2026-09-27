import { expect, test, type Page } from '@playwright/test'

/**
 * US-AD34 (Must, M3) + US-AD35 (Must, M3) + US-AD33 — tab Approvals di drawer.
 *
 * Kenapa test ini ada:
 *
 *  1. `GET /api/v1/approvals` **tidak menerima `task_id`** — ia mengembalikan
 *     antrean seluruh organisasi. Jadi tab di dalam task harus menyaring sendiri,
 *     dan yang paling mudah salah adalah justru penyaringnya: satu approval milik
 *     task LAIN harus tetap tersembunyi. Itu asert yang paling berharga di sini,
 *     karena kesalahan tipe itu membuat tab menampilkan pekerjaan orang lain.
 *  2. `POST /approvals/{id}/reject` **menolak tanpa `reason`**. Tombol tolak
 *     karena itu mati sampai alasannya diisi — perilaku yang gampang "diperbaiki"
 *     jadi selalu hidup oleh orang yang tidak tahu servernya menuntut alasan.
 *  3. Menyetujui dari dalam drawer harus **kembali mengubah daftarnya**: kartunya
 *     hilang karena `decision` bukan lagi `pending`, dan itu diuji lewat UI.
 *  4. `viewer` tidak boleh melihat tombol keputusan sama sekali.
 *
 * Cara menyemainya sengaja lewat API, bukan lewat UI: `POST /tasks/{id}/approvals`
 * adalah jalur WORKER (US-AD33), tidak ada form di produk untuk itu. Menyemai
 * lewat API menguji hal yang benar — bacaan drawer atas data yang sudah ada.
 */
const API = '/api/v1'

const PASSWORD = 'correct-horse-battery-staple'

async function signUp(page: Page): Promise<string> {
  const email = `e2e-approvals-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
  const res = await page.request.post(`${API}/auth/register`, {
    data: { email, password: PASSWORD, name: 'Approval Owner', org_name: 'Approval Co' },
  })
  expect(res.status(), await res.text()).toBe(201)
  const [session] = (await page.context().cookies()).filter((c) => c.name === 'agentdeck_session')
  expect(session, 'registration did not set a session cookie').toBeTruthy()
  // Cookie sesi di-issue `Secure`, dan request context Playwright menolak
  // mengirimnya lewat http. Salinan non-Secure-nya dipasang di sini; catatan
  // lengkapnya ada di `board-live.spec.ts`.
  await page.context().addCookies([{ ...session, secure: false }])
  return ((await res.json()) as { workspace_id: string }).workspace_id
}

async function createBoard(page: Page, orgID: string): Promise<{ projectID: string; boardID: string }> {
  // Project dibuat tanpa `X-Org-ID`: sesi yang baru mendaftar sudah membawa org
  // aktifnya. `slug` wajib — tanpanya API menjawab "invalid input".
  const projectRes = await page.request.post(`${API}/projects`, { data: { name: 'Approvals', slug: 'approvals' } })
  expect(projectRes.status(), await projectRes.text()).toBe(201)
  const projectID = ((await projectRes.json()) as { id: string }).id

  const boardRes = await page.request.post(`${API}/projects/${projectID}/boards`, {
    headers: { 'X-Org-ID': orgID },
    data: { name: 'Approvals', slug: 'approvals' },
  })
  expect(boardRes.status(), await boardRes.text()).toBe(201)
  return { projectID, boardID: ((await boardRes.json()) as { id: string }).id }
}

/** Task yang sudah di-claim, plus satu approval tertunda di atasnya. */
async function seedHold(page: Page, orgID: string, projectID: string, boardID: string, title: string) {
  const agentRes = await page.request.post(`${API}/projects/${projectID}/agents`, {
    headers: { 'X-Org-ID': orgID },
    data: { name: `gate-${Date.now()}`, provider: 'openai_compatible', model: 'gpt-4o' },
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

  // Gate dipasang terhadap RUN yang sedang berjalan, jadi task-nya harus
  // di-claim lebih dulu: `approvals.run_id` NOT NULL dan FK ke `runs`, dan tanpa
  // claim `task.CurrentRunID` masih kosong -> API menolak sebagai invalid input.
  const claimRes = await page.request.post(`${API}/tasks/${taskID}/claim`, { headers: { 'X-Org-ID': orgID } })
  expect(claimRes.status(), `claim: ${await claimRes.text()}`).toBeLessThan(300)

  const holdRes = await page.request.post(`${API}/tasks/${taskID}/approvals`, {
    headers: { 'X-Org-ID': orgID },
    data: {
      preview_json: JSON.stringify({ action: 'rm -rf build', nested: { branch: 'main' } }),
      reason: 'mau hapus direktori build',
    },
  })
  expect(holdRes.status(), await holdRes.text()).toBe(201)
  const approval = (await holdRes.json()) as { id: string }

  return { taskID, approvalID: approval.id }
}

async function openTab(page: Page, orgID: string, boardID: string, title: string, tab: RegExp) {
  await page.goto(`/app/${orgID}/boards/${boardID}`)
  const card = page.getByText(title).first()
  await expect(card).toBeVisible({ timeout: 30_000 })
  await card.click()
  const drawer = page.getByRole('complementary', { name: 'Task Detail Drawer' })
  await drawer.getByRole('tab', { name: tab }).click()
  return drawer
}

test.describe('the approvals tab', () => {
  let orgID: string
  let projectID: string
  let boardID: string

  test.beforeEach(async ({ page }) => {
    orgID = await signUp(page)
    ;({ projectID, boardID } = await createBoard(page, orgID))
  })

  test('shows the pending hold for this task, verbatim, and hides another task’s', async ({ page }) => {
    const mine = await seedHold(page, orgID, projectID, boardID, 'Butuh persetujuan')
    // Task kedua dengan hold-nya sendiri: inilah yang membuktikan penyaringnya
    // bekerja. Dua-duanya ada di antrean org yang SAMA.
    await seedHold(page, orgID, projectID, boardID, 'Punya orang lain')

    const drawer = await openTab(page, orgID, boardID, 'Butuh persetujuan', /^(Approvals|Persetujuan)$/)

    const card = drawer.locator(`[data-approval-id="${mine.approvalID}"]`)
    await expect(card).toBeVisible({ timeout: 30_000 })

    // Payload dirender apa adanya — nilai bersarangnya harus terbaca, karena
    // itulah yang sedang disetujui.
    await expect(card).toContainText('rm -rf build')
    await expect(card).toContainText('branch')

    // Dan keputusan task lain tidak ikut terbawa ke sini.
    await expect(drawer.getByText('Punya orang lain')).toHaveCount(0)
    await expect(drawer.locator('[data-approval-id]')).toHaveCount(1)

    // Menyetujui lewat UI mengubah bacaan: kartunya hilang karena keputusannya
    // bukan lagi `pending`, dan itu datang dari invalidasi tag, bukan state lokal.
    await card.getByRole('button', { name: /^(Approve|Setujui)$/ }).click()
    await expect(drawer.locator('[data-approval-id]')).toHaveCount(0, { timeout: 15_000 })
    await expect(drawer.getByText(/Nothing is waiting|Tidak ada yang menunggu/)).toBeVisible()
  })

  test('cannot reject without a reason, because the API refuses one', async ({ page }) => {
    const mine = await seedHold(page, orgID, projectID, boardID, 'Tolak tanpa alasan')

    const drawer = await openTab(page, orgID, boardID, 'Tolak tanpa alasan', /^(Approvals|Persetujuan)$/)
    const card = drawer.locator(`[data-approval-id="${mine.approvalID}"]`)
    await expect(card).toBeVisible({ timeout: 30_000 })

    const reject = card.getByRole('button', { name: /^(Reject|Tolak)$/ })
    await expect(reject).toBeDisabled()

    await card.getByLabel(/^(Reason for rejecting|Alasan penolakan)$/).fill('branch-nya salah')
    await expect(reject).toBeEnabled()

    await reject.click()
    await expect(drawer.locator('[data-approval-id]')).toHaveCount(0, { timeout: 15_000 })
  })

  test('a viewer sees the hold but no decision controls', async ({ page }) => {
    // Role gate-nya membaca `admin`, sama dengan yang dituntut server (US-AD35
    // AC4). Yang diuji: VIEWER tidak melihat tombol sama sekali — bukan tombol
    // mati, karena kontrol yang tampil lalu menolak mengajarkan operator bahwa
    // app-nya rusak, bukan bahwa mereka tidak berwenang.
    //
    // Perannya diturunkan dengan men-stub `GET /auth/me`, cara yang sama dengan
    // `approvals.spec.ts`: pendaftar adalah owner, dan `ChangeRole` menolak
    // menurunkan owner terakhir (`ErrLastOwner`), jadi menurunkannya lewat API
    // memang mustahil. Yang diuji di sini hanya apa yang dilakukan layar dengan
    // peran yang diberikan padanya.
    const mine = await seedHold(page, orgID, projectID, boardID, 'Hanya lihat')
    await page.route(`**${API}/auth/me`, async (route) => {
      const response = await route.fetch()
      const me = (await response.json()) as { workspaces: { id: string; role: string }[] }
      for (const workspace of me.workspaces) workspace.role = 'viewer'
      await route.fulfill({ response, json: me })
    })

    const drawer = await openTab(page, orgID, boardID, 'Hanya lihat', /^(Approvals|Persetujuan)$/)
    const card = drawer.locator(`[data-approval-id="${mine.approvalID}"]`)
    await expect(card).toBeVisible({ timeout: 30_000 })
    await expect(card.getByRole('button', { name: /^(Approve|Setujui|Reject|Tolak)$/ })).toHaveCount(0)
    // Antreannya sendiri tetap terlihat: viewer boleh memantau.
    await expect(card).toContainText('rm -rf build')
  })
})
