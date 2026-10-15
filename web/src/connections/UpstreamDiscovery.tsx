import Button from '../Button'
import { TagBadge } from '../Badge'
import { filterItem } from '../search-filters'
import { SearchField, ActiveFilters, ResultCount } from '../SearchControls'
import { RadioGroup } from '../SelectionControls'
import { DataTable, LoadMore } from '../DataTable'
import { useTableSort } from '../table-sorting'
import { EmptyState, LoadingState } from '../Feedback'
import { useState } from 'react'
import { Boxes, RefreshCw } from 'lucide-react'
import type { Catalog, Tool } from '../api'
import { btn } from '../styles'
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
  const sorting=useTableSort(shown,{Kind:r=>r.kind,'Name / URI':r=>r.name,Description:r=>r.description})
  return <section aria-label="Upstream discovery" className="space-y-6">
    <div className="flex flex-wrap items-start justify-between gap-4"><div><h2 className="section-title">Discovered capabilities</h2><p className="section-description mt-1 text-sm text-ink-muted">Stored tools, resources, templates and prompts for this upstream.</p></div><Button loading={loading} className={btn} onClick={()=>reload()} disabled={loading}><RefreshCw className={`size-3.5 ${loading?'animate-spin':''}`}/>Reload discovery snapshots</Button></div>
    {error && <ErrorBox title="Discovery snapshots could not be loaded" onRetry={reload} busy={loading}>{error}. Successfully loaded snapshots are retained; the catalog may be incomplete or older.</ErrorBox>}
    <ServerPanel title="Discovery snapshots" description={`${rows.length} loaded objects · saved discovery only`} icon={Boxes} footer="Reload reads stored discovery only. Use Refresh capabilities in the upstream list to discover changes.">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line p-4"><RadioGroup label="Capability family" value={kind} onChange={value=>{setKind(value);setLimit(50)}} options={[{value:'',label:`All (${rows.length})`},...families.map(([value,label])=>({value,label:`${label} (${rows.filter(r=>r.kind===value).length})`}))]}/><SearchField label="Search discovered capabilities" value={query} onChange={value=>{setQuery(value);setLimit(50)}} placeholder="Search name or description" className="relative min-w-0 w-full sm:w-64"/></div>
      <ActiveFilters className="border-b border-line px-4 py-3" label="Active objects filters" items={[filterItem("Search",query,()=>{setQuery('');setLimit(50)}),filterItem("Kind",kind,()=>{setKind('');setLimit(50)})]} onReset={()=>{setQuery('');setKind('');setLimit(50)}}><ResultCount matched={shown.length} total={rows.length} noun="objects" busy={loading} note={error?'catalog may be incomplete':undefined}/></ActiveFilters>
      {loading && <LoadingState label="Loading discovery snapshots…" compact/>}
      {shown.length>0?<DataTable caption="Stored upstream capabilities" columns={['Kind','Name / URI','Description']} sorting={sorting} primary={1} minWidth={640}>{sorting.rows.slice(0,limit).map((r,i)=><tr key={JSON.stringify([r.kind,r.name,i])} className="border-t border-line hover:bg-canvas/50"><td className="px-5 py-3"><TagBadge label={r.kind}/></td><td className="max-w-80 break-all px-5 py-3 font-mono text-xs">{r.name}</td><td className="max-w-md break-words px-5 py-3 text-xs leading-relaxed text-ink-muted">{r.description || 'No description provided.'}</td></tr>)}</DataTable>:!loading && <EmptyState icon={Boxes} title={rows.length?'No matching capabilities':error?'Discovery unavailable':'No stored capabilities'} description={rows.length?'Try another search or capability family.':error?'Retry loading the saved discovery snapshots.':'Refresh capabilities from the upstream list to discover available objects.'}/>}
      <LoadMore shown={Math.min(limit,shown.length)} total={shown.length} noun="capabilities" label="Show 50 more discovered capabilities" onLoad={()=>setLimit(n=>n+50)}/>
    </ServerPanel>
  </section>
}
