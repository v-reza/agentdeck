import { expect, test } from '@playwright/test'

/**
 * Auth screens must match their designs, not merely work.
 *
 * These four screens drifted once already: the Tailwind migration replaced the
 * dashboard's styling but left the auth pages rendering English copy with no
 * brand header, no route chip, no system footer, and no "Lupa password?" link —
 * and every behavioural test still passed, because a form that submits is a
 * form that submits. So the assertions here are about what the design actually
 * specifies: the composition (brand header, 400px card, system footer) and the
 * copy, which the PRD writes itself.
 *
 * The copy is Indonesian because that is the contract, not a preference:
 * US-AD02 AC5 requires the button to read "Log in" and the link to read
 * "Lupa password?". Design source: design/stitch-output/v2/0{1,2,3,4}-*.html.
 */

/** The brand header, the route chip, and the system footer every design shares. */
async function expectAuthChrome(page: import('@playwright/test').Page, route: string) {
  // Brand mark + wordmark, above the card.
  await expect(page.getByText('AgentDeck').first()).toBeVisible()
  // The route chip names the screen, and the tagline sits beside it.
  await expect(page.getByText(route, { exact: true })).toBeVisible()
  await expect(page.getByText('Fleet Orchestration')).toBeVisible()
  // The single-binary / isolation footer.
  await expect(page.getByText('self-hosted')).toBeVisible()
  await expect(page.getByText('isolated')).toBeVisible()
  await expect(page.getByText('agentdeck v2.0')).toBeVisible()

  // The card is exactly 400px wide in every auth design ("kartu 400px tengah").
  const card = page.locator('main > section')
  expect((await card.boundingBox())?.width).toBe(400)
}

