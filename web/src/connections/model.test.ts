import { expect, test } from 'vitest'
import { capabilityCounts, capabilitiesOf, connectionSettings, filterRelationships, keyState, relationships } from './model'
import { binding, key, server, upstream } from './fixtures'

test('flow keeps distinct grants and groups selected capabilities by upstream with aliases and policy',()=>{
 const edges=relationships([server],[upstream],[key],[binding])
 expect(edges.map(e=>e.kind)).toEqual(['agent-key','binding','upstream'])
 expect(edges[1].state).toBe('conditional')
 expect(edges[1].detail).toContain('azp equals team')
 expect(edges[2].capabilities).toHaveLength(4)
 expect(edges[2].detail).toContain('lookup → lookup (deny)')
 expect(edges[2].detail).toContain('doc://guide → doc://team')
 expect(capabilityCounts(server)).toBe('1 tool · 1 resource · 1 template · 1 prompt')
 expect(capabilitiesOf(server).find(c=>c.kind==='prompt')?.missing).toBe(true)
})
test('revoked, expired and disabled grants are never described as active clients',()=>{
 expect(keyState({...key,status:'active',expiresAt:'2020-01-01T00:00:00Z'})).toBe('expired')
 expect(keyState({...key,status:'active',revokedAt:'2020-01-01T00:00:00Z'})).toBe('revoked')
 const edges=relationships([{...server,enabled:false}],[upstream],[{...key,status:'revoked'}],[{...binding,enabled:false}])
 expect(edges[0].state).toBe('revoked · server disabled')
 expect(edges[1].state).toBe('disabled · server disabled')
})
test('public servers explicitly expose anonymous access and credential bypass',()=>{
 const edges=relationships([{...server,authRequired:false}],[upstream],[key],[])
 expect(edges.find(e=>e.kind==='anonymous')?.state).toBe('public access')
 expect(edges.find(e=>e.kind==='agent-key')?.state).toContain('bypasses credentials')
 const settings=JSON.parse(connectionSettings({...server,authRequired:false},'https://gateway.example/mcp'))
 expect(settings.mcpServers.demo.headers).toBeUndefined()
 expect(connectionSettings(server,'https://gateway.example/mcp')).toContain('<VMCP_AGENT_KEY_OR_ACCESS_TOKEN>')
})
test('missing servers, upstreams and capabilities remain inspectable and searchable',()=>{
 const edges=relationships([server],[],[{...key,virtualServers:['demo','deleted','deleted']}],[])
 expect(edges.filter(e=>e.server==='deleted')).toHaveLength(1)
 expect(edges.find(e=>e.server==='deleted')?.state).toBe('missing virtual server')
 expect(edges.find(e=>e.kind==='upstream')?.state).toBe('missing upstream')
 expect(filterRelationships(edges,'missing object','demo')).toHaveLength(1)
 expect(filterRelationships(edges,'doc://team','demo')).toHaveLength(1)
})
test('large installations have linear edge counts rather than credential/upstream cross products',()=>{
 const servers=Array.from({length:1000},(_,i)=>({...server,slug:`s-${i}`}))
 const keys=servers.map(s=>({...key,slug:`key-${s.slug}`,virtualServers:[s.slug]}))
 const edges=relationships(servers,[upstream],keys,[])
 expect(edges).toHaveLength(2000)
 expect(new Set(edges.map(e=>e.id)).size).toBe(2000)
 expect(filterRelationships(edges,'key-s-999')).toHaveLength(1)
})

test('filters include friendly server and upstream names as well as slugs',()=>{
 const edges=relationships([{...server,name:'Finance gateway'}],[{...upstream,name:'Customer database'}],[key],[])
 expect(filterRelationships(edges,'finance gateway')).toHaveLength(2)
 expect(filterRelationships(edges,'customer database')).toHaveLength(1)
})
