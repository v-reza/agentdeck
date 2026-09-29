import { expect, test, type Page } from '@playwright/test'

/**
 * US-AD107 — the workspace skill library.
 *
 * The screen is new; the backend has been there since F9 with zero frontend
 * callers. What is asserted here is the part a unit test cannot see: that the
 * library is reachable, that a skill can be written and edited through the real
 * API, that the version rises (AC6), and that a member gets no controls and a 403
 * from the API itself (AC4) — hiding a button is not the boundary.
 *
 * AC3 (raw HTML is never executed) is NOT tested here. A browser can only observe
 * that a script did not run, which is indistinguishable from a script that ran and
 * did nothing visible; the negative is unprovable from the outside. It is pinned
 * in `src/lib/markdown.test.ts` on the output string instead, where the property
 * "no tag in the output came from the input" is directly checkable. This file
 * still sends a hostile body through the real API and the real renderer, so the
 * integration is covered — what it does not do is claim that proves AC3.
 */

const API = '/api/v1'
const PASSWORD = 'Sup3rSecret!2026'
const ULID = /[0-9A-HJKMNP-TV-Z]{26}/

async function signUp(page: Page, email: string, name: string): Promise<string> {
  const res = await page.request.post(`${API}/auth/register`, {
    data: { email, password: PASSWORD, name },
  })
  const body = await res.text()
  expect(res.status(), body).toBe(201)

  // The session cookie is issued `Secure`, and Playwright's request context will
  // not send one over the plain-http test origin. Re-adding the same value with
  // `secure: false` leaves one jar that the browser and the request context share.
  const [session] = (await page.context().cookies()).filter((c) => c.name === 'agentdeck_session')
  expect(session, 'registration did not set a session cookie').toBeTruthy()
  await page.context().addCookies([{ ...session, secure: false }])

  return (JSON.parse(body) as { workspace_id: string }).workspace_id
}

