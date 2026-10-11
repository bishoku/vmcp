// Thin typed client for the vMCP control plane (api/openapi.yaml).
const TOKEN_KEY = 'vmcp.adminToken'

export const getToken = () => localStorage.getItem(TOKEN_KEY) ?? ''
export const setToken = (t: string) => localStorage.setItem(TOKEN_KEY, t)
export const clearToken = () => localStorage.removeItem(TOKEN_KEY)

export interface NodeHealth {
  node: string
  state: 'healthy' | 'unhealthy'
  breaker: 'closed' | 'open' | 'half_open'
  url: string
  latencyMs: number
  error: string | null
  checkedAt: string
}

export interface CertInfo {
  subject: string
  notAfter: string
}

export interface UpstreamTLS {
  caPem: string
  serverName: string
  clientCertPem: string
  hasClientKey: boolean
  caCerts: CertInfo[]
  clientCert: CertInfo | null
}

/** Writable TLS block. clientKeyPem is write-only; omitted keeps the stored key. */
export interface UpstreamTLSInput {
  caPem?: string
  serverName?: string
  clientCertPem?: string
  clientKeyPem?: string
}

export interface UpstreamAuth {
  type: 'none' | 'oauth_client_credentials'
  tokenUrl?: string
  clientId?: string
  scopes?: string[]
  audience?: string
  hasClientSecret?: boolean
}

export interface UpstreamAuthInput {
  type: 'none' | 'oauth_client_credentials'
  tokenUrl?: string
  clientId?: string
  clientSecret?: string
  scopes?: string[]
  audience?: string
}

export interface Upstream {
  version: number
  slug: string
  name: string
  description: string
  url: string
  fallbackUrls: string[]
  health: { state: 'unknown' | 'healthy' | 'degraded' | 'unhealthy'; nodes: NodeHealth[] }
  transport: 'streamable_http' | 'sse'
  enabled: boolean
  headerNames: string[]
  tls: UpstreamTLS
  auth?: UpstreamAuth
  toolCount: number
  lastRefreshAt: string | null
  lastRefreshError: string | null
}

export interface UpstreamInput {
  name: string
  description?: string
  url: string
  fallbackUrls?: string[]
  transport: 'streamable_http' | 'sse'
  enabled?: boolean
  /** Write-only. Omit to keep stored headers; null value deletes one header (PATCH). */
  headers?: Record<string, string | null>
  tls?: UpstreamTLSInput
  auth?: UpstreamAuthInput
}

export interface Tool {
  name: string
  description?: string
  annotations?: { readOnlyHint?: boolean }
}

export type PolicyMode = 'allow' | 'deny' | 'read_only'
export interface ToolPolicy {
  default: PolicyMode
  tools?: Record<string, PolicyMode> | null
}

export interface RefreshResult {
  toolCount: number
  added: string[]
  removed: string[]
  changed: string[]
}

export interface ConnectionTestResult {
  ok: boolean
  error?: string
  toolCount?: number
  [k: string]: unknown
}

export interface ToolSelection {
  upstream: string
  tool: string
  alias?: string
  /** Overrides the upstream description in tools/list of this virtual server; empty = upstream text. */
  description?: string
}

export interface PiiConfig {
  enabled: boolean
  detectors: string[]
  customRules: { name: string; pattern: string }[]
  maskArguments: boolean
}

export interface AgentKey {
  version: number
  slug: string
  description: string
  keyPrefix: string
  virtualServers: string[]
  ipAllowlist: string[]
  rateLimitPerMinute: number | null
  expiresAt: string | null
  revokedAt: string | null
  status: 'active' | 'expired' | 'revoked'
  createdAt: string
}
export interface AgentKeyInput {
  slug?: string
  description?: string
  virtualServers?: string[]
  ipAllowlist?: string[]
  rateLimitPerMinute?: number | null
  expiresAt?: string | null
}

export interface AdminToken {
  version: number
  slug: string
  description: string
  scope: 'admin' | 'read'
  tokenPrefix: string
  expiresAt: string | null
  revokedAt: string | null
  lastUsedAt: string | null
  status: 'active' | 'expired' | 'revoked'
  createdAt: string
}

export interface Condition {
  claim: string
  op: 'equals' | 'contains' | 'exists'
  value?: string
}
export interface IdentityBinding {
  version: number
  slug: string
  description: string
  conditions: Condition[]
  virtualServers: string[]
  ipAllowlist: string[]
  rateLimitPerMinute: number | null
  enabled: boolean
}
export interface BindingEvaluation {
  decision: { allowed: boolean; principal?: string; reason?: string }
  matches?: { slug: string; virtualServers: string[] }[]
}

