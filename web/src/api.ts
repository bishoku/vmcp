// Thin typed client for the vMCP control plane (api/openapi.yaml).
// Token login is deliberately memory-only. SSO uses the server's session cookie.
let token = ''
try { localStorage.removeItem('vmcp.adminToken') } catch { /* Storage may be disabled; token login still works. */ }
export const getToken = () => token
export const setToken = (value: string) => { token = value }
export const clearToken = () => { token = '' }

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

export interface RESTImportOperation {
 id:string; method:string; path:string; summary:string; tags:string[]
 operation?:RESTOperation; warnings:string[]; errors:string[]
}
export interface RESTImportResult { title:string; version:string; servers:string[]; warnings:string[]; operations:RESTImportOperation[] }
export interface RESTAuth {
 type: 'none' | 'basic' | 'bearer' | 'oauth2'
 username?: string; password?: string; token?: string; tokenUrl?: string; clientId?: string; clientSecret?: string
 scopes?: string[]; audience?: string; tokenAuthMethod?: 'client_secret_basic' | 'client_secret_post'
 hasPassword?: boolean; hasToken?: boolean; hasClientSecret?: boolean
}
export interface MockRule {name:string;priority:number;request?:Record<string,unknown>;response:{status:number;headers?:Record<string,string>;jsonBody?:unknown;body?:string;fixedDelayMilliseconds?:number;transformers?:string[]}}
export interface MockStatus {state:'active'|'pending'|'disabled'|'unavailable';message?:string;ruleCount?:number;namespace?:string}
export interface RESTOperation {
 mockRules?:MockRule[]
 name: string; description: string; enabled: boolean; method: 'GET'|'POST'|'PUT'|'PATCH'|'DELETE'; path: string
 parameters: {name:string; in:'path'|'query'; required:boolean; exposeToMcp?:boolean; defaultValue?:string|number|boolean; schema:Record<string,unknown>}[]
 bodySchema?: unknown; responseSchema?: unknown; responseFields?: string[]; responseFieldPaths?: string[][]; successStatuses?: number[]
 headers?: Record<string,string>; headerNames?:string[]; readOnly:boolean
}
export interface RESTConfig {
 mock?:{namespace?:string}
 auth: RESTAuth; forwardIdentity:boolean; requireIdentity:boolean; allowInsecureHttp:boolean; timeoutSeconds:number; operations:RESTOperation[]
}

export interface RequestHeaderMapping { source:string; target:string; required?:boolean }

export interface CallerContextConfig {
 enabled:boolean; metaKey:string; fields:{name:string;header?:string;bearer?:boolean;required?:boolean}[]
}

export interface Upstream {
 ownerTeam?: string
  requestHeaderMappings?: RequestHeaderMapping[]
  callerContext?: CallerContextConfig
  kind?: "mcp" | "rest"
  rest?: RESTConfig
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
 ownerTeam?: string
  requestHeaderMappings?: RequestHeaderMapping[]
  callerContext?: CallerContextConfig
  kind?: "mcp" | "rest"
  rest?: RESTConfig
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
  orphanedSelections?: { virtualServer: string; tool: string }[]
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
  decision?: { allowed: boolean; principal?: string; reason?: string }
  matches?: { slug: string; virtualServers: string[] }[]
}

