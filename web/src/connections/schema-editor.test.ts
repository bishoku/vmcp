import { expect, test } from 'vitest'
import { inferSchema, mergeExample } from './schema-editor'

test('infers nested objects and arrays without assuming required fields', () => {
  expect(inferSchema({ user: { name: 'Ada' }, rows: [{ id: 1 }, { id: 2, active: true }] })).toEqual({ type: 'object', properties: { user: { type: 'object', properties: { name: { type: 'string' } } }, rows: { type: 'array', items: { type: 'object', properties: { id: { type: 'integer' }, active: { type: 'boolean' } } } } } })
})
test('ambiguous examples remain unconstrained and mixed numbers widen', () => {
  expect(inferSchema([null])).toEqual({ type: 'array', items: {} })
  expect(inferSchema([])).toEqual({ type: 'array', items: {} })
  expect(inferSchema([{ id: 1 }, { id: 'x' }])).toEqual({ type: 'array', items: { type: 'object', properties: { id: {} } } })
  expect(inferSchema([1, 1.5])).toEqual({ type: 'array', items: { type: 'number' } })
})
test('example generation preserves constraints recursively and respects closed schemas', () => {
  const old = { type: 'object', additionalProperties: false, required: ['status'], properties: { status: { type: 'string', enum: ['ok'], minLength: 2 }, rows: { type: 'array', minItems: 1, items: { type: 'number', minimum: 0 } } } }
  expect(mergeExample(old, inferSchema({ status: 'bad', rows: [2], extra: true }))).toEqual(old)
  expect(mergeExample({ oneOf: [{ type: 'string' }, { type: 'integer' }] }, inferSchema('x'))).toEqual({ oneOf: [{ type: 'string' }, { type: 'integer' }] })
  expect(mergeExample(false, inferSchema({}))).toBe(false)
})
test('property names cannot mutate prototypes', () => {
  const example = JSON.parse('{"__proto__":{"polluted":true}}')
  expect(Object.keys((mergeExample({ type: 'object' }, inferSchema(example)) as Record<string, unknown>).properties as object)).toEqual(['__proto__'])
  expect(({} as Record<string, unknown>).polluted).toBeUndefined()
})
