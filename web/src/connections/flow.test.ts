import { expect, test } from 'vitest'
import { server, upstream, key, binding } from './fixtures'
import { relationships } from './model'
import { connectionGraph, highlightedEdges, nodeId, scopedGraph } from './flow'

const second = {...server,slug:'second',name:'Second',tools:[{...server.tools[0],tool:'other',exposedName:'other'}],catalog:[]}
const third = {...server,slug:'third',name:'Third',tools:[{...server.tools[0],alias:'renamed',exposedName:'renamed'}],catalog:[]}
const keys = [key,{...key,slug:'second-key',virtualServers:['second']},{...key,slug:'third-key',virtualServers:['third']}]
const servers = [server,second,third]
const rels = relationships(servers,[upstream],keys,[binding])
const graph = connectionGraph(servers,[upstream],keys,[binding],rels)

test('upstream tool traces every exact mapping, excluding other tools on the same upstream',()=>{
 const active=highlightedEdges(graph.edges,rels,undefined,{upstream:'alpha',original:'lookup'})
 expect(rels.filter(r=>active.has(r.id)).map(r=>r.server).sort()).toEqual(['demo','demo','demo','third','third'])
 const alias=highlightedEdges(graph.edges,rels,undefined,{upstream:'alpha',original:'lookup',server:'third'})
 expect(rels.filter(r=>alias.has(r.id)).map(r=>r.server)).toEqual(['third','third'])
})
test('identity highlighting does not include other identities sharing its virtual server',()=>{
 const active=highlightedEdges(graph.edges,rels,nodeId('agent-key','team-key'))
 expect(rels.filter(r=>active.has(r.id)).map(r=>r.kind).sort()).toEqual(['agent-key','upstream'])
})
test('drilling into identities restricts all three columns to their neighborhood',()=>{
 const visible=scopedGraph(graph.nodes,graph.edges,[{column:0,ids:[nodeId('agent-key','second-key')],label:'second-key'}])
 expect(visible.nodes.map(n=>n.id).sort()).toEqual([nodeId('agent-key','second-key'),nodeId('server','second'),nodeId('upstream','alpha')].sort())
 expect(visible.edges).toHaveLength(2)
})
test('disconnected nodes, non-tool connections and missing references remain represented',()=>{
 const isolated={...server,slug:'empty',tools:[],catalog:[]}
 const missingKey={...key,virtualServers:['missing']}
 const local=relationships([isolated, {...server,tools:[]}],[upstream],[missingKey],[])
 const overview=connectionGraph([isolated,{...server,tools:[]}],[upstream],[missingKey],[],local)
 expect(overview.nodes.some(n=>n.id===nodeId('server','empty'))).toBe(true)
 expect(overview.nodes.find(n=>n.id===nodeId('server','missing'))?.status).toBe('missing configuration')
 expect(overview.edges.find(e=>e.to===nodeId('upstream','alpha'))?.tools).toBe(0)
})