test.describe('auth screens match their designs', () => {
  test('01-login — US-AD02 AC5', async ({ page }) => {
    await page.goto('/login')

    await expectAuthChrome(page, '/login')
    await expect(page.getByRole('heading', { name: 'Masuk ke AgentDeck' })).toBeVisible()
    await expect(page.getByText('Masukkan kredensial akun untuk mengakses fleet.')).toBeVisible()

    await expect(page.getByLabel('Email')).toBeVisible()
    await expect(page.getByLabel('Password')).toBeVisible()

    // AC5: the forgot-password link is part of the criterion, and it has to
    // lead somewhere real.
    const forgot = page.getByRole('link', { name: 'Lupa password?' })
    await expect(forgot).toBeVisible()
    await expect(forgot).toHaveAttribute('href', '/reset')

    // AC5: the primary button reads exactly "Log in".
    await expect(page.getByRole('button', { name: 'Log in' })).toBeVisible()

    // AC5: no social login button.
    await expect(page.getByRole('button', { name: /google|github|facebook|apple/i })).toHaveCount(0)
  })

  test('02-register asks for email and password only — US-AD01 AC5/AC6', async ({ page }) => {
    await page.goto('/register')

    await expectAuthChrome(page, '/register')
    await expect(page.getByRole('heading', { name: 'Daftar ke AgentDeck' })).toBeVisible()

    // AC5/AC6: the personal workspace and the display name are the server's
    // job, so the form must not ask for a name or a workspace name.
    await expect(page.getByLabel('Email')).toBeVisible()
    await expect(page.getByLabel('Password')).toBeVisible()
    await expect(page.getByLabel('Password')).toHaveAttribute('minlength', '8')
    await expect(page.getByText('min. 8 karakter')).toBeVisible()
    // Exactly two controls — email and password — and nothing else. The point
    // is the *absence* of the name and workspace-name fields the design drops,
    // so assert the count and then name the fields that must not exist.
    await expect(page.getByRole('textbox')).toHaveCount(2)
    await expect(page.getByLabel(/nama|name|workspace name|org/i)).toHaveCount(0)

    // The onboarding banner the design pairs with AC5.
    await expect(page.getByText('Workspace Personal Otomatis')).toBeVisible()

    await expect(page.getByRole('button', { name: 'Daftar Akun' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Masuk' })).toHaveAttribute('href', '/login')
  })

  test('03-reset-request — US-AD88 AC1/AC5', async ({ page }) => {
    await page.goto('/reset')

    await expectAuthChrome(page, '/reset')
    await expect(page.getByRole('heading', { name: 'Lupa password' })).toBeVisible()
    await expect(page.getByLabel('Email')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Kirim Tautan Reset' })).toBeVisible()
    // The notice is the design's own callout, titled after the criterion it
    // states — not a copy of the page heading.
    await expect(page.getByText('Ketentuan Keamanan (US-AD88 AC2)')).toBeVisible()
    await expect(page.getByRole('link', { name: 'Masuk' })).toHaveAttribute('href', '/login')
  })

  test('04-reset-confirm — US-AD88 AC2', async ({ page }) => {
    await page.goto('/reset/some-token')

    await expectAuthChrome(page, '/reset/:token')
    await expect(page.getByRole('heading', { name: 'Buat password baru' })).toBeVisible()
    // `exact` matters here: "Konfirmasi password baru" contains "password baru",
    // so a substring match resolves to both fields and fails strict mode.
    await expect(page.getByLabel('Password baru', { exact: true })).toBeVisible()
    await expect(page.getByLabel('Konfirmasi password baru')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Simpan Password Baru' })).toBeVisible()
  })
})

/**
 * The four auth designs share a composition but NOT a set of numbers. An
 * earlier pass averaged them into one shell — every card 10px radius, every
 * title 17px/600, every button 36px/500 — which is invisible to a copy
 * assertion and to every behavioural test. These are the numbers read off
 * `design/stitch-output/v2/0{1,2,3,4}-*.html`, asserted on the rendered DOM so
 * the drift cannot come back quietly.
 */
const GEOMETRY = [
  {
    route: '/login',
    variant: '01-login',
    cardRadius: '10px',
    // `shadow-sm` resolves to Tailwind v4's value, which is one pixel
    // different from the CDN default the design file itself renders with.
    cardShadow: 'rgba(0, 0, 0, 0.1) 0px 1px 3px 0px, rgba(0, 0, 0, 0.1) 0px 1px 2px -1px',
    titleSize: '17px',
    titleWeight: '600',
    subtitleSize: '13px',
    inputPadding: '0px 12px',
    buttonHeight: 36,
    buttonWeight: '500',
    // header→card, card→system footer, system footer→/register link (mt-3).
    // Login keeps that link outside the card, so `main` has one more child
    // than the other three designs.
    mainGaps: [24, 24, 12],
  },
  {
    route: '/register',
    variant: '02-register',
    cardRadius: '14px',
    cardShadow: 'rgba(0, 0, 0, 0.04) 0px 4px 20px 0px',
    titleSize: '18px',
    titleWeight: '700',
    subtitleSize: '12px',
    inputPadding: '0px 10px',
    buttonHeight: 36,
    buttonWeight: '600',
    // `.system-meta-footer { margin-top: 20px }` — register is the only design
    // whose card→footer gap is not 24.
    mainGaps: [24, 20],
  },
  {
    route: '/reset',
    variant: '03-reset-request',
    cardRadius: '14px',
    cardShadow: 'rgba(12, 26, 22, 0.05) 0px 1px 3px 0px, rgba(12, 26, 22, 0.04) 0px 10px 24px -6px',
    titleSize: '18px',
    titleWeight: '700',
    subtitleSize: '12px',
    inputPadding: '0px 12px',
    buttonHeight: 36,
    buttonWeight: '600',
    mainGaps: [24, 24],
  },
  {
    route: '/reset/some-token',
    variant: '04-reset-confirm',
    cardRadius: '14px',
    cardShadow: 'rgba(12, 26, 22, 0.05) 0px 1px 3px 0px, rgba(12, 26, 22, 0.04) 0px 10px 24px -6px',
    titleSize: '18px',
    titleWeight: '700',
    subtitleSize: '12px',
    inputPadding: '0px 12px',
    buttonHeight: 38,
    buttonWeight: '600',
    mainGaps: [24, 24],
  },
] as const

test.describe('auth screens use their own design numbers, not an average', () => {
  for (const g of GEOMETRY) {
    test(`${g.variant} matches its own card, type, and button metrics`, async ({ page }) => {
      await page.goto(g.route)

      const card = page.locator('main > section')
      const cardStyle = await card.evaluate((el) => {
        const s = getComputedStyle(el)
        // Tailwind v4 composes `shadow-*` through four `--tw-shadow` slots, so
        // Chromium reports the unused ones as `rgba(0,0,0,0) 0px 0px 0px 0px`.
        // Drop those and the remaining declaration is the design's own shadow.
        const shadow = s.boxShadow
          .split(/,(?![^(]*\))/)
          .map((part) => part.trim())
          .filter((part) => !part.startsWith('rgba(0, 0, 0, 0) 0px 0px 0px 0px'))
          .join(', ')
        return { radius: s.borderRadius, shadow, padding: s.paddingTop }
      })
      expect(cardStyle.radius).toBe(g.cardRadius)
      expect(cardStyle.shadow).toBe(g.cardShadow)
      // Every design pads the card 24px; this is what the unlayered reset in
      // public.css silently zeroed out before it was moved into @layer base.
      expect(cardStyle.padding).toBe('24px')

      const title = await page
        .getByRole('heading')
        .first()
        .evaluate((el) => {
          const s = getComputedStyle(el)
          return { size: s.fontSize, weight: s.fontWeight }
        })
      expect(title.size).toBe(g.titleSize)
      expect(title.weight).toBe(g.titleWeight)

      const subtitleSize = await page
        .locator('main > section p')
        .first()
        .evaluate((el) => getComputedStyle(el).fontSize)
      expect(subtitleSize).toBe(g.subtitleSize)

      const inputPadding = await page
        .locator('main input')
        .first()
        .evaluate((el) => getComputedStyle(el).padding)
      expect(inputPadding).toBe(g.inputPadding)

      const button = await page
        .getByRole('button')
        .first()
        .evaluate((el) => {
          const s = getComputedStyle(el)
          return { height: Math.round(el.getBoundingClientRect().height), weight: s.fontWeight }
        })
      expect(button.height).toBe(g.buttonHeight)
      expect(button.weight).toBe(g.buttonWeight)

      // Header, card, and footer are three stacked blocks, not one flush pile.
      // The gaps went to 0 when the reset was unlayered.
      const gaps = await page.locator('main').evaluate((el) => {
        const kids = Array.from(el.children)
        const boxes = kids.map((k) => k.getBoundingClientRect())
        return boxes.slice(1).map((b, i) => Math.round(b.top - boxes[i].bottom))
      })
      // The gap between each pair of stacked blocks, in design order. Asserted
      // as a whole array: a scalar would have hidden register's 20px footer.
      expect(gaps, `gaps=${JSON.stringify(gaps)}`).toEqual([...g.mainGaps])
    })
  }
})

test.describe('password reset end to end', () => {
  /**
   * US-AD88 AC5 — the request endpoint answers the same way for an unknown
   * address, and the screen must not contradict it by claiming the mail was
   * sent.
   */
  test('an unknown address still shows the neutral confirmation — US-AD88 AC5', async ({ page }) => {
    await page.goto('/reset')
    await page.getByLabel('Email').fill(`nobody-${Date.now()}@example.com`)
    await page.getByRole('button', { name: 'Kirim Tautan Reset' }).click()

    await expect(page.getByText('Kalau email itu terdaftar, tautan reset sedang dikirim.')).toBeVisible({
      timeout: 15_000,
    })
    // A screen that said "not found" would leak account existence from the
    // client side even though the API hides it.
    await expect(page.getByText(/tidak terdaftar|not found/i)).toHaveCount(0)
  })

  /**
   * US-AD88 AC3 — a token that does not exist is refused, and the screen shows
   * the server's own message rather than a generic failure.
   */
  test('an unknown token is refused with 410 — US-AD88 AC3', async ({ page }) => {
    await page.goto('/reset/not-a-real-token')
    await page.getByLabel('Password baru', { exact: true }).fill('password-baru-123')
    await page.getByLabel('Konfirmasi password baru').fill('password-baru-123')
    await page.getByRole('button', { name: 'Simpan Password Baru' }).click()

    // Scoped to the card: the toast (if any) renders outside <main>, so this
    // asserts the inline error specifically rather than matching either.
    await expect(page.getByRole('main').getByText('reset token invalid')).toBeVisible({ timeout: 15_000 })
  })

  /**
   * The failure is stated once, inline. A toast repeating it would say the same
   * sentence twice and also surface the raw API string as a toast title.
   */
  test('a refused token is reported once, not also as a toast', async ({ page }) => {
    await page.goto('/reset/not-a-real-token')
    await page.getByLabel('Password baru', { exact: true }).fill('password-baru-123')
    await page.getByLabel('Konfirmasi password baru').fill('password-baru-123')
    await page.getByRole('button', { name: 'Simpan Password Baru' }).click()

    await expect(page.getByRole('main').getByText('reset token invalid')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByText('reset token invalid')).toHaveCount(1)
    await expect(page.getByText('Request failed')).toHaveCount(0)
  })

  /**
   * A mismatched confirmation is a typo, not a server round trip. It must be
   * caught before the single-use token is spent.
   */
  test('a mismatched confirmation never reaches the server', async ({ page }) => {
    await page.goto('/reset/not-a-real-token')
    await page.getByLabel('Password baru', { exact: true }).fill('password-baru-123')
    await page.getByLabel('Konfirmasi password baru').fill('password-beda-456')
    await page.getByRole('button', { name: 'Simpan Password Baru' }).click()

    await expect(page.getByText('Dua password tidak sama.')).toBeVisible()
    // No request was made, so the 410 from the server must not appear.
    await expect(page.getByText(/reset token invalid/i)).toHaveCount(0)
  })
})
