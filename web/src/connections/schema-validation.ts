import { api } from '../api'

export const schemaBytes = (schema: unknown) =>
  schema === undefined ? 0 : new TextEncoder().encode(JSON.stringify(schema)).length
const references = (value: unknown): boolean =>
  value !== null &&
  typeof value === 'object' &&
  Object.entries(value).some(
    ([key, child]) => ['$ref', '$dynamicRef', '$recursiveRef'].includes(key) || references(child),
  )
export function schemaLimitError(value: unknown): string {
  if (value === undefined) return ''
  if (
    typeof value !== 'boolean' &&
    (value === null || typeof value !== 'object' || Array.isArray(value))
  )
    return 'Use a JSON Schema object or boolean.'
  if (schemaBytes(value) > 65536)
    return 'Schema exceeds 64 KiB. Remove unused fields or split this operation.'
  if (references(value)) return 'Schema references are not supported. Use expanded inline schemas.'
  return ''
}

type Check = { label: string; schema: unknown; sample?: unknown }

export async function checkSchemas(checks: Check[]): Promise<string[]> {
  const applicable = checks.filter((c) => c.schema !== undefined)
  const local = applicable.flatMap((c) => {
    const issue = schemaLimitError(c.schema)
    return issue ? [c.label + issue] : []
  })
  if (local.length || !applicable.length) return local
  try {
    const { issues } = await api.validateRESTSchemas(
      applicable.map(({ schema, sample }) => ({ schema, sample })),
    )
    if (issues.length !== applicable.length) throw Error('Incomplete validation response.')
    return issues.flatMap((messages, i) => messages.map((message) => applicable[i].label + message))
  } catch (error) {
    return [
      'Schema checks could not be completed. Retry validation. ' +
        (error instanceof Error ? error.message : ''),
    ]
  }
}

export function checkSchema(schema: unknown, sample?: { value: unknown }): Promise<string[]> {
  return checkSchemas([{ label: '', schema, sample: sample?.value }])
}