async function api<T>(
  page: Page,
  orgID: string,
  method: 'GET' | 'POST' | 'PATCH' | 'DELETE',
  path: string,
  body?: unknown,
): Promise<{ status: number; data: T; text: string }> {
  return page.evaluate(
    async ({ api, orgID, method, path, body }) => {
      const response = await fetch(`${api}${path}`, {
        method,
        headers: body ? { 'Content-Type': 'application/json', 'X-Org-ID': orgID } : { 'X-Org-ID': orgID },
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
    { api: API, orgID, method, path, body },
  )
}

interface Skill {
  id: string
  slug: string
  name: string
  body_md: string
  version: number
  is_system: boolean
  used_by: number
}

test.describe('skill library — US-AD107', () => {
  let orgID: string
  let slug: string

  test.beforeEach(async ({ page }) => {
    orgID = await signUp(page, `e2e-skill-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`, 'Skill Owner')
    slug = `e2e_skill_${Date.now()}`
    // `api()` runs an in-page `fetch` with a RELATIVE url, so the document needs
    // an origin before the first call — on `about:blank` the URL fails to parse
    // and every assertion reads as a broken API. Landing on the login screen is
    // enough; the session cookie is already in the jar.
    await page.goto('/login')
  })

  test('AC5 — a fresh workspace is seeded with the eight default skills', async ({ page }) => {
    const listed = await api<Skill[]>(page, orgID, 'GET', '/agent-skills')
    expect(listed.status, listed.text).toBe(200)

    const systemSlugs = listed.data
      .filter((s) => s.is_system)
      .map((s) => s.slug)
      .sort()
    expect(systemSlugs).toEqual(
      ['code_review', 'debug', 'docs', 'e2e_test', 'migration', 'refactor', 'security_review', 'test_write'].sort(),
    )
  })

  test('AC1/AC2/AC6 — a skill is created, previewed as markdown, and edited to a new version', async ({ page }) => {
    await page.goto(`/app/${orgID}/skills`)
    await expect(page).toHaveURL(new RegExp(`/app/${ULID.source}/skills$`))

    // AC5 again, this time through the screen rather than the API.
    await expect(page.getByTestId('skill-row-code_review')).toBeVisible()
    await expect(page.getByTestId('skill-system-badge')).toBeVisible()

    await page.getByTestId('skill-create').click()
    await page.getByTestId('skill-slug').fill(slug)
    await page.getByTestId('skill-name').fill('E2E Review Skill')
    await page.getByTestId('skill-body').fill('# Heading\n\nUse **bold** here.')

    // AC2: the form previews the same body it will save.
    await page.getByTestId('skill-form-preview-toggle').click()
    const preview = page.getByTestId('skill-form-preview')
    await expect(preview.locator('h1')).toHaveText('Heading')
    await expect(preview.locator('strong')).toHaveText('bold')

    await page.getByTestId('skill-save').click()

    // The new skill is selected and shown rendered, not as source.
    const detail = page.getByTestId('skill-preview')
    await expect(detail.locator('h1')).toHaveText('Heading', { timeout: 15_000 })
    await expect(page.getByTestId('skill-used-by')).toHaveText('0')

    const created = await api<Skill[]>(page, orgID, 'GET', '/agent-skills')
    const mine = created.data.find((s) => s.slug === slug)!
    expect(mine.version, 'a new skill starts at v1').toBe(1)
    expect(mine.is_system).toBe(false)

    // AC6: the edit raises the version, and the row says so.
    await page.getByTestId('skill-edit').click()
    // The slug is not editable — agents store it, so PATCH omits it.
    await expect(page.getByTestId('skill-slug')).toBeDisabled()
    await page.getByTestId('skill-body').fill('# Changed\n\nSecond version.')
    await page.getByTestId('skill-save').click()
    await expect(detail.locator('h1')).toHaveText('Changed', { timeout: 15_000 })

    const after = await api<Skill[]>(page, orgID, 'GET', '/agent-skills')
    const bumped = after.data.find((s) => s.slug === slug)!
    expect(bumped.version, 'an edit must raise the version, not rewrite v1').toBe(2)
  })

  test('AC3 integration — a hostile body renders as text, and the page survives', async ({ page }) => {
    const hostile = '<img src=x onerror="window.__xss=1"> and <script>window.__xss=1</script>'
    const created = await api<Skill>(page, orgID, 'POST', '/agent-skills', {
      slug,
      name: 'Hostile',
      body_md: hostile,
    })
    expect(created.status, created.text).toBe(201)

    await page.goto(`/app/${orgID}/skills`)
    await page.getByTestId(`skill-row-${slug}`).click()

    const preview = page.getByTestId('skill-preview')
    await expect(preview).toBeVisible()
    // The tag is shown as text: the literal string is present, and no element was
    // created from it.
    await expect(preview).toContainText('<img src=x onerror=')
    expect(await preview.locator('img').count()).toBe(0)
    expect(await preview.locator('script').count()).toBe(0)
    // And nothing executed.
    expect(await page.evaluate(() => (window as { __xss?: number }).__xss)).toBeUndefined()
  })

  test('AC4 — a member reads the library, gets no controls, and the API refuses the write', async ({
    page,
    browser,
  }) => {
    // The owner is the test's own `page`, not a fresh tab: `browser.newPage()`
    // opens a NEW context with an empty cookie jar, so it renders as signed out
    // and the create control is legitimately absent. The member is the one that
    // needs a separate context, because the two sessions must not share a jar.
    await page.goto(`/app/${orgID}/skills`)
    await expect(page.getByTestId('skill-create')).toBeVisible()

    const memberEmail = `e2e-skillmember-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
    const memberContext = await browser.newContext()
    const memberPage = await memberContext.newPage()
    const registered = await memberPage.request.post(`${API}/auth/register`, {
      data: { email: memberEmail, password: PASSWORD, name: 'E2E Skill Member' },
    })
    expect(registered.status(), await registered.text()).toBe(201)
    const invited = await api(page, orgID, 'POST', `/orgs/${orgID}/members`, {
      email: memberEmail,
      role: 'member',
    })
    expect(invited.status, 'inviting the member').toBe(201)

    await memberPage.goto(`/app/${orgID}/skills`)
    // Reads: the library is there, system skills included.
    await expect(memberPage.getByTestId('skill-row-code_review')).toBeVisible()
    // Writes: no control at all.
    await expect(memberPage.getByTestId('skill-create')).toHaveCount(0)
    await expect(memberPage.getByTestId('skill-edit')).toHaveCount(0)
    await expect(memberPage.getByTestId('skill-delete')).toHaveCount(0)

    // And the boundary is the server, not the missing button.
    const denied = await api(memberPage, orgID, 'POST', '/agent-skills', {
      slug: `${slug}_denied`,
      name: 'Denied',
      body_md: 'nope',
    })
    expect(denied.status, 'POST /agent-skills is Admin-gated').toBe(403)

    await memberContext.close()
  })

  test('a system skill cannot be deleted, and the control says so instead of failing', async ({ page }) => {
    await page.goto(`/app/${orgID}/skills`)
    await page.getByTestId('skill-row-code_review').click()
    await expect(page.getByTestId('skill-delete')).toBeDisabled()

    // The refusal is the server's, not only the disabled attribute.
    const listed = await api<Skill[]>(page, orgID, 'GET', '/agent-skills')
    const system = listed.data.find((s) => s.slug === 'code_review')!
    const refused = await api(page, orgID, 'DELETE', `/agent-skills/${system.id}`)
    expect(refused.status).toBe(409)
  })
})
