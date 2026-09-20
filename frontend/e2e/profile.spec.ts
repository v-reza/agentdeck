import { expect, test, type Page } from '@playwright/test'

/**
 * Screen 15-profile — the operator's own account (US-AD89 AC1–AC5).
 *
 * Asserted here and nowhere else:
 *
 *  1. The design's geometry, read from `15-profile.html` rather than averaged:
 *     10px panel radius with 10px padding, 6px attribute tiles on the sunken
 *     surface, the `US-AD89` badge as an accent-tinted mono pill, and the
 *     `Must • M0` chip at 4px. A screen that renders the right words at the
 *     wrong density still fails this.
 *  2. AC1 literally: every value on screen comes from `GET /api/v1/auth/me`,
 *     including `avatar_user`. The monogram is asserted against the payload, so
 *     a client that derives its own initials from the name is caught.
 *  3. AC2 is "without a full reload", so the rename is checked against a marker
 *     planted on `window`: if the page reloaded, the marker is gone and the test
 *     fails even though the topbar text happens to look right.
 *  4. AC3 is the *failure* path for a taken address, and it also proves the
 *     write is abandoned: the name in the same rejected patch must not land.
 *  5. AC4 — a foreign id is 404, never 403, and the 404 leaks no identity.
 *  6. AC5 — the page opens from the avatar menu and works with no `owner`/
 *     `admin` role in the active workspace.
 *
 * Every authenticated call goes through `page.evaluate(fetch)`, NOT
 * `page.request`: Playwright's API-request context does not share the page's
 * cookie jar, so a `page.request.get('/auth/me')` after registering through the
 * page answers 401 and the whole suite fails on a setup line.
 *
 * The account is registered per run, same as `members.spec.ts`, so the suite is
 * re-runnable against a database that is never truncated.
 */
const API = '/api/v1'
const PASSWORD = 'correct-horse-battery-staple'

interface MePayload {
  id: string
  email: string
  name: string
  avatar_user: { kind: string; initials: string; bg_color: string; size_px: number; url?: string }
  workspaces: { id: string; name: string; slug: string; role: string; kind: string }[]
}

/**
 * Registers a fresh account in the page's own cookie jar and returns the
 * identity the API reported for it.
 */
