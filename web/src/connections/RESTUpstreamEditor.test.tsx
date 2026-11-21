import { api, type RESTConfig } from '../api'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { schemaCheckFixture } from '../../test-fixtures/schema-checks'
import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { useState } from 'react'
import { render } from '../test-utils'
import { upstream } from './fixtures'
import RESTUpstreamEditor, { RESTConnectionSettings } from './RESTUpstreamEditor'
import { emptyREST } from './rest-config'
import { RoleContext } from '../interaction'
import orderFixture from './testdata/order-mapping.json'

beforeEach(() => {
  vi.spyOn(api, 'validateRESTSchemas').mockImplementation(async (checks) =>
    schemaCheckFixture(checks),
  )
})

HTMLDialogElement.prototype.showModal = function () {
  this.setAttribute('open', '')
}
HTMLDialogElement.prototype.close = function () {
  this.removeAttribute('open')
}
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})
const config: RESTConfig = {
  ...emptyREST(),
  auth: { type: 'basic', username: 'service', hasPassword: true },
  operations: [
    {
      name: 'lookup',
      description: 'Look up an order',
      enabled: true,
      method: 'GET',
      path: '/orders',
      parameters: [],
      readOnly: true,
      headerNames: ['X-Key'],
    },
  ],
}
function Draft({ saved = false }: { saved?: boolean }) {
  const [value, setValue] = useState(config)
  return (
    <>
      <RESTConnectionSettings value={value} onChange={setValue} />
      <RESTUpstreamEditor
        value={value}
        onChange={setValue}
        upstream={saved ? { ...upstream, kind: 'rest', rest: config } : null}
      />
      <output data-testid="rest-draft">{JSON.stringify(value)}</output>
    </>
  )
}
const operation = () => screen.getByRole('dialog', { name: 'Edit REST operation' })
function edit() {
  fireEvent.click(screen.getByRole('button', { name: 'Edit operation lookup' }))
  return within(operation())
}
async function apply() {
  fireEvent.click(
    within(operation()).getByRole('button', { name: 'Apply operation to upstream draft' }),
  )
  await waitFor(() =>
    expect(screen.queryByRole('dialog', { name: 'Edit REST operation' })).toBeNull(),
  )
}
test('operation edits are staged in modal; apply and cancel preserve credential handling', async () => {
  vi.spyOn(window, 'confirm').mockReturnValue(true)
  const save = vi.spyOn(api, 'patchUpstream')
  render(<Draft />)
  expect(screen.queryByLabelText('Tool name')).toBeNull()
  const editor = edit()
  fireEvent.change(editor.getByLabelText('Tool name'), { target: { value: 'find_order' } })
  expect(JSON.parse(screen.getByTestId('rest-draft').textContent!).operations[0].name).toBe(
    'lookup',
  )
  fireEvent.click(editor.getByRole('button', { name: 'Cancel operation changes' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  fireEvent.change(edit().getByLabelText('Tool name'), { target: { value: 'find_order' } })
  await apply()
  expect(JSON.parse(screen.getByTestId('rest-draft').textContent!).operations[0].name).toBe(
    'find_order',
  )
  expect(save).not.toHaveBeenCalled()
  expect(screen.getByLabelText('Password')).toHaveProperty('value', '')
})
test('nested order schema applies atomically through schema and operation drafts', async () => {
  render(<Draft />)
  const editor = edit()
  fireEvent.change(editor.getByLabelText('HTTP method'), { target: { value: 'POST' } })
  fireEvent.click(editor.getByRole('button', { name: 'Request & response schemas' }))
  fireEvent.click(editor.getByRole('button', { name: 'Edit request body schema' }))
  const schema = within(screen.getByRole('dialog', { name: 'Request body schema' }))
  fireEvent.change(schema.getByLabelText('Request body example JSON'), {
    target: { value: JSON.stringify(orderFixture.body) },
  })
  fireEvent.click(schema.getByRole('button', { name: 'Generate request body schema' }))
  await schema.findByRole('button', { name: 'Expand all fields' })
  fireEvent.click(schema.getByRole('button', { name: 'Apply schema' }))
  await waitFor(() =>
    expect(screen.queryByRole('dialog', { name: 'Request body schema' })).toBeNull(),
  )
  await apply()
  const op = JSON.parse(screen.getByTestId('rest-draft').textContent!).operations[0]
  expect(op.bodySchema).toEqual(orderFixture.bodySchema)
})
test('parameter editing preserves constraints and updates the local MCP preview', async () => {
  render(<Draft />)
  const editor = edit()
  fireEvent.change(editor.getByLabelText('Operation path'), { target: { value: '/orders/{id}' } })
  fireEvent.click(editor.getByRole('button', { name: 'Parameters (1)' }))
  fireEvent.change(editor.getByLabelText('Parameter 1 type'), { target: { value: 'integer' } })
  fireEvent.click(editor.getByRole('button', { name: 'Add query parameter' }))
  fireEvent.change(editor.getByLabelText('Parameter 2 name'), { target: { value: 'expanded' } })
  expect(
    JSON.parse(editor.getByLabelText('MCP tool definition').textContent!).inputSchema.properties.id,
  ).toEqual({ type: 'integer' })
  await apply()
  expect(
    JSON.parse(screen.getByTestId('rest-draft').textContent!).operations[0].parameters,
  ).toHaveLength(2)
})
test('invalid advanced schema blocks Apply without changing the upstream draft', () => {
  render(<Draft />)
  const editor = edit()
  fireEvent.click(editor.getByRole('button', { name: 'Advanced settings' }))
  fireEvent.change(editor.getByLabelText('Response schema'), { target: { value: '{' } })
  expect(editor.getByRole('button', { name: 'Apply operation to upstream draft' })).toHaveProperty(
    'disabled',
    true,
  )
  expect(
    JSON.parse(screen.getByTestId('rest-draft').textContent!).operations[0].responseSchema,
  ).toBeUndefined()
})
test('import opens a modal, shows blocked reasons and appends selected tools without saving', async () => {
  vi.spyOn(api, 'importREST').mockResolvedValue({
    title: 'Orders',
    version: '3.1',
    servers: [],
    warnings: [],
    operations: [
      {
        id: 'GET /new',
        method: 'GET',
        path: '/new',
        summary: 'New',
        tags: [],
        warnings: [],
        errors: [],
        operation: { ...config.operations[0], name: 'new_lookup', path: '/new' },
      },
    ],
  })
  const save = vi.spyOn(api, 'patchUpstream')
  render(<Draft />)
  fireEvent.click(screen.getByRole('button', { name: 'Import OpenAPI / Swagger' }))
  const dialog = within(screen.getByRole('dialog', { name: 'Import OpenAPI / Swagger' }))
  fireEvent.change(dialog.getByLabelText('OpenAPI / Swagger definition'), {
    target: { value: 'definition' },
  })
  fireEvent.click(dialog.getByRole('button', { name: 'Analyze definition' }))
  fireEvent.click(await dialog.findByRole('checkbox', { name: 'GET /new' }))
  fireEvent.click(dialog.getByRole('button', { name: 'Add 1 to draft' }))
  expect(screen.getByRole('button', { name: 'Edit operation new_lookup' })).toBeTruthy()
  expect(save).not.toHaveBeenCalled()
})
test('response schema and projection are applied together', async () => {
  render(<Draft />)
  const editor = edit()
  fireEvent.click(editor.getByRole('button', { name: 'Request & response schemas' }))
  fireEvent.click(editor.getByRole('button', { name: 'Edit response schema' }))
  const schema = within(screen.getByRole('dialog', { name: 'Response schema' }))
  fireEvent.change(schema.getByLabelText('Response example JSON'), {
    target: { value: '{"id":1,"secret":"hidden"}' },
  })
  fireEvent.click(schema.getByRole('button', { name: 'Generate response schema' }))
  await schema.findByRole('button', { name: 'Selected fields only' })
  fireEvent.click(schema.getByRole('button', { name: 'Selected fields only' }))
  fireEvent.click(schema.getByLabelText('Return field secret'))
  fireEvent.click(schema.getByRole('button', { name: 'Apply schema' }))
  await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Response schema' })).toBeNull())
  await apply()
  const op = JSON.parse(screen.getByTestId('rest-draft').textContent!).operations[0]
  expect(op.responseFields).toEqual(['id'])
  expect(op.responseSchema.properties.secret).toEqual({ type: 'string' })
})
test('saved preview is explicit and execution requires confirmation', async () => {
  const request = vi
    .spyOn(api, 'restOperation')
    .mockResolvedValue({ method: 'GET', url: 'https://example.com/orders', headers: {} })
  vi.spyOn(window, 'confirm').mockReturnValue(false)
  render(<Draft saved />)
  expect(request).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: 'Preview request' }))
  await waitFor(() =>
    expect(request).toHaveBeenCalledWith(upstream.slug, 'preview', 'lookup', {}, upstream.version),
  )
  fireEvent.click(screen.getByRole('button', { name: 'Execute test request' }))
  await waitFor(() => expect(window.confirm).toHaveBeenCalled())
  expect(request).toHaveBeenCalledTimes(1)
})
test('saved test response generates a staged schema without the MCP envelope', async () => {
  vi.spyOn(window, 'confirm').mockReturnValue(true)
  vi.spyOn(api, 'restOperation').mockResolvedValue({
    structuredContent: { result: { origin: '127.0.0.1' } },
    content: [],
  })
  render(<Draft saved />)
  fireEvent.click(screen.getByRole('button', { name: 'Execute test request' }))
  fireEvent.click(await screen.findByRole('button', { name: 'Use test response as schema' }))
  expect(
    JSON.parse(within(operation()).getByLabelText('MCP tool definition').textContent!).outputSchema
      .properties.result.properties.origin,
  ).toEqual({ type: 'string' })
  expect(
    JSON.parse(screen.getByTestId('rest-draft').textContent!).operations[0].responseSchema,
  ).toBeUndefined()
  await apply()
})
test('viewers cannot execute administrative REST tests', () => {
  render(
    <RoleContext.Provider value="viewer">
      <RESTUpstreamEditor
        value={config}
        upstream={{ ...upstream, kind: 'rest', rest: { ...config, requireIdentity: true } }}
      />
    </RoleContext.Provider>,
  )
  expect(screen.getByRole('button', { name: 'Execute test request' })).toHaveProperty(
    'disabled',
    true,
  )
  expect(screen.getByRole('button', { name: 'Preview request' })).toHaveProperty('disabled', true)
})
test('combined request schema size is checked before the upstream draft is changed', async () => {
  render(<Draft />)
  const editor = edit()
  fireEvent.click(editor.getByRole('button', { name: 'Advanced settings' }))
  fireEvent.change(editor.getByLabelText('Request parameters (JSON)'), {
    target: {
      value: JSON.stringify(
        ['first', 'second'].map((name) => ({
          name,
          in: 'query',
          required: false,
          schema: { type: 'string', description: 'x'.repeat(33000) },
        })),
      ),
    },
  })
  fireEvent.click(editor.getByRole('button', { name: 'Apply operation to upstream draft' }))
  await editor.findByText(
    'Combined request: Schema exceeds 64 KiB. Remove unused fields or split this operation.',
  )
  expect(
    JSON.parse(screen.getByTestId('rest-draft').textContent!).operations[0].parameters,
  ).toEqual([])
})

