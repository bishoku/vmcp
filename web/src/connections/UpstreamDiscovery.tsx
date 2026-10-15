import { useState } from 'react'
import { Boxes, RefreshCw, Search } from 'lucide-react'
import type { Catalog, Tool } from '../api'
import { btn, input } from '../styles'
import { ErrorBox } from '../ui'
import { ServerPanel } from './ServerPanels'

interface Props {tools:Tool[];catalog:Catalog|null;loading:boolean;error:string;reload:()=>Promise<void>}
export default function UpstreamDiscovery({tools,catalog,loading,error,reload}:Props) {
  const [query,setQuery]=useState('')
  const [kind,setKind]=useState('')
  const [limit,setLimit]=useState(50)
  const rows=[
    ...tools.map(t=>({kind:'tool',name:t.name,description:t.description ?? ''})),
    ...(catalog?.resources ?? []).map(o=>({kind:'resource',name:String(o.uri ?? o.name ?? ''),description:String(o.description ?? '')})),
    ...(catalog?.resourceTemplates ?? []).map(o=>({kind:'template',name:String(o.uriTemplate ?? o.name ?? ''),description:String(o.description ?? '')})),
    ...(catalog?.prompts ?? []).map(o=>({kind:'prompt',name:String(o.name ?? ''),description:String(o.description ?? '')})),
  ]
  const families=[['tool','Tools'],['resource','Resources'],['template','Templates'],['prompt','Prompts']] as const
  const shown=rows.filter(r=>(!kind || r.kind===kind) && `${r.name} ${r.description}`.toLowerCase().includes(query.toLowerCase()))
  return <section aria-label="Upstream discovery" className="space-y-6">
    <div className="flex flex-wrap items-start justify-between gap-4"><div><h2 className="text-lg font-semibold tracking-tight">Discovered capabilities</h2><p className="mt-1 text-sm text-ink-muted">Stored tools, resources, templates and prompts for this upstream.</p></div><button className={btn} onClick={()=>reload()} disabled={loading}><RefreshCw className={`size-3.5 ${loading?'animate-spin':''}`}/>Reload discovery snapshots</button></div>
    {error && <ErrorBox>{error}. Successfully loaded snapshots are retained; the catalog may be incomplete or older.</ErrorBox>}
    <ServerPanel title="Discovery snapshots" description={`${rows.length} loaded objects · saved discovery only`} icon={Boxes} footer="Reload reads stored discovery only. Use Refresh capabilities in the upstream list to discover changes.">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line p-4"><div className="flex flex-wrap gap-2"><button className={`${btn} ${!kind?'bg-accent-50 text-accent-700':''}`} aria-pressed={!kind} onClick={()=>{setKind('');setLimit(50)}}>All ({rows.length})</button>{families.map(([value,label])=><button className={`${btn} ${kind===value?'bg-accent-50 text-accent-700':''}`} key={value} aria-pressed={kind===value} onClick={()=>{setKind(value);setLimit(50)}}>{label} ({rows.filter(r=>r.kind===value).length})</button>)}</div><label className="relative min-w-0 w-full sm:w-64"><span className="sr-only">Search discovered capabilities</span><Search className="absolute left-3 top-2.5 size-4 text-ink-muted"/><input className={`${input} pl-9`} value={query} onChange={e=>{setQuery(e.target.value);setLimit(50)}} placeholder="Search name or description"/></label></div>
      {loading && <p role="status" className="px-5 py-4 text-xs text-ink-muted">Loading discovery snapshots…</p>}
      {shown.length>0?<div className="overflow-x-auto"><table className="w-full min-w-[640px] text-left text-sm"><caption className="sr-only">Stored upstream capabilities</caption><thead className="bg-canvas text-xs text-ink-muted"><tr>{['Kind','Name / URI','Description'].map(h=><th key={h} scope="col" className="px-5 py-3 font-medium">{h}</th>)}</tr></thead><tbody>{shown.slice(0,limit).map((r,i)=><tr key={JSON.stringify([r.kind,r.name,i])} className="border-t border-line hover:bg-canvas/50"><td className="px-5 py-3"><span className="rounded-md bg-canvas px-2 py-1 text-xs font-medium">{r.kind}</span></td><td className="max-w-80 break-all px-5 py-3 font-mono text-xs">{r.name}</td><td className="max-w-md break-words px-5 py-3 text-xs leading-relaxed text-ink-muted">{r.description || 'No description provided.'}</td></tr>)}</tbody></table></div>:!loading && <div className="px-5 py-10 text-center"><Boxes className="mx-auto size-6 text-accent-700"/><h4 className="mt-3 text-sm font-medium">{rows.length?'No matching capabilities':error?'Discovery unavailable':'No stored capabilities'}</h4><p className="mt-2 text-xs text-ink-muted">{rows.length?'Try another search or capability family.':error?'Retry loading the saved discovery snapshots.':'Refresh capabilities from the upstream list to discover available objects.'}</p></div>}
      {shown.length>limit && <div className="p-5"><button className={btn} onClick={()=>setLimit(n=>n+50)}>Show 50 more discovered capabilities</button></div>}
    </ServerPanel>
  </section>
}
