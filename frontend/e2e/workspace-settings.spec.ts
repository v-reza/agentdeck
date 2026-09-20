import { expect, test, type Page } from '@playwright/test'

/** Screen 37-workspace-settings — US-AD03, US-AD77, and US-AD93. */
const API = '/api/v1'
const PASSWORD = 'correct-horse-battery-staple'

async function signUp(page: Page): Promise<string> {
  const email = `e2e-settings-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
  const response = await page.request.post(`${API}/auth/register`, {
    data: { email, password: PASSWORD, name: 'Settings Owner', org_name: 'Settings Fleet' },
  })
  const body = await response.text()
  expect(response.status(), body).toBe(201)
  return (JSON.parse(body) as { workspace_id: string }).workspace_id
}

test.describe('workspace settings and active context', () => {
  test('renders real workspace values and renames without changing the slug', async ({ page }) => {
    const orgID = await signUp(page)
    await page.goto(`/app/${orgID}/settings/workspace`)

    await expect(page.getByRole('main').getByRole('heading', { name: 'Pengaturan ruang kerja' })).toBeVisible()
    await expect(page.getByRole('main').getByText('Settings Fleet', { exact: true })).toBeVisible()
    await expect(page.getByRole('combobox', { name: 'Pindah ruang kerja' })).toHaveCount(0)
    await expect(page.getByRole('main').getByText(orgID, { exact: true })).toBeVisible()
    const slugCard = page.getByRole('main').getByText('Slug', { exact: true }).locator('..')
    const originalSlug = await slugCard.locator('span').nth(1).textContent()
    expect(originalSlug).toBeTruthy()

    await page.getByRole('button', { name: 'Ganti nama ruang kerja' }).first().click()
    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()
    await expect(dialog.locator('input[name="name"]')).toHaveValue('Settings Fleet')
    await expect(dialog.locator('input[name="name"]')).toBeFocused()

    await dialog.locator('input[name="name"]').fill('Renamed Settings Fleet')
    await dialog.getByRole('button', { name: 'Simpan nama' }).click()
    await expect(dialog).toHaveCount(0)
    await expect(page.getByRole('main').getByText('Renamed Settings Fleet', { exact: true })).toBeVisible()
    await expect(
      page.getByRole('main').getByText('Slug', { exact: true }).locator('..').locator('span').nth(1),
    ).toHaveText(originalSlug ?? '')
  })

  test('shows a picker only for multi-workspace users and switches context without logout', async ({ page }) => {
    const orgID = await signUp(page)
    await page.goto(`/app/${orgID}/settings/workspace`)
    const created = await page.evaluate(
      async ({ api, slug }) => {
        const response = await fetch(`${api}/orgs`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'include',
          body: JSON.stringify({ name: 'Second Fleet', slug }),
        })
        return { status: response.status, body: await response.text() }
      },
      { api: API, slug: `second-${Date.now()}` },
    )
    expect(created.status, created.body).toBe(201)

    await page.goto(`/app/${orgID}/settings/workspace`)
    const picker = page.getByRole('combobox', { name: 'Pindah ruang kerja' })
    await expect(picker).toBeVisible()
    await expect(picker.locator('option')).toHaveCount(2)

    await picker.selectOption({ label: 'Second Fleet' })
    await expect(page).toHaveURL(/\/app\/[^/]+\/settings\/workspace$/)
    await expect(page.getByRole('main').getByText('Second Fleet', { exact: true })).toBeVisible()
    await expect(page.getByRole('main').getByRole('button', { name: 'Ganti nama ruang kerja' })).toBeVisible()
  })
})
