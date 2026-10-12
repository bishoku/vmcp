import type { AgentKey, IdentityBinding, Upstream, VirtualServer } from '../api'

export const serverPath = (slug: string, view = 'overview') => `/virtual-servers/${encodeURIComponent(slug)}/${view}`
export const editorPath = (page: string, slug: string) => page === 'upstreams' || page === 'virtual-servers' ? `/${page}/${encodeURIComponent(slug)}` : `/${page}?inspect=${encodeURIComponent(slug)}`
export interface Capability {
  kind: 'tool' | 'resource' | 'template' | 'prompt'
  original: string
  exposed: string
  upstream: string
  policy: string
  missing: boolean
}
export function capabilitiesOf(server: VirtualServer): Capability[] {
  return [
    ...server.tools.map(tool => ({ kind: 'tool' as const, original: tool.tool,
      exposed: tool.exposedName || tool.alias || `${tool.upstream}__${tool.tool}`, upstream: tool.upstream,
      policy: server.policy?.tools?.[tool.exposedName || tool.alias || `${tool.upstream}__${tool.tool}`] ?? server.policy?.default ?? 'allow', missing: tool.orphaned })),
    ...(server.catalog ?? []).map(object => ({kind: object.kind, original: object.name,
      exposed: object.exposedName || object.alias || object.name, upstream: object.upstream,
      policy: 'Selected catalog access', missing: object.orphaned})),
  ]
}
export function capabilityCounts(server: VirtualServer) {
  const objects = capabilitiesOf(server)
  return (['tool', 'resource', 'template', 'prompt'] as const).map(kind => {
    const count = objects.filter(o => o.kind === kind).length
    return `${count} ${kind}${count === 1 ? '' : 's'}`
  }).join(' · ')
}
export function keyState(key: AgentKey, now = Date.now()): string {
  if (key.revokedAt || key.status === 'revoked') return 'revoked'
  if (key.status === 'expired' || (key.expiresAt && new Date(key.expiresAt).getTime() <= now)) return 'expired'
  return key.status
}
export interface Relationship {
  id: string
  kind: 'agent-key' | 'binding' | 'anonymous' | 'upstream'
  from: string
  to: string
  server: string
  state: string
  detail: string
  href: string
  capabilities: Capability[]
  search?: string
}
/** Edges represent saved configuration, never active sessions or guaranteed authorization. */
export function relationships(servers: VirtualServer[], upstreams: Upstream[], keys: AgentKey[], bindings: IdentityBinding[], now = Date.now()): Relationship[] {
  const byServer = new Map(servers.map(s => [s.slug, s]))
  const byUpstream = new Map(upstreams.map(u => [u.slug, u]))
  const edges: Relationship[] = []
  const grant = (kind: 'agent-key' | 'binding', slug: string, targets: string[], state: string, detail: string) => {
    for (const target of new Set(targets)) {
      const server = byServer.get(target)
      edges.push({id: JSON.stringify([kind, slug, target]), kind, from: slug, to: target, server: target,
        state: !server ? 'missing virtual server' : !server.enabled ? `${state} · server disabled` : !server.authRequired ? `${state} · anonymous access bypasses credentials` : state,
        detail, search:server?.name, href: editorPath(kind === 'agent-key' ? 'agent-keys' : 'identities', slug), capabilities: []})
    }
  }
  for (const key of keys) grant('agent-key', key.slug, key.virtualServers, keyState(key, now),
    `IP allowlist: ${key.ipAllowlist.join(', ') || 'unrestricted'}; key limit: ${key.rateLimitPerMinute ?? 'unlimited'}/minute; expiry: ${key.expiresAt ?? 'none'}`)
  for (const binding of bindings) grant('binding', binding.slug, binding.virtualServers, binding.enabled ? 'conditional' : 'disabled',
    `Verified JWT and matching claims required: ${binding.conditions.map(c => `${c.claim} ${c.op} ${c.value ?? ''}`).join('; ')}. Binding precedence and IP restrictions also apply. IP allowlist: ${binding.ipAllowlist.join(', ') || 'unrestricted'}`)
  for (const server of servers) {
    if (!server.authRequired) edges.push({id: JSON.stringify(['anonymous', server.slug]), kind:'anonymous',from:'Anonymous',to:server.slug,server:server.slug,
      state:server.enabled ? 'public access' : 'server disabled', search:server.name,detail:'Any caller that can reach this endpoint; no credential required.',href:serverPath(server.slug,'access'),capabilities:[]})
    const groups = new Map<string, Capability[]>()
    for (const object of capabilitiesOf(server)) {
      const group = groups.get(object.upstream)
      if(group) group.push(object)
      else groups.set(object.upstream,[object])
    }
    for (const [slug, objects] of groups) {
      const upstream = byUpstream.get(slug)
      edges.push({id:JSON.stringify(['upstream',server.slug,slug]),kind:'upstream',from:server.slug,to:slug,server:server.slug,
        state:!upstream ? 'missing upstream' : !upstream.enabled ? 'upstream disabled' : upstream.health?.state ?? 'unknown',
        detail:objects.map(o=>`${o.kind}: ${o.original} → ${o.exposed} (${o.policy}${o.missing ? ', missing object' : ''})`).join('; '),
        search:`${server.name} ${upstream?.name ?? ''}`,href:editorPath('upstreams',slug),capabilities:objects})
    }
  }
  return edges
}
export function filterRelationships(edges: Relationship[], query: string, server = '') {
  const needle = query.trim().toLowerCase()
  return edges.filter(edge => (!server || edge.server === server) &&
    `${edge.kind} ${edge.from} ${edge.to} ${edge.state} ${edge.detail} ${edge.search ?? ''}`.toLowerCase().includes(needle))
}
export function connectionSettings(server: VirtualServer, endpoint: string) {
  return JSON.stringify({mcpServers:{[server.slug]:{url:endpoint,
    ...(server.authRequired && {headers:{Authorization:'Bearer <VMCP_AGENT_KEY_OR_ACCESS_TOKEN>'}})}}}, null, 2)
}
