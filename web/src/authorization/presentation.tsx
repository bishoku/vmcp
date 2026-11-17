import { useEffect, useState } from 'react'
import { ApiError, api } from '../api'
import type {
  AccessScope,
  AssignmentInput,
  IdentityMapping,
  MappingCondition,
  ScopeOption,
} from './client'
import type { AccessData } from './state'

export function conditionLabel(c: MappingCondition) {
  const operator = {
    equals: 'equals',
    array_contains: 'contains',
    scope_contains: 'includes scope',
  }[c.operator]
  return `${c.path} ${operator} “${c.value}”`
}
export const mappingName = (m: IdentityMapping) =>
  m.name?.trim() || m.conditions.map(conditionLabel).join(' AND ') || 'Unnamed mapping'
export const mappingConditions = (m: IdentityMapping) =>
  m.conditions.map(conditionLabel).join(' AND ')
export function identityName(a: AssignmentInput, data: AccessData, directory: Directory = {}) {
  if (a.mappingId !== undefined) {
    const m = data.mappings.find((v) => v.id === a.mappingId)
    return m ? mappingName(m) : 'Unavailable claim mapping'
  }
  if (a.principal?.kind === 'control_token')
    return (
      directory.control_token?.find((v) => v.id === a.principal?.id)?.name ||
      `Control token · ${a.principal.id || 'not selected'}`
    )
  return `OIDC subject · ${a.principal?.id || 'not selected'}`
}
export type Directory = Record<string, ScopeOption[]>
export function scopeName(scope: AccessScope, directory: Directory = {}) {
  if (scope.kind === 'tenant') return 'Entire deployment'
  if (scope.kind === 'unmanaged') return 'Unmanaged resources'
  const resource = directory[scope.kind]?.find((v) => v.id === scope.id)
  return `${scope.kind.replaceAll('_', ' ')} · ${resource?.name || scope.id || 'not selected'}`
}
export function assignmentWarnings(a: AssignmentInput, data: AccessData) {
  const warnings: string[] = []
  const role = data.roles.find((r) => r.id === a.roleId)
  if (!role) warnings.push('The role is unavailable.')
  else {
    const applicable = role.permissions.filter((id) =>
      data.permissions.find((p) => p.id === id)?.scopes.includes(a.scope.kind),
    )
    if (!role.permissions.length)
      warnings.push('This role has no permissions and grants no access.')
    else if (applicable.length !== role.permissions.length)
      warnings.push(
        `${role.permissions.length - applicable.length} of ${role.permissions.length} permissions do not apply to this scope.`,
      )
    if (
      role.permissions.some((id) => data.permissions.find((p) => p.id === id)?.captureRestricted) &&
      !a.captureScopes.length
    )
      warnings.push('Payload permissions need an explicit capture allowlist.')
  }
  const mapping = data.mappings.find((m) => m.id === a.mappingId)
  if (a.mappingId !== undefined) {
    if (!mapping) warnings.push('The claim mapping is unavailable.')
    else if (!mapping.enabled)
      warnings.push(
        'The claim mapping is disabled; this assignment cannot grant access through it.',
      )
  }
  const issuer = a.mappingId !== undefined ? mapping?.issuer : a.principal?.issuer
  if (
    (a.mappingId !== undefined || a.principal?.kind === 'oidc') &&
    (!data.status.trustedIssuer || issuer !== data.status.trustedIssuer)
  )
    warnings.push('The identity provider does not match the configured OIDC issuer.')
  return warnings
}
// Directory failures never turn a UUID into a misleading name or imply lost access.
export function useAccessDirectory(kinds: string[], onDenied: () => void) {
  const key = [...new Set(kinds.filter((k) => k !== 'tenant' && k !== 'unmanaged'))]
    .sort()
    .join(',')
  const [snapshot, setSnapshot] = useState<{ key: string; directory: Directory; failed: boolean }>({
    key: '',
    directory: {},
    failed: false,
  })
  useEffect(() => {
    if (!key) return
    const controller = new AbortController()
    void Promise.all(
      key.split(',').map(async (kind) => {
        try {
          return [kind, (await api.accessScopes(kind, controller.signal)).items] as const
        } catch (error) {
          if (
            !controller.signal.aborted &&
            error instanceof ApiError &&
            (error.status === 401 || error.status === 403)
          )
            onDenied()
          return [kind, undefined] as const
        }
      }),
    ).then((results) => {
      if (!controller.signal.aborted)
        setSnapshot({
          key,
          directory: Object.fromEntries(
            results.flatMap(([kind, items]) => (items ? [[kind, items]] : [])),
          ),
          failed: results.some(([, items]) => items === undefined),
        })
    })
    return () => controller.abort()
  }, [key, onDenied])
  return {
    directory: snapshot.key === key ? snapshot.directory : {},
    failed: snapshot.key === key && snapshot.failed,
  }
}
