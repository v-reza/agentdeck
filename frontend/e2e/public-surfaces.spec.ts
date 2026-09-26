import { expect, test } from '@playwright/test'

/**
 * Regression guard for the public surfaces (US-AD100 / US-AD103).
 *
 * The dashboard was migrated to Tailwind, but the public pages address a
 * component layer by class name. Deleting the stylesheet that defined it left
 * ~159 class names with no rule behind them: the pages still rendered, they just
 * rendered unstyled. Nothing failed, because "unstyled" is not an error.
 *
 * This asserts the layer is actually loaded and applied, so the same regression
 * cannot pass silently again.
 */
const PUBLIC_PAGES = ['/github', '/pricing', '/features', '/docs/quickstart', '/changelog']

test.describe('public surfaces are styled', () => {
  for (const path of PUBLIC_PAGES) {
    test(`${path} applies the public component layer`, async ({ page }) => {
      await page.goto(path)
      await expect(page.locator('header.navbar')).toBeVisible({ timeout: 30_000 })

      // `.nav-link` is 500 in the component layer and 600 when `.active` marks
      // the current page. The UA default for a link is 400, so either weight
      // proves the rule is applied; 400 means the layer is missing.
      const weight = await page
        .locator('.nav-link')
        .first()
        .evaluate((el) => getComputedStyle(el).fontWeight)
      expect(
        ['500', '600'],
        `.nav-link has UA weight (${weight}) — the public component layer is not applied`,
      ).toContain(weight)

      // The page must not fall back to the UA serif stack.
      const font = await page.locator('body').evaluate((el) => getComputedStyle(el).fontFamily)
      expect(font.toLowerCase()).toContain('inter')
    })
  }

  test('the pricing cards render as designed cards, not stacked text', async ({ page }) => {
    await page.goto('/pricing')
    const card = page.locator('.pricing-card').first()
    await expect(card).toBeVisible({ timeout: 30_000 })

    const box = await card.boundingBox()
    expect(box?.width ?? 0).toBeGreaterThan(200)
    const bg = await card.evaluate((el) => getComputedStyle(el).backgroundColor)
    expect(bg, '.pricing-card has no background — the component layer is not applied').not.toBe('rgba(0, 0, 0, 0)')
  })

  /**
   * The roadmap on /github is the only place the public site states which
   * milestone is in flight, so a stale entry is a published lie rather than a
   * cosmetic drift. It shipped that way: M0 read "NEXT" and was the highlighted
   * card long after it was finished, because the marker was the literal
   * `id === 'M0'` in the component and the states were hand-maintained strings.
   *
   * This pins the two things that were actually wrong — exactly one current
   * milestone, and it is not one that has already shipped — so the next
   * milestone update fails loudly if the data and the highlight disagree.
   */
  test('the roadmap marks exactly one in-flight milestone and no shipped one as current', async ({ page }) => {
    await page.goto('/github')

    const cards = page.locator('.roadmap-milestone')
    await expect(cards.first()).toBeVisible({ timeout: 30_000 })
    expect(await cards.count()).toBeGreaterThan(1)

    const states = await cards.evaluateAll((nodes) =>
      nodes.map((node) => ({
        id: node.querySelector('.roadmap-node span')?.textContent?.trim() ?? '',
        state: node.querySelector('.roadmap-state')?.textContent?.trim() ?? '',
        current: node.classList.contains('current'),
      })),
    )

    const current = states.filter((entry) => entry.current)
    expect(current, `exactly one milestone may be current, got ${JSON.stringify(current)}`).toHaveLength(1)

    // The highlighted card is the one the reader will assume is being built, so
    // it must not be a milestone whose own label says it already shipped.
    expect(current[0].state.toUpperCase()).not.toContain('SHIPPED')

    // And a shipped milestone must never be the highlighted one.
    const shipped = states.filter((entry) => entry.state.toUpperCase().includes('SHIPPED'))
    expect(
      shipped.every((entry) => !entry.current),
      `a shipped milestone is marked current: ${JSON.stringify(shipped)}`,
    ).toBe(true)
  })
})

/**
 * 05-landing: the two glyphs the design draws that the implementation did not.
 *
 * The inventory flagged this screen as `9/0` — nine SVGs in the mockup, zero in
 * the implementation. Two of them are not decoration:
 *
 *   - the plan-list check is a *stroked* 16px glyph (`polyline`, stroke-width 2,
 *     viewBox 16). The component drew a filled 20px check, which is a different
 *     shape, not a different size.
 *   - the "Read the docs" arrow is a 14px stroked icon. The page used a literal
 *     `→` character, which follows the surrounding font rather than the icon set,
 *     so it changes shape with font fallback and never matches the design grid.
 *
 * Both are asserted by attributes rather than by screenshot: `fill`, `viewBox`
 * and the presence of `polyline` are what tell the two checks apart, and a
 * downscaled full-page screenshot does not resolve a 16px glyph's stroke style.
 */
test.describe('05-landing draws the design glyphs', () => {
  test('the plan check is stroked at 16px, not a filled 20px glyph', async ({ page }) => {
    await page.goto('/')
    const check = page.locator('.plan-feature-item .check-icon').first()
    await expect(check).toBeVisible()

    const box = await check.boundingBox()
    expect(box?.width).toBeCloseTo(16, 0)
    expect(box?.height).toBeCloseTo(16, 0)
    expect(await check.getAttribute('viewBox')).toBe('0 0 16 16')
    expect(await check.getAttribute('fill')).toBe('none')
    expect(await check.getAttribute('stroke-width')).toBe('2')
    expect(await check.locator('polyline').count()).toBe(1)
    expect(await check.locator('path').count()).toBe(0)
  })

  test('the docs link renders an icon, not a text arrow', async ({ page }) => {
    await page.goto('/')
    const link = page.getByRole('link', { name: /read the docs/i })
    const arrow = link.locator('svg')
    await expect(arrow).toBeVisible()

    const box = await arrow.boundingBox()
    expect(box?.width).toBeCloseTo(14, 0)
    expect(box?.height).toBeCloseTo(14, 0)

    // The character it replaced must be gone, or the button carries both.
    expect(await link.locator('text=→').count()).toBe(0)
  })
})