export interface AuditEvent {
  eventId: string
  operationId?: string | null
  requestId?: string | null
  traceId: string
  schemaVersion: number
  actorType: string
  actorId: string
  issuer: string
  action: string
  target: string
  configRevision: number
  stage: 'event' | 'admission' | 'outcome' | 'mutation'
  id: number
  time: string
  virtualServer: string
  agentKey?: string | null
  clientIp?: string | null
  subject?: string | null
  method: string
  tool?: string | null
  upstream?: string | null
  status: 'started' | 'ok' | 'tool_error' | 'error' | 'denied'
  reason?: string | null
  durationMs: number
  redactions?: number
}
export interface AuditQuery {
  incomplete?: string
  operationId?: string
  requestId?: string
  actorId?: string
  stage?: string
  virtualServer?: string
  agentKey?: string
  subject?: string
  tool?: string
  status?: string
  from?: string
  to?: string
  cursor?: string
}

export interface VirtualServer {
  version: number
  slug: string
  name: string
  description: string
  enabled: boolean
  authRequired: boolean
  pii?: PiiConfig
  policy?: ToolPolicy
  rateLimitPerMinute?: number | null
  endpoint: string
  tools: (ToolSelection & { exposedName: string; upstreamDescription: string; descriptionDrift: boolean; orphaned: boolean })[]
}

export interface ValidationProblem {
  field?: string
  code?: string
  message: string
}

export interface Me {
  name: string
  role: 'viewer' | 'editor' | 'admin'
  via: 'bootstrap-token' | 'admin-token' | 'oidc'
}

export async function authConfig(): Promise<{ oidc: boolean }> {
  try {
    return await (await fetch('/auth/config')).json()
  } catch {
    return { oidc: false }
  }
}

/** Returns the identity provider's logout URL when the session came from SSO. */
export async function signOut(): Promise<string | null> {
  clearToken()
  const res = await fetch('/auth/logout', { method: 'POST' }).catch(() => undefined)
  if (res?.ok && res.status === 200) {
    const body = (await res.json().catch(() => null)) as { endSessionUrl?: string } | null
    return body?.endSessionUrl ?? null
  }
  return null
}

export class ApiError extends Error {
  status: number
  code?: string
  errors: ValidationProblem[]
  constructor(status: number, message: string, code?: string, errors: ValidationProblem[] = []) {
    super(message)
    this.status = status
    this.code = code
    this.errors = errors
  }
}

