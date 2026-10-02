import { expect, test, type Page } from '@playwright/test'

/**
 * US-AD98 — closing your own account.
 *
 * AC1 is why the form has two gates and the spec asserts the request never
 * leaves while they are unmet. A screen that enabled the button and let the
 * server answer 400 would look identical in a screenshot and be a different
 * product: the AC says the typed email IS the confirmation.
 *
 * AC2 is checked by consequence, not by the status code: after the 202 the
 * session is gone (the page lands on /login) and the old credentials are
 * refused. A 202 alone is compatible with a handler that closed nothing.
 *
 * AC3 and AC4 need other people in the workspace, so they use the real invite
 * path rather than a hand-written membership row. AC4 is structural on the
 * server — the route has no `{id}` — so the spec asserts the shape that makes
 * it structural, instead of pretending to test a forbidden request that has no
 * way to be expressed.
 */

const API = '/api/v1'
const PASSWORD = 'Sup3rSecret!2026'

interface Session {
  orgID: string
  email: string
}

async function signUp(page: Page, email: string, name = 'Close Owner', org = 'Close Co'): Promise<Session> {
  const res = await page.request.post(`${API}/auth/register`, {
    data: { email, password: PASSWORD, name, org_name: org },
  })
  const body = await res.text()
  expect(res.status(), body).toBe(201)
  const [cookie] = (await page.context().cookies()).filter((c) => c.name === 'agentdeck_session')
  expect(cookie, 'registration did not set a session cookie').toBeTruthy()
  await page.context().addCookies([{ ...cookie, secure: false }])
  return { orgID: (JSON.parse(body) as { workspace_id: string }).workspace_id, email }
}

function uniqueEmail(tag: string): string {
  return `e2e-close-${tag}-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
}

test.describe('close account — US-AD98', () => {
  test('AC1 — the button stays disabled until the typed email matches exactly', async ({ page }) => {
    const session = await signUp(page, uniqueEmail('gate'))
    await page.goto(`/app/${session.orgID}/settings/close`)

    const submit = page.getByTestId('close-account-submit')
    await expect(submit).toBeVisible({ timeout: 15_000 })

    // Nothing typed: blocked. The gate line says why, so the disabled button is
    // not a dead end.
    await expect(submit).toBeDisabled()
    await expect(page.getByTestId('close-account-gate')).toBeVisible()

    // The checkbox alone is not enough — the email is the confirmation AC1 asks
    // for, and either gate on its own must not open the door.
    await page.getByTestId('close-account-ack').check()
    await expect(submit).toBeDisabled()

    // A near miss stays blocked. Case and surrounding space are the two the
    // server tolerates; a substring is not one of them.
    await page.getByTestId('close-account-confirm-email').fill(session.email.slice(0, -1))
    await expect(submit).toBeDisabled()

    await page.getByTestId('close-account-confirm-email').fill(session.email)
    await expect(submit).toBeEnabled()
  })

  test('AC1/AC2 — the wrong email changes nothing, the right one ends every session', async ({ page }) => {
    const session = await signUp(page, uniqueEmail('close'))

    // The screen shows the account it is about to close, read from the API
    // rather than from anything the operator typed.
    await page.goto(`/app/${session.orgID}/settings/close`)
    await expect(page.getByTestId('close-account-email')).toHaveText(session.email, { timeout: 15_000 })

    // AC1's server half: a mismatch is a refusal and the account survives.
    const refused = await page.evaluate(
      async ({ api, email }) => {
        const response = await fetch(`${api}/auth/me`, {
          method: 'DELETE',
          credentials: 'include',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ confirm_email: `${email}.nope` }),
        })
        return response.status
      },
      { api: API, email: session.email },
    )
    expect(refused, 'a mismatched confirmation must be refused').toBe(400)

    const stillThere = await page.evaluate(async (api) => {
      const response = await fetch(`${api}/auth/me`, { credentials: 'include' })
      return response.status
    }, API)
    expect(stillThere, 'the account must survive a refused confirmation').toBe(200)

    // Now the real thing, through the form.
    await page.getByTestId('close-account-confirm-email').fill(session.email)
    await page.getByTestId('close-account-ack').check()
    await page.getByTestId('close-account-submit').click()

    // AC2: the session is gone, so the shell leaves the dashboard.
    await page.waitForURL(/\/login/, { timeout: 15_000 })

    // And the credentials no longer work. This is the assertion that separates
    // "the cookie was cleared" from "the account was closed".
    const relogin = await page.request.post(`${API}/auth/login`, {
      data: { email: session.email, password: PASSWORD },
    })
    expect(relogin.status(), 'a closed account must not sign in again').toBeGreaterThanOrEqual(400)
  })

  test('AC3 — the last owner of a shared workspace is refused with 409', async ({ page, request }) => {
    const session = await signUp(page, uniqueEmail('owner'))

    // Somebody else joins, so the caller is no longer the only member.
    const memberEmail = uniqueEmail('member')
    const reg = await request.post(`${API}/auth/register`, {
      data: { email: memberEmail, password: PASSWORD, name: 'Close Member', org_name: 'Member Co' },
    })
    expect(reg.status(), await reg.text()).toBe(201)
    const invited = await page.request.post(`${API}/orgs/${session.orgID}/members`, {
      data: { email: memberEmail, role: 'member' },
    })
    expect(invited.status(), await invited.text()).toBeLessThan(300)

    await page.goto(`/app/${session.orgID}/settings/close`)
    const submit = page.getByTestId('close-account-submit')
    await expect(submit).toBeVisible({ timeout: 15_000 })

    await page.getByTestId('close-account-confirm-email').fill(session.email)
    await page.getByTestId('close-account-ack').check()
    await submit.click()

    // AC3 names 409, and the screen says what to do about it — inline, against
    // the form, not as a toast.
    const error = page.getByTestId('close-account-error')
    await expect(error).toBeVisible({ timeout: 15_000 })
    await expect(error).toContainText(/ownership|kepemilikan/i)

    // The account is still usable: a refusal that half-closed it would be worse
    // than no refusal.
    const me = await page.evaluate(async (api) => {
      const response = await fetch(`${api}/auth/me`, { credentials: 'include' })
      return response.status
    }, API)
    expect(me, 'the account must survive the refusal').toBe(200)
  })

  test('AC4 — the route cannot name another account', async ({ page }) => {
    const session = await signUp(page, uniqueEmail('self'))

    // AC4 is structural: there is no `{id}` anywhere in the closure route, so a
    // caller has no way to express "close that other user". The screen's own
    // request is the only shape that exists, and it carries no id.
    await page.goto(`/app/${session.orgID}/settings/close`)
    await expect(page.getByTestId('close-account-submit')).toBeVisible({ timeout: 15_000 })

    const seen = await page.evaluate(async (api) => {
      // A closure aimed at another account is not a 403 — it is not a request
      // the API can parse, because the only closure route is /auth/me.
      const response = await fetch(`${api}/auth/me/01ARZ3NDEKTSV4RRFFQ69G5FAV`, {
        method: 'DELETE',
        credentials: 'include',
      })
      return response.status
    }, API)
    expect(seen, 'there must be no route that closes a named account').toBe(404)

    // And the caller is untouched by that attempt.
    const me = await page.evaluate(async (api) => {
      const response = await fetch(`${api}/auth/me`, { credentials: 'include' })
      return response.status
    }, API)
    expect(me).toBe(200)
  })
})
