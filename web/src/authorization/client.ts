export type ScopeKind = 'tenant' | 'unmanaged' | 'team' | 'project' | 'upstream' | 'virtual_server'
export interface AccessScope {
  kind: ScopeKind
  id?: string
  epoch?: string
  ownerTeam?: string
}
export interface AccessPrincipal {
  kind: 'oidc' | 'control_token'
  id: string
  issuer?: string
}
export interface Permission {
  id: string
  scopes: ScopeKind[]
  captureRestricted: boolean
}
export interface AccessRole {
  id: string
  slug: string
  name: string
  description: string
  builtin: boolean
  version: number
  permissions: string[]
}
export interface RoleInput {
  slug?: string
  name: string
  description: string
  permissions?: string[]
  copyFrom?: string
}
export interface MappingCondition {
  path: string
  operator: 'equals' | 'array_contains' | 'scope_contains'
  value: string
}
export interface IdentityMapping {
  id: string
  issuer: string
  principalKind: 'oidc'
  conditions: MappingCondition[]
  enabled: boolean
  version: number
}
export type MappingInput = Omit<IdentityMapping, 'id' | 'version'>
export interface RoleAssignment {
  id: string
  roleId: string
  principal: AccessPrincipal
  mappingId?: string
  scope: AccessScope
  captureScopes: string[]
  enabled: boolean
  version: number
}
export interface AssignmentInput {
  roleId: string
  principal?: AccessPrincipal
  mappingId?: string
  scope: AccessScope
  captureScopes: string[]
  enabled: boolean
}
export interface ScopeOption {
  id: string
  slug: string
  name: string
  ownerTeam?: string
  project?: string
  epoch?: string
}
export interface ResourceEpoch {
  id: string
  resourceId: string
  kind: 'upstream' | 'virtual_server'
  ownerTeam?: string
  project?: string
  startRevision: number
  endRevision: number | null
}
export interface AccessStatus {
  authorizationRevision: number
  schemaVersion: number
  trustedIssuer: string
  claimMappings: string
  externalOAuthBearer: boolean
  blockers: string[]
  counts: Record<string, number>
}
export interface EvaluationInput {
  principal: AccessPrincipal
  claims?: Record<string, unknown>
  permission: string
  resource?: { kind: Exclude<ScopeKind, 'unmanaged'>; id?: string }
  auditEventId?: string
}
export interface Evaluation {
  simulationOnly: boolean
  authenticationValidated: boolean
  authorizationRevision: number
  claimMappings: string
  matchedMappingIds: string[]
  candidateAssignmentIds: string[]
  decision: { allowed: boolean; reason: string; assignmentId?: string }
}
interface Page<T> {
  items: T[]
  nextCursor?: string | null
  authorizationRevision?: number
}
type Request = <T>(
  method: string,
  path: string,
  body?: unknown,
  version?: number,
  signal?: AbortSignal,
) => Promise<T>
export function authorizationClient(request: Request) {
  async function all<T>(path: string, signal?: AbortSignal): Promise<Page<T>> {
    const items: T[] = [],
      seen = new Set<string>()
    let cursor = '',
      revision: number | undefined
    do {
      const q = new URLSearchParams({ limit: '200' })
      if (cursor) q.set('cursor', cursor)
      const page = await request<Page<T>>(
        'GET',
        `${path}${path.includes('?') ? '&' : '?'}${q}`,
        undefined,
        undefined,
        signal,
      )
      if (signal?.aborted) throw new DOMException('Request aborted', 'AbortError')
      if (revision !== undefined && page.authorizationRevision !== revision)
        throw new Error('The access policy changed while loading. Refresh before editing.')
      revision = page.authorizationRevision
      items.push(...page.items)
      cursor = page.nextCursor ?? ''
      if (cursor && seen.has(cursor)) throw new Error('The server repeated a pagination cursor.')
      seen.add(cursor)
    } while (cursor)
    return { items, authorizationRevision: revision }
  }
  return {
    accessPermissions: (signal?: AbortSignal) =>
      request<Page<Permission>>('GET', '/permissions', undefined, undefined, signal),
    accessRoles: (signal?: AbortSignal) =>
      request<Page<AccessRole>>('GET', '/roles', undefined, undefined, signal),
    accessRole: (slug: string, signal?: AbortSignal) =>
      request<AccessRole>(
        'GET',
        `/roles/${encodeURIComponent(slug)}`,
        undefined,
        undefined,
        signal,
      ),
    createAccessRole: (input: RoleInput, signal?: AbortSignal) =>
      request<AccessRole>('POST', '/roles', input, undefined, signal),
    updateAccessRole: (role: AccessRole, input: RoleInput, signal?: AbortSignal) =>
      request<AccessRole>(
        'PUT',
        `/roles/${encodeURIComponent(role.slug)}`,
        input,
        role.version,
        signal,
      ),
    deleteAccessRole: (role: AccessRole, signal?: AbortSignal) =>
      request<void>(
        'DELETE',
        `/roles/${encodeURIComponent(role.slug)}`,
        undefined,
        role.version,
        signal,
      ),
    accessMappings: (signal?: AbortSignal) => all<IdentityMapping>('/identity-mappings', signal),
    accessMapping: (id: string, signal?: AbortSignal) =>
      request<IdentityMapping>(
        'GET',
        `/identity-mappings/${encodeURIComponent(id)}`,
        undefined,
        undefined,
        signal,
      ),
    createAccessMapping: (input: MappingInput, signal?: AbortSignal) =>
      request<IdentityMapping>('POST', '/identity-mappings', input, undefined, signal),
    updateAccessMapping: (item: IdentityMapping, input: MappingInput, signal?: AbortSignal) =>
      request<IdentityMapping>(
        'PUT',
        `/identity-mappings/${encodeURIComponent(item.id)}`,
        input,
        item.version,
        signal,
      ),
    deleteAccessMapping: (item: IdentityMapping, signal?: AbortSignal) =>
      request<void>(
        'DELETE',
        `/identity-mappings/${encodeURIComponent(item.id)}`,
        undefined,
        item.version,
        signal,
      ),
    accessAssignments: (signal?: AbortSignal) => all<RoleAssignment>('/role-assignments', signal),
    accessAssignment: (id: string, signal?: AbortSignal) =>
      request<RoleAssignment>(
        'GET',
        `/role-assignments/${encodeURIComponent(id)}`,
        undefined,
        undefined,
        signal,
      ),
    createAccessAssignment: (input: AssignmentInput, signal?: AbortSignal) =>
      request<RoleAssignment>('POST', '/role-assignments', input, undefined, signal),
    updateAccessAssignment: (item: RoleAssignment, input: AssignmentInput, signal?: AbortSignal) =>
      request<RoleAssignment>(
        'PUT',
        `/role-assignments/${encodeURIComponent(item.id)}`,
        input,
        item.version,
        signal,
      ),
    deleteAccessAssignment: (item: RoleAssignment, signal?: AbortSignal) =>
      request<void>(
        'DELETE',
        `/role-assignments/${encodeURIComponent(item.id)}`,
        undefined,
        item.version,
        signal,
      ),
    accessScopes: (kind: string, signal?: AbortSignal) =>
      all<ScopeOption>(`/access/scopes?kind=${encodeURIComponent(kind)}`, signal),
    accessEpochs: (kind: string, id: string, signal?: AbortSignal) =>
      all<ResourceEpoch>(
        `/access/resource-epochs?kind=${encodeURIComponent(kind)}&resourceId=${encodeURIComponent(id)}`,
        signal,
      ),
    evaluateAccess: (input: EvaluationInput, signal?: AbortSignal) =>
      request<Evaluation>('POST', '/access/evaluate', input, undefined, signal),
    accessAccessStatus: (signal?: AbortSignal) =>
      request<AccessStatus>('GET', '/access/status', undefined, undefined, signal),
  }
}
