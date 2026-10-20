import { expect, test } from 'vitest'
import type { RESTOperation } from '../api'
import { pathParameters, restToolPreview } from './rest-tool-preview'

const operation: RESTOperation = {
  name: 'create_order', description: 'Create an order', method: 'POST', path: '/orders/{id}',
  enabled: true, readOnly: false,
  parameters: [{ name: 'id', in: 'path', required: false, schema: { type: 'string', pattern: '^ORD-', minLength: 5 } }],
  bodySchema: { type: 'object', properties: { quantity: { type: 'integer', minimum: 1 } }, required: ['quantity'] },
  responseSchema: { type: 'object', properties: { id: { type: 'string' }, internal: { type: 'string' } }, required: ['id', 'internal'] },
  responseFields: ['id'], headers: { 'X-API-Key': 'secret-value' },
}

test('MCP preview matches the runtime body, projected result wrapper and annotations without headers', () => {
  const tool = restToolPreview(operation)
  expect(tool.inputSchema).toEqual({
    type: 'object', properties: { id: operation.parameters[0].schema, body: operation.bodySchema },
    required: ['id', 'body'], additionalProperties: false,
  })
  expect(tool.outputSchema).toEqual({
    type: 'object', properties: { result: { type: 'object', properties: { id: { type: 'string' } }, required: ['id'], additionalProperties: false } },
    required: ['result'], additionalProperties: false,
  })
  expect(tool.annotations).toEqual({ readOnlyHint: false, destructiveHint: true, idempotentHint: false, openWorldHint: true })
  expect(JSON.stringify(tool)).not.toContain('secret-value')
  expect(JSON.stringify(tool)).not.toContain('X-API-Key')
})

test('path editing retains imported constraints and query parameters, and deduplicates placeholders', () => {
  const query = { name: 'page', in: 'query' as const, required: false, schema: { type: 'integer', minimum: 1 } }
  expect(pathParameters('/orders/{id}/items/{item}/{id}', [...operation.parameters, query])).toEqual([
    operation.parameters[0], { name: 'item', in: 'path', required: true, schema: { type: 'string' } }, query,
  ])
  expect(pathParameters('/orders', [...operation.parameters, query])).toEqual([query])
})
