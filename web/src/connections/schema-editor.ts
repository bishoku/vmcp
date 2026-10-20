export type Schema = boolean | Record<string, unknown>
export const objectSchema = (value: unknown): Record<string, unknown> => value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}

// Examples supply missing information; they never replace imported constraints.
export function mergeExample(existing: unknown, inferred: Schema): Schema {
  if (existing !== undefined) {
    if (typeof existing === 'boolean' || Array.isArray(existing)) return existing as Schema
    const old = objectSchema(existing), next = objectSchema(inferred)
    if (old.type !== next.type || ['$ref', 'allOf', 'anyOf', 'oneOf', 'not', 'if', 'prefixItems'].some(k => k in old)) return old
    if (old.type === 'object') {
      const properties = Object.assign(Object.create(null) as Record<string, unknown>, objectSchema(old.properties))
      for (const [name, value] of Object.entries(objectSchema(next.properties))) {
        // Closed imported objects must not gain unapproved properties.
        if (name in properties || old.additionalProperties !== false) properties[name] = mergeExample(properties[name], value as Schema)
      }
      return { ...old, properties }
    }
    if (old.type === 'array') return { ...old, items: mergeExample(old.items, next.items as Schema) }
    return old
  }
  return inferred
}
export function inferSchema(value: unknown, depth = 0): Schema {
  if (depth > 20) throw new Error('Use an example with at most 20 nesting levels.')
  if (value === null) return {} // Null alone cannot establish the intended type.
  if (Array.isArray(value)) {
    const schemas = value.map(item => inferSchema(item, depth + 1))
    const types = new Set(schemas.map(s => objectSchema(s).type))
    let items: Schema = {}
    if (types.size === 1 && !types.has(undefined)) items = schemas.reduce(combineSamples)
    else if (types.size > 0 && [...types].every(t => t === 'integer' || t === 'number')) items = { type: 'number' }
    return { type: 'array', items }
  }
  if (typeof value === 'object') return { type: 'object', properties: Object.fromEntries(Object.entries(value).map(([k, v]) => [k, inferSchema(v, depth + 1)])) }
  return { type: typeof value === 'number' && Number.isInteger(value) ? 'integer' : typeof value }
}

function combineSamples(left: Schema, right: Schema): Schema {
  const a = objectSchema(left), b = objectSchema(right)
  if ([a.type, b.type].every(t => t === 'integer' || t === 'number')) return { type: a.type === b.type ? a.type : 'number' }
  if (a.type !== b.type) return {}
  if (a.type === 'object') {
    const ap = objectSchema(a.properties), bp = objectSchema(b.properties)
    return { type: 'object', properties: Object.fromEntries([...new Set([...Object.keys(ap), ...Object.keys(bp)])].map(k => [k, Object.hasOwn(ap, k) && Object.hasOwn(bp, k) ? combineSamples(ap[k] as Schema, bp[k] as Schema) : (Object.hasOwn(ap, k) ? ap[k] : bp[k])])) }
  }
  if (a.type === 'array') return { type: 'array', items: combineSamples(a.items as Schema, b.items as Schema) }
  return left
}
