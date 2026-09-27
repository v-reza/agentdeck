import { expect, test, type Page } from '@playwright/test'

/**
 * US-AD34 + US-AD35 — layar detail approval (`/approvals/:id`).
 *
 * Kenapa test ini ada:
 *
 *  1. `/approvals/:id` menumpang antrean di `/approvals`: rute bersarang tanpa
 *     `index` berarti URL detail tidak boleh menelan antreannya, dan sebaliknya.
 *     Yang paling gampang salah adalah jalur kembali — karena itu `Queue` diuji.
 *  2. Layar ini membaca `GET /approvals/{id}`, yang selama ini **tidak punya
 *     pemanggil** walau kliennya sudah ada. Test ini membuat klaim itu berhenti
 *     jadi klaim.
 *  3. Setelah diputus, layar harus berubah dari "menunggu keputusan" jadi
 *     "riwayat keputusan" — termasuk alasan penolakan. Itu bacaan yang berbeda,
 *     bukan hiasan.
 *  4. Alasan tolak: server menuntut non-kosong, design menulis "minimum 10
 *     karakter". Test ini menyematkan yang SERVER terima (alasan pendek sah),
 *     supaya aturan mockup tidak diam-diam ikut masuk.
 */
const API = '/api/v1'
const PASSWORD = 'correct-horse-battery-staple'

async function signUp(page: Page): Promise<string> {
  const email = `e2e-detail-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
  const res = await page.request.post(`${API}/auth/register`, {
    data: { email, password: PASSWORD, name: 'Detail Owner', org_name: 'Detail Co' },
  })
  expect(res.status(), await res.text()).toBe(201)
  const [session] = (await page.context().cookies()).filter((c) => c.name === 'agentdeck_session')
  expect(session, 'registration did not set a session cookie').toBeTruthy()
  await page.context().addCookies([{ ...session, secure: false }])
  return ((await res.json()) as { workspace_id: string }).workspace_id
}

async function createBoard(page: Page, orgID: string) {
  const projectRes = await page.request.post(`${API}/projects`, { data: { name: 'Detail', slug: 'detail' } })
  expect(projectRes.status(), await projectRes.text()).toBe(201)
  const projectID = ((await projectRes.json()) as { id: string }).id
  const boardRes = await page.request.post(`${API}/projects/${projectID}/boards`, {
    headers: { 'X-Org-ID': orgID },
    data: { name: 'Detail', slug: 'detail' },
  })
  expect(boardRes.status(), await boardRes.text()).toBe(201)
  return { projectID, boardID: ((await boardRes.json()) as { id: string }).id }
}

/** Task ter-claim plus satu gate tertunda di atasnya. */
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
  const claimRes = await page.request.post(`${API}/tasks/${taskID}/claim`, { headers: { 'X-Org-ID': orgID } })
  expect(claimRes.status(), `claim: ${await claimRes.text()}`).toBeLessThan(300)

  const holdRes = await page.request.post(`${API}/tasks/${taskID}/approvals`, {
    headers: { 'X-Org-ID': orgID },
    data: {
      preview_json: JSON.stringify({ tool: 'infra_cli', nested: { action: 'drop_replica', zone: 'prod-sg-1' } }),
      reason: 'menghapus replika yang sudah usang',
    },
  })
  expect(holdRes.status(), await holdRes.text()).toBe(201)
  const approval = (await holdRes.json()) as { id: string }
  return { taskID, approvalID: approval.id, title }
}

test.describe('the approval detail screen', () => {
  let orgID: string
  let projectID: string
  let boardID: string

  test.beforeEach(async ({ page }) => {
    orgID = await signUp(page)
    ;({ projectID, boardID } = await createBoard(page, orgID))
  })

  test('reaches the screen from the queue and shows the gated action verbatim', async ({ page }) => {
    const hold = await seedHold(page, orgID, projectID, boardID, 'Hapus replika usang')

    await page.goto(`/app/${orgID}/approvals`)
    // Kartu antrean adalah jalan masuknya (design: Antrean / <id> / Detail).
    // `aria-label` kartunya adalah `gate_mode + short id`, jadi nama aksesibelnya
    // dimulai dengan mode gerbangnya — bukan dengan kata "task".
    const entry = page.getByRole('link', { name: /^(require|auto)\b/ })
    await expect(entry).toBeVisible({ timeout: 30_000 })
    await entry.click()

    await expect(page).toHaveURL(new RegExp(`/approvals/${hold.approvalID}$`))

    // Konteksnya nyata: judul task yang di-hold, pemohon, dan aksi yang ditahan.
    await expect(page.getByText('Hapus replika usang')).toBeVisible({ timeout: 30_000 })
    // Payload-nya diperiksa di elemen `pre`-nya, bukan lewat getByText: DiffViewer
    // menulis label "tool: infra_cli" di header DAN isi payloadnya, jadi teks itu
    // sengaja muncul dua kali dan asert yang longgar akan ambigu.
    const payload = page.locator('pre').first()
    await expect(payload).toContainText('infra_cli')
    // Bersarang harus terbaca utuh — inilah yang sedang disetujui.
    await expect(payload).toContainText('drop_replica')
    await expect(payload).toContainText('prod-sg-1')

    // Jalur kembali: rute detail menumpang antrean, jadi Queue harus membawa
    // kembali ke /approvals, bukan ke suatu tempat yang tidak ada.
    await page.getByRole('link', { name: /^(Queue|Antrean)$/ }).click()
    await expect(page).toHaveURL(new RegExp(`/approvals$`))
  })

  test('a rejection with a short reason is accepted, because the server accepts it', async ({ page }) => {
    const hold = await seedHold(page, orgID, projectID, boardID, 'Tolak dengan alasan pendek')

    await page.goto(`/app/${orgID}/approvals/${hold.approvalID}`)
    const reject = page.getByRole('button', { name: /^(Reject|Tolak)$/ })
    await expect(reject).toBeVisible({ timeout: 30_000 })
    await expect(reject).toBeDisabled()

    // Enam karakter. Design menyebut minimum 10; server hanya menuntut non-kosong
    // setelah TrimSpace. Kalau aturan mockup itu ikut masuk, tombolnya akan tetap
    // mati di sini dan tes ini gagal.
    const shortReason = 'nggak'
    await page.getByLabel(/^(Reason for rejecting|Alasan penolakan)$/).fill(shortReason)
    await expect(reject).toBeEnabled()
    await reject.click()

    // Setelah diputus, layarnya berganti jadi riwayat keputusan: hasil, pemutus,
    // dan alasan penolakannya.
    await expect(page.getByText(/^(Outcome|Hasil)$/)).toBeVisible({ timeout: 30_000 })
    await expect(page.getByText('rejected')).toBeVisible()
    // Persis sekali: panel aksi tidak lagi mengulangnya dengan label yang salah.
    await expect(page.getByText(shortReason)).toHaveCount(1)
    // Dan form keputusannya tidak lagi ditawarkan.
    await expect(page.getByRole('button', { name: /^(Approve & run|Setujui & jalankan)$/ })).toHaveCount(0)
  })
})