test('fixed query parameters are omitted from MCP and optional defaults survive modal Apply', async () => {
  render(<Draft />)
  const editor = edit()
  fireEvent.click(editor.getByRole('button', { name: 'Parameters (0)' }))
  fireEvent.click(editor.getByRole('button', { name: 'Add query parameter' }))
  fireEvent.change(editor.getByLabelText('Parameter 1 name'), { target: { value: 'type' } })
  fireEvent.change(editor.getByLabelText('Parameter 1 type'), { target: { value: 'integer' } })
  fireEvent.click(editor.getByLabelText('Parameter 1 expose to MCP'))
  fireEvent.change(editor.getByLabelText('Parameter 1 fixed value'), { target: { value: '1' } })
  expect(
    JSON.parse(editor.getByLabelText('MCP tool definition').textContent!).inputSchema.properties,
  ).not.toHaveProperty('type')
  fireEvent.click(editor.getByRole('button', { name: 'Add query parameter' }))
  fireEvent.change(editor.getByLabelText('Parameter 2 name'), { target: { value: 'content' } })
  fireEvent.click(editor.getByLabelText('Parameter 2 use default value'))
  fireEvent.change(editor.getByLabelText('Parameter 2 default value'), { target: { value: 'all' } })
  expect(
    JSON.parse(editor.getByLabelText('MCP tool definition').textContent!).inputSchema.properties
      .content,
  ).toEqual({ type: 'string', default: 'all' })
  await apply()
  const parameters = JSON.parse(screen.getByTestId('rest-draft').textContent!).operations[0]
    .parameters
  expect(parameters[0]).toMatchObject({ exposeToMcp: false, defaultValue: 1 })
  expect(parameters[1]).toMatchObject({ defaultValue: 'all' })
})
test('numeric default editing retains decimal keystrokes and rejects invalid text', async () => {
  render(<Draft />)
  const editor = edit()
  fireEvent.click(editor.getByRole('button', { name: 'Parameters (0)' }))
  fireEvent.click(editor.getByRole('button', { name: 'Add query parameter' }))
  fireEvent.change(editor.getByLabelText('Parameter 1 name'), { target: { value: 'amount' } })
  fireEvent.change(editor.getByLabelText('Parameter 1 type'), { target: { value: 'number' } })
  fireEvent.click(editor.getByLabelText('Parameter 1 use default value'))
  const input = editor.getByLabelText('Parameter 1 default value')
  fireEvent.change(input, { target: { value: '1.' } })
  expect(input).toHaveProperty('value', '1.')
  fireEvent.change(input, { target: { value: '1.5' } })
  expect(input).toHaveProperty('value', '1.5')
  fireEvent.change(input, { target: { value: '-' } })
  expect(editor.getByRole('button', { name: 'Apply operation to upstream draft' })).toHaveProperty(
    'disabled',
    true,
  )
  fireEvent.change(input, { target: { value: '-0.5' } })
  await apply()
  expect(
    JSON.parse(screen.getByTestId('rest-draft').textContent!).operations[0].parameters[0]
      .defaultValue,
  ).toBe(-0.5)
})

