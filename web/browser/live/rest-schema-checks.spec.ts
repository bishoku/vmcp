import { expect, test } from '@playwright/test'

test('production CSP permits draft schema checks before explicit Save', async ({
  page,
  request,
}) => {
  const slug = 'schema-check-' + Date.now().toString(36)
  const url = process.env.VMCP_LIVE_API + '/api/v1/upstreams/' + slug
  const headers = { Authorization: 'Bearer demo-admin-token' }
  const document = await page.goto('/upstreams/new')
  expect(document?.headers()['content-security-policy']).toContain("script-src 'self';")
  await page.getByLabel('Admin token').fill('demo-admin-token')
  await page.getByRole('button', { name: 'Continue', exact: true }).click()
  await page.getByLabel('Upstream type', { exact: true }).selectOption('rest')
  await page.getByLabel('Slug', { exact: true }).fill(slug)
  await page.getByLabel('URL', { exact: true }).fill('https://example.com')
  await page.getByRole('button', { name: 'Import OpenAPI / Swagger' }).click()
  await page.getByLabel('OpenAPI / Swagger definition').fill(
    JSON.stringify({
      openapi: '3.1.0',
      paths: {
        '/items': {
          get: {
            operationId: 'items',
            parameters: [{ in: 'query', name: 'limit', schema: { type: 'integer', minimum: 2 } }],
            responses: {
              '200': {
                content: {
                  'application/json': {
                    schema: { type: 'object', properties: { id: { type: 'integer' } } },
                  },
                },
              },
            },
          },
        },
      },
    }),
  )
  await page.getByRole('button', { name: 'Analyze definition' }).click()
  await page.getByRole('checkbox', { name: 'GET /items', exact: true }).check()
  await page.getByRole('button', { name: 'Add 1 to draft' }).click()
  await page.getByRole('button', { name: 'Edit operation items' }).click()
  const op = page.getByRole('dialog', { name: 'Edit REST operation' })
  await op.getByRole('button', { name: 'Parameters (1)' }).click()
  await op.getByLabel('Parameter 1 use default value').check()
  await op.getByLabel('Parameter 1 default value').fill('1')
  await op.getByRole('button', { name: 'Apply operation' }).click()
  await expect(op.getByText(/Parameter limit:.*minimum/i)).toBeVisible()
  await op.getByLabel('Parameter 1 default value').fill('2')
  await op.getByRole('button', { name: 'Request & response schemas' }).click()
  await op.getByRole('button', { name: 'Edit response schema' }).click()
  const schema = page.getByRole('dialog', { name: 'Response schema', exact: true })
  await schema.getByLabel('Response example JSON').fill('{"id":null}')
  await schema.getByRole('button', { name: 'Apply schema' }).click()
  await expect(schema.getByText(/\/id:.*integer/)).toBeVisible()
  await schema.getByLabel('Response example JSON').fill('{"id":2}')
  await schema.getByRole('button', { name: 'Apply schema' }).click()
  await expect(schema).not.toBeVisible()
  await op.getByRole('button', { name: 'Apply operation' }).click()
  await expect(op).not.toBeVisible()
  expect((await request.get(url, { headers })).status()).toBe(404)
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect.poll(async () => (await request.get(url, { headers })).status()).toBe(200)
  const saved = await (await request.get(url, { headers })).json()
  expect(saved.rest.operations[0].parameters[0].defaultValue).toBe(2)
})