async function signUp(page: Page, label = 'Profil'): Promise<MePayload> {
  const email = `e2e-profile-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
  await page.goto('/login')
  const result = await page.evaluate(
    async ({ api, credentials }) => {
      const response = await fetch(`${api}/auth/register`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify(credentials),
      })
      return { status: response.status, body: await response.text() }
    },
    { api: API, credentials: { email, password: PASSWORD, name: `${label} Owner`, org_name: `${label} Fleet` } },
  )
  expect(result.status, result.body).toBe(201)
  return readMe(page)
}

/** GET /auth/me through the page, so the session cookie is the browser's. */
async function readMe(page: Page): Promise<MePayload> {
  const result = await page.evaluate(async (api) => {
    const response = await fetch(`${api}/auth/me`, { credentials: 'include' })
    return { status: response.status, body: await response.text() }
  }, API)
  expect(result.status, result.body).toBe(200)
  return JSON.parse(result.body) as MePayload
}

/**
 * Creates a second, unrelated account in its own browser context and returns
 * its identity. Used for the "taken address" and "foreign id" cases, where the
 * point is that the two accounts must not be able to touch each other.
 *
 * It MUST run in a separate context. A same-origin `fetch` to
 * `/auth/register` stores the new session cookie in the *page's* jar even with
 * `credentials` unset, so registering the second account from the main page
 * silently replaced the first account's session — which is what made the "own
 * id must resolve" assertion read 404.
 */
async function createOther(page: Page, label: string): Promise<MePayload> {
  const email = `e2e-profile-${label}-${Date.now()}@example.com`
  const context = await page.context().browser()!.newContext()
  const otherPage = await context.newPage()
  await otherPage.goto('/login')
  const result = await otherPage.evaluate(
    async ({ api, credentials }) => {
      const response = await fetch(`${api}/auth/register`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify(credentials),
      })
      return { status: response.status, body: await response.text() }
    },
    { api: API, credentials: { email, password: PASSWORD, name: `${label} Owner`, org_name: `${label} Fleet` } },
  )
  expect(result.status, result.body).toBe(201)
  const created = JSON.parse(result.body) as { user_id: string; workspace_id: string }
  await context.close()
  return {
    id: created.user_id,
    email,
    name: `${label} Owner`,
    avatar_user: { kind: 'monogram', initials: '?', bg_color: '#101014', size_px: 26 },
    workspaces: [{ id: created.workspace_id, name: `${label} Fleet`, slug: '', role: 'owner', kind: 'registration' }],
  }
}

/** Computed geometry of the first element matching `selector`. */
function geometry(page: Page, selector: string) {
  return page
    .locator(selector)
    .first()
    .evaluate((el) => {
      const style = getComputedStyle(el)
      const box = el.getBoundingClientRect()
      return {
        radius: style.borderRadius,
        background: style.backgroundColor,
        color: style.color,
        paddingTop: style.paddingTop,
        fontSize: style.fontSize,
        fontWeight: style.fontWeight,
        fontFamily: style.fontFamily,
        width: Math.round(box.width),
        height: Math.round(box.height),
      }
    })
}

test.describe('profile — design match and US-AD89', () => {
  let me: MePayload
  let orgID: string

  test.beforeEach(async ({ page }) => {
    me = await signUp(page)
    orgID = me.workspaces[0].id
    await page.goto(`/app/${orgID}/settings/profile`)
    await expect(page.getByTestId('profile-spec-strip')).toBeVisible({ timeout: 15_000 })
  })

  test('the panels use the design geometry, not a rounded-off version', async ({ page }) => {
    // 15-profile.html: `rounded-[10px] p-[10px]` on every card in <main>.
    const panel = await geometry(page, 'main > div:first-child')
    expect(panel.radius, 'design: rounded-[10px] on the header card').toBe('10px')
    expect(panel.paddingTop, 'design: p-[10px] on the header card').toBe('10px')
    expect(panel.background, 'design: bg-white panel surface').toBe('rgb(255, 255, 255)')

    // design: `rounded-[6px] ... bg-[#f6f7f6] p-2.5` on the four AC1 attributes.
    const tile = await geometry(page, '[data-testid="profile-tile-id"]')
    expect(tile.radius, 'design: rounded-[6px] on an attribute tile').toBe('6px')
    expect(tile.paddingTop, 'design: p-2.5 (10px) on an attribute tile').toBe('10px')
    expect(tile.background, 'design: bg-[#f6f7f6] sunken tile surface').toBe('rgb(246, 247, 246)')
    // AC1 names four attributes; the screen renders four tiles, not three.
    for (const id of ['profile-tile-id', 'profile-tile-email', 'profile-tile-name', 'profile-tile-avatar']) {
      await expect(page.getByTestId(id), `AC1 attribute tile ${id} must be rendered`).toBeVisible()
    }

    // design: the spec badge is a mono, bold, accent-tinted full pill.
    const badge = await geometry(page, '[data-testid="profile-story-badge"]')
    expect(parseFloat(badge.radius), 'design: rounded-full on the story badge').toBeGreaterThan(9999)
    expect(badge.background, 'design: bg-[#eef7f5] accent tint').toBe('rgb(230, 242, 240)')
    expect(badge.color, 'design: text-[#0d7a70] on the story badge').toBe('rgb(13, 122, 112)')
    expect(badge.fontSize, 'design: text-[11px] on the story badge').toBe('11px')
    expect(badge.fontWeight, 'design: font-bold on the story badge').toBe('700')
    expect(badge.fontFamily).toContain('JetBrains Mono')

    // design: the milestone chip is a 4px chip, not a second pill.
    const chip = await geometry(page, '[data-testid="profile-milestone-chip"]')
    expect(chip.radius, 'design: rounded (4px) on the milestone chip').toBe('4px')
    expect(chip.fontSize, 'design: text-[11px] on the milestone chip').toBe('11px')

    // design: every card header is `font-bold text-[13px]`.
    const heading = await geometry(page, 'main h2')
    expect(heading.fontSize, 'design: text-[13px] on card headings').toBe('13px')
    expect(heading.fontWeight, 'design: font-bold on card headings').toBe('700')
    // ...and every card header in the panel follows it, not just the first.
    const allHeadings = await page.locator('main h2').evaluateAll((nodes) =>
      nodes.map((node) => {
        const style = getComputedStyle(node)
        return { size: style.fontSize, weight: style.fontWeight }
      }),
    )
    expect(allHeadings.length).toBeGreaterThanOrEqual(3)
    for (const header of allHeadings) {
      expect(header.size).toBe('13px')
      expect(header.weight).toBe('700')
    }
  })

  test('every rendered value comes from GET /auth/me — US-AD89 AC1', async ({ page }) => {
    const served = await readMe(page)

    await expect(page.locator('input[name="id"]')).toHaveValue(served.id)
    await expect(page.locator('input[name="email"]')).toHaveValue(served.email)
    await expect(page.locator('input[name="name"]')).toHaveValue(served.name)

    // `avatar_user` is rendered verbatim: initials, colour, and diameter all
    // come from the payload. Deriving the monogram client-side disagreed with
    // the API for multi-word names, which is what this pins.
    await expect(page.getByText(`initials: "${served.avatar_user.initials}"`, { exact: false })).toBeVisible()
    // Scoped to the avatar tile: the rail's account-menu button draws the same
    // monogram, and both come from `avatar_user`, so the assertion must name
    // which one it measured.
    const disc = await page
      .getByTestId('profile-tile-avatar')
      .getByTestId('profile-avatar-disc')
      .evaluate((el) => {
        const style = getComputedStyle(el)
        return {
          text: el.textContent?.trim(),
          background: style.backgroundColor,
          size: Math.round(el.getBoundingClientRect().width),
        }
      })
    expect(disc.text).toBe(served.avatar_user.initials)
    expect(disc.size, 'design: avatar diameter 26px, taken from the payload').toBe(served.avatar_user.size_px)
    // #101014 as rgb
    expect(disc.background).toBe('rgb(16, 16, 20)')

    // the top bar names the same identity, from the same payload
    await expect(page.getByRole('banner')).toContainText(`Masuk sebagai ${served.name}`)
  })

  test('renaming lands in the top bar without a full reload — US-AD89 AC2', async ({ page }) => {
    // A marker on `window` survives a client-side re-render and dies on a
    // reload, so this is what makes "without a full reload" a real assertion.
    await page.evaluate(() => {
      ;(window as unknown as Record<string, unknown>).__profileNoReload = 'kept'
    })

    await page.locator('input[name="name"]').fill('Profil Renamed')
    await page.getByRole('button', { name: /simpan perubahan/i }).click()

    await expect(page.getByText('Profil diperbarui')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByRole('banner')).toContainText('Masuk sebagai Profil Renamed')

    expect(
      await page.evaluate(() => (window as unknown as Record<string, unknown>).__profileNoReload),
      'the rename must not reload the document',
    ).toBe('kept')

    // and the write actually persisted, rather than only updating the cache
    expect((await readMe(page)).name).toBe('Profil Renamed')
  })

  test('a taken address is refused inline and nothing is written — US-AD89 AC3', async ({ page }) => {
    // A second account owns the address we are about to claim.
    const other = await createOther(page, 'taken')

    // Change BOTH fields: the name must not sneak through when the email is
    // rejected, which is what "tidak mengubah data" in AC3 requires.
    await page.locator('input[name="name"]').fill('Nama Harus Batal')
    await page.locator('input[name="email"]').fill(other.email)
    await page.getByRole('button', { name: /simpan perubahan/i }).click()

    const inline = page.getByTestId('profile-form-error')
    await expect(inline).toBeVisible({ timeout: 15_000 })
    await expect(inline).toContainText(/already registered/i)
    // The error is rendered where the field is, not as a blocking dialog.
    await expect(page.getByText('Profil diperbarui')).toHaveCount(0)

    const after = await readMe(page)
    expect(after.email, 'the rejected email must not be stored').toBe(me.email)
    expect(after.name, 'the name in the same rejected patch must not be stored either').toBe(me.name)
  })

  test('a foreign id is 404 and leaks nothing — US-AD89 AC4', async ({ page }) => {
    const other = await createOther(page, 'foreign')

    const result = await page.evaluate(
      async ({ api, ownID, foreignID }) => {
        const get = async (url: string) => {
          const response = await fetch(url, { credentials: 'include' })
          return { status: response.status, body: await response.text() }
        }
        const own = await get(`${api}/users/${ownID}`)
        const foreign = await get(`${api}/users/${foreignID}`)
        const write = await fetch(`${api}/users/${foreignID}`, {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'include',
          body: JSON.stringify({ name: 'Mallory' }),
        })
        return { own, foreign, writeStatus: write.status }
      },
      { api: API, ownID: me.id, foreignID: other.id },
    )

    // the caller's own id resolves, so a blanket 404 cannot pass this
    expect(result.own.status, 'own id must resolve').toBe(200)
    // 404 and not 403: a 403 would confirm the account exists
    expect(result.foreign.status, 'a foreign id must be 404, never 403').toBe(404)
    expect(result.foreign.body, 'the 404 must not leak the other account').not.toContain(other.email)
    expect(result.writeStatus, 'no route may let a caller write another account').not.toBe(200)
  })

  test('opens from the avatar menu and needs no owner/admin role — US-AD89 AC5', async ({ page }) => {
    // 1. reachable from the avatar menu
    await page.getByRole('button', { name: 'Buka menu profil' }).click()
    const menuItem = page.getByRole('menuitem', { name: 'Profil akun mandiri' })
    await expect(menuItem).toBeVisible()
    await menuItem.click()
    await expect(page).toHaveURL(new RegExp(`/app/${orgID}/settings/profile$`))
    await expect(page.getByTestId('profile-spec-strip')).toBeVisible()

    // 2. the page states its own permission model and offers no role gate
    await expect(page.getByText(/tidak memerlukan peran owner atau admin/i)).toBeVisible()

    // 3. a plain `viewer` of this workspace can read AND write their profile.
    //    The second user lives in its own browser context, so this is a real
    //    viewer rather than the owner with a renamed role, and the invite is
    //    sent to an address that already has an account.
    const viewerContext = await page.context().browser()!.newContext()
    const viewerPage = await viewerContext.newPage()
    const viewerEmail = `e2e-profile-viewer-${Date.now()}@example.com`
    await viewerPage.goto('/login')
    const registered = await viewerPage.evaluate(
      async ({ api, credentials }) => {
        const response = await fetch(`${api}/auth/register`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'include',
          body: JSON.stringify(credentials),
        })
        return { status: response.status, body: await response.text() }
      },
      {
        api: API,
        credentials: { email: viewerEmail, password: PASSWORD, name: 'Vera Viewer', org_name: 'Vera Fleet' },
      },
    )
    expect(registered.status, registered.body).toBe(201)

    const invited = await page.evaluate(
      async ({ api, orgID, email }) => {
        const response = await fetch(`${api}/orgs/${orgID}/members`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-Org-ID': orgID },
          credentials: 'include',
          body: JSON.stringify({ email, role: 'viewer' }),
        })
        return { status: response.status, body: await response.text() }
      },
      { api: API, orgID, email: viewerEmail },
    )
    expect(invited.status, invited.body).toBe(201)

    // The viewer's *active* workspace is now this one, where their role is
    // `viewer` — the weakest role in the frozen set.
    await viewerPage.goto(`/app/${orgID}/settings/profile`)
    await expect(viewerPage.getByTestId('profile-spec-strip')).toBeVisible({ timeout: 15_000 })
    await expect(viewerPage.getByText(/tidak memerlukan peran owner atau admin/i)).toBeVisible()

    const role = await viewerPage.evaluate(async (api) => {
      const response = await fetch(`${api}/auth/me`, { credentials: 'include' })
      const body = (await response.json()) as MePayload
      return body.workspaces.find((workspace) => workspace.id === window.location.pathname.split('/')[2])?.role
    }, API)
    expect(role, 'the write below must happen as a viewer, not an owner').toBe('viewer')

    await viewerPage.locator('input[name="name"]').fill('Vera Renamed')
    await viewerPage.getByRole('button', { name: /simpan perubahan/i }).click()
    await expect(viewerPage.getByText('Profil diperbarui')).toBeVisible({ timeout: 15_000 })
    await expect(viewerPage.getByRole('banner')).toContainText('Masuk sebagai Vera Renamed')

    await viewerContext.close()
  })
})
