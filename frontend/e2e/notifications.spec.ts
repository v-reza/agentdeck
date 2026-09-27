import { execFileSync } from 'node:child_process'
import { expect, test, type Page } from '@playwright/test'

/**
 * US-AD61 — notifikasi in-app: bell + badge (AC1), navigasi (AC2), tahan putus
 * (AC3), isolasi tenant (AC4).
 *
 * Kenapa barisnya ditulis ke Postgres, bukan di-stub:
 *
 *   AC3 berbunyi "jika kanal realtime putus, notifikasi tetap tersimpan dan
 *   tampil saat reconnect". Itu hanya bisa diuji kalau barisnya benar-benar
 *   TERSIMPAN di suatu tempat yang bertahan. Kalau `GET /notifications`
 *   di-stub, yang diuji cuma JSON buatan tes — bukan ketahanan apa pun. Jadi
 *   barisnya ditulis langsung ke tabel `notifications` container, dan layarnya
 *   membaca endpoint asli.
 *
 *   Satu-satunya endpoint notifikasi adalah GET (daftar) dan POST read; tidak
 *   ada POST create, dan dispatcher yang memproduksinya TIDAK punya route tick.
 *   Jadi menulis baris ke DB adalah satu-satunya cara menyemai tanpa memalsukan
 *   jawaban server.
 *
 * Kenapa barisnya dihapus lagi:
 *
 *   `notifications` tidak punya FK ke task, jadi baris sisa akan menumpuk di
 *   database dev dan badge pengguna berikutnya menghitungnya. Pembersihan
 *   memakai `user_id` yang unik per tes, jadi ia tidak bisa menyentuh baris
 *   milik siapa pun.
 */
const API = '/api/v1'
const PASSWORD = 'correct-horse-battery-staple'
const DB_CONTAINER = 'agentdeck-db'

/**
 * ULID 26 karakter, dibuat di sisi tes.
 *
 * `notifications.id` adalah `text` dengan CHECK `char_length(id) = 26`; tidak ada
 * `generate_ulid()` di database (fungsi itu milik aplikasi, bukan Postgres).
 * Yang dibutuhkan baris ini cuma unik dan panjangnya benar.
 */
const CROCKFORD = '0123456789ABCDEFGHJKMNPQRSTVWXYZ'
function ulid(): string {
  let out = ''
  for (let i = 0; i < 26; i += 1) out += CROCKFORD[Math.floor(Math.random() * 32)]
  return out
}

/** Jalankan satu statement psql di container DB dan kembalikan stdout-nya. */
function psql(sql: string): string {
  return execFileSync('docker', ['exec', DB_CONTAINER, 'psql', '-U', 'agentdeck', '-d', 'agentdeck', '-tAc', sql], {
    encoding: 'utf8',
  }).trim()
}

async function signUp(page: Page): Promise<{ orgID: string; userID: string }> {
  const email = `e2e-notif-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
  const res = await page.request.post(`${API}/auth/register`, {
    data: { email, password: PASSWORD, name: 'Notif Owner', org_name: 'Notif Co' },
  })
  expect(res.status(), await res.text()).toBe(201)
  const [session] = (await page.context().cookies()).filter((c) => c.name === 'agentdeck_session')
  expect(session, 'registration did not set a session cookie').toBeTruthy()
  // Cookie sesi di-issue `Secure`, dan request context Playwright menolak
  // mengirimnya lewat http. Lihat catatan lengkap di `board-live.spec.ts`.
  await page.context().addCookies([{ ...session, secure: false }])
  const body = (await res.json()) as { workspace_id: string; user_id: string }
  return { orgID: body.workspace_id, userID: body.user_id }
}

/** Satu baris notifikasi nyata, langsung di tabel. */
function seedNotification(
  userID: string,
  orgID: string,
  kind: string,
  title: string,
  body: string,
  targetType: string,
  targetID: string,
): void {
  const esc = (s: string) => s.replace(/'/g, "''")
  psql(
    `INSERT INTO notifications (id, user_id, org_id, kind, title, body, target_type, target_id) ` +
      `VALUES ('${ulid()}', '${esc(userID)}', '${esc(orgID)}', '${esc(kind)}', '${esc(title)}', ` +
      `'${esc(body)}', '${esc(targetType)}', '${esc(targetID)}');`,
  )
}

function cleanup(userID: string): void {
  psql(`DELETE FROM notifications WHERE user_id = '${userID.replace(/'/g, "''")}';`)
}

