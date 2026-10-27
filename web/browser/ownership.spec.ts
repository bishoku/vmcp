import { expect, test, type Page } from '@playwright/test'

async function fixture(page: Page, role = 'admin') {
 const teams: Record<string, unknown>[] = [{ slug: 'crm', name: 'CRM owners', issuer: 'https://idp.example', group: 'crm-owners', version: 1, canManage: true }]
 const projects: Record<string, unknown>[] = [{ slug: 'default', name: 'Default', ownerTeam: '', version: 1 }]
 const grants: Record<string, unknown>[] = []
 const state = { teams, projects, grants, catalogReads: 0, failTeams: false, failSave: false, failSaveStatus: 500 }
 await page.route('**/auth/config', route => route.fulfill({ json: { oidc: false } }))
 await page.route('**/api/v1/**', route => {
  const request = route.request(), path = new URL(request.url()).pathname
  if (path.endsWith('/me')) return route.fulfill({ json: { role, name: 'Platform admin' } })
  const items = path.endsWith('/teams') ? teams : path.endsWith('/projects') ? projects : path.endsWith('/tool-grants') ? grants : null
  if (items) {
   if (path.endsWith('/teams') && state.failTeams) return route.fulfill({ status: 503, json: { message: 'Team service unavailable' } })
   if (request.method() === 'POST') {
    if (state.failSave) return route.fulfill({ status: state.failSaveStatus, json: { message: 'Save failed' } })
    const value = request.postDataJSON(), i = items.findIndex(old => value.slug ? old.slug === value.slug : old.virtualServer === value.virtualServer && old.tool === value.tool && old.upstream === value.upstream)
    if (i >= 0) items[i] = { ...value, version: Number(value.version) + 1 }
    else items.push({ ...value, version: 1 })
    return route.fulfill({ json: { ok: true } })
   }
   return route.fulfill({ json: { items } })
  }
  if (path.endsWith('/virtual-servers')) return route.fulfill({ json: { items: [{ slug: 'chat', name: 'Customer chat', project: 'customer-chat' }] } })
  if (path.endsWith('/tool-catalog')) { state.catalogReads++; return route.fulfill({ json: { items: [{ upstream: 'crm', ownerTeam: 'crm', name: 'get_customer', description: 'Read customer information' }] } }) }
  return route.fulfill({ json: { items: [] } })
 })
 return state
}
const section = (page: Page, name: string) => page.getByRole('navigation', { name: 'Ownership sections' }).getByRole('button', { name, exact: true }).click()

