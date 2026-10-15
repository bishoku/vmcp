import { filterItem } from '../search-filters'
import { SearchField, ActiveFilters, ResultCount } from '../SearchControls'
import { Checkbox } from '../SelectionControls'
import { LoadMore } from '../DataTable'
import { EmptyState } from '../Feedback'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowUpRight, Layers3, ShieldCheck } from 'lucide-react'
import type { VirtualServer } from '../api'
import { Pill } from '../ui'
import { serverPath } from './model'
import { DetailRow, ServerPanel } from './ServerPanels'

export function GrantedServers({slugs,servers}:{slugs:string[];servers:VirtualServer[]}) {
  return <ServerPanel title="Virtual server access" description={`${slugs.length} configured grants · runtime restrictions still apply`} icon={Layers3}>
    <div className="divide-y divide-line">{slugs.map(slug=>{
      const server=servers.find(s=>s.slug===slug)
      return <Link key={slug} to={serverPath(slug)} className="flex items-start justify-between gap-3 px-5 py-4 hover:bg-canvas"><div className="min-w-0"><p className="break-all text-sm font-medium text-accent-700">{server?.name ?? slug}</p><p className="mt-1 break-all font-mono text-xs text-ink-muted">{slug}</p>{server && !server.authRequired && <p className="mt-1 text-xs text-amber-700">Anonymous endpoint · credentials bypassed</p>}</div><span className="inline-flex shrink-0 items-center gap-2"><Pill status={!server?'unavailable':server.enabled?'enabled':'disabled'}/><ArrowUpRight className="size-3 text-ink-muted"/></span></Link>
    })}{slugs.length===0 && <EmptyState title="No virtual servers granted." compact/>}</div>
  </ServerPanel>
}

export function AccessRestrictions({ips,rate,expires}:{ips:string[];rate:number|null;expires?:string}) {
  return <ServerPanel title="Access restrictions" description="Network, rate and lifetime settings" icon={ShieldCheck}>
    <dl className="divide-y divide-line px-5"><DetailRow label="IP allowlist"><span className="break-all">{ips.join(', ') || 'Unrestricted'}</span></DetailRow><DetailRow label="Rate limit">{rate==null?'Unlimited':`${rate} calls / minute`}</DetailRow>{expires!==undefined && <DetailRow label="Expires">{expires}</DetailRow>}</dl>
  </ServerPanel>
}

export function ServerGrantPicker({servers,selected,toggle}:{servers:VirtualServer[];selected:string[];toggle:(slug:string)=>void}) {
  const [query,setQuery]=useState('')
  const [limit,setLimit]=useState(25)
  const choices=[...servers.map(s=>({slug:s.slug,label:s.name,status:s.enabled?'enabled':'disabled'})),...selected.filter(slug=>!servers.some(s=>s.slug===slug)).map(slug=>({slug,label:slug,status:'unavailable'}))]
  const shown=choices.filter(s=>`${s.label} ${s.slug}`.toLowerCase().includes(query.toLowerCase()))
  return <div className="p-5"><SearchField label="Search virtual servers" value={query} onChange={value=>{setQuery(value);setLimit(25)}} placeholder="Search virtual servers" className="relative block"/><ActiveFilters className="mt-3" label="Active server picker filters" items={[filterItem('Search',query,()=>{setQuery('');setLimit(25)})]} onReset={()=>{setQuery('');setLimit(25)}}><ResultCount matched={shown.length} total={choices.length} noun="servers"/></ActiveFilters><p className="my-3 text-xs text-ink-muted">{selected.length} selected · Filtering preserves selections.</p><div className="divide-y divide-line rounded-lg border border-line">{shown.slice(0,limit).map(s=><Checkbox key={s.slug} label={s.label} description={<span className="break-all font-mono">{s.slug}</span>} end={<Pill status={s.status}/>} variant="row" checked={selected.includes(s.slug)} onChange={()=>toggle(s.slug)}/>)}{shown.length>limit&&<LoadMore shown={Math.min(limit,shown.length)} total={shown.length} noun="servers" label="Show 25 more servers" onLoad={()=>setLimit(n=>n+25)}/>}{shown.length===0 && <EmptyState title={choices.length?'No matching virtual servers.':'No virtual servers.'} compact/>}</div></div>
}
