import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowLeft, ArrowUpRight, ChevronRight, Layers3, Maximize2, Network, Search, Waypoints } from 'lucide-react'
import type { AgentKey, IdentityBinding, Upstream, VirtualServer } from '../api'
import { btn, input } from '../styles'
import { connectionGraph, highlightedEdges, nodeId, scopedGraph, type FlowNode, type FlowScope, type ToolFocus } from './flow'
import type { Relationship } from './model'

interface Props { servers:VirtualServer[]; upstreams:Upstream[]; keys:AgentKey[]; bindings:IdentityBinding[]; relationships:Relationship[] }
interface DiagramNode { id:string; column:number; label:string; status:string; href?:string; base?:FlowNode; tool?:ToolFocus; alias?:boolean }
interface DiagramEdge { id:string; from:string; to:string; tools:number; relationship:string; tool?:ToolFocus }
interface ToolScope { column:number; ids:string[]; label:string }
const colors = ['#6366f1', '#0891b2', '#8b5cf6', '#059669', '#059669']
const aliasId = (server:string,upstream:string,original:string,exposed:string)=>JSON.stringify(['alias',server,upstream,original,exposed])
const originalId = (upstream:string,original:string)=>JSON.stringify(['tool',upstream,original])
const matchesTool = (mapping:ToolFocus,focus:ToolFocus) => mapping.upstream===focus.upstream && mapping.original===focus.original && (!focus.server || mapping.server===focus.server)

