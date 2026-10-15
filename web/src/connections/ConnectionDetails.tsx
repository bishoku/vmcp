import { EmptyState } from '../Feedback'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowUpRight, GitBranch, ListChecks, Search, ShieldCheck } from 'lucide-react'
import type { Upstream } from '../api'
import { btn, input } from '../styles'
import { Modal, Pill } from '../ui'
import { serverPath, type Relationship } from './model'
import { DetailRow, ServerPanel, SummaryTile } from './ServerPanels'
import CapabilityTable from './CapabilityTable'
import Health from './Health'

export default function ConnectionDetails({relationship:r,upstream,onClose}:{relationship:Relationship;upstream?:Upstream;onClose:()=>void}) {
  const [query,setQuery]=useState('')
  const [kind,setKind]=useState('')
  const [limit,setLimit]=useState(50)
  const shown=r.capabilities.filter(c=>(!kind||c.kind===kind)&&`${c.original} ${c.exposed} ${c.policy} ${c.missing?'missing':''}`.toLowerCase().includes(query.trim().toLowerCase()))
  return <Modal title={`${r.from} → ${r.to}`} onClose={onClose} size="wide">
    <div className="space-y-6">
      <p className="text-xs leading-relaxed text-ink-muted">Saved configuration relationship. Grants and selected capabilities remain subject to authentication, conditions and policy.</p>
      <div className="grid gap-3 sm:grid-cols-2"><SummaryTile label="Relationship kind" value={r.kind} detail={r.kind==='upstream'?'Virtual server → upstream':'Configured identity → virtual server'} icon={GitBranch}/><SummaryTile label={r.kind==='upstream'?'Configured capabilities':'Virtual server'} value={r.kind==='upstream'?r.capabilities.length:r.server} detail={r.kind==='upstream'?`${r.capabilities.filter(c=>c.kind==='tool').length} configured tools`:'Access is defined by saved configuration'} icon={ListChecks}/></div>
      <ServerPanel title="Connection configuration" description="Source, destination and recorded relationship state" icon={GitBranch}>
        <dl className="divide-y divide-line px-5"><DetailRow label="From"><Link className="inline-flex items-center gap-1 text-accent-700 hover:underline" to={r.kind==='upstream'?serverPath(r.from):r.href}>{r.from}<ArrowUpRight className="size-3.5 shrink-0"/></Link></DetailRow><DetailRow label="To"><Link className="inline-flex items-center gap-1 text-accent-700 hover:underline" to={r.kind==='upstream'?r.href:serverPath(r.to)}>{r.to}<ArrowUpRight className="size-3.5 shrink-0"/></Link></DetailRow><DetailRow label="State"><Pill status={r.state}/></DetailRow></dl>
        {r.kind!=='upstream'&&<p className="mx-5 mb-5 break-words rounded-lg bg-canvas p-4 text-xs leading-relaxed text-ink-muted">{r.detail}</p>}
      </ServerPanel>
      {r.kind==='upstream'&&<>
        <ServerPanel title="Selected capability mappings" description="Original upstream definitions and exposed names on this virtual server" icon={ListChecks}>
          <div className="grid gap-3 p-5 sm:grid-cols-[2fr_1fr]"><label className="text-xs font-medium">Search mappings<div className="relative mt-2"><Search className="absolute left-3 top-2.5 size-4 text-ink-muted"/><input className={`${input} pl-9`} value={query} onChange={e=>{setQuery(e.target.value);setLimit(50)}} placeholder="Name, alias, policy or missing"/></div></label><label className="text-xs font-medium">Mapping kind<select className={`${input} mt-2`} value={kind} onChange={e=>{setKind(e.target.value);setLimit(50)}}><option value="">All kinds</option><option value="tool">Tools</option><option value="resource">Resources</option><option value="template">Templates</option><option value="prompt">Prompts</option></select></label></div>
          <div className="border-t border-line px-5 pb-5"><div className="max-h-[480px] overflow-auto">{shown.length?<CapabilityTable objects={shown.slice(0,limit)}/>:<EmptyState title={r.capabilities.length?'No matching mappings. Adjust your search or kind.':'No capabilities selected.'} compact/>}</div><div className="mt-4 flex flex-wrap items-center justify-between gap-3"><p className="text-xs text-ink-muted">{Math.min(shown.length,limit)} of {shown.length} matching mappings · {r.capabilities.length} configured</p>{shown.length>limit&&<button className={btn} onClick={()=>setLimit(n=>n+50)}>Show 50 more mappings</button>}</div></div>
        </ServerPanel>
        <ServerPanel title="Upstream observations" description="Stored discovery and node health; no live check is performed" icon={ShieldCheck}><div className="p-5">{upstream?<Health upstream={upstream}/>:<p className="text-xs text-ink-muted">This upstream is missing from the loaded configuration.</p>}</div></ServerPanel>
      </>}
      <div className="flex flex-wrap justify-end gap-3 border-t border-line pt-4"><Link className={btn} to={serverPath(r.server,'activity')}>Recent activity</Link><Link className={btn} to={r.href}>Open configuration<ArrowUpRight className="size-3.5"/></Link></div>
    </div>
  </Modal>
}
