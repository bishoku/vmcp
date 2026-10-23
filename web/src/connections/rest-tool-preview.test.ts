import { expect, test } from 'vitest'
import type { RESTOperation } from '../api'
import { pathParameters, restToolPreview } from './rest-tool-preview'
import { inferSchema } from './schema-editor'
import orderFixture from './testdata/order-mapping.json'

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

test('nested order example produces the body schema exercised by the virtual MCP HTTP test', () => {
  const bodySchema = inferSchema(orderFixture.body)
  expect(bodySchema).toEqual(orderFixture.bodySchema)
  const tool = restToolPreview({ ...operation, path: '/orders', parameters: [], bodySchema })
  expect(tool.inputSchema).toEqual({
    type: 'object', properties: { body: orderFixture.bodySchema }, required: ['body'], additionalProperties: false,
  })
  // Inferring types does not infer the upstream's business-required fields.
  expect(orderFixture.bodySchema.properties.order.properties.items.items).not.toHaveProperty('required')
})

test('fixed values stay private and explicit defaults make MCP arguments optional',()=>{
 const tool=restToolPreview({...operation,bodySchema:undefined,parameters:[
  {name:'type',in:'query',required:true,schema:{type:'integer'},exposeToMcp:false,defaultValue:1},
  {name:'id',in:'path',required:true,schema:{type:'integer'},defaultValue:0},
  {name:'flag',in:'query',required:true,schema:{type:'boolean'},defaultValue:false},
  {name:'content',in:'query',required:false,schema:{type:'string'},defaultValue:''},
  {name:'mandatory',in:'query',required:true,schema:{type:'string'}},
 ]})
 expect(tool.inputSchema.properties).toEqual({id:{type:'integer',default:0},flag:{type:'boolean',default:false},content:{type:'string',default:''},mandatory:{type:'string'}})
 expect(tool.inputSchema.required).toEqual(['mandatory'])
})