for (const width of [390, 1440]) test(`ownership workflow and shared responsive components ${width}`, async ({ page }) => {
 // This scenario covers team/project creation and the full grant approval/revocation
 // lifecycle with screenshots. Give the complete workflow a bounded CI budget;
 // individual Playwright assertions retain their normal timeouts.
 test.setTimeout(60_000)
 await page.setViewportSize({ width, height: 1000 })
 const state = await fixture(page)
 await page.goto('/ownership')
 await expect(page.getByRole('table', { name: 'Configured teams', exact: true })).toBeVisible()
 expect(state.catalogReads).toBe(0)
 await page.screenshot({ path: test.info().outputPath(`teams-${width}.png`), fullPage: true })
 await page.getByRole('button', { name: 'New team', exact: true }).click()
 const team = page.getByRole('dialog', { name: 'New team', exact: true })
 await team.getByLabel('Name', { exact: true }).fill('Chat owners')
 await team.getByLabel('Identifier', { exact: true }).fill('chat-owners')
 await team.getByLabel('OIDC issuer', { exact: true }).fill('https://idp.example')
 await team.getByLabel('Group claim value', { exact: true }).fill('customer-chat-owners')
 await team.getByRole('button', { name: 'Save team', exact: true }).click()
 await expect(team).toHaveCount(0)
 await expect.poll(() => state.teams.length).toBe(2)
 await section(page, 'Projects')
 await page.getByRole('button', { name: 'New project', exact: true }).click()
 const project = page.getByRole('dialog', { name: 'New project', exact: true })
 await project.getByLabel('Name', { exact: true }).fill('Customer chat')
 await project.getByLabel('Identifier', { exact: true }).fill('customer-chat')
 // Newly created teams appear without a page reload.
 await project.getByLabel('Owner team', { exact: true }).selectOption('chat-owners')
 await page.screenshot({ path: test.info().outputPath(`project-editor-${width}.png`), fullPage: true })
 await project.getByRole('button', { name: 'Save project', exact: true }).click()
 await expect(project).toHaveCount(0)
 await expect.poll(() => state.projects.length).toBe(2)
 await page.screenshot({ path: test.info().outputPath(`projects-${width}.png`), fullPage: true })
 await section(page, 'Tool access')
 await page.getByRole('button', { name: 'Request access', exact: true }).first().click()
 const request = page.getByRole('dialog', { name: 'Request tool access', exact: true })
 await request.getByLabel('Virtual server', { exact: true }).selectOption('chat')
 await request.getByLabel('Search available tools', { exact: true }).fill('get_customer')
 await request.getByLabel('Tool', { exact: true }).selectOption(JSON.stringify(['crm', 'get_customer']))
 await request.getByLabel('Reason', { exact: true }).fill('Customer support')
 await request.getByRole('button', { name: 'Submit request', exact: true }).click()
 await expect(request).toHaveCount(0)
 await expect.poll(() => state.grants[0]?.status).toBe('pending')
 await page.getByRole('button', { name: 'Review request', exact: true }).click()
 await page.screenshot({ path: test.info().outputPath(`access-review-${width}.png`), fullPage: true })
 await page.getByRole('dialog', { name: 'Tool access request', exact: true }).getByRole('button', { name: 'Approve', exact: true }).click()
 await page.getByRole('dialog', { name: 'Approve tool access?', exact: true }).getByRole('button', { name: 'Approve access', exact: true }).click()
 await expect.poll(() => state.grants[0]?.status).toBe('approved')
 await expect(page.getByRole('dialog')).toHaveCount(0)
 await page.getByRole('button', { name: 'Review request', exact: true }).click()
 await page.getByRole('dialog', { name: 'Tool access request', exact: true }).getByRole('button', { name: 'Revoke', exact: true }).click()
 await page.getByRole('dialog', { name: 'Revoke tool access?', exact: true }).getByRole('button', { name: 'Revoke access', exact: true }).click()
 await expect.poll(() => state.grants[0]?.status).toBe('revoked')
 await expect(page.getByRole('dialog')).toHaveCount(0)
 await page.getByLabel('Access request status').selectOption('pending')
 await expect(page.getByText('No matching results', { exact: true })).toBeVisible()
 await page.getByRole('button', { name: 'Clear filters', exact: true }).first().click()
 await expect(page.getByRole('table', { name: 'Tool access requests', exact: true }).getByText('Revoked', { exact: true })).toBeVisible()
 expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
 await page.screenshot({ path: test.info().outputPath(`tool-access-${width}.png`), fullPage: true })
})

test('validation, failed saves and dirty dismissal retain entered values', async ({ page }) => {
 const state = await fixture(page)
 await page.goto('/ownership')
 await page.getByRole('button', { name: 'New team', exact: true }).click()
 const dialog = page.getByRole('dialog', { name: 'New team', exact: true })
 await dialog.getByRole('button', { name: 'Save team', exact: true }).click()
 await expect(dialog.getByLabel('Identifier', { exact: true })).toHaveAttribute('aria-invalid', 'true')
 // Validation moves focus on the next animation frame. Wait for that transition
 // before typing into another field so it cannot redirect Playwright's input.
 await expect(dialog.getByLabel('Identifier', { exact: true })).toBeFocused()
 await dialog.getByLabel('Name', { exact: true }).fill('My team')
 await dialog.getByLabel('Identifier', { exact: true }).fill('my-team')
 await dialog.getByLabel('OIDC issuer').fill('https://idp.example')
 await dialog.getByLabel('Group claim value').fill('my-group')
 state.failSave = true
 const failedSave = page.waitForResponse(response => response.url().endsWith('/api/v1/teams') && response.request().method() === 'POST')
 await dialog.getByRole('button', { name: 'Save team', exact: true }).click()
 expect((await failedSave).status()).toBe(500)
 await expect(dialog.getByRole('alert')).toContainText('Internal Server Error')
 await expect(dialog.getByLabel('Name', { exact: true })).toHaveValue('My team')
 await dialog.getByRole('button', { name: 'Cancel', exact: true }).click()
 const confirm = page.getByRole('dialog', { name: 'Discard unsaved changes?', exact: true })
 await confirm.getByRole('button', { name: 'Keep editing', exact: true }).click()
 await expect(dialog.getByLabel('Name', { exact: true })).toHaveValue('My team')
 await dialog.getByRole('button', { name: 'Cancel', exact: true }).click()
 await confirm.getByRole('button', { name: 'Discard changes', exact: true }).click()
 await expect(dialog).toHaveCount(0)
 expect(state.teams.length).toBe(1)
})

