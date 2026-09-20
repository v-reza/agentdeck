import { expect, test, type Page } from '@playwright/test'

/** US-AD07 AC1/AC3 — tenant boundary is enforced on real project endpoints. */
const API = '/api/v1'
const PASSWORD = 'correct-horse-battery-staple'

async function register(page: Page, name: string): Promise<string> {
  const response = await page.request.post(`${API}/auth/register`, {
    data: {
      email: `e2e-isolation-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`,
      password: PASSWORD,
      name,
      org_name: `${name} Fleet`,
    },
  })
  const body = await response.text()
  expect(response.status(), body).toBe(201)
  return (JSON.parse(body) as { workspace_id: string }).workspace_id
}

async function createProject(page: Page, orgID: string, name: string) {
  const result = await page.evaluate(
    async ({ api, orgID, name }) => {
      const response = await fetch(`${api}/projects`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-Org-ID': orgID },
        credentials: 'include',
        body: JSON.stringify({ name, slug: name.toLowerCase().replaceAll(' ', '-') }),
      })
      return { status: response.status, body: await response.text() }
    },
    { api: API, orgID, name },
  )
  expect(result.status, result.body).toBe(201)
}

async function listProjects(page: Page, orgID: string) {
  return page.evaluate(
    async ({ api, orgID }) => {
      const response = await fetch(`${api}/projects`, {
        headers: { 'X-Org-ID': orgID },
        credentials: 'include',
      })
      return { status: response.status, body: await response.text() }
    },
    { api: API, orgID },
  )
}

test('cross-workspace project reads are denied and do not leak data', async ({ browser }) => {
  const ownerA = await browser.newPage()
  const ownerB = await browser.newPage()
  const orgA = await register(ownerA, 'Tenant Alpha')
  const orgB = await register(ownerB, 'Tenant Beta')
  await ownerA.goto(`/app/${orgA}/projects`)
  await createProject(ownerA, orgA, 'Alpha Secret')

  const foreignRead = await listProjects(ownerA, orgB)
  expect(foreignRead.status).toBe(403)
  expect(foreignRead.body).not.toContain('Alpha Secret')

  const ownRead = await listProjects(ownerA, orgA)
  expect(ownRead.status).toBe(200)
  expect(ownRead.body).toContain('Alpha Secret')

  await ownerA.close()
  await ownerB.close()
})
