import Button from '../Button'
import { TagBadge } from '../Badge'
import { filterItem } from '../search-filters'
import { SearchField, ActiveFilters, ResultCount } from '../SearchControls'
import Select from '../Select'
import { DataTable, LoadMore } from '../DataTable'
import { useTableSort } from '../table-sorting'
import { EmptyState, LoadingState } from '../Feedback'
import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowRight, BookOpen, GitBranch, KeyRound, Layers, Network, RefreshCw, Server, SlidersHorizontal } from 'lucide-react'
import { useInventory } from '../connections/useInventory'
import { filterRelationships, relationships, serverPath, type Relationship } from '../connections/model'
import ConnectionFlow from '../connections/ConnectionFlow'
import ConnectionDetails from '../connections/ConnectionDetails'
import { ServerPanel, SummaryTile } from '../connections/ServerPanels'
import { fmt } from '../format'
import { btn, input } from '../styles'
import { ErrorBox, Pill } from '../ui'

export default function Connections() {
  const inventory = useInventory()
  const [query, setQuery] = useState('')
  const [server, setServer] = useState('')
  const [kind,setKind] = useState('')
  const [limit, setLimit] = useState(50)
  const [selected, setSelected] = useState<Relationship | null>(null)
  const edges = useMemo(()=>relationships(inventory.servers,inventory.upstreams,inventory.keys,inventory.bindings),[inventory.servers,inventory.upstreams,inventory.keys,inventory.bindings])
  const shown = filterRelationships(edges,query,server).filter(edge=>!kind||edge.kind===kind)
  const sorting=useTableSort(shown,{Kind:r=>r.kind,From:r=>r.from,To:r=>r.to,State:r=>r.state})
  return <div className="page-layout">
    <header className="flex flex-wrap items-start justify-between gap-4"><div><h1 className="page-title">Connections</h1><p className="page-description mt-2 max-w-2xl text-sm leading-relaxed text-ink-muted">Configured access → virtual MCP → upstream MCP. These relationships are permissions, not active clients or live traffic.</p></div><div className="flex flex-wrap gap-2"><Link className={btn} to="/setup"><BookOpen aria-hidden="true" className="icon-inline"/>Connection setup guide</Link><Button loading={inventory.loading} className={btn} disabled={inventory.loading} onClick={()=>{setSelected(null);void inventory.load()}}><RefreshCw aria-hidden="true" className="icon-inline"/>Refresh connections</Button></div></header>
    {inventory.loading && <LoadingState label="Loading connections…" compact/>}
    {inventory.readError && <ErrorBox title="Configuration could not be loaded" onRetry={inventory.load} busy={inventory.loading}>{inventory.readError}. The diagram may be incomplete or contain data from an earlier refresh.</ErrorBox>}
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4"><SummaryTile label="Configured identities" value={inventory.keys.length+inventory.bindings.length+inventory.servers.filter(s=>!s.authRequired).length} detail="Agent keys, JWT bindings and anonymous routes" icon={KeyRound}/><SummaryTile label="Virtual servers" value={inventory.servers.length} detail="Saved client-facing capability compositions" icon={Layers}/><SummaryTile label="Upstream servers" value={inventory.upstreams.length} detail="Registered physical MCP connections" icon={Server}/><SummaryTile label="Relationships" value={edges.length} detail="All configured grants and upstream routes" icon={GitBranch}/></div>
    <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-ink-muted"><p className="inline-flex items-center gap-2"><Network aria-hidden="true" className="icon-inline"/>Explore the full configuration, then unfold servers and tools inside the Sankey.</p><p>Last received configuration: {fmt(inventory.checkedAt)}. Refresh to update.</p></div>
    <ConnectionFlow servers={inventory.servers} upstreams={inventory.upstreams} keys={inventory.keys} bindings={inventory.bindings} relationships={edges} onInspect={setSelected} loading={inventory.loading} error={inventory.readError}/>
    <ServerPanel title="Find relationships" description="These filters narrow the relationship index below. The Sankey retains its own drill-down scope." icon={SlidersHorizontal}>
      <div className="flex flex-wrap items-end gap-4 panel-body"><SearchField label="Filter relationships" value={query} onChange={value=>{setQuery(value);setLimit(50)}} placeholder="Name, capability, alias, policy or status" className="min-w-0 flex-[2_1_240px] text-xs font-medium" visibleLabel/>
        <label className="min-w-0 flex-[1_1_160px] text-xs font-medium">Virtual server<Select className={`${input} mt-2`} value={server} onChange={e=>{setServer(e.target.value);setLimit(50)}}><option value="">All servers</option>{inventory.servers.map(s=><option key={s.slug} value={s.slug}>{s.name}</option>)}</Select></label>
        <label className="min-w-0 flex-[1_1_160px] text-xs font-medium">Relationship kind<Select className={`${input} mt-2`} value={kind} onChange={e=>{setKind(e.target.value);setLimit(50)}}><option value="">All kinds</option><option value="agent-key">Agent key</option><option value="binding">JWT binding</option><option value="anonymous">Anonymous</option><option value="upstream">Upstream</option></Select></label>
      </div><ActiveFilters className="border-t border-line panel-inset py-3" label="Active relationship filters" resetLabel="Clear relationship filters" items={[filterItem('Search',query,()=>{setQuery('');setLimit(50)}),filterItem('Virtual server',inventory.servers.find(s=>s.slug===server)?.name??server,()=>{setServer('');setLimit(50)}),filterItem('Kind',kind,()=>{setKind('');setLimit(50)})]} onReset={()=>{setQuery('');setServer('');setKind('');setLimit(50)}}><ResultCount matched={shown.length} total={edges.length} noun="relationships" busy={inventory.loading}/></ActiveFilters>
    </ServerPanel>
    <ServerPanel title="Relationships" description="Inspect saved access grants, upstream routes and capability mappings" icon={GitBranch}>
      <p className="panel-inset py-4 text-xs font-medium text-ink-muted">Configured relationships ({shown.length})</p><DataTable caption="Configured relationships" columns={['Kind','From','To','State','Details']} sorting={sorting} primary={1} actions={4}>{sorting.rows.slice(0,limit).map(edge=><tr key={edge.id} className="border-t border-line hover:bg-canvas/50"><td className="panel-inset py-4"><TagBadge label={edge.kind}/></td><td className="break-all panel-inset py-4"><Link className="text-xs font-medium text-accent-700 hover:underline" to={edge.kind==='upstream'?serverPath(edge.from):edge.href}>{edge.from}</Link></td><td className="break-all panel-inset py-4"><Link className="text-xs font-medium text-accent-700 hover:underline" to={edge.kind==='upstream'?edge.href:serverPath(edge.to)}>{edge.to}</Link></td><td className="panel-inset py-4"><Pill status={edge.state}/></td><td className="panel-inset py-4"><button className={btn} aria-label={`Inspect ${edge.kind} ${edge.from} to ${edge.to}`} onClick={()=>setSelected(edge)}>Inspect relationship<ArrowRight aria-hidden="true" className="icon-inline"/></button></td></tr>)}</DataTable>
      {!inventory.loading && shown.length===0 && <EmptyState title={inventory.readError&&!edges.length?'Relationships unavailable':'No matching relationships.'} description={inventory.readError&&!edges.length?'The configuration read failed; retry before assuming there are no relationships.':'Follow the setup guide or adjust filters.'} actions={query||server||kind?<button className={btn} onClick={()=>{setQuery('');setServer('');setKind('');setLimit(50)}}>Reset relationship search</button>:!inventory.readError?<Link className={btn} to="/setup">Follow the setup guide</Link>:undefined}/>}
      <LoadMore shown={Math.min(limit,shown.length)} total={shown.length} noun="relationships" label="Show 50 more relationships" onLoad={()=>setLimit(n=>n+50)}/>
    </ServerPanel>
    {selected && <ConnectionDetails relationship={selected} upstream={inventory.upstreams.find(u=>u.slug===selected.to)} onClose={()=>setSelected(null)}/>}
  </div>
}