test('project owner changes need confirmation and Default ownership stays locked', async ({ page }) => {
 const state = await fixture(page)
 state.projects.push({ slug: 'chat', name: 'Chat', ownerTeam: 'crm', version: 4 })
 await page.goto('/ownership?section=projects')
 await page.getByRole('button', { name: 'Default', exact: true }).click()
 await page.getByRole('button', { name: 'Edit project', exact: true }).click()
 await expect(page.getByLabel('Owner team', { exact: true })).toBeDisabled()
 await page.getByRole('dialog').getByRole('button', { name: 'Cancel', exact: true }).click()
 await page.getByRole('button', { name: 'Chat', exact: true }).click()
 await page.getByRole('button', { name: 'Edit project', exact: true }).click()
 await page.getByLabel('Owner team', { exact: true }).selectOption('')
 await page.getByRole('button', { name: 'Save project', exact: true }).click()
 const confirm = page.getByRole('dialog', { name: 'Change project ownership?', exact: true })
 await expect(confirm).toContainText('grants')
 await confirm.getByRole('button', { name: 'Cancel', exact: true }).click()
 expect(state.projects[1].ownerTeam).toBe('crm')
 await page.getByRole('button', { name: 'Save project', exact: true }).click()
 await confirm.getByRole('button', { name: 'Change owner', exact: true }).click()
 await expect.poll(() => state.projects[1].ownerTeam).toBe('')
 expect(state.projects[1].version).toBe(5)
})

for (const role of ['viewer', 'editor']) test(`${role} sees permitted actions only`, async ({ page }) => {
 const state = await fixture(page, role)
 state.grants.push({ virtualServer: 'chat', upstream: 'crm', tool: 'get_customer', status: 'pending', reason: 'Support', version: 1, canDecide: false })
 await page.goto('/ownership')
 await expect(page.getByRole('button', { name: 'New team', exact: true })).toHaveCount(0)
 await page.getByRole('button', { name: 'CRM owners', exact: true }).click()
 await expect(page.getByRole('button', { name: 'Edit team', exact: true })).toHaveCount(0)
 await page.getByRole('dialog').getByRole('button', { name: 'Close', exact: true }).last().click()
 await page.getByRole('navigation', { name: 'Ownership sections' }).getByRole('button', { name: /Tool access/ }).click()
 await page.getByRole('button', { name: 'View request', exact: true }).click()
 await expect(page.getByRole('button', { name: 'Approve', exact: true })).toHaveCount(0)
 await expect(page.getByRole('dialog')).toContainText('Only the upstream owner')
})

test('partial read failure preserves other panels and retry recovers', async ({ page }) => {
 const state = await fixture(page)
 state.failTeams = true
 await page.goto('/ownership?section=projects')
 await expect(page.getByRole('alert')).toContainText('Saved configuration could not be loaded')
 await expect(page.getByRole('button', { name: 'Default', exact: true })).toBeVisible()
 await expect(page.getByRole('button', { name: 'New project', exact: true })).toBeDisabled()
 state.failTeams = false
 await page.getByRole('button', { name: 'Try again', exact: true }).click()
 await expect(page.getByRole('alert')).toHaveCount(0)
 await section(page, 'Teams')
 await page.getByLabel('Search teams').fill('not-present')
 await expect(page.getByText('No matching results', { exact: true })).toBeVisible()
})


test('stale project reload uses the latest ownership and version for subsequent confirmation', async ({ page }) => {
 const state = await fixture(page)
 state.projects.push({ slug: 'chat', name: 'Chat', ownerTeam: 'crm', version: 4 })
 await page.goto('/ownership?section=projects')
 await page.getByRole('button', { name: 'Chat', exact: true }).click()
 await page.getByRole('button', { name: 'Edit project', exact: true }).click()
 await page.getByLabel('Name', { exact: true }).fill('My draft')
 state.projects[1] = { slug: 'chat', name: 'Updated remotely', ownerTeam: '', version: 5 }
 state.failSave = true; state.failSaveStatus = 412
 await page.getByRole('button', { name: 'Save project', exact: true }).click()
 await expect(page.getByText('A newer version is available', { exact: true })).toBeVisible()
 state.failSave = false
 await page.getByRole('button', { name: 'Reload latest and discard draft', exact: true }).click()
 await page.getByRole('dialog', { name: 'Discard this draft and load the latest version?', exact: true }).getByRole('button', { name: 'Load latest and discard', exact: true }).click()
 await expect(page.getByLabel('Name', { exact: true })).toHaveValue('Updated remotely')
 await expect(page.getByLabel('Owner team', { exact: true })).toHaveValue('')
 await page.getByLabel('Owner team', { exact: true }).selectOption('crm')
 await page.getByRole('button', { name: 'Save project', exact: true }).click()
 await page.getByRole('dialog', { name: 'Change project ownership?', exact: true }).getByRole('button', { name: 'Change owner', exact: true }).click()
 await expect.poll(() => state.projects[1].version).toBe(6)
 expect(state.projects[1].ownerTeam).toBe('crm')
})
