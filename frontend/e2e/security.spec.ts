import { expect, test, type Page } from '@playwright/test'

/**
 * US-AD90 — change password and active sessions.
 *
 * AC1 is the reason this file uses TWO browser contexts: "the other sessions are
 * revoked and this one stays" cannot be observed from a single session. A second
 * context signs in as the same user, the first changes the password, and then the
 * second must be refused while the first keeps working. Asserting only the 204
 * would pass on an implementation that revoked nothing.
 *
 * AC3's "and changes nothing" is checked the same way — by proving the OLD
 * password still signs in after the refusal. A 401 alone is compatible with a
 * handler that wrote the new hash first and reported failure after.
 *
 * WHAT THE API DOES NOT HAVE, and is therefore not tested: a bulk "revoke all
 * other sessions" endpoint. `DELETE /auth/sessions/{id}` takes one id, and the
 * list only carries the caller's own rows, so the design's bulk button has no
 * route behind it. The password change IS the bulk revoke, and that is what AC1
 * above exercises.
 */

const API = '/api/v1'
const PASSWORD = 'Sup3rSecret!2026'
const NEW_PASSWORD = 'Ev3nBetter!2026x'

async function register(page: Page, email: string): Promise<string> {
  const res = await page.request.post(`${API}/auth/register`, {
    data: { email, password: PASSWORD, name: 'Security Owner' },
  })
  const body = await res.text()
  expect(res.status(), body).toBe(201)

  const [session] = (await page.context().cookies()).filter((c) => c.name === 'agentdeck_session')
  expect(session, 'registration did not set a session cookie').toBeTruthy()
  await page.context().addCookies([{ ...session, secure: false }])
  return (JSON.parse(body) as { workspace_id: string }).workspace_id
}

/**
 * Sign a second context in as the same user, over the plain-http test origin.
 *
 * The landing on `/login` is part of the helper, not of the caller: `api()` runs
 * an in-page `fetch` with a RELATIVE url, and a fresh context sits on
 * `about:blank` where that URL fails to parse — every later assertion would read
 * as a broken API rather than a broken session. Any context this returns is ready
 * to make calls.
 */
async function signIn(page: Page, email: string, password: string): Promise<number> {
  const res = await page.request.post(`${API}/auth/login`, { data: { email, password } })
  const [session] = (await page.context().cookies()).filter((c) => c.name === 'agentdeck_session')
  if (session) await page.context().addCookies([{ ...session, secure: false }])
  await page.goto('/login')
  return res.status()
}

async function api<T>(
  page: Page,
  method: 'GET' | 'POST' | 'DELETE',
  path: string,
  body?: unknown,
): Promise<{ status: number; data: T; text: string }> {
  return page.evaluate(
    async ({ api, method, path, body }) => {
      const response = await fetch(`${api}${path}`, {
        method,
        headers: body ? { 'Content-Type': 'application/json' } : undefined,
        credentials: 'include',
        body: body ? JSON.stringify(body) : undefined,
      })
      const text = await response.text()
      let data: unknown = null
      try {
        data = text ? JSON.parse(text) : null
      } catch {
        data = null
      }
      return { status: response.status, data: data as never, text }
    },
    { api: API, method, path, body },
  )
}

interface Session {
  id: string
  user_agent: string
  ip: string
  last_seen_at: string
  created_at: string
  current: boolean
}

