import { filterItem } from '../search-filters'
import { SearchField, ActiveFilters, ResultCount } from '../SearchControls'
import Select from '../Select'
import { LoadMore } from '../DataTable'
import { EmptyState } from '../Feedback'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowUpRight, ChevronDown, Boxes, FileText, Layers3, MessageSquare, Wrench } from 'lucide-react'
import type { Upstream, VirtualServer } from '../api'
import { Pill } from '../ui'
import { input } from '../styles'
import { fmt } from '../format'
import { capabilitiesOf, editorPath, serverPath, type Capability } from './model'
import CapabilityTable from './CapabilityTable'
import Health from './Health'
import { ServerPanel, SummaryTile } from './ServerPanels'

export default function ServerCapabilities({server,upstreams}:{server:VirtualServer;upstreams:Upstream[]}) {
  const [query,setQuery]=useState('')
  const [kind,setKind]=useState('')
  const [limit,setLimit]=useState(50)
  const upstreamObjects=capabilitiesOf(server)
  const local:Capability[]=[
    ...(server.guidance?.resources??[]).filter(r=>r.enabled).map(r=>({source:'local' as const,kind:'resource' as const,original:r.name,exposed:`vmcp://guidance/resource/${r.name}`,upstream:'Local',policy:'Virtual server access',missing:false,description:r.description})),
    ...(server.guidance?.prompts??[]).filter(p=>p.enabled).map(p=>({source:'local' as const,kind:'prompt' as const,original:p.name,exposed:`vmcp_local__${p.name}`,upstream:'Local',policy:'Virtual server access',missing:false,description:p.description})),
    ...(server.guidance?.upstreamInstructions??[]).map(slug=>({source:'local' as const,kind:'resource' as const,original:`${slug} instructions`,exposed:`vmcp://guidance/upstream/${slug}`,upstream:slug,policy:'Virtual server access',missing:!upstreams.some(u=>u.slug===slug&&u.enabled),description:'Published when this upstream has discovered instructions.'})),
  ]
  const objects=[...upstreamObjects,...local]
  const sources=[...new Set([...upstreamObjects.map(o=>o.upstream),...(server.guidance?.upstreamInstructions??[])])]
  const shown=objects.filter(o=>(!kind || o.kind===kind) && `${o.original} ${o.exposed} ${o.upstream} ${o.policy} ${o.missing?'missing':'discovered'}`.toLowerCase().includes(query.toLowerCase()))
  const families=[{kind:'tool',label:'Tools',icon:Wrench},{kind:'resource',label:'Resources',icon:FileText},{kind:'template',label:'Templates',icon:Boxes},{kind:'prompt',label:'Prompts',icon:MessageSquare}] as const
  return <section aria-label="Server capabilities" className="layout-stack">
    <div><h2 className="section-title">Capabilities</h2><p className="section-description mt-1 text-sm text-ink-muted">Upstream selections and enabled local guidance. Runtime policy and discovery determine what agents can access.</p></div>
    <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">{families.map(f=><SummaryTile key={f.kind} label={f.label} value={objects.filter(o=>o.kind===f.kind).length} detail="Saved configuration selections" icon={f.icon}/>)}</div>
    <ServerPanel headingLevel={3} title="Selected capabilities" description={`${objects.length} selections · ${objects.filter(o=>o.missing).length} missing objects`} icon={Layers3} footer={<span>{Math.min(limit,shown.length)} of {shown.length} matching selections shown. Counts do not imply permission to execute.</span>}>
      <div className="flex flex-wrap items-center gap-3 border-b border-line panel-body"><SearchField label="Search capabilities" value={query} onChange={value=>{setQuery(value);setLimit(50)}} placeholder="Search name, alias, upstream or policy" className="relative min-w-0 w-full sm:max-w-sm"/><Select aria-label="Capability kind" className={`${input} w-full sm:w-44`} value={kind} onChange={e=>{setKind(e.target.value);setLimit(50)}}><option value="">All kinds</option>{families.map(f=><option key={f.kind} value={f.kind}>{f.label}</option>)}</Select></div>
      <ActiveFilters className="border-b border-line panel-footer" label="Active selections filters" items={[filterItem("Search",query,()=>{setQuery('');setLimit(50)}),filterItem("Kind",kind,()=>{setKind('');setLimit(50)})]} onReset={()=>{setQuery('');setKind('');setLimit(50)}}><ResultCount matched={shown.length} total={objects.length} noun="selections"/></ActiveFilters>
      {shown.length>0?<div className="panel-inset pb-5"><CapabilityTable objects={shown} limit={limit}/></div>:<EmptyState icon={Boxes} title={objects.length?'No matching capabilities':'No capabilities selected'} description={objects.length?'Try another search or capability kind.':'Select upstream tools or catalog objects in the server editor.'}/>}
      <LoadMore shown={Math.min(limit,shown.length)} total={shown.length} noun="capabilities" label="Show 50 more capabilities" onLoad={()=>setLimit(n=>n+50)}/>
    </ServerPanel>
    {server.hasGuidance&&<Link className="text-sm font-medium text-accent-700" to={serverPath(server.slug,'guidance')}>View guidance content, references and agent previews<ArrowUpRight className="ml-1 inline icon-inline" aria-hidden="true"/></Link>}
    <ServerPanel headingLevel={3} title="Upstream health observations" description="Recorded discovery and node health for selected upstreams" icon={Layers3} footer="Observations reflect their recorded timestamps. This view does not initiate a connectivity test.">
      <div className="divide-y divide-line">{sources.map(slug=>{
        const upstream=upstreams.find(u=>u.slug===slug)
        return <details key={slug} className="group"><summary className="flex cursor-pointer list-none flex-wrap items-center justify-between gap-3 panel-inset py-4 hover:bg-canvas"><div><span className="text-sm font-medium">{upstream?.name ?? slug}</span><p className="mt-1 text-xs text-ink-muted">Discovery: {fmt(upstream?.lastRefreshAt)} · Expand for node observations</p></div><span className="inline-flex items-center gap-3"><Pill status={!upstream?'missing upstream':upstream.enabled?upstream.health?.state ?? 'unknown':'disabled'}/><ChevronDown aria-hidden="true" className="icon-inline text-ink-muted transition-transform group-open:rotate-180"/></span></summary><div className="border-t border-line panel-inset pb-5">{upstream?<Health upstream={upstream}/>:<p className="py-4 text-sm text-ink-muted">This selected upstream is missing from the loaded configuration.</p>}<Link className="mt-3 inline-flex items-center gap-1 text-xs font-medium text-accent-700" to={editorPath('upstreams',slug)}>Open upstream configuration<ArrowUpRight aria-hidden="true" className="icon-inline"/></Link></div></details>
      })}{sources.length===0 && <EmptyState title="No upstreams selected." compact/>}</div>
    </ServerPanel>
  </section>
}