async function request<T>(method: string, path: string, body?: unknown, version?: number): Promise<T> {
  const res = await fetch(`/api/v1${path}`, {
    method,
    headers: {
      // With SSO there is no token: the session cookie authenticates and the CSRF header is required.
      ...(getToken() && { Authorization: `Bearer ${getToken()}` }),
      'X-VMCP-CSRF': '1',
      ...(version !== undefined && { 'If-Match': `"${version}"` }),
      ...(body !== undefined && {
        'Content-Type': method === 'PATCH' ? 'application/merge-patch+json' : 'application/json',
      }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (res.status === 204) return undefined as T
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new ApiError(res.status, res.status === 412 ? "This item changed in another session. Reload before saving again." : data.detail ?? data.title ?? res.statusText, data.code, data.errors)
  return data as T
}

async function listAll<T>(path: string): Promise<T[]> {
 const all: T[] = []
 const seen = new Set<string>()
 let cursor = ''
 do {
  const params = new URLSearchParams({limit:'200'})
  if (cursor) params.set('cursor',cursor)
  const page = await request<{items:T[];nextCursor?:string|null}>('GET',`${path}?${params}`)
  all.push(...page.items)
  cursor=page.nextCursor ?? ''
  if (cursor && seen.has(cursor)) throw new Error('The server repeated a pagination cursor.')
  seen.add(cursor)
 } while(cursor)
 return all
}

export interface ConfigDocument { apiVersion: string; upstreams: unknown[]; virtualServers: unknown[]; bindings?: unknown[] }
export interface ApplyResult { changes: {kind:string;slug:string;action:string}[]; configRevision?: number | null }
export const api = {
  exportConfig: () => request<ConfigDocument>('GET','/export'),
  applyConfig: (document: ConfigDocument, dryRun = true, prune = false, revision?:number) => request<ApplyResult>('POST',`/apply?dryRun=${dryRun}&prune=${prune}`, document, revision),
  testStoredConnection: (slug:string) => request<ConnectionTestResult>('POST',`/upstreams/${encodeURIComponent(slug)}/connection-test`),
  me: () => request<Me>('GET', '/me'),
  info: () => request<{ version: string; configRevision: number }>('GET', '/info'),
  listUpstreams: async () => listAll<Upstream>('/upstreams'),
  createUpstream: (slug: string, input: UpstreamInput) =>
    request<Upstream>('POST', '/upstreams', { slug, ...input }),
  patchUpstream: (slug: string, input: Partial<UpstreamInput>, version?: number) =>
    request<Upstream>('PATCH', `/upstreams/${encodeURIComponent(slug)}`, input, version),
  deleteUpstream: (slug: string, force = false, version?: number) =>
    request<void>('DELETE', `/upstreams/${encodeURIComponent(slug)}${force ? '?force=true' : ''}`, undefined, version),
  listTools: async (slug: string) =>
    listAll<Tool>(`/upstreams/${encodeURIComponent(slug)}/tools`),
  refreshTools: (slug: string) =>
    request<RefreshResult>('POST', `/upstreams/${encodeURIComponent(slug)}/refresh-tools`),
  testConnection: (input: { url: string; transport: string; headers?: Record<string, string>; tls?: UpstreamTLSInput; auth?: UpstreamAuthInput }) =>
    request<ConnectionTestResult>('POST', '/connection-tests', input),
  listVirtualServers: async () =>
    listAll<VirtualServer>('/virtual-servers'),
  putVirtualServer: (
    slug: string,
    v: { name: string; description?: string; enabled: boolean; authRequired: boolean; pii?: PiiConfig; policy?: ToolPolicy; rateLimitPerMinute?: number | null; tools: ToolSelection[] },
    version?: number,
  ) => request<VirtualServer>('PUT', `/virtual-servers/${encodeURIComponent(slug)}`, v, version),
  deleteVirtualServer: (slug: string, version?: number) =>
    request<void>('DELETE', `/virtual-servers/${encodeURIComponent(slug)}`, undefined, version),

  listAgentKeys: async () => listAll<AgentKey>('/agent-keys'),
  createAgentKey: (i: AgentKeyInput) => request<AgentKey & { key: string }>('POST', '/agent-keys', i),
  patchAgentKey: (slug: string, i: AgentKeyInput, version?: number) =>
    request<AgentKey>('PATCH', `/agent-keys/${encodeURIComponent(slug)}`, i, version),
  rotateAgentKey: (slug: string, version?: number) =>
    request<AgentKey & { key: string }>('POST', `/agent-keys/${encodeURIComponent(slug)}/rotate`, undefined, version),
  revokeAgentKey: (slug: string, version?: number) => request<AgentKey>('POST', `/agent-keys/${encodeURIComponent(slug)}/revoke`, undefined, version),
  deleteAgentKey: (slug: string, version?: number) => request<void>('DELETE', `/agent-keys/${encodeURIComponent(slug)}`, undefined, version),
  listBindings: async () => listAll<IdentityBinding>('/identity-bindings'),
  createBinding: (b: Partial<IdentityBinding>) => request<IdentityBinding>('POST', '/identity-bindings', b),
  patchBinding: (slug: string, b: Partial<IdentityBinding>, version?: number) =>
    request<IdentityBinding>('PATCH', `/identity-bindings/${encodeURIComponent(slug)}`, b, version),
  deleteBinding: (slug: string, version?: number) => request<void>('DELETE', `/identity-bindings/${encodeURIComponent(slug)}`, undefined, version),
  evaluateBindings: (body: { claims: unknown; virtualServer?: string; clientIp?: string }) =>
    request<BindingEvaluation>('POST', '/identity-bindings/evaluate', body),
  listAdminTokens: async () => listAll<AdminToken>('/admin-tokens'),
  createAdminToken: (i: { slug: string; description?: string; scope: 'admin' | 'read'; expiresAt?: string | null }) =>
    request<AdminToken & { token: string }>('POST', '/admin-tokens', i),
  rotateAdminToken: (slug: string, version?: number) =>
    request<AdminToken & { token: string }>('POST', `/admin-tokens/${encodeURIComponent(slug)}/rotate`, undefined, version),
  revokeAdminToken: (slug: string, version?: number) =>
    request<AdminToken>('POST', `/admin-tokens/${encodeURIComponent(slug)}/revoke`, undefined, version),
  deleteAdminToken: (slug: string, version?: number) => request<void>('DELETE', `/admin-tokens/${encodeURIComponent(slug)}`, undefined, version),
  listAudit: (q: AuditQuery) => {
    const p = new URLSearchParams({ limit: '50' })
    Object.entries(q).forEach(([k, v]) => v && p.set(k, v))
    return request<{ items: AuditEvent[]; nextCursor?: string | null }>('GET', `/audit-events?${p}`)
  },
}