test.describe('security — US-AD90', () => {
  test('AC2 — the list names the device, the IP, the last activity, and which one is this device', async ({ page }) => {
    const email = `e2e-sec-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
    const orgID = await register(page, email)
    await page.goto(`/app/${orgID}/settings/security`)

    const list = page.getByTestId('session-list')
    await expect(list).toBeVisible({ timeout: 15_000 })

    // Exactly one row, and it is the current device — a fresh account has one
    // session, so "N rows" here is a fact about this run.
    await expect(page.getByTestId('session-row')).toHaveCount(1)
    await expect(page.getByTestId('session-current')).toBeVisible()
    await expect(page.getByTestId('session-row')).toHaveAttribute('data-current', 'true')

    // The current row offers no revoke button: it is the session rendering it.
    await expect(page.getByTestId('session-revoke')).toHaveCount(0)

    // The raw user agent is printed, and it is the browser's — not a placeholder.
    const raw = await page.getByTestId('session-row').locator('span.font-mono').first().textContent()
    expect(raw?.length ?? 0).toBeGreaterThan(10)

    // And the API carries all four fields AC2 names.
    const listed = await api<Session[]>(page, 'GET', '/auth/sessions')
    expect(listed.status, listed.text).toBe(200)
    const row = listed.data[0]
    expect(row.current).toBe(true)
    expect(row).toHaveProperty('user_agent')
    expect(row).toHaveProperty('ip')
    expect(row).toHaveProperty('last_seen_at')
  })

  test('AC1/AC3 — a wrong password is refused and changes nothing; a right one revokes the other session only', async ({
    page,
    browser,
  }) => {
    const email = `e2e-sec2-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
    const orgID = await register(page, email)

    // A second, independent session for the same user. Its own context, so it has
    // its own cookie jar and its own row in `sessions`.
    const otherContext = await browser.newContext()
    const otherPage = await otherContext.newPage()
    expect(await signIn(otherPage, email, PASSWORD), 'the second session must sign in').toBe(200)

    await page.goto(`/app/${orgID}/settings/security`)
    const mine = await api<Session[]>(page, 'GET', '/auth/sessions')
    expect(mine.data.length, 'two sessions before the change').toBe(2)

    // AC3 first: the wrong current password is a 401 and the form says so.
    await page.getByTestId('security-old-password').fill('definitely-not-the-password')
    await page.getByTestId('security-new-password').fill(NEW_PASSWORD)
    await page.getByTestId('security-confirm-password').fill(NEW_PASSWORD)
    await page.getByTestId('security-save').click()
    await expect(page.getByTestId('security-error')).toHaveText('Kata sandi saat ini salah', { timeout: 15_000 })

    // "Changes nothing" — proven, not assumed: the OLD password still signs in,
    // and the second session is still alive.
    const stillOld = await otherContext.newPage()
    expect(await signIn(stillOld, email, PASSWORD), 'the old password must still work').toBe(200)
    await stillOld.close()
    expect((await api<Session[]>(otherPage, 'GET', '/auth/sessions')).status).toBe(200)

    // AC1: the real change.
    await page.getByTestId('security-old-password').fill(PASSWORD)
    await page.getByTestId('security-save').click()
    await expect(page.getByTestId('security-done')).toBeVisible({ timeout: 15_000 })

    // This session survives…
    const after = await api<Session[]>(page, 'GET', '/auth/sessions')
    expect(after.status, 'the caller keeps its own session').toBe(200)
    expect(after.data.length, 'the other session is gone').toBe(1)
    expect(after.data[0].current).toBe(true)

    // …and the other one is refused.
    const revoked = await api<Session[]>(otherPage, 'GET', '/auth/sessions')
    expect(revoked.status, 'the revoked session must no longer be usable').toBe(401)

    await otherContext.close()
  })

  test('AC4 — a revoke control only ever targets a session of the caller, and the API refuses a foreign one', async ({
    page,
    browser,
  }) => {
    const email = `e2e-sec3-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
    const orgID = await register(page, email)

    const otherContext = await browser.newContext()
    const otherPage = await otherContext.newPage()
    expect(await signIn(otherPage, email, PASSWORD)).toBe(200)

    await page.goto(`/app/${orgID}/settings/security`)
    await expect(page.getByTestId('session-row')).toHaveCount(2, { timeout: 15_000 })

    // Only the non-current row has a button, so the UI can only name the other
    // session of the SAME user — there is no control that takes an id.
    await expect(page.getByTestId('session-revoke')).toHaveCount(1)

    // Revoking it drops the row and ends that session.
    await page.getByTestId('session-revoke').click()
    await expect(page.getByTestId('session-row')).toHaveCount(1, { timeout: 15_000 })
    expect((await api<Session[]>(otherPage, 'GET', '/auth/sessions')).status).toBe(401)

    // The boundary is the server, not the absent button: a stranger's session id
    // is refused. The caller is owner of their own workspace, so this is the
    // owner path being denied a session outside their membership.
    const strangerContext = await browser.newContext()
    const strangerPage = await strangerContext.newPage()
    const strangerEmail = `e2e-sec3b-${Date.now()}@example.com`
    await register(strangerPage, strangerEmail)
    await strangerPage.goto('/login')
    const strangerSessions = await api<Session[]>(strangerPage, 'GET', '/auth/sessions')
    const strangerID = strangerSessions.data[0].id

    const denied = await api(page, 'DELETE', `/auth/sessions/${strangerID}`)
    expect(denied.status, 'a session outside the caller’s workspace is not addressable').toBe(404)
    // And the stranger is untouched.
    expect((await api<Session[]>(strangerPage, 'GET', '/auth/sessions')).status).toBe(200)

    await strangerContext.close()
    await otherContext.close()
  })
})