test.describe('notification centre — US-AD61', () => {
  let orgID: string
  let userID: string

  test.beforeEach(async ({ page }) => {
    const session = await signUp(page)
    orgID = session.orgID
    userID = session.userID
  })

  test.afterEach(() => {
    if (userID) cleanup(userID)
  })

  test('AC1 + AC2 — the bell counts unread rows, and a row leads to what it is about', async ({ page }) => {
    // Dua belum dibaca + satu sudah dibaca: badge harus berbunyi 2, dan baris
    // yang sudah dibaca tidak boleh ikut menghitung.
    const runID = `01${Date.now().toString().padStart(24, '0')}`
    seedNotification(userID, orgID, 'run.failed', 'Run gagal: Refactor auth', 'Kegagalan exit_code.', 'run', runID)
    seedNotification(userID, orgID, 'budget.warning', 'Anggaran 82%', 'Board Sprint 24.', 'board', runID)
    seedNotification(userID, orgID, 'credential.invalid', 'Kredensial kedaluwarsa', '', 'board', runID)
    psql(`UPDATE notifications SET read_at = now() WHERE user_id = '${userID}' AND kind = 'credential.invalid';`)

    await page.goto(`/app/${orgID}/notifications`)

    // AC1: badge di topbar. Angkanya dari server, bukan dari panjang daftar.
    const badge = page.getByTestId('notification-badge')
    await expect(badge).toBeVisible({ timeout: 30_000 })
    await expect(badge).toHaveText('2')

    // Isinya nyata, dan yang sudah dibaca tidak mendapat chip "baru".
    await expect(page.getByText('Run gagal: Refactor auth')).toBeVisible()
    await expect(page.getByText('Anggaran 82%')).toBeVisible()
    await expect(page.getByText('baru')).toHaveCount(2)

    // AC2: barisnya menuju ke targetnya. `run` membawa ke layar detail run.
    const link = page.getByRole('link', { name: 'Run gagal: Refactor auth' })
    await expect(link).toHaveAttribute('href', new RegExp(`/runs/${runID}$`))
  })

  test('AC3 — a notification survives the channel being gone, because it is a row', async ({ page }) => {
    seedNotification(userID, orgID, 'run.failed', 'Gagal saat kanal putus', 'Body.', 'board', '01JUNK')

    // Kanal putus: setiap request notifikasi digagalkan, persis seperti offline.
    let offline = true
    await page.route(`**${API}/notifications*`, async (route) => {
      if (offline) return route.abort('failed')
      return route.continue()
    })

    await page.goto(`/app/${orgID}/notifications`)
    await expect(page.getByText('Gagal saat kanal putus')).toHaveCount(0)

    // Kanal pulih. Barisnya tidak pernah hilang: ia ada di Postgres, bukan di
    // stream, jadi reconnect cuma berarti fetch berikutnya berhasil.
    offline = false
    await page.reload()
    await expect(page.getByText('Gagal saat kanal putus')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('notification-badge')).toHaveText('1')
  })

  test('AC1 — marking read clears the badge, and the write is the server’s', async ({ page }) => {
    seedNotification(userID, orgID, 'run.failed', 'Satu saja', 'Body.', 'board', '01JUNK')

    await page.goto(`/app/${orgID}/notifications`)
    await expect(page.getByTestId('notification-badge')).toHaveText('1', { timeout: 30_000 })

    await page.getByRole('button', { name: /^(Mark all read|Tandai semua terbaca)$/ }).click()
    await expect(page.getByTestId('notification-badge')).toHaveCount(0)

    // Bukti bahwa yang berubah adalah BARISNYA, bukan layarnya: hitung langsung
    // di database.
    expect(psql(`SELECT count(*) FROM notifications WHERE user_id = '${userID}' AND read_at IS NULL;`)).toBe('0')
  })

  test('AC4 — another user’s notification never appears', async ({ page, browser }) => {
    // Pengguna kedua, org kedua. Barisnya nyata dan belum dibaca.
    const other = await browser.newContext()
    const otherPage = await other.newPage()
    const second = await signUp(otherPage)
    seedNotification(second.userID, second.orgID, 'run.failed', 'Milik orang lain', 'Body.', 'board', '01JUNK')
    await other.close()

    // Pengguna pertama membuka pusat notifikasinya sendiri.
    seedNotification(userID, orgID, 'run.failed', 'Milik saya', 'Body.', 'board', '01JUNK')
    await page.goto(`/app/${orgID}/notifications`)
    await expect(page.getByText('Milik saya')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByText('Milik orang lain')).toHaveCount(0)
    await expect(page.getByTestId('notification-badge')).toHaveText('1')

    cleanup(second.userID)
  })
})
