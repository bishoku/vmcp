import type { ErrorObject } from 'ajv'

export const schemaBytes = (schema: unknown) => schema === undefined ? 0 : new TextEncoder().encode(JSON.stringify(schema)).length
const references = (value: unknown): boolean => value !== null && typeof value === 'object' && Object.entries(value).some(([key, child]) => ['$ref', '$dynamicRef', '$recursiveRef'].includes(key) || references(child))
export function schemaLimitError(value: unknown): string {
 if (value === undefined) return ''
 if (typeof value !== 'boolean' && (value === null || typeof value !== 'object' || Array.isArray(value))) return 'Use a JSON Schema object or boolean.'
 if (schemaBytes(value) > 65536) return 'Schema exceeds 64 KiB. Remove unused fields or split this operation.'
 if (references(value)) return 'Schema references are not supported. Use expanded inline schemas.'
 return ''
}
const describe = (errors: ErrorObject[] | null | undefined) => (errors ?? []).slice(0, 8).map(error => `${error.instancePath || '/'}: ${error.message}${error.keyword === 'required' ? ` (${error.params.missingProperty})` : ''}`)
/** Loaded on explicit validation only. Server validation remains authoritative. */
export async function checkSchema(value: unknown, sample?: { value: unknown }): Promise<string[]> {
 const limit = schemaLimitError(value)
 if (limit) return [limit]
 if (value === undefined) return []
 try {
  const { default: Ajv } = await import('ajv/dist/2020.js')
  const ajv = new Ajv({ strict: false, allErrors: true, validateFormats: false })
  if (!ajv.validateSchema(value as object | boolean)) return describe(ajv.errors)
  const validate = ajv.compile(value as object | boolean)
  if (sample && !validate(sample.value)) return describe(validate.errors)
  return []
 } catch (error) { return [error instanceof Error ? error.message : 'Unable to validate this schema.'] }
}
