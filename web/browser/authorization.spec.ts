import { expect, test, type Page } from '@playwright/test'
import { accessFixture, ids } from '../src/authorization/fixtures'
import { answerConfirmation } from './confirmation'

async function accessRoutes(page: Page, role = 'admin') {
  const state = structuredClone(accessFixture),
    requests: {
      method: string
      path: string
      body: Record<string, unknown> | undefined
      version: string | undefined
    }[] = []
  let serial = 10,
    deny = false
  const newId = () => `90000000-0000-0000-0000-${String(serial++).padStart(12, '0')}`
  await page.route('**/auth/config', (r) => r.fulfill({ json: { oidc: true } }))
  await page.route('**/api/v1/**', async (route) => {
    const req = route.request(),
      url = new URL(req.url()),
      path = url.pathname.replace('/api/v1', ''),
      method = req.method()
    if (path === '/configuration-access')
      return route.fulfill({
        json: {
          authorizationRevision: state.status.authorizationRevision,
          allowed: req.postDataJSON().checks.map(() => role === 'admin' && !deny),
        },
      })
    if (path === '/me')
      return route.fulfill({ json: { name: 'Access Reviewer', role, via: 'oidc' } })
    if (path === '/audit-events')
      return route.fulfill({ json: { items: [], capabilities: { canFilterIdentities: false } } })
    const body = req.postDataJSON() as Record<string, unknown> | undefined
    requests.push({ method, path, body, version: req.headers()['if-match'] })
    if (role !== 'admin' || deny)
      return route.fulfill({ status: 403, json: { detail: 'Platform administrator required' } })
    if (path === '/permissions') return route.fulfill({ json: { items: state.permissions } })
    if (path === '/roles' && method === 'GET')
      return route.fulfill({
        json: { items: state.roles, authorizationRevision: state.status.authorizationRevision },
      })
    if (path === '/role-assignments' && method === 'GET')
      return route.fulfill({
        json: {
          items: state.assignments,
          authorizationRevision: state.status.authorizationRevision,
        },
      })
    if (path === '/identity-mappings' && method === 'GET')
      return route.fulfill({
        json: { items: state.mappings, authorizationRevision: state.status.authorizationRevision },
      })
    if (path === '/access/status') return route.fulfill({ json: state.status })
    if (path === '/access/scopes')
      return route.fulfill({
        json: {
          items:
            url.searchParams.get('kind') === 'control_token'
              ? [{ id: ids.token, slug: 'reader-token', name: 'Reader token' }]
              : [
                  {
                    id: ids.resource,
                    slug: 'crm',
                    name: 'CRM',
                    epoch: ids.newEpoch,
                    ownerTeam: ids.newOwner,
                  },
                ],
        },
      })
    if (path === '/access/resource-epochs')
      return route.fulfill({
        json: {
          items: [
            {
              id: ids.oldEpoch,
              kind: 'upstream',
              resourceId: ids.resource,
              ownerTeam: ids.owner,
              startRevision: 1,
              endRevision: 3,
            },
            {
              id: ids.newEpoch,
              kind: 'upstream',
              resourceId: ids.resource,
              ownerTeam: ids.newOwner,
              startRevision: 4,
              endRevision: null,
            },
          ],
        },
      })
    if (path === '/access/evaluate')
      return route.fulfill({
        json: {
          simulationOnly: true,
          authenticationValidated: false,
          authorizationRevision: state.status.authorizationRevision,
          claimMappings: 'live-audit',
          matchedMappingIds: [],
          candidateAssignmentIds: [],
          decision: { allowed: false, reason: 'audit_policy_denied' },
        },
      })
    const group = path.startsWith('/roles')
      ? 'role'
      : path.startsWith('/role-assignments')
        ? 'assignment'
        : path.startsWith('/identity-mappings')
          ? 'mapping'
          : ''
    if (!group)
      return route.fulfill({ status: 404, json: { detail: 'Unknown management endpoint' } })
    const collection =
      group === 'role' ? state.roles : group === 'assignment' ? state.assignments : state.mappings
    const key = decodeURIComponent(path.split('/')[2] ?? ''),
      existing = collection.find((v) => ('slug' in v ? v.slug : v.id) === key)
    if (method === 'GET')
      return route.fulfill({ json: existing ?? {}, status: existing ? 200 : 404 })
    if (existing && 'builtin' in existing && existing.builtin)
      return route.fulfill({ status: 403, json: { detail: 'Builtin roles cannot be changed' } })
    if (existing && req.headers()['if-match'] !== `"${existing.version}"`)
      return route.fulfill({ status: 412, json: { detail: 'Version changed' } })
    state.status.authorizationRevision++
    if (method === 'DELETE') {
      const list = collection as { id: string }[]
      list.splice(
        list.findIndex((v) => v.id === existing!.id),
        1,
      )
      return route.fulfill({ status: 204 })
    }
    const saved = {
      ...existing,
      ...body,
      id: existing?.id ?? newId(),
      version: (existing?.version ?? 0) + 1,
      ...(group === 'role' ? { builtin: false } : {}),
      ...(group === 'assignment' && !body?.principal ? { principal: { kind: '', id: '' } } : {}),
    }
    if (existing) Object.assign(existing, saved)
    else (collection as unknown[]).push(saved)
    return route.fulfill({ status: existing ? 200 : 201, json: saved })
  })
  return {
    state,
    requests,
    deny: () => {
      deny = true
    },
  }
}
for (const width of [390, 1440])
  test(`admin manages roles mappings assignments and simulation at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 950 })
    const f = await accessRoutes(page)
    await page.goto('/access')
    await expect(page.getByRole('heading', { name: 'Access management' })).toBeVisible()
    const observer = page
      .getByRole('article')
      .filter({ has: page.getByRole('heading', { name: 'Observer', exact: true }) })
    await page
      .getByRole('searchbox', { name: 'Search access configuration', exact: true })
      .fill('Observer')
    await expect(observer).toBeVisible()
    await expect(page.getByRole('article')).toHaveCount(1)
    await page
      .getByRole('searchbox', { name: 'Search access configuration', exact: true })
      .fill('no-matching-role')
    await expect(page.getByRole('article')).toHaveCount(0)
    await page.getByRole('searchbox', { name: 'Search access configuration', exact: true }).fill('')
    await expect(observer).toBeVisible()
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true)
    await page.screenshot({ path: `/tmp/vmcp-access-inventory-${width}.png`, fullPage: true })
    await observer.getByRole('button', { name: 'Copy', exact: true }).click()
    const copy = page.getByRole('dialog', { name: 'Copy role' })
    const requestPermission = copy.getByRole('checkbox', {
      name: 'Read captured requests',
      exact: true,
    })
    await expect(requestPermission).toHaveAccessibleDescription(/Does not grant response access/)
    await expect(requestPermission.locator('..')).toHaveCSS('align-items', 'flex-start')
    await copy.getByRole('searchbox', { name: 'Find permissions', exact: true }).fill('metadata')
    await expect(
      copy.getByRole('checkbox', { name: 'View audit events', exact: true }),
    ).toBeVisible()
    await expect(requestPermission).toHaveCount(0)
    await copy.getByRole('searchbox', { name: 'Find permissions', exact: true }).fill('')
    expect(await copy.evaluate((d) => d.scrollWidth <= d.clientWidth)).toBe(true)
    await page.screenshot({ path: `/tmp/vmcp-access-role-${width}.png`, fullPage: true })
    await requestPermission.locator('..').scrollIntoViewIfNeeded()
    await page.screenshot({ path: `/tmp/vmcp-access-permissions-${width}.png`, fullPage: true })
    await copy.getByLabel('Role identifier', { exact: true }).fill('audit-copy')
    await copy.getByLabel('Role name', { exact: true }).fill('Audit copy')
    await copy.getByRole('checkbox', { name: 'View audit events', exact: true }).check()
    await copy
      .getByRole('checkbox', {
        name: 'Read captured requests',
        exact: true,
      })
      .check()
    await copy.getByRole('button', { name: 'Save role', exact: true }).click()
    await expect(copy).toHaveCount(0)
    const created = f.requests.find((r) => r.path === '/roles' && r.method === 'POST')!
    expect(created.body).not.toHaveProperty('builtin')
    expect(created.body!.permissions).not.toContain('audit.payloads.response.read')
    const card = page
      .getByRole('article')
      .filter({ has: page.getByRole('heading', { name: 'Audit copy', exact: true }) })
    await card.getByRole('button', { name: 'Edit', exact: true }).click()
    await page.getByLabel('Role name', { exact: true }).fill('Audit copy updated')
    await page.getByRole('button', { name: 'Save role', exact: true }).click()
    await expect(page.getByRole('dialog')).toHaveCount(0)
    expect(
      f.requests.find((r) => r.path === '/roles/audit-copy' && r.method === 'PUT')?.version,
    ).toBe('"1"')
    await page.getByRole('button', { name: 'Claim mappings', exact: true }).click()
    await page.getByRole('button', { name: 'Create mapping', exact: true }).click()
    await expect(page.getByRole('region', { name: 'Claim conditions', exact: true })).toBeVisible()
    await page.screenshot({ path: `/tmp/vmcp-access-mapping-${width}.png`, fullPage: true })
    await page.getByLabel('Claim path 1', { exact: true }).fill('scope')
    await page.getByLabel('Operator 1', { exact: true }).selectOption('scope_contains')
    await page.getByLabel('Claim value 1', { exact: true }).fill('audit:read')
    await page.getByRole('button', { name: 'Save mapping', exact: true }).click()
    await expect(page.getByRole('dialog')).toHaveCount(0)
    const mapping = f.state.mappings.at(-1)!
    await page.getByRole('button', { name: 'Assignments', exact: true }).click()
    await page.getByRole('button', { name: 'Create assignment', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: 'Create role assignment' })
    await expect(
      dialog.getByRole('region', { name: 'Role and identity', exact: true }),
    ).toBeVisible()
    await page.screenshot({ path: `/tmp/vmcp-access-assignment-${width}.png`, fullPage: true })
    await dialog.getByLabel('Assigned role', { exact: true }).selectOption(f.state.roles.at(-1)!.id)
    await dialog.getByLabel('Identity source', { exact: true }).selectOption('mapping')
    await dialog.getByLabel('Claim mapping', { exact: true }).selectOption(mapping.id)
    await dialog.getByLabel('Scope kind', { exact: true }).selectOption('upstream')
    await dialog.getByLabel('Resource', { exact: true }).selectOption(ids.resource)
    await dialog.getByLabel('Ownership period', { exact: true }).selectOption(ids.oldEpoch)
    await dialog.getByRole('checkbox', { name: 'control', exact: true }).check()
    await dialog.screenshot({ path: `/tmp/vmcp-access-assignment-${width}.png` })
    await dialog.getByRole('button', { name: 'Save assignment', exact: true }).click()
    await expect(dialog).toHaveCount(0)
    const assigned = f.requests.find((r) => r.path === '/role-assignments' && r.method === 'POST')!
    expect(assigned.body).not.toHaveProperty('principal')
    expect(assigned.body!.scope).toEqual({
      kind: 'upstream',
      id: ids.resource,
      epoch: ids.oldEpoch,
      ownerTeam: ids.owner,
    })
    expect(assigned.body!.captureScopes).toEqual(['control'])
    await page.getByRole('button', { name: 'Simulation', exact: true }).click()
    await page.getByLabel('OIDC subject', { exact: true }).fill('sample-reviewer')
    await page.getByLabel('Sample OIDC claims', { exact: true }).fill('{"scope":"audit:read"}')
    await page
      .getByLabel('Permission to evaluate', { exact: true })
      .selectOption('audit.payloads.request.read')
    await page.getByLabel('Evaluation target', { exact: true }).selectOption('event')
    await page.getByLabel('Audit event UUID', { exact: true }).fill(ids.event)
    await page.getByRole('button', { name: 'Evaluate permission', exact: true }).click()
    await expect(page.getByRole('heading', { name: 'Denied by stored policy' })).toBeVisible()
    expect(f.requests.filter((r) => r.path.includes('/payload'))).toHaveLength(0)
    await page.getByRole('button', { name: 'Access status', exact: true }).click()
    await expect(page.getByRole('heading', { name: 'Access warnings' })).toBeVisible()
    await expect(page.getByRole('button', { name: /activate|apply enforcement/i })).toHaveCount(0)
    await page.screenshot({ path: `/tmp/vmcp-access-migration-${width}.png`, fullPage: true })
    // Confirmation and If-Match deletion operate on the selected assignment only.
    await page.getByRole('button', { name: 'Assignments', exact: true }).click()
    const assignedCard = page
      .getByRole('article')
      .filter({ has: page.getByRole('heading', { name: 'Audit copy updated', exact: true }) })
    await assignedCard.getByRole('button', { name: 'Delete', exact: true }).click()
    await answerConfirmation(page, true)
    await expect(assignedCard).toHaveCount(0)
    expect(f.requests.find((r) => r.method === 'DELETE')?.version).toBe('"1"')
  })
for (const role of ['viewer', 'editor', 'team', 'custom'])
  test(`${role} direct management URL never downloads management data`, async ({ page }) => {
    const f = await accessRoutes(page, role)
    await page.goto('/access?section=simulate')
    await expect(page.getByRole('alert')).toContainText('administrator')
    expect(f.requests).toHaveLength(0)
    await expect(page.getByRole('link', { name: 'Access management', exact: true })).toHaveCount(0)
  })
test('server-side admin denial removes the open form and saved management metadata', async ({
  page,
}) => {
  const f = await accessRoutes(page)
  await page.goto('/access?section=mappings')
  await page.getByRole('button', { name: 'Edit', exact: true }).click()
  await page.getByLabel('Claim value 1', { exact: true }).fill('sensitive-unsubmitted-claim')
  f.deny()
  await page.getByRole('button', { name: 'Save mapping', exact: true }).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.getByRole('alert')).toContainText('Access is no longer available')
  await expect(page.locator('body')).not.toContainText('sensitive-unsubmitted-claim')
  await expect(page.locator('body')).not.toContainText('https://trusted.example')
})
