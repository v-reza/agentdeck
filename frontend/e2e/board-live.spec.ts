import { expect, test, type Page } from '@playwright/test'

/**
 * US-AD39 (Must, M3) — papan board hidup tanpa reload.
 *
 * Kenapa test ini ada, dan kenapa ia berupa test dan bukan assertion lain:
 *
 *  1. Seluruh jalur SSE sudah ada sejak lama — `stream.ts` (158 baris),
 *     `use-sse-cache.ts`, dan endpoint `GET /boards/{id}/events` — tapi nol
 *     komponen memakainya. Jadi "streamnya jalan" tidak pernah diuji dari UI.
 *  2. Bentuk kegagalannya SENYAP. `EventSource` tersambung, frame datang, tail
 *     terisi, dan board tetap diam karena invalidasi tidak pernah sampai ke
 *     store. Tidak ada error, tidak ada log. Operator menyimpulkan realtime
 *     rusak padahal tidak ada satu pun bagian yang melaporkan gagal.
 *  3. Kegagalan itu benar-benar terjadi: versi pertama perbaikan ini menulis
 *     `invalidatesTags` pada endpoint ber-`queryFn`, yang tipenya `never` dan
 *     diabaikan RTK. Hanya `tsc` yang menangkapnya, dan tidak ada test lain
 *     yang bisa melihatnya.
 *
 * Yang membedakan test ini dari pemeriksaan source-level: satu klien membuat
 * task, klien lain harus melihatnya TANPA reload. Tidak ada `page.reload()` di
 * antara keduanya — kalau ada, test ini akan hijau pada implementasi yang sama
 * sekali tidak punya stream.
 *
 * Urutannya mengikat: indikator "Live" dipastikan terlihat LEBIH DULU, karena
 * itu satu-satunya bukti stream sudah terbuka. Membuat task sebelum stream
 * tersambung akan lolos di mesin cepat dan gagal di mesin lambat.
 *
 * Panggilan API memakai `page.request`, bukan `page.evaluate(fetch)` seperti
 * spec lain: `evaluate` sesudah `goto('/login')` kalah balapan dengan navigasi
 * SPA dan gagal sebagai "Execution context was destroyed" — persis yang terjadi
 * pada run pertama file ini. `page.request` berbagi cookie dengan konteks
 * browser, jadi sesinya tetap sama tanpa bergantung pada dokumen mana pun.
 */
const API = '/api/v1'
const PASSWORD = 'correct-horse-battery-staple'
const ULID = /[0-9A-HJKMNP-TV-Z]{26}/
const LIVE = /^(Live|Langsung)$/

async function signUp(page: Page): Promise<string> {
  const email = `e2e-sse-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
  const res = await page.request.post(`${API}/auth/register`, {
    data: { email, password: PASSWORD, name: 'SSE Owner', org_name: 'SSE Co' },
  })
  expect(res.status(), await res.text()).toBe(201)
  const workspace = (await res.json()) as { workspace_id: string }

  // The session cookie is issued `Secure`. Playwright's request context refuses
  // to send a Secure cookie over the plain-http test origin, so every
  // `page.request` call after registration answered 401 — while the browser
  // itself sent it without complaint, which is why the rest of the suite gets
  // away with page-level fetches. Re-adding the same value with `secure: false`
  // (the origin really is plain http) leaves one jar that both share. The
  // existing domain is reused rather than hardcoded.
  const [session] = (await page.context().cookies()).filter((c) => c.name === 'agentdeck_session')
  expect(session, 'registration did not set a session cookie').toBeTruthy()
  await page.context().addCookies([{ ...session, secure: false }])
  return workspace.workspace_id
}

/** Board dibuat lewat API supaya test tidak bergantung pada layar board-list. */
async function createBoard(page: Page, orgID: string): Promise<string> {
  const projectRes = await page.request.post(`${API}/projects`, {
    data: { name: 'SSE', slug: 'sse' },
  })
  expect(projectRes.status(), await projectRes.text()).toBe(201)
  const project = (await projectRes.json()) as { id: string }

  const boardRes = await page.request.post(`${API}/projects/${project.id}/boards`, {
    headers: { 'X-Org-ID': orgID },
    data: { name: 'SSE Board', slug: 'sse' },
  })
  expect(boardRes.status(), await boardRes.text()).toBe(201)
  return ((await boardRes.json()) as { id: string }).id
}

/** Task dibuat dari "klien lain" — request terpisah, bukan dari halaman. */
async function createTask(page: Page, orgID: string, boardID: string, title: string) {
  const res = await page.request.post(`${API}/boards/${boardID}/tasks`, {
    headers: { 'X-Org-ID': orgID },
    data: { title, status: 'backlog' },
  })
  expect(res.status(), await res.text()).toBe(201)
}

test.describe('the board is live from the event stream', () => {
  let orgID: string
  let boardID: string

  test.beforeEach(async ({ page }) => {
    orgID = await signUp(page)
    boardID = await createBoard(page, orgID)
  })

  test('the toolbar says the stream is live, and only then', async ({ page }) => {
    await page.goto(`/app/${orgID}/boards/${boardID}`)
    await expect(page).toHaveURL(new RegExp(`/app/${ULID.source}/boards/${ULID.source}$`))

    // The indicator is drawn only while the stream is actually open, so seeing
    // it is proof the subscription is up — not merely that nothing threw.
    await expect(page.getByText(LIVE)).toBeVisible()
  })

  test('a task created by another client appears without a reload', async ({ page }) => {
    await page.goto(`/app/${orgID}/boards/${boardID}`)
    // Order matters: the stream must be open before the event happens, or this
    // passes or fails on timing rather than on the feature.
    await expect(page.getByText(LIVE)).toBeVisible()

    await createTask(page, orgID, boardID, 'Streamed from another client')

    // Deliberately no reload: the card can only appear because the event
    // invalidated the board's task list and RTK refetched it.
    await expect(page.getByText('Streamed from another client')).toBeVisible()
  })

  test('the stream survives a burst of tasks', async ({ page }) => {
    await page.goto(`/app/${orgID}/boards/${boardID}`)
    await expect(page.getByText(LIVE)).toBeVisible()

    // A dispatcher finishing a run emits several events at once. Refreshes are
    // coalesced, so the board must still converge on every change rather than
    // refresh once and drop the rest.
    const titles = ['Burst one', 'Burst two', 'Burst three']
    for (const title of titles) await createTask(page, orgID, boardID, title)

    for (const title of titles) await expect(page.getByText(title)).toBeVisible()
  })
})
