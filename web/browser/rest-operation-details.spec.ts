import { expect, test, type Page } from '@playwright/test'
import { configurationFixture } from './configuration-fixtures'
import { upstream } from '../src/connections/fixtures'
import type { RESTOperation, Upstream } from '../src/api'
import { auditSettings } from './audit-fixtures'

async function setup(page: Page, role: 'reader' | 'administrator') {
  const op: RESTOperation = {
    name: 'browse',
    description: 'Directory',
    enabled: true,
    readOnly: true,
    method: 'GET',
    path: '/repos/{repo}/browse/{path}',
    parameters: [
      { name: 'repo', in: 'path', required: true, schema: { type: 'string' } },
      {
        name: 'path',
        in: 'path',
        required: false,
        pathMode: 'relative',
        schema: { type: 'string' },
      },
      { name: 'page', in: 'query', required: false, defaultValue: 1, schema: { type: 'integer' } },
      {
        name: 'fixed',
        in: 'query',
        required: false,
        defaultValue: 'fixed',
        exposeToMcp: false,
        schema: { type: 'string' },
      },
      ...['one', 'two', 'three'].map((name) => ({
        name,
        in: 'query' as const,
        required: false,
        schema: { type: 'string' },
      })),
    ],
    responseSchema: {
      type: 'object',
      properties: { a: { type: 'string' }, b: { type: 'number' }, c: { type: 'string' } },
    },
    responseFields: ['a', 'b', 'c'],
  }
  let saved: Upstream = {
    ...upstream,
    kind: 'rest',
    rest: {
      auth: { type: 'none' },
      forwardIdentity: false,
      requireIdentity: false,
      allowInsecureHttp: false,
      timeoutSeconds: 10,
      operations: [op, { ...op, name: 'second' }],
    },
  }
  const executions: string[] = []
  let writes = 0
  await page.route('**/auth/config', (r) => r.fulfill({ json: { oidc: false } }))
  await page.route('**/api/v1/**', (r) => {
    const capability = configurationFixture(r, role)
    if (capability) return capability
    const req = r.request(),
      path = new URL(req.url()).pathname
    if (path.endsWith('/me'))
      return r.fulfill({ json: { role: role === 'reader' ? 'viewer' : 'admin', name: 'Tester' } })
    if (path.endsWith('/audit-settings')) return r.fulfill({ json: auditSettings })
    if (path.endsWith('/rest/preview') || path.endsWith('/rest/test')) {
      const accept =
        saved.rest!.operations[0].responseFormat === 'text' ? 'text/plain' : 'application/json'
      if (path.endsWith('/preview'))
        return r.fulfill({ json: { method: 'GET', headers: { Accept: accept } } })
      executions.push(accept)
      return r.fulfill({
        json: {
          structuredContent: { result: accept === 'text/plain' ? 'first\nsecond' : { a: 'first' } },
        },
      })
    }
    if (path.endsWith('/upstreams/alpha')) {
      if (req.method() === 'PATCH') {
        writes++
        saved = { ...saved, ...req.postDataJSON(), version: saved.version + 1 }
      }
      return r.fulfill({ json: saved })
    }
    if (path.endsWith('/upstreams')) return r.fulfill({ json: { items: [saved] } })
    if (path.endsWith('/catalog'))
      return r.fulfill({
        json: { resources: [], resourceTemplates: [], prompts: [], capabilities: {} },
      })
    return r.fulfill({ json: { items: [] } })
  })
  await page.goto('/upstreams/alpha')
  return { executions, writes: () => writes }
}

test('reader can inspect inputs, request and projected result without editing or execution', async ({
  page,
}) => {
  const state = await setup(page, 'reader')
  await page.getByRole('button', { name: 'Details browse', exact: true }).click()
  const details = page.getByRole('dialog', { name: 'REST operation details · browse' })
  await expect(details.getByText('6 agent inputs · 1 fixed')).toBeVisible()
  await expect(details.getByText('Selected fields: a, b, c')).toBeVisible()
  await expect(details.getByText('Accept: application/json')).toBeVisible()
  await expect(details.getByText(/Optional · default 1/)).toBeVisible()
  await details.getByText('Full MCP tool definition').click()
  await expect(details.getByLabel('MCP tool definition')).toContainText('"required"')
  expect(state.executions).toHaveLength(0)
  expect(state.writes()).toBe(0)
})

test('convert a saved JSON operation, inspect its draft, save and test text', async ({ page }) => {
  const state = await setup(page, 'administrator')
  await page.getByRole('button', { name: 'Edit upstream', exact: true }).click()
  await page.getByRole('button', { name: 'Edit operation browse', exact: true }).click()
  const editor = page.getByRole('dialog', { name: 'Edit REST operation', exact: true })
  await editor.getByRole('button', { name: 'Edit path parameter path', exact: true }).press('Enter')
  const row = editor.getByRole('group', { name: 'Parameter 2', exact: true })
  await expect(row.locator('summary').first()).toBeFocused()
  await expect(editor.getByLabel('Path mode for path')).toBeVisible()
  const search = editor.getByLabel('Find parameters')
  await search.fill('path')
  await editor.getByLabel('Parameter 2 description').fill('Select this directory')
  await row.locator('summary').first().click()
  await expect(row.locator('summary').first()).toContainText('omit to request the parent path')
  await search.fill('page')
  await expect(editor.getByText('page', { exact: true })).toBeVisible()
  await search.fill('path')
  await row.locator('summary').first().click()
  await expect(editor.getByLabel('Parameter 2 description')).toHaveValue('Select this directory')
  await editor.getByLabel('Parameter 2 use default value').check()
  await editor.getByLabel('Parameter 2 default value').fill('src')
  await expect(row.locator('summary').first()).toContainText('default "src"')
  await expect(row.locator('summary').first()).not.toContainText('omit')
  await editor.getByRole('button', { name: 'Request & response schemas' }).click()
  const format = editor.getByLabel('Response format')
  await format.selectOption('text')
  await expect(page.getByText(/3 selected response fields/)).toBeVisible()
  await page.getByTestId('cancel-action').click()
  await expect(format).toHaveValue('json')
  await format.selectOption('text')
  await page.getByRole('button', { name: 'Switch to plain text', exact: true }).click()
  await editor.getByRole('button', { name: 'Apply operation to upstream draft' }).click()
  await expect(page.getByText('Unsaved change', { exact: true })).toHaveCount(1)
  await page.getByLabel('Operation', { exact: true }).selectOption('second')
  await expect(page.getByText(/This operation has unsaved changes/)).toHaveCount(0)
  await page.getByLabel('Operation', { exact: true }).selectOption('browse')
  await expect(page.getByText(/This operation has unsaved changes/)).toBeVisible()
  await page.getByRole('button', { name: 'Preview request' }).click()
  await expect(page.getByLabel('REST operation result')).toContainText('application/json')
  expect(state.executions).toHaveLength(0)
  const execute = async () => {
    await page.getByRole('button', { name: 'Execute test request' }).click()
    await page.getByRole('button', { name: 'Execute request', exact: true }).click()
  }
  await execute()
  await expect(page.getByLabel('Agent result')).toContainText('"a": "first"')
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect.poll(state.writes).toBe(1)
  await expect(page.getByText(/Preview and test use the saved Plain text operation/)).toBeVisible()
  await expect(page.getByText('Unsaved change', { exact: true })).toHaveCount(0)
  await execute()
  await expect(page.getByLabel('Agent result')).toHaveJSProperty('textContent', 'first\nsecond')
  expect(state.executions).toEqual(['application/json', 'text/plain'])
})