export default function ConnectionFlow(props:Props) {
  const graph = useMemo(()=>connectionGraph(props.servers,props.upstreams,props.keys,props.bindings,props.relationships),[props.servers,props.upstreams,props.keys,props.bindings,props.relationships])
  const [scopes,setScopes] = useState<FlowScope[]>([])
  const [toolScopes,setToolScopes] = useState<ToolScope[]>([])
  const [expanded,setExpanded] = useState<string>()
  const [selected,setSelected] = useState<string>()
  const [preview,setPreview] = useState<string>()
  const [tool,setTool] = useState<ToolFocus>()
  const [previewTool,setPreviewTool] = useState<ToolFocus>()
  const [toolQuery,setToolQuery] = useState('')
  const scoped = scopedGraph(graph.nodes,graph.edges,scopes)
  const focusTool=previewTool ?? tool
  const active = highlightedEdges(graph.edges,props.relationships,preview ?? selected,focusTool)
  const focusing = !!(preview || selected || focusTool)
  const headings=expanded?['Configured identities','Virtual MCP servers','Exposed tools','Upstream definitions','Upstream MCP servers']:['Configured identities','Virtual MCP servers','Upstream MCP servers']
  const nodes:DiagramNode[]=scoped.nodes.map(n=>({...n,column:expanded && n.column===2?4:n.column,base:n}))
  const edges:DiagramEdge[]=scoped.edges.filter(e=>!expanded || e.to===nodeId('server',e.server)).map(e=>({...e,relationship:e.id}))
  const visibleRelationships=new Set(scoped.edges.map(e=>e.id))
  const mappings=props.relationships.filter(r=>expanded && r.kind==='upstream' && visibleRelationships.has(r.id)).flatMap(r=>r.capabilities.filter(c=>c.kind==='tool').map(c=>({...c,server:r.server,relationship:r.id})))
  const visibleMappings=mappings.filter(c=>`${c.original} ${c.exposed} ${c.upstream} ${c.server} ${c.policy}`.toLowerCase().includes(toolQuery.toLowerCase()) && toolScopes.every(s=>s.ids.includes(s.column===2?aliasId(c.server,c.upstream,c.original,c.exposed):originalId(c.upstream,c.original))))
  const seen=new Set(nodes.map(n=>n.id))
  for (const c of visibleMappings) {
    const alias=aliasId(c.server,c.upstream,c.original,c.exposed),original=originalId(c.upstream,c.original)
    const mapping={upstream:c.upstream,original:c.original,server:c.server}
    if(!seen.has(alias)){nodes.push({id:alias,column:2,label:c.exposed,status:`${c.server} · ${c.missing?'missing':c.policy}`,tool:mapping,alias:true});seen.add(alias)}
    if(!seen.has(original)){nodes.push({id:original,column:3,label:c.original,status:c.upstream,tool:{upstream:c.upstream,original:c.original}});seen.add(original)}
    for(const [from,to,stage] of [[nodeId('server',c.server),alias,'exposed'],[alias,original,'mapping'],[original,nodeId('upstream',c.upstream),'upstream']]) {
      edges.push({id:JSON.stringify([c.relationship,alias,stage]),from,to,tools:1,relationship:c.relationship,tool:mapping})
    }
  }
  // Catalog-only routes still cross the canvas, without inventing tool nodes.
  if(expanded) for(const e of scoped.edges.filter(e=>e.tools===0 && e.from===nodeId('server',e.server)))edges.push({...e,relationship:e.id})

  const edgeHighlighted=(e:DiagramEdge)=>active.has(e.relationship) && (!focusTool || !e.tool || matchesTool(e.tool,focusTool))
  const activeNodes=new Set(edges.filter(edgeHighlighted).flatMap(e=>[e.from,e.to]))
  if(selected)activeNodes.add(selected)
  const columns=headings.map((_,column)=>{
    const members=nodes.filter(n=>n.column===column).sort((a,b)=>a.label.localeCompare(b.label) || a.id.localeCompare(b.id))
    const size=Math.max(1,Math.ceil(members.length/12))
    return Array.from({length:Math.ceil(members.length/size)},(_,i)=>{
      const group=members.slice(i*size,(i+1)*size)
      return {id:group[0].id,column,members:group,label:group.length===1?group[0].label:`${group.length} ${column===0?'identities':expanded && (column===2 || column===3)?'tools':'servers'}`,status:group.length===1?group[0].status:`${group[0].label} … ${group.at(-1)!.label}`}
    })
  })
  const owner=new Map(columns.flatMap(col=>col.flatMap(n=>n.members.map(m=>[m.id,n.id] as const))))
  const bundles=new Map<string,{from:string;to:string;tools:number;edges:DiagramEdge[]}>()
  for(const e of edges){
    const from=owner.get(e.from)!,to=owner.get(e.to)!,id=JSON.stringify([from,to]),existing=bundles.get(id)
    if(existing){existing.tools+=e.tools;existing.edges.push(e)}else bundles.set(id,{from,to,tools:e.tools,edges:[e]})
  }
  const links=[...bundles.values()]
  const total=Math.max(1,...columns.map(col=>col.reduce((sum,n)=>sum+Math.max(links.filter(e=>e.from===n.id).reduce((s,e)=>s+e.tools,0),links.filter(e=>e.to===n.id).reduce((s,e)=>s+e.tools,0)),0)))
  const ribbonWidth=(count:number)=>Math.max(2,count*Math.min(14,240/total))
  const canvasWidth=1180,nodeWidth=expanded?180:245,spacing=expanded?240:390
  const columnHeights=columns.map(col=>col.map(n=>Math.max(94,...(['from','to'] as const).map(side=>links.filter(e=>e[side]===n.id).reduce((sum,e)=>sum+ribbonWidth(e.tools),0)+24))))
  const contentHeight=Math.max(180,...columnHeights.map(heights=>heights.reduce((sum,h)=>sum+h+20,0)))
  const layout=columns.map((col,i)=>{
    let y=66+(contentHeight-columnHeights[i].reduce((sum,h)=>sum+h+20,0))/2
    return col.map((n,j)=>{const height=columnHeights[i][j],node={...n,x:20+i*spacing,y,height};y+=height+20;return node})
  })
  const positions=new Map(layout.flat().map(n=>[n.id,n]))
  const ports=new Map<string,{from:number;to:number}>()
  for(const n of layout.flat())for(const side of ['from','to'] as const){
    const ordered=links.filter(e=>e[side]===n.id).sort((a,b)=>positions.get(a[side==='from'?'to':'from'])!.y-positions.get(b[side==='from'?'to':'from'])!.y)
    let port=n.y+(n.height-ordered.reduce((sum,e)=>sum+ribbonWidth(e.tools),0))/2
    for(const e of ordered){const id=JSON.stringify([e.from,e.to]),entry=ports.get(id) ?? {from:0,to:0};entry[side]=port+ribbonWidth(e.tools)/2;ports.set(id,entry);port+=ribbonWidth(e.tools)}
  }
  const inspect=graph.nodes.find(n=>n.id===expanded)
  const reset=()=>{setScopes([]);setToolScopes([]);setExpanded(undefined);setSelected(undefined);setPreview(undefined);setPreviewTool(undefined);setTool(undefined);setToolQuery('')}
  const clearHighlight=()=>{setTool(undefined);setPreviewTool(undefined);setPreview(undefined)}
  const expand=(n:DiagramNode)=>{
    clearHighlight()
    if(n.tool){setTool(n.tool);return}
    if(!n.base)return
    setSelected(n.id)
    if(n.base.column!==0){
      const next=n.id===expanded?scopes.slice(0,-1):[...scopes,{column:n.base.column,ids:[n.id],label:n.label}]
      setScopes(next);setExpanded(n.id===expanded?undefined:n.id);setToolScopes([]);setToolQuery('')
    }
  }
  const back=()=>{
    clearHighlight()
    if(toolScopes.length){setToolScopes(toolScopes.slice(0,-1));return}
    if(expanded){
      const next=scopes.slice(0,-1),previous=next.at(-1)
      setExpanded(previous && previous.column!==0 && previous.ids.length===1?previous.ids[0]:undefined);setScopes(next);setSelected(undefined);setToolQuery('');return
    }
    setScopes(scopes.slice(0,-1));setSelected(undefined)
  }

  return <section aria-label="Connection flow" className="overflow-hidden rounded-card border border-line bg-surface shadow-card">
    <div className="flex flex-wrap justify-between items-start gap-4 border-b border-line p-5 sm:px-6">
      <div className="flex gap-3"><div className="flex size-10 items-center justify-center rounded-xl bg-accent-50 text-accent-700"><Network className="size-5"/></div><div><h2 className="font-semibold">{expanded?'Tool routing':'Configuration flow'}</h2><p className="mt-1 text-xs text-ink-muted">{graph.nodes.length} nodes · {graph.edges.length} connections. {scopes.length?'Focused neighborhood':'All configured connections'}.</p></div></div>
      <button className={btn} onClick={reset}><Maximize2 className="size-3.5"/>Reset to overview</button>
    </div>
    <div className="flex flex-wrap items-center justify-between gap-3 px-5 py-3 sm:px-6 bg-canvas/50">
      <nav aria-label="Flow drill-down" className="flex flex-wrap items-center gap-2 text-xs"><button className="font-medium text-accent-700" onClick={reset}>All connections</button>{scopes.map((s,i)=><span className="inline-flex items-center gap-2" key={i}><ChevronRight className="size-3 text-ink-muted"/><button onClick={()=>{clearHighlight();setScopes(scopes.slice(0,i+1));setExpanded(s.column!==0 && s.ids.length===1?s.ids[0]:undefined);setToolQuery('');setToolScopes([]);setSelected(undefined)}}>{s.label}</button></span>)}{toolScopes.map((s,i)=><span className="inline-flex items-center gap-2" key={`tool-${i}`}><ChevronRight className="size-3 text-ink-muted"/><button onClick={()=>{clearHighlight();setToolScopes(toolScopes.slice(0,i+1))}}>{s.label}</button></span>)}</nav>
      {(scopes.length>0 || expanded) && <button className="inline-flex items-center gap-1 text-xs font-medium text-ink-muted" onClick={back}><ArrowLeft className="size-3"/>Back one level</button>}
    </div>
    {expanded && <div className="flex flex-wrap justify-between items-center gap-3 border-b border-line px-5 py-4 sm:px-6">
      <div><p className="text-sm font-medium">{inspect?.label} <span className="text-ink-muted font-normal">/ tool mappings</span></p><p className="mt-1 text-xs text-ink-muted">{visibleMappings.length} of {mappings.length} mappings · Select a tool to pin its path.</p></div>
      <label className="relative w-full sm:w-72"><span className="sr-only">Find tool mapping</span><Search className="absolute left-3 top-2.5 size-4 text-ink-muted"/><input className={`${input} pl-9 text-xs`} value={toolQuery} onChange={e=>{setToolQuery(e.target.value);clearHighlight()}} placeholder="Search tool, alias or policy"/></label>
    </div>}
    <div className="px-5 pt-4 sm:px-6 flex flex-wrap items-center justify-between gap-3">
      <p className="text-xs text-ink-muted inline-flex items-center gap-2"><Waypoints className="size-3.5"/>{expanded?'Follow exposed aliases to their original upstream tools. Hover or focus to preview paths.':'Select a server to unfold its tools inside the flow. Expand groups to explore larger inventories.'}</p>
      {inspect && <Link className="inline-flex items-center gap-1 text-xs font-medium text-accent-700" to={inspect.href}>Open configuration<ArrowUpRight className="size-3"/></Link>}
    </div>
    {tool && <div role="status" className="mx-5 mt-3 flex flex-wrap items-center gap-3 rounded-lg border border-accent-500/20 bg-accent-50 px-3 py-2 text-xs sm:mx-6"><span className="font-medium">Tracing {tool.upstream} / {tool.original}{tool.server?` through ${tool.server}`:' through all virtual servers'}</span><button className="text-accent-700 underline" onClick={clearHighlight}>Clear tool highlight</button></div>}
    <div className="overflow-auto max-h-[680px] p-4" tabIndex={0} aria-label="Scrollable configuration diagram">
      <svg width="100%" viewBox={`0 0 ${canvasWidth} ${contentHeight+80}`} style={{minWidth:880}} role="group" aria-label={expanded?'Identities through exposed tools to upstream definitions':'Identities to virtual servers to upstreams'}>
        <defs><pattern id="flow-grid" width="24" height="24" patternUnits="userSpaceOnUse"><circle cx="1" cy="1" r=".7" fill="currentColor" opacity=".08"/></pattern>{links.map((e,i)=><linearGradient key={i} id={`ribbon-${i}`} gradientUnits="userSpaceOnUse" x1={positions.get(e.from)!.x+nodeWidth} x2={positions.get(e.to)!.x} y1={ports.get(JSON.stringify([e.from,e.to]))!.from} y2={ports.get(JSON.stringify([e.from,e.to]))!.to}><stop offset="0%" stopColor={colors[positions.get(e.from)!.column]}/><stop offset="100%" stopColor={colors[positions.get(e.to)!.column]}/></linearGradient>)}</defs>
        <rect width={canvasWidth} height={contentHeight+80} fill="url(#flow-grid)" rx="12"/>
        {headings.map((h,i)=><g key={h}><circle cx={25+i*spacing} cy={27} r="4" fill={colors[i]}/><text x={37+i*spacing} y={32} fill="currentColor" fontSize={13} fontWeight={600}>{h}</text></g>)}
        {links.map((e,i)=>{
          const source=positions.get(e.from)!,target=positions.get(e.to)!,port=ports.get(JSON.stringify([e.from,e.to]))!,highlighted=e.edges.some(edgeHighlighted)
          return <path key={JSON.stringify([e.from,e.to])} data-testid="flow-ribbon" data-highlighted={focusing?String(highlighted):undefined} d={`M ${source.x+nodeWidth} ${port.from} C ${source.x+nodeWidth+55} ${port.from}, ${target.x-55} ${port.to}, ${target.x} ${port.to}`} fill="none" stroke={`url(#ribbon-${i})`} strokeWidth={ribbonWidth(e.tools)} strokeDasharray={e.tools===0?'4 4':undefined} opacity={focusing?(highlighted ? .85 : .05):.36} style={{transition:'opacity 180ms ease'}}><title>{source.label} → {target.label}: {e.tools} configured tool selections</title></path>
        })}
        {layout.flat().map(n=>{
          const grouped=n.members.length>1,first=n.members[0],highlighted=n.members.some(m=>activeNodes.has(m.id)),toolCount=links.filter(e=>n.column===headings.length-1?e.to===n.id:e.from===n.id).reduce((s,e)=>s+e.tools,0)
          return <foreignObject key={n.id} x={n.x} y={n.y} width={nodeWidth} height={n.height} opacity={focusing && !highlighted ? .3 : 1} style={{transition:'opacity 180ms ease'}}>
            <div className={`h-full rounded-xl border bg-surface shadow-sm overflow-hidden ${highlighted?'border-accent-500 ring-1 ring-accent-500/20':'border-line'}`} style={{borderLeftWidth:3,borderLeftColor:colors[n.column]}}>
              <button className="group block w-full px-3 pt-3 text-left focus-visible:outline-2 focus-visible:outline-accent-500" aria-label={grouped?`Expand ${n.label}`:first.tool?`Trace ${first.alias?'alias':'upstream tool'} ${first.tool.server && first.alias?first.tool.server:first.tool.upstream} / ${first.alias?first.label:first.tool.original}`:n.label} title={`${n.label} · ${n.status}`} aria-pressed={!grouped && (selected===n.id || (!!tool && !!first.tool && matchesTool(first.tool,tool)))} onMouseEnter={()=>{if(!grouped){if(first.tool)setPreviewTool(first.tool);else setPreview(n.id)}}} onMouseLeave={()=>{setPreview(undefined);setPreviewTool(undefined)}} onFocus={()=>{if(!grouped){if(first.tool)setPreviewTool(first.tool);else setPreview(n.id)}}} onBlur={()=>{setPreview(undefined);setPreviewTool(undefined)}} onClick={()=>{
                setPreview(undefined);setPreviewTool(undefined)
                if(grouped){clearHighlight();if(first.base){setScopes([...scopes,{column:first.base.column,ids:n.members.map(m=>m.id),label:n.status}]);setSelected(undefined)}else setToolScopes([...toolScopes,{column:n.column,ids:n.members.map(m=>m.id),label:n.status}])}
                else expand(first)
              }}><span className="flex items-center justify-between gap-2"><span className="truncate text-base font-semibold">{grouped?'Expand ':''}{n.label}</span>{grouped?<Layers3 className="size-3.5 shrink-0 text-ink-muted"/>:first.base && first.base.column!==0?<ChevronRight className="size-3.5 shrink-0 text-ink-muted group-hover:text-accent-700"/>:null}</span><span className="mt-1 block truncate text-xs text-ink-muted">{n.status}</span></button>
              <div className="flex items-center justify-between gap-2 px-3 pb-3 pt-2 text-[11px] text-ink-muted"><span>{first.tool&&!grouped?(first.alias?'Exposed alias':'Original definition'):`${toolCount} tool ${toolCount===1?'selection':'selections'}`}</span>{!grouped && first.href && <Link to={first.href} aria-label={`Open ${n.label} configuration`} className="text-accent-700"><ArrowUpRight className="size-3.5"/></Link>}</div>
            </div>
          </foreignObject>
        })}
      </svg>
    </div>
    {expanded && visibleMappings.length===0 && <p className="px-6 pb-5 text-sm text-ink-muted">No matching configured tool mappings. Catalog-only routes remain visible.</p>}
    {graph.nodes.length===0 && <p className="px-6 pb-5 text-sm text-ink-muted">No configured identities or servers.</p>}
    <div className="border-t border-line px-5 py-3 sm:px-6 text-[11px] text-ink-muted flex flex-wrap gap-x-5 gap-y-1"><span>Width = configured tool selections</span><span>Dashed = zero tools / catalog access</span><span>Conditional, disabled, denied and missing mappings remain visible.</span><span>Configuration only · counts repeat across identity grants</span></div>
  </section>
}
