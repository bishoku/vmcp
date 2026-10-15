import type { AgentKey, IdentityBinding, Upstream, VirtualServer } from '../api'
import { editorPath, keyState, serverPath, type Relationship } from './model'

export type Column = 0 | 1 | 2
export interface FlowNode { id: string; column: Column; label: string; status: string; href: string }
export interface FlowEdge { id: string; from: string; to: string; server: string; tools: number }
export interface FlowScope { column: Column; ids: string[]; label: string }
export interface ToolFocus { upstream: string; original: string; server?: string }
export const nodeId = (kind: string, slug: string) => JSON.stringify([kind, slug])

export function connectionGraph(servers: VirtualServer[], upstreams: Upstream[], keys: AgentKey[], bindings: IdentityBinding[], relationships: Relationship[]) {
  const nodes: FlowNode[] = [
    ...keys.map(k => ({id:nodeId('agent-key',k.slug),column:0 as const,label:k.slug,status:`agent key · ${keyState(k)}`,href:editorPath('agent-keys',k.slug)})),
    ...bindings.map(b => ({id:nodeId('binding',b.slug),column:0 as const,label:b.slug,status:b.enabled?'conditional JWT':'disabled JWT',href:editorPath('identities',b.slug)})),
    ...servers.map(s => ({id:nodeId('server',s.slug),column:1 as const,label:s.name,status:s.enabled?'enabled':'disabled',href:serverPath(s.slug)})),
    ...upstreams.map(u => ({id:nodeId('upstream',u.slug),column:2 as const,label:u.name,status:u.enabled?'enabled':'disabled',href:editorPath('upstreams',u.slug)})),
  ]
  const known = new Set(nodes.map(n=>n.id))
  const toolCounts = new Map(servers.map(s=>[s.slug,s.tools.length]))
  const edges = relationships.map(r => {
    const upstream = r.kind === 'upstream'
    const from = nodeId(upstream?'server':r.kind, r.from)
    const to = nodeId(upstream?'upstream':'server',r.to)
    for (const [id, column, label] of [[from,upstream?1:0,r.from],[to,upstream?2:1,r.to]] as const) {
      if (!known.has(id)) { nodes.push({id,column,label,status:r.kind==='anonymous'?'public access':'missing configuration',href:column===1?serverPath(label):column===2?editorPath('upstreams',label):r.href}); known.add(id) }
    }
    return {id:r.id,from,to,server:r.server,tools:upstream?r.capabilities.filter(c=>c.kind==='tool').length:(toolCounts.get(r.server) ?? 0)}
  })
  return {nodes,edges}
}

/** Exact configured paths, including conditional, disabled and denied mappings. */
export function highlightedEdges(edges: FlowEdge[], relationships: Relationship[], selected?: string, tool?: ToolFocus): Set<string> {
  if (tool) {
    const outgoing = relationships.filter(r=>r.kind==='upstream' && r.to===tool.upstream && (!tool.server || r.server===tool.server) && r.capabilities.some(c=>c.kind==='tool' && c.original===tool.original))
    const servers = new Set(outgoing.map(r=>r.server))
    return new Set(relationships.filter(r=>r.kind==='upstream'?outgoing.includes(r):servers.has(r.server)).map(r=>r.id))
  }
  if (!selected) return new Set()
  const adjacent = edges.filter(e=>e.from===selected || e.to===selected)
  const servers = new Set(adjacent.map(e=>e.server))
  const identity = selected.startsWith('["agent-key",') || selected.startsWith('["binding",') || selected.startsWith('["anonymous",')
  const upstream = selected.startsWith('["upstream",')
  return new Set(edges.filter(e=>servers.has(e.server) && (identity?(e.from===selected || e.from===nodeId('server',e.server)):upstream?(e.to===selected || e.to===nodeId('server',e.server)):true)).map(e=>e.id))
}

export function scopedGraph(nodes: FlowNode[], edges: FlowEdge[], scopes: FlowScope[]) {
  const columns = new Map(nodes.map(n=>[n.id,n.column]))
  const constraints = scopes.map(s=>({...s,ids:new Set(s.ids)}))
  const allowed = (id:string) => constraints.every(s=>columns.get(id)!==s.column || s.ids.has(id))
  const serverSets = scopes.filter(s=>s.column!==1).map(s=>new Set(edges.filter(e=>s.ids.includes(s.column===0?e.from:e.to)).map(e=>e.server)))
  const visibleEdges = edges.filter(e=>allowed(e.from) && allowed(e.to) && serverSets.every(set=>set.has(e.server)))
  const connected = new Set(visibleEdges.flatMap(e=>[e.from,e.to]))
  const visibleNodes = nodes.filter(n=>allowed(n.id) && (!scopes.length || connected.has(n.id) || scopes.some(s=>s.ids.includes(n.id))))
  return {nodes:visibleNodes,edges:visibleEdges}
}
