import type { RESTOperation } from '../api'

export const schemaObject = (value: unknown): Record<string, unknown> =>
  value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}

/** Mirrors restapi.Compile's public tool shape. Never includes request credentials. */
export function restToolPreview(operation: RESTOperation) {
  const properties = Object.fromEntries(operation.parameters.map(parameter => [parameter.name, parameter.schema]))
  const required = operation.parameters.filter(parameter => parameter.required || parameter.in === 'path').map(parameter => parameter.name)
  if (operation.bodySchema !== undefined) {
    properties.body = operation.bodySchema as Record<string, unknown>
    required.push('body')
  }
  let result: unknown = operation.responseSchema ?? {}
  if (operation.responseFields?.length) {
    const schema = schemaObject(result)
    const fields = operation.responseFields
    result = {
      type: 'object',
      properties: Object.fromEntries(fields.map(field => [field, schemaObject(schema.properties)[field]])),
      required: (Array.isArray(schema.required) ? schema.required : []).filter(field => fields.includes(field as string)),
      additionalProperties: false,
    }
  }
  return {
    name: operation.name,
    description: operation.description,
    inputSchema: { type: 'object', properties, required, additionalProperties: false },
    outputSchema: { type: 'object', properties: { result }, required: ['result'], additionalProperties: false },
    annotations: { readOnlyHint: operation.readOnly, destructiveHint: !operation.readOnly, idempotentHint: false, openWorldHint: true },
  }
}

/** Retain existing schema constraints; only add/remove parameters owned by the path. */
export function pathParameters(path: string, parameters: RESTOperation['parameters']) {
  const names = [...new Set([...path.matchAll(/\{([A-Za-z0-9_.-]+)\}/g)].map(match => match[1]))]
  return [
    ...names.map(name => parameters.find(parameter => parameter.in === 'path' && parameter.name === name) ?? {
      name, in: 'path' as const, required: true, schema: { type: 'string' },
    }),
    ...parameters.filter(parameter => parameter.in === 'query'),
  ]
}

export function responseSelectionError(schema: unknown, fields: string[] = []): string {
  if (!fields.length) return ''
  const root = schemaObject(schema), properties = schemaObject(root.properties)
  if (root.type !== 'object') return 'Selected response fields require an object response schema. Restore the schema or choose the full response.'
  if (fields.some(name => !Object.hasOwn(properties, name))) return 'Some selected fields are no longer declared in the response schema. Remove them from the selection or restore the fields.'
  if (new Set(fields).size !== fields.length) return 'Response fields must be unique.'
  return ''
}