test('operation files update matches, add new tools and retain local credentials on repeat import', async () => {
  render(<Draft />)
  const operations = [
    { ...config.operations[0], description: 'Revised', headers: { 'X-Key': 'source-secret' } },
    {
      ...config.operations[0],
      name: 'list_orders',
      path: '/orders/all',
      responseFieldPaths: [['id']],
    },
  ]
  const upload = () =>
    fireEvent.change(screen.getByLabelText('Import operations file'), {
      target: {
        files: [
          {
            size: 512,
            text: async () => JSON.stringify({ format: 'vmcp.rest-operations.v1', operations }),
          },
        ],
      },
    })
  upload()
  fireEvent.click(await screen.findByRole('button', { name: 'Apply to upstream draft' }))
  const value = JSON.parse(screen.getByTestId('rest-draft').textContent!)
  expect(value.auth).toEqual(config.auth)
  expect(value.operations[0]).toEqual({ ...config.operations[0], description: 'Revised' })
  expect(value.operations[1].responseFieldPaths).toEqual([['id']])
  expect(value.operations[1].headerNames).toBeUndefined()
  upload()
  expect(await screen.findByRole('button', { name: 'Apply to upstream draft' })).toHaveProperty(
    'disabled',
    true,
  )
})

test('incomplete candidates can be repaired before selection', async () => {
  vi.spyOn(api, 'importREST').mockResolvedValue({
    title: 'Orders',
    version: '3.1',
    servers: [],
    warnings: [],
    operations: [
      {
        id: 'GET /pending',
        path: '/pending',
        method: 'GET',
        summary: 'Pending',
        tags: [],
        warnings: [],
        errors: ['Review response'],
        repairDraft: { ...config.operations[0], enabled: false, name: 'pending', path: '/pending' },
      },
    ],
  })
  render(<Draft />)
  fireEvent.click(screen.getByRole('button', { name: 'Import OpenAPI / Swagger' }))
  fireEvent.change(screen.getByLabelText('OpenAPI / Swagger definition'), {
    target: { value: 'definition' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Analyze definition' }))
  fireEvent.click(await screen.findByRole('button', { name: 'Repair operation' }))
  fireEvent.change(screen.getByLabelText('Tool name'), { target: { value: 'reviewed' } })
  fireEvent.click(screen.getByRole('button', { name: 'Accept reviewed definition' }))
  fireEvent.click(screen.getByRole('checkbox', { name: 'GET /pending' }))
  fireEvent.click(screen.getByRole('button', { name: 'Add 1 to draft' }))
  expect(screen.getByRole('button', { name: 'Edit operation reviewed' })).toBeTruthy()
})

test('validation failure keeps the operation modal and original draft for retry', async () => {
  const check = vi.spyOn(api, 'validateRESTSchemas').mockRejectedValueOnce(new Error('Offline'))
  render(<Draft />)
  const editor = edit()
  fireEvent.change(editor.getByLabelText('Operation path'), { target: { value: '/orders/{id}' } })
  fireEvent.click(editor.getByRole('button', { name: 'Apply operation to upstream draft' }))
  await editor.findByText(/Retry validation.*Offline/)
  expect(JSON.parse(screen.getByTestId('rest-draft').textContent!).operations[0].path).toBe(
    '/orders',
  )
  check.mockResolvedValueOnce({ issues: [[]] })
  await apply()
  expect(JSON.parse(screen.getByTestId('rest-draft').textContent!).operations[0].path).toBe(
    '/orders/{id}',
  )
})

test('optional repository-relative path survives applying and reopening the operation', async () => {
  render(<Draft />)
  const editor = edit()
  fireEvent.change(editor.getByLabelText('Operation path'), { target: { value: '/browse/{path}' } })
  fireEvent.click(editor.getByRole('button', { name: 'Parameters (1)' }))
  fireEvent.change(editor.getByLabelText('Path mode for path'), { target: { value: 'relative' } })
  fireEvent.click(editor.getByLabelText('Parameter 1 required'))
  const schema = JSON.parse(editor.getByLabelText('MCP tool definition').textContent!).inputSchema
  expect(schema.required).toEqual([])
  expect(schema.properties.path.description).toContain('src/main')
  await apply()
  const reopened = edit()
  fireEvent.click(reopened.getByRole('button', { name: 'Parameters (1)' }))
  expect(reopened.getByLabelText('Path mode for path')).toHaveProperty('value', 'relative')
  expect(reopened.getByLabelText('Parameter 1 required')).toHaveProperty('checked', false)
})

test('plain text format survives Apply and advertises one string result', async () => {
  render(<Draft />)
  const editor = edit()
  fireEvent.change(editor.getByLabelText('Response format'), { target: { value: 'text' } })
  const definition = JSON.parse(editor.getByLabelText('MCP tool definition').textContent!)
  expect(definition.outputSchema.properties.result).toEqual({ type: 'string' })
  fireEvent.click(editor.getByRole('button', { name: 'Request & response schemas' }))
  expect(editor.queryByRole('button', { name: 'Edit response schema' })).toBeNull()
  await apply()
  expect(edit().getByLabelText('Response format')).toHaveProperty('value', 'text')
})

test('creates a POST with a JSON body and a text response and reopens its definition', async () => {
  render(<Draft />)
  fireEvent.click(screen.getByRole('button', { name: 'Add operation' }))
  const editor = within(operation())
  fireEvent.change(editor.getByLabelText('HTTP method'), { target: { value: 'POST' } })
  fireEvent.change(editor.getByLabelText('Operation path'), { target: { value: '/report/{id}' } })
  fireEvent.change(editor.getByLabelText('Tool name'), { target: { value: 'report' } })
  fireEvent.change(editor.getByLabelText('Tool description'), { target: { value: 'Run report' } })
  fireEvent.click(editor.getByRole('button', { name: 'Edit path parameter id' }))
  fireEvent.change(editor.getByLabelText('Parameter 1 description'), {
    target: { value: 'Report ID' },
  })
  fireEvent.click(editor.getByRole('button', { name: 'Request & response schemas' }))
  fireEvent.click(editor.getByRole('button', { name: 'Edit request body schema' }))
  const body = within(screen.getByRole('dialog', { name: 'Request body schema' }))
  fireEvent.change(body.getByLabelText('Request body example JSON'), {
    target: { value: '{"count":1}' },
  })
  fireEvent.click(body.getByRole('button', { name: 'Generate request body schema' }))
  fireEvent.click(await body.findByRole('button', { name: 'Apply schema' }))
  await waitFor(() =>
    expect(screen.queryByRole('dialog', { name: 'Request body schema' })).toBeNull(),
  )
  fireEvent.change(editor.getByLabelText('Response format'), { target: { value: 'text' } })
  await apply()
  fireEvent.click(screen.getByRole('button', { name: 'Edit operation report' }))
  const reopened = within(operation())
  const tool = JSON.parse(reopened.getByLabelText('MCP tool definition').textContent!)
  expect(tool.inputSchema.required).toContain('body')
  expect(tool.inputSchema.properties.body.properties).toHaveProperty('count')
  expect(tool.outputSchema.properties.result).toEqual({ type: 'string' })
})
