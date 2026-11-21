import { responseProjection } from './response-projection'
import type { RESTOperation } from '../api'

export const schemaObject = (value: unknown): Record<string, unknown> =>
  value !== null && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {}

/** Mirrors restapi.Compile's public tool shape. Never includes request credentials. */
export function restToolPreview(operation: RESTOperation) {
  const exposed = operation.parameters.filter((parameter) => parameter.exposeToMcp !== false)
  const properties = Object.fromEntries(
    exposed.map((parameter) => [
      parameter.name,
      {
        ...parameter.schema,
        ...(parameter.pathMode === 'relative'
          ? {
              description: (
                (parameter.schema.description ?? '') +
                ' Repository-relative path without a leading slash, for example src/main.'
              ).trim(),
            }
          : {}),
        ...(parameter.defaultValue === undefined ? {} : { default: parameter.defaultValue }),
      },
    ]),
  )
  const required = exposed
    .filter(
      (parameter) =>
        parameter.defaultValue === undefined &&
        (parameter.required || (parameter.in === 'path' && !parameter.pathMode)),
    )
    .map((parameter) => parameter.name)
  const body = schemaObject(operation.bodySchema)
  const flat = body.type === 'object'
  const hidden = operation.parameters.filter((p) => p.exposeToMcp === false).map((p) => p.name)
  const bodyArgument = operation.bodyArgumentName || 'body'
  const bodyFields = flat
    ? schemaObject(body.properties)
    : operation.bodySchema === undefined
      ? {}
      : { [bodyArgument]: operation.bodySchema }
  if (flat && Array.isArray(body.required))
    required.push(...body.required.filter((key): key is string => typeof key === 'string'))
  if (!flat && operation.bodySchema !== undefined) required.push(bodyArgument)
  const inputSchema = {
    ...(flat ? body : {}),
    type: 'object',
    properties: { ...properties, ...bodyFields },
    required,
    ...(!flat && { additionalProperties: false }),
    ...(flat && hidden.length ? { propertyNames: { not: { enum: hidden } } } : {}),
  }
  let result: unknown =
    operation.responseFormat === 'text' ? { type: 'string' } : (operation.responseSchema ?? {})
  if (operation.responseFields?.length || operation.responseFieldPaths?.length) {
    result = responseProjection(
      result,
      operation.responseFields,
      operation.responseFieldPaths,
    ).schema
  }

  return {
    name: operation.name,
    description: operation.description,
    inputSchema,
    outputSchema: {
      type: 'object',
      properties: { result },
      required: ['result'],
      additionalProperties: false,
    },
    annotations: {
      readOnlyHint: operation.readOnly,
      destructiveHint: !operation.readOnly,
      idempotentHint: false,
      openWorldHint: true,
    },
  }
}

/** Retain existing schema constraints; only add/remove parameters owned by the path. */
export function pathParameters(path: string, parameters: RESTOperation['parameters']) {
  const names = [...new Set([...path.matchAll(/\{([A-Za-z0-9_.-]+)\}/g)].map((match) => match[1]))]
  return [
    ...names.map(
      (name) =>
        parameters.find((parameter) => parameter.in === 'path' && parameter.name === name) ?? {
          name,
          in: 'path' as const,
          required: true,
          schema: { type: 'string' },
        },
    ),
    ...parameters.filter((parameter) => parameter.in === 'query'),
  ]
}

export function responseSelectionError(
  schema: unknown,
  fields: string[] = [],
  paths: string[][] = [],
): string {
  return responseProjection(schema, fields, paths).error
}
