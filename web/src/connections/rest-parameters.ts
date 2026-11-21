import type { RESTOperation } from '../api'
export type RESTParameter = RESTOperation['parameters'][number]
export const hasParameterDefault = (parameter: RESTParameter) =>
  parameter.defaultValue !== undefined
export const initialParameterDefault = (parameter: RESTParameter): string | number | boolean => {
  const type = parameter.schema.type
  return (
    (parameter.schema.default as string | number | boolean) ??
    (type === 'boolean' ? false : type === 'number' || type === 'integer' ? 0 : '')
  )
}
export function parseParameterDefault(text: string, type: unknown): string | number {
  if (type !== 'integer' && type !== 'number') return text
  // Retain invalid intermediate text so validation can block Apply without
  // silently changing an invalid number into zero or losing integer precision.
  const value = text.trim() ? Number(text) : NaN
  return Number.isFinite(value) && (type !== 'integer' || Number.isSafeInteger(value))
    ? value
    : text
}
export function parameterValueError(parameter: RESTParameter, path?: string): string {
  if (
    parameter.pathMode !== undefined &&
    (parameter.pathMode !== 'relative' ||
      parameter.in !== 'path' ||
      parameter.schema.type !== 'string' ||
      (path !== undefined && !path.endsWith('/{' + parameter.name + '}')))
  )
    return 'Relative path mode requires a final string path parameter.'
  if (parameter.exposeToMcp !== undefined && typeof parameter.exposeToMcp !== 'boolean')
    return 'Expose to MCP must be a boolean.'
  if (parameter.exposeToMcp === false && !hasParameterDefault(parameter))
    return 'A hidden parameter requires a fixed value.'
  if (!hasParameterDefault(parameter)) return ''
  const value = parameter.defaultValue,
    type = parameter.schema.type
  const valid =
    type === 'string'
      ? typeof value === 'string'
      : type === 'boolean'
        ? typeof value === 'boolean'
        : (type === 'integer' || type === 'number') &&
          typeof value === 'number' &&
          Number.isFinite(value) &&
          (type !== 'integer' || Number.isInteger(value))
  if (!valid) return 'The configured value must match the parameter type; null is not supported.'
  if (parameter.pathMode === 'relative') {
    const invalid = String(value)
      .split('/')
      .some(
        (segment) =>
          !segment || segment === '.' || segment === '..' || /[\\%\p{Cc}]/u.test(segment),
      )
    return invalid
      ? 'Use a repository-relative path with valid segments, for example src/main.'
      : ''
  }
  if (
    parameter.in === 'path' &&
    (value === '.' ||
      value === '..' ||
      /[\\/%]/.test(String(value)) ||
      String(value).includes('\0'))
  )
    return 'The configured path value contains unsafe characters.'
  return ''
}
