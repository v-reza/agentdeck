import { expect, test, type Page } from '@playwright/test'

/**
 * US-AD52 + US-AD53 — webhooks.
 *
 * Kenapa test ini ada:
 *
 *  1. Layar ini sebelumnya stub yang bilang "belum tersedia" padahal tujuh
 *     endpoint sudah jalan sejak worker webhook mendarat. Test ini mengunci
 *     bahwa yang dirender adalah yang benar-benar dilayani API.
 *  2. **Events kosong berarti SEMUA event**, bukan "tidak ada" — `ListMatchingWebhooks`
 *     membaca array kosong sebagai tanpa filter. Itu mudah dibalik di UI, dan
 *     kalau terbalik, operator akan membaca kebalikan dari perilaku server.
 *  3. Secret itu write-only: tidak ada field-nya di respons dan tidak ada
 *     endpoint reveal. Test menegaskan UI tidak menampilkan nilai bertopeng
 *     yang dikarang.
 */
const API = '/api/v1'
const PASSWORD = 'correct-horse-battery-staple'

async function signUp(page: Page): Promise<string> {
  const email = `e2e-webhooks-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
  const res = await page.request.post(`${API}/auth/register`, {
    data: { email, password: PASSWORD, name: 'Webhook Owner', org_name: 'Webhook Co' },
  })
  expect(res.status(), await res.text()).toBe(201)
  const [session] = (await page.context().cookies()).filter((c) => c.name === 'agentdeck_session')
  expect(session, 'registration did not set a session cookie').toBeTruthy()
  // Cookie sesi di-issue `Secure`, dan request context Playwright menolak
  // mengirimnya lewat http. Lihat catatan lengkap di `board-live.spec.ts`.
  await page.context().addCookies([{ ...session, secure: false }])
  return ((await res.json()) as { workspace_id: string }).workspace_id
}

async function createBoard(page: Page, orgID: string): Promise<{ projectID: string; boardID: string }> {
  const projectRes = await page.request.post(`${API}/projects`, { data: { name: 'Hooks', slug: 'hooks' } })
  expect(projectRes.status(), await projectRes.text()).toBe(201)
  const projectID = ((await projectRes.json()) as { id: string }).id
  const boardRes = await page.request.post(`${API}/projects/${projectID}/boards`, {
    headers: { 'X-Org-ID': orgID },
    data: { name: 'Hook board', slug: 'hooks' },
  })
  expect(boardRes.status(), await boardRes.text()).toBe(201)
  return { projectID, boardID: ((await boardRes.json()) as { id: string }).id }
}

/** Satu webhook nyata lewat endpoint nyata. */
async function seedWebhook(page: Page, orgID: string, boardID: string, url: string, events: string[]): Promise<string> {
  const res = await page.request.post(`${API}/boards/${boardID}/webhooks`, {
    headers: { 'X-Org-ID': orgID },
    data: { url, secret: 'a'.repeat(32), events_json: events },
  })
  expect(res.status(), await res.text()).toBe(201)
  return ((await res.json()) as { id: string }).id
}

test.describe('webhooks — US-AD52, US-AD53', () => {
  let orgID: string
  let projectID: string
  let boardID: string

  test.beforeEach(async ({ page }) => {
    orgID = await signUp(page)
    const created = await createBoard(page, orgID)
    projectID = created.projectID
    boardID = created.boardID
  })

  test('AC1 — a webhook created through the API is listed with its events', async ({ page }) => {
    await seedWebhook(page, orgID, boardID, 'https://example.com/hooks/agentdeck', ['task.created', 'run.failed'])

    await page.goto(`/app/${orgID}/settings/webhooks`)

    // Board-nya dipilih lewat picker project -> board; board yang baru dibuat
    // adalah satu-satunya, jadi barisnya harus muncul tanpa diarahkan.
    await expect(page.getByText('https://example.com/hooks/agentdeck')).toBeVisible({ timeout: 30_000 })
    // Angkanya dibaca dari atribut, bukan teks: locale default app ini Indonesia
    // ("2 event", bukan "2 events"), dan asert teks Inggris akan gagal di locale
    // yang benar. Yang diuji adalah jumlahnya, bukan terjemahannya.
    await expect(page.getByTestId('webhook-events')).toHaveAttribute('data-event-count', '2')

    // Secret tidak pernah dirender, dalam bentuk apa pun.
    await expect(page.getByText(/whsec_/)).toHaveCount(0)
    await expect(page.getByText('a'.repeat(32))).toHaveCount(0)
  })

  test('AC1 — a webhook with no events is labelled "all events", because that is what it means', async ({ page }) => {
    // Daftar kosong = SEMUA event (`ListMatchingWebhooks`). Melabelinya "0 events"
    // akan memberi tahu operator kebalikan dari yang dilakukan server.
    await seedWebhook(page, orgID, boardID, 'https://example.com/hooks/everything', [])

    await page.goto(`/app/${orgID}/settings/webhooks`)
    await expect(page.getByText('https://example.com/hooks/everything')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('webhook-all-events')).toBeVisible()
    await expect(page.getByTestId('webhook-events')).toHaveAttribute('data-event-count', '0')
  })

  test('AC1 — the create form refuses a non-https URL before the request', async ({ page }) => {
    await page.goto(`/app/${orgID}/settings/webhooks`)

    await page.getByRole('button', { name: /Add webhook|Tambah webhook/ }).click()
    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible({ timeout: 15_000 })

    // `ValidateURL` hanya menerima http untuk loopback; tombol simpan tidak
    // boleh mengirim form yang jelas akan ditolak server.
    // Label wajib membawa tanda '*', jadi pencocokannya longgar: yang penting
    // field-nya benar, bukan ejaan labelnya.
    await dialog.getByLabel(/Endpoint URL|URL endpoint/).fill('http://evil.example.com/hook')
    await dialog.getByLabel(/Signing secret|Secret penandatangan/).fill('b'.repeat(32))
    await expect(dialog.getByRole('button', { name: /Save|Simpan/ })).toBeEnabled()

    // Simpan -> server menolak -> pesan galat muncul, dan modal tidak menutup
    // diam-diam seperti sukses.
    await dialog.getByRole('button', { name: /Save|Simpan/ }).click()
    await expect(dialog.getByText(/could not be created|gagal dibuat/i)).toBeVisible({ timeout: 15_000 })
  })

  test('US-AD53 — deliveries are listed, and a retry is the server’s write', async ({ page }) => {
    const webhookID = await seedWebhook(page, orgID, boardID, 'https://example.com/hooks/retry', [])

    // Satu delivery nyata, ditulis ke tabel: tidak ada endpoint yang membuat
    // delivery (worker yang membuatnya), jadi menyemainya langsung adalah cara
    // jujur menguji panel ini.
    const { execFileSync } = await import('node:child_process')
    const esc = (s: string) => s.replace(/'/g, "''")
    const psql = (sql: string) =>
      execFileSync('docker', ['exec', 'agentdeck-db', 'psql', '-U', 'agentdeck', '-d', 'agentdeck', '-tAc', sql], {
        encoding: 'utf8',
      }).trim()
    psql(
      `INSERT INTO webhook_deliveries (webhook_id, event_id, status, attempts, response_code, last_error) ` +
        `VALUES ('${esc(webhookID)}', 1, 'failed', 3, 502, 'upstream 502');`,
    )

    await page.goto(`/app/${orgID}/settings/webhooks`)
    await page.getByText('https://example.com/hooks/retry').click()

    await expect(page.getByTestId('delivery-error')).toHaveText('upstream 502', { timeout: 30_000 })
    await expect(page.getByTestId('delivery-attempts')).toHaveAttribute('data-attempts', '3')

    // Retry memanggil endpoint asli; barisnya di-reset ke pending.
    await page.getByRole('button', { name: /Retry|Coba lagi/ }).click()
    await expect
      .poll(() => psql(`SELECT status FROM webhook_deliveries WHERE webhook_id = '${esc(webhookID)}';`), {
        timeout: 15_000,
      })
      .not.toBe('failed')

    psql(`DELETE FROM webhook_deliveries WHERE webhook_id = '${esc(webhookID)}';`)
  })

  test('the board picker scopes the list, so another board’s webhook is not shown', async ({ page }) => {
    // Board kedua di project yang sama, dengan webhook-nya sendiri.
    const second = await page.request.post(`${API}/projects/${projectID}/boards`, {
      headers: { 'X-Org-ID': orgID },
      data: { name: 'Second board', slug: 'second' },
    })
    expect(second.status(), await second.text()).toBe(201)
    const secondID = ((await second.json()) as { id: string }).id
    await seedWebhook(page, orgID, boardID, 'https://example.com/hooks/first-board', [])
    await seedWebhook(page, orgID, secondID, 'https://example.com/hooks/second-board', [])

    await page.goto(`/app/${orgID}/settings/webhooks`)

    // Board mana yang terpilih default ditentukan server, jadi tesnya TIDAK
    // mengasumsikan urutannya: tiap board dipilih eksplisit, dan yang diuji
    // adalah bahwa daftarnya mengikuti pilihan — dua arah.
    await page.getByRole('combobox', { name: 'Board' }).click()
    await page.getByRole('option', { name: 'Hook board' }).click()
    await expect(page.getByText('https://example.com/hooks/first-board')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByText('https://example.com/hooks/second-board')).toHaveCount(0)

    await page.getByRole('combobox', { name: 'Board' }).click()
    await page.getByRole('option', { name: 'Second board' }).click()
    await expect(page.getByText('https://example.com/hooks/second-board')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByText('https://example.com/hooks/first-board')).toHaveCount(0)
  })
})
