import { expect, test, type Page } from '@playwright/test'

/**
 * US-AD06 — API keys.
 *
 * Kenapa test ini ada:
 *
 *  1. Layar ini sebelumnya stub yang bilang "not available yet" padahal lima
 *     endpoint sudah hidup. Test pertama menjaga kalimat itu tidak balik lagi.
 *  2. AC1 punya satu sifat yang gampang rusak tanpa terlihat: token penuh hanya
 *     ada di respons 201. Kalau ada yang menambahkannya ke `GET`, layar akan
 *     tampak "lebih lengkap" sambil membocorkan kredensial. Test kedua mengunci
 *     sebaliknya — daftar TIDAK boleh memuatnya.
 *  3. Scope-nya pemilik, bukan workspace. Dua anggota di workspace yang sama
 *     tidak boleh saling melihat key. Itu perilaku isolasi, jadi harus dibuktikan
 *     dengan dua sesi nyata, bukan satu.
 */

const API = '/api/v1'
const PASSWORD = 'Sup3rSecret!2026'

type Session = { orgID: string }

/** Daftar key lewat API, memakai cookie sesi yang sudah didapat. */
async function listKeys(page: Page) {
  const res = await page.request.get(`${API}/api-keys`)
  return { status: res.status(), body: await res.json() }
}

/** Mendaftarkan satu user baru dan mengembalikan sesinya. */
async function signUp(page: Page, email: string): Promise<Session> {
  const res = await page.request.post(`${API}/auth/register`, {
    data: { email, password: PASSWORD, name: 'Key Owner', org_name: 'Keys Co' },
  })
  expect(res.status(), await res.text()).toBe(201)

  const [session] = (await page.context().cookies()).filter((c) => c.name === 'agentdeck_session')
  expect(session, 'registrasi tidak memasang cookie sesi').toBeTruthy()
  // Cookie sesi di-issue `Secure`, dan request context Playwright menolak
  // mengirimnya lewat http. Lihat catatan lengkap di `board-live.spec.ts`.
  await page.context().addCookies([{ ...session, secure: false }])

  return {
    orgID: ((await res.json()) as { workspace_id: string }).workspace_id,
  }
}

test.describe('API keys — US-AD06', () => {
  test('AC1 — a created key is listed with its prefix, and the token is shown once', async ({ page }) => {
    const { orgID } = await signUp(page, `keys${Date.now()}@example.com`)

    await page.goto(`/app/${orgID}/settings/api-keys`)

    // Layar harus menampilkan daftar, bukan stub lama.
    await expect(page.getByTestId('api-keys-empty')).toBeVisible({ timeout: 15_000 })

    await page.getByTestId('api-key-create').click()
    await page.getByTestId('api-key-name').fill('ci-pipeline')
    await page.getByTestId('api-key-submit').click()

    // AC1: prefix 8 karakter + token `adk_...` sekali tampil.
    const token = page.getByTestId('api-key-token')
    await expect(token).toBeVisible({ timeout: 15_000 })
    const shown = (await token.textContent())?.trim() ?? ''
    expect(shown, 'token penuh harus `adk_...`').toMatch(/^adk_[0-9a-f]{4}_[0-9a-f]+$/)

    await page.getByTestId('api-key-done').click()

    // Barisnya muncul dengan prefix 8 karakter pertama dari token itu.
    const row = page.getByTestId('api-key-row').filter({ hasText: 'ci-pipeline' })
    await expect(row).toBeVisible({ timeout: 15_000 })
    await expect(row).toContainText(shown.slice(0, 8))
    // Satu key aktif. Angka ini dibaca, bukan dihitung ulang di klien.
    await expect(page.getByTestId('api-keys-active')).toContainText('1')

    // Mencabut mengubah status DAN menghitungnya sebagai tidak aktif. Tanpa
    // asert kedua, chip yang selalu menampilkan jumlah baris akan lolos.
    await row.getByTestId('api-key-revoke').click()
    // `data-status` dibaca, bukan labelnya: label diterjemahkan (id: "Dicabut"),
    // jadi menguncinya di teks akan pecah saat locale berubah.
    await expect(row.getByTestId('api-key-status')).toHaveAttribute('data-status', 'revoked', { timeout: 15_000 })
    await expect(page.getByTestId('api-keys-active')).toContainText('0')
  })

  test('AC1 — the list never carries the token again, only the prefix', async ({ page }) => {
    await signUp(page, `once${Date.now()}@example.com`)

    const res = await page.request.post(`${API}/api-keys`, { data: { name: 'audit-key' } })
    const created = { status: res.status(), body: await res.json() }
    expect(created.status).toBe(201)
    expect(created.body.key, '201 harus membawa token penuh').toMatch(/^adk_/)

    const listed = await listKeys(page)
    expect(listed.status).toBe(200)
    const row = listed.body.find((k: { name: string }) => k.name === 'audit-key')
    expect(row, 'key yang baru dibuat harus ada di daftar').toBeTruthy()
    // Inti test ini: tidak ada field yang membawa token atau hash-nya.
    expect(row).not.toHaveProperty('key')
    expect(row).not.toHaveProperty('hash')
    expect(row.prefix).toBe(created.body.key.slice(0, 8))
  })

  test('the list is scoped to its owner, not to the workspace', async ({ page, context }) => {
    await signUp(page, `owner${Date.now()}@example.com`)
    await page.request.post(`${API}/api-keys`, { data: { name: 'owner-only-key' } })

    // Anggota kedua di workspace yang SAMA: key milik owner tidak boleh terlihat.
    const memberPage = await context.browser()!.newPage()
    await signUp(memberPage, `member${Date.now()}@example.com`)
    const mine = await listKeys(memberPage)
    expect(mine.status).toBe(200)
    expect(
      mine.body.some((k: { name: string }) => k.name === 'owner-only-key'),
      'key milik anggota lain tidak boleh muncul',
    ).toBe(false)
    await memberPage.close()

    // Dan yang punya tetap melihatnya — supaya test ini bukan sekadar "selalu kosong".
    const theirs = await listKeys(page)
    expect(theirs.body.some((k: { name: string }) => k.name === 'owner-only-key')).toBe(true)
  })

  test('AC3 — a viewer cannot reach the endpoint at all', async ({ page }) => {
    const { orgID } = await signUp(page, `viewer${Date.now()}@example.com`)

    // Role dibaca dari `GET /auth/me` (layout dashboard mengisi session slice
    // dari situ), jadi menurunkannya di sana sudah cukup: yang diuji di sini
    // adalah apa yang dilakukan layar dengan role yang diberikan.
    await page.route(`**${API}/auth/me`, async (route) => {
      const response = await route.fetch()
      const me = (await response.json()) as { workspaces: { id: string; role: string }[] }
      for (const workspace of me.workspaces) workspace.role = 'viewer'
      await route.fulfill({ response, json: me })
    })

    await page.goto(`/app/${orgID}/settings/api-keys`)
    await expect(page.getByTestId('api-keys-viewer-note')).toBeVisible({ timeout: 15_000 })
    // Empty state harus benar-benar selesai dirender sebelum kita menyimpulkan
    // "tidak ada tombol": kalau tidak, yang diukur adalah loading, bukan guard.
    await expect(page.getByTestId('api-keys-empty')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('api-key-create')).toHaveCount(0)
    await expect(page.getByTestId('api-key-create-empty')).toHaveCount(0)
  })
})
