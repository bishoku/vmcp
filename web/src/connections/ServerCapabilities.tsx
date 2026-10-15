import { EmptyState } from '../Feedback'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowUpRight, ChevronDown, Boxes, FileText, Layers3, MessageSquare, Search, Wrench } from 'lucide-react'
import type { Upstream, VirtualServer } from '../api'
import { Pill } from '../ui'
import { input, btn } from '../styles'
import { fmt } from '../format'
import { capabilitiesOf, editorPath } from './model'
import CapabilityTable from './CapabilityTable'
import Health from './Health'
import { ServerPanel, SummaryTile } from './ServerPanels'

export default function ServerCapabilities({server,upstreams}:{server:VirtualServer;upstreams:Upstream[]}) {
  const [query,setQuery]=useState('')
  const [kind,setKind]=useState('')
  const [limit,setLimit]=useState(50)
  const objects=capabilitiesOf(server)
  const sources=[...new Set(objects.map(o=>o.upstream))]
  const shown=objects.filter(o=>(!kind || o.kind===kind) && `${o.original} ${o.exposed} ${o.upstream} ${o.policy} ${o.missing?'missing':'discovered'}`.toLowerCase().includes(query.toLowerCase()))
  const families=[{kind:'tool',label:'Tools',icon:Wrench},{kind:'resource',label:'Resources',icon:FileText},{kind:'template',label:'Templates',icon:Boxes},{kind:'prompt',label:'Prompts',icon:MessageSquare}] as const
  return <section aria-label="Server capabilities" className="space-y-6">
    <div><h2 className="text-lg font-semibold tracking-tight">Capabilities</h2><p className="mt-1 text-sm text-ink-muted">Selected tools and catalog objects composed from your upstream servers.</p></div>
    <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">{families.map(f=><SummaryTile key={f.kind} label={f.label} value={objects.filter(o=>o.kind===f.kind).length} detail="Saved configuration selections" icon={f.icon}/>)}</div>
    <ServerPanel title="Selected capabilities" description={`${objects.length} selections · ${objects.filter(o=>o.missing).length} missing objects`} icon={Layers3} footer={<span>{Math.min(limit,shown.length)} of {shown.length} matching selections shown. Counts do not imply permission to execute.</span>}>
      <div className="flex flex-wrap items-center gap-3 border-b border-line p-4"><label className="relative min-w-0 w-full sm:max-w-sm"><span className="sr-only">Search capabilities</span><Search className="absolute left-3 top-2.5 size-4 text-ink-muted"/><input className={`${input} pl-9`} value={query} onChange={e=>{setQuery(e.target.value);setLimit(50)}} placeholder="Search name, alias, upstream or policy"/></label><select aria-label="Capability kind" className={`${input} w-full sm:w-44`} value={kind} onChange={e=>{setKind(e.target.value);setLimit(50)}}><option value="">All kinds</option>{families.map(f=><option key={f.kind} value={f.kind}>{f.label}</option>)}</select></div>
      {shown.length>0?<div className="px-5 pb-5"><CapabilityTable objects={shown.slice(0,limit)}/></div>:<EmptyState icon={Boxes} title={objects.length?'No matching capabilities':'No capabilities selected'} description={objects.length?'Try another search or capability kind.':'Select upstream tools or catalog objects in the server editor.'}/>}
      {shown.length>limit && <div className="px-5 pb-5"><button className={btn} onClick={()=>setLimit(n=>n+50)}>Show 50 more capabilities</button></div>}
    </ServerPanel>
    <ServerPanel title="Upstream health observations" description="Recorded discovery and node health for selected upstreams" icon={Layers3} footer="Observations reflect their recorded timestamps. This view does not initiate a connectivity test.">
      <div className="divide-y divide-line">{sources.map(slug=>{
        const upstream=upstreams.find(u=>u.slug===slug)
        return <details key={slug} className="group"><summary className="flex cursor-pointer list-none flex-wrap items-center justify-between gap-3 px-5 py-4 hover:bg-canvas"><div><span className="text-sm font-medium">{upstream?.name ?? slug}</span><p className="mt-1 text-xs text-ink-muted">Discovery: {fmt(upstream?.lastRefreshAt)} · Expand for node observations</p></div><span className="inline-flex items-center gap-3"><Pill status={!upstream?'missing upstream':upstream.enabled?upstream.health?.state ?? 'unknown':'disabled'}/><ChevronDown className="size-3.5 text-ink-muted transition-transform group-open:rotate-180"/></span></summary><div className="border-t border-line px-5 pb-5">{upstream?<Health upstream={upstream}/>:<p className="py-4 text-sm text-ink-muted">This selected upstream is missing from the loaded configuration.</p>}<Link className="mt-3 inline-flex items-center gap-1 text-xs font-medium text-accent-700" to={editorPath('upstreams',slug)}>Open upstream configuration<ArrowUpRight className="size-3"/></Link></div></details>
      })}{sources.length===0 && <EmptyState title="No upstreams selected." compact/>}</div>
    </ServerPanel>
  </section>
}