export interface AuditRule { name:string; pattern:string }
export interface AuditSettings {
 omni?:{candidates?:boolean;includeDenied?:boolean;maxCandidates?:number}
 mode:''|'strict'|'relax'; retentionDays:number; maxPayloadBytes:number; revision:number
 scopes:Record<string,{request:boolean;response:boolean}>; rules:AuditRule[]
 virtualServers:Record<string,AuditRule[]>; upstreams:Record<string,AuditRule[]>;exportPayloads:Record<string,boolean>
}
export interface AuditPreview {state:string;content?:unknown;originalBytes:number;storedBytes:number;redactions:number;revision:number;matches:{source:string;name:string;count:number}[]}
export interface AuditEvent {
 auditMode?:string;policyRevision?:number;scope?:string;boundary?:string;direction?:string;parentOperationId?:string|null
 payloadState?:string;payloadOriginalBytes?:number;payloadStoredBytes?:number;payloadRedactions?:number

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
  activity?: string
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

export interface ClientFeatures {sampling:boolean;roots:boolean;elicitation:boolean}
export interface CatalogSelection {upstream:string;kind:'resource'|'template'|'prompt';name:string;alias?:string;description?:string}
export interface Catalog {instructions?:string;resources:Record<string,unknown>[];resourceTemplates:Record<string,unknown>[];prompts:Record<string,unknown>[];capabilities:Record<string,unknown>}
export interface GuidanceResource {name:string;title:string;description:string;enabled:boolean;mimeType:'text/plain'|'text/markdown';text:string}
export interface GuidanceArgument {name:string;description:string;required:boolean}
export interface GuidanceMessage {role:'user'|'assistant';text:string}
export interface GuidancePrompt {name:string;title:string;description:string;enabled:boolean;arguments:GuidanceArgument[];messages:GuidanceMessage[]}
export interface Guidance {instructions:string;resources:GuidanceResource[];prompts:GuidancePrompt[];upstreamInstructions:string[];toolReferences:{key:string;upstream:string;tool:string}[]}
export interface VirtualServer {
 project?: string
 ownerTeam?: string
 guidance?:Guidance
 hasGuidance?:boolean
  version: number
  slug: string
  name: string
  description: string
  enabled: boolean
  omniEnabled?: boolean
  authRequired: boolean
  // The server omits empty lists and optional masking settings in read responses.
  pii?: Pick<PiiConfig, 'enabled'> & Partial<Omit<PiiConfig, 'enabled'>>
  policy?: ToolPolicy
  rateLimitPerMinute?: number | null
  clientFeatures?:ClientFeatures
  endpoint: string
 catalog?: (CatalogSelection & {exposedName:string;orphaned:boolean;descriptionDrift:boolean;upstreamDescription:string})[]
  tools: (ToolSelection & { exposedName: string; approvalStatus?:string; upstreamDescription: string; descriptionDrift: boolean; orphaned: boolean })[]
}

export interface ValidationProblem {
  field?: string
  code?: string
  message: string
}

export interface Me {
  name: string
  role: 'viewer' | 'editor' | 'admin' | 'team'
 teamMember?: boolean
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

async function request<T>(method: string, path: string, body?: unknown, version?: number, signal?: AbortSignal, responseType: 'json' | 'blob' = 'json', extraHeaders: Record<string,string> = {}): Promise<T> {
  const res = await fetch(`/api/v1${path}`, {
    method,
    signal,
    headers: {
      // With SSO there is no token: the session cookie authenticates and the CSRF header is required.
      ...(getToken() && { Authorization: `Bearer ${getToken()}` }),
      'X-VMCP-CSRF': '1',
      ...extraHeaders,
      ...(version !== undefined && { 'If-Match': `"${version}"` }),
      ...(body !== undefined && {
        'Content-Type': method === 'PATCH' ? 'application/merge-patch+json' : 'application/json',
      }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (res.status === 401) {
    clearToken()
    window.dispatchEvent(new Event('vmcp:session-expired'))
  }
  if (res.status === 204) return undefined as T
  if (res.ok && responseType === 'blob') return await res.blob() as T
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new ApiError(res.status, res.status === 401 ? "Your session expired. Sign in again to continue." : res.status === 412 ? "This item changed in another session. Reload before saving again." : data.detail ?? data.title ?? res.statusText, data.code, data.errors)
  return data as T
}

async function listAll<T>(path: string, signal?: AbortSignal): Promise<T[]> {
 const all: T[] = []
 const seen = new Set<string>()
 let cursor = ''
 do {
  const params = new URLSearchParams({limit:'200'})
  if (cursor) params.set('cursor',cursor)
  const page = await request<{items:T[];nextCursor?:string|null}>('GET',`${path}?${params}`, undefined, undefined, signal)
  all.push(...page.items)
  cursor=page.nextCursor ?? ''
  if (cursor && seen.has(cursor)) throw new Error('The server repeated a pagination cursor.')
  seen.add(cursor)
 } while(cursor)
 return all
}

export interface ConfigDocument { teams?:Team[];projects?:Project[];toolGrants?:ToolGrant[]; apiVersion: string; upstreams: unknown[]; virtualServers: unknown[]; bindings?: unknown[];auditSettings?:AuditSettings }
export interface ApplyResult { changes: {kind:string;slug:string;action:string}[]; configRevision?: number | null }
export interface Team {canManage?:boolean;slug:string;name:string;issuer:string;group:string;version:number}
export interface Project {slug:string;name:string;ownerTeam:string;version:number}
export interface BulkToolGrantResult {items:{virtualServer:string;upstream:string;tool:string;result:'changed'|'unchanged'|'failed';code?:string;message?:string}[];changed:number;unchanged:number;failed:number}
export interface ToolGrant {consumerProject?:string;consumerTeam?:string;upstreamOwnerTeam?:string;canDecide?:boolean;virtualServer:string;upstream:string;tool:string;status:'pending'|'approved'|'rejected'|'revoked';reason:string;actor?:string;version:number}
export interface ToolCatalogEntry {upstream:string;ownerTeam:string;name:string;description:string}
export interface OmniStatus { configured:boolean; endpoint:string; index?:{rerankEnabled?:boolean;rerankNoMatchScore?:number;rerankConfidenceScore?:number;rerankCandidates?:number;rerankMinScore?:number;rerankFailurePolicy?:string;embeddedDocuments?:number;dimensions?:number;embeddingProfile?:string;ready:boolean;mode:string;documents:number;revision:number;updatedAt:string;error?:string;refreshing:boolean} }
export interface OmniHit {tool_ref:string;server_id:string;server_name:string;name:string;description:string;project:string;score:number}
export const api = {
  omniStatus: (signal?:AbortSignal) => request<OmniStatus>('GET','/omni',undefined,undefined,signal),
  omniReindex: () => request<{accepted:boolean}>('POST','/omni/reindex'),
  omniPreview: (token:string, query:string, ref?:string) => request<{tools?:OmniHit[];search_mode?:string;tool?:unknown;schema_version?:string}>('POST','/omni/preview',{query,tool_ref:ref},undefined,undefined,'json',{'X-VMCP-Runtime-Authorization':`Bearer ${token}`}),
 teams:(signal?:AbortSignal)=>request<{items:Team[]}>('GET','/teams',undefined,undefined,signal),
 saveTeam:(team:Team)=>request('POST','/teams',team),
 projects:(signal?:AbortSignal)=>request<{items:Project[]}>('GET','/projects',undefined,undefined,signal),
 saveProject:(project:Project)=>request('POST','/projects',project),
 toolGrants:(signal?:AbortSignal)=>request<{items:ToolGrant[]}>('GET','/tool-grants',undefined,undefined,signal),
 bulkToolGrants:(items:ToolGrant[])=>request<BulkToolGrantResult>('POST','/tool-grants/bulk',{items}),
 saveToolGrant:(grant:ToolGrant)=>request('POST','/tool-grants',grant),
 toolCatalog:(signal?:AbortSignal)=>listAll<ToolCatalogEntry>('/tool-catalog',signal),
 auditSettings:(signal?:AbortSignal)=>request<AuditSettings>('GET','/audit-settings',undefined,undefined,signal),
 saveAuditSettings:(settings:AuditSettings)=>request<AuditSettings>('PUT','/audit-settings',settings),
 previewAudit:(body:{editScope?:'global'|'virtual'|'upstream';settings:AuditSettings;scope:string;direction:string;virtualServer:string;upstream:string;payload:unknown})=>request<AuditPreview>('POST','/audit-preview',body),
 auditPayload:(id:string)=>request<{state:string;content?:unknown}>('GET',`/audit-events/${encodeURIComponent(id)}/payload`),

  downloadOpenAPI: (format: 'json' | 'yaml') => request<Blob>('GET', `/openapi.${format}`, undefined, undefined, undefined, 'blob'),
  exportConfig: () => request<ConfigDocument>('GET','/export'),
  applyConfig: (document: ConfigDocument, dryRun = true, prune = false, revision?:number) => request<ApplyResult>('POST',`/apply?dryRun=${dryRun}&prune=${prune}`, document, revision),
  testStoredConnection: (slug:string) => request<ConnectionTestResult>('POST',`/upstreams/${encodeURIComponent(slug)}/connection-test`),
  importREST: (document:string) => request<RESTImportResult>('POST','/rest-imports',{document}),
  restOperation: (slug:string, action:'preview'|'test', operation:string, args:unknown, version:number) => request<unknown>('POST', `/upstreams/${encodeURIComponent(slug)}/rest/${action}`, {operation,arguments:args},version),
  me: () => request<Me>('GET', '/me'),
  mockProvider:(signal?:AbortSignal)=>request<{enabled:boolean;url:string}>('GET','/mock-provider',undefined,undefined,signal),
  mockStatus:(slug:string,signal?:AbortSignal)=>request<MockStatus>('GET',`/upstreams/${encodeURIComponent(slug)}/mock/status`,undefined,undefined,signal),
  syncMock:(slug:string,version:number)=>request<MockStatus>('POST',`/upstreams/${encodeURIComponent(slug)}/mock/sync`,{},version),
  info: (signal?: AbortSignal) => request<{ version: string; configRevision: number; tenantId?: string }>('GET', '/info', undefined, undefined, signal),
  listUpstreams: async (signal?: AbortSignal) => listAll<Upstream>('/upstreams', signal),
  createUpstream: (slug: string, input: UpstreamInput) =>
    request<Upstream>('POST', '/upstreams', { slug, ...input }),
  patchUpstream: (slug: string, input: Partial<UpstreamInput>, version?: number) =>
    request<Upstream>('PATCH', `/upstreams/${encodeURIComponent(slug)}`, input, version),
  deleteUpstream: (slug: string, force = false, version?: number) =>
    request<void>('DELETE', `/upstreams/${encodeURIComponent(slug)}${force ? '?force=true' : ''}`, undefined, version),
  listCatalog: (slug:string, signal?: AbortSignal)=>request<Catalog>('GET',`/upstreams/${encodeURIComponent(slug)}/catalog`, undefined, undefined, signal),
  listTools: async (slug: string, signal?: AbortSignal) =>
    listAll<Tool>(`/upstreams/${encodeURIComponent(slug)}/tools`, signal),
  refreshTools: (slug: string) =>
    request<RefreshResult>('POST', `/upstreams/${encodeURIComponent(slug)}/refresh-tools`),
  testConnection: (input: { url: string; transport: string; headers?: Record<string, string>; tls?: UpstreamTLSInput; auth?: UpstreamAuthInput }) =>
    request<ConnectionTestResult>('POST', '/connection-tests', input),
  getVirtualServer: (slug:string,signal?:AbortSignal)=>request<VirtualServer>('GET',`/virtual-servers/${encodeURIComponent(slug)}`,undefined,undefined,signal),
  listVirtualServers: async (signal?: AbortSignal) =>
    listAll<VirtualServer>('/virtual-servers', signal),
  putVirtualServer: (
    slug: string,
    v: { project?:string; guidance?:Guidance; name: string; description?: string; enabled: boolean; omniEnabled?: boolean; authRequired: boolean; pii?: PiiConfig; policy?: ToolPolicy; rateLimitPerMinute?: number | null; tools: ToolSelection[];catalog?:CatalogSelection[];clientFeatures?:ClientFeatures },
    version?: number,
  ) => request<VirtualServer>('PUT', `/virtual-servers/${encodeURIComponent(slug)}`, v, version),
  deleteVirtualServer: (slug: string, version?: number) =>
    request<void>('DELETE', `/virtual-servers/${encodeURIComponent(slug)}`, undefined, version),

  listAgentKeys: async (signal?: AbortSignal) => listAll<AgentKey>('/agent-keys', signal),
  createAgentKey: (i: AgentKeyInput) => request<AgentKey & { key: string }>('POST', '/agent-keys', i),
  patchAgentKey: (slug: string, i: AgentKeyInput, version?: number) =>
    request<AgentKey>('PATCH', `/agent-keys/${encodeURIComponent(slug)}`, i, version),
  rotateAgentKey: (slug: string, version?: number) =>
    request<AgentKey & { key: string }>('POST', `/agent-keys/${encodeURIComponent(slug)}/rotate`, undefined, version),
  revokeAgentKey: (slug: string, version?: number) => request<AgentKey>('POST', `/agent-keys/${encodeURIComponent(slug)}/revoke`, undefined, version),
  deleteAgentKey: (slug: string, version?: number) => request<void>('DELETE', `/agent-keys/${encodeURIComponent(slug)}`, undefined, version),
  listBindings: async (signal?: AbortSignal) => listAll<IdentityBinding>('/identity-bindings', signal),
  createBinding: (b: Partial<IdentityBinding>) => request<IdentityBinding>('POST', '/identity-bindings', b),
  patchBinding: (slug: string, b: Partial<IdentityBinding>, version?: number) =>
    request<IdentityBinding>('PATCH', `/identity-bindings/${encodeURIComponent(slug)}`, b, version),
  deleteBinding: (slug: string, version?: number) => request<void>('DELETE', `/identity-bindings/${encodeURIComponent(slug)}`, undefined, version),
  evaluateBindings: (body: { claims: unknown; virtualServer?: string; clientIp?: string }) =>
    request<BindingEvaluation>('POST', '/identity-bindings/evaluate', body),
  listAdminTokens: async (signal?: AbortSignal) => listAll<AdminToken>('/admin-tokens', signal),
  createAdminToken: (i: { slug: string; description?: string; scope: 'admin' | 'read'; expiresAt?: string | null }) =>
    request<AdminToken & { token: string }>('POST', '/admin-tokens', i),
  rotateAdminToken: (slug: string, version?: number) =>
    request<AdminToken & { token: string }>('POST', `/admin-tokens/${encodeURIComponent(slug)}/rotate`, undefined, version),
  revokeAdminToken: (slug: string, version?: number) =>
    request<AdminToken>('POST', `/admin-tokens/${encodeURIComponent(slug)}/revoke`, undefined, version),
  deleteAdminToken: (slug: string, version?: number) => request<void>('DELETE', `/admin-tokens/${encodeURIComponent(slug)}`, undefined, version),
  listAudit: (q: AuditQuery, signal?: AbortSignal) => {
    const p = new URLSearchParams({ limit: '50' })
    Object.entries(q).forEach(([k, v]) => v && p.set(k, v))
    return request<{ items: AuditEvent[]; nextCursor?: string | null }>('GET', `/audit-events?${p}`, undefined, undefined, signal)
  },
}
