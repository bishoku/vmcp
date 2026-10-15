import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowRight, BookOpen, GitBranch, KeyRound, Layers, Network, RefreshCw, Search, Server, SlidersHorizontal, X } from 'lucide-react'
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
  return <div className="mx-auto max-w-7xl space-y-6 px-4 py-6 sm:px-6 lg:px-10 lg:py-10">
    <header className="flex flex-wrap items-start justify-between gap-4"><div><h1 className="text-2xl font-semibold tracking-tight">Connections</h1><p className="mt-2 max-w-2xl text-sm leading-relaxed text-ink-muted">Configured access → virtual MCP → upstream MCP. These relationships are permissions, not active clients or live traffic.</p></div><div className="flex flex-wrap gap-2"><Link className={btn} to="/setup"><BookOpen className="size-3.5"/>Connection setup guide</Link><button className={btn} disabled={inventory.loading} onClick={()=>{setSelected(null);void inventory.load()}}><RefreshCw className="size-3.5"/>Refresh connections</button></div></header>
    {inventory.loading && <p role="status" className="text-xs text-ink-muted">Loading connections…</p>}
    {inventory.readError && <ErrorBox>{inventory.readError}. The diagram may be incomplete or contain data from an earlier refresh.</ErrorBox>}
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4"><SummaryTile label="Configured identities" value={inventory.keys.length+inventory.bindings.length+inventory.servers.filter(s=>!s.authRequired).length} detail="Agent keys, JWT bindings and anonymous routes" icon={KeyRound}/><SummaryTile label="Virtual servers" value={inventory.servers.length} detail="Saved client-facing capability compositions" icon={Layers}/><SummaryTile label="Upstream servers" value={inventory.upstreams.length} detail="Registered physical MCP connections" icon={Server}/><SummaryTile label="Relationships" value={edges.length} detail="All configured grants and upstream routes" icon={GitBranch}/></div>
    <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-ink-muted"><p className="inline-flex items-center gap-2"><Network className="size-3.5"/>Explore the full configuration, then unfold servers and tools inside the Sankey.</p><p>Last received configuration: {fmt(inventory.checkedAt)}. Refresh to update.</p></div>
    <ConnectionFlow servers={inventory.servers} upstreams={inventory.upstreams} keys={inventory.keys} bindings={inventory.bindings} relationships={edges} onInspect={setSelected}/>
    <ServerPanel title="Find relationships" description="These filters narrow the relationship index below. The Sankey retains its own drill-down scope." icon={SlidersHorizontal}>
      <div className="flex flex-wrap items-end gap-4 p-5"><label className="min-w-0 flex-[2_1_240px] text-xs font-medium">Filter relationships<div className="relative mt-2"><Search className="absolute left-3 top-2.5 size-4 text-ink-muted"/><input className={`${input} pl-9`} value={query} onChange={e=>{setQuery(e.target.value);setLimit(50)}} placeholder="Name, capability, alias, policy or status"/></div></label>
        <label className="min-w-0 flex-[1_1_160px] text-xs font-medium">Virtual server<select className={`${input} mt-2`} value={server} onChange={e=>{setServer(e.target.value);setLimit(50)}}><option value="">All servers</option>{inventory.servers.map(s=><option key={s.slug} value={s.slug}>{s.name}</option>)}</select></label>
        <label className="min-w-0 flex-[1_1_160px] text-xs font-medium">Relationship kind<select className={`${input} mt-2`} value={kind} onChange={e=>{setKind(e.target.value);setLimit(50)}}><option value="">All kinds</option><option value="agent-key">Agent key</option><option value="binding">JWT binding</option><option value="anonymous">Anonymous</option><option value="upstream">Upstream</option></select></label>
        {(query||server||kind)&&<button className={btn} onClick={()=>{setQuery('');setServer('');setKind('');setLimit(50)}}><X className="size-3.5"/>Clear relationship filters</button>}
      </div>
    </ServerPanel>
    <ServerPanel title="Relationships" description="Inspect saved access grants, upstream routes and capability mappings" icon={GitBranch}>
      <div className="overflow-x-auto"><table className="w-full min-w-[720px] text-left text-sm"><caption className="px-5 py-4 text-left text-xs font-medium text-ink-muted">Configured relationships ({shown.length})</caption><thead className="bg-canvas text-xs text-ink-muted"><tr>{['Kind','From','To','State','Details'].map(t=><th scope="col" className="px-5 py-3 font-medium" key={t}>{t}</th>)}</tr></thead>
        <tbody>{shown.slice(0,limit).map(edge=><tr key={edge.id} className="border-t border-line hover:bg-canvas/50"><td className="px-5 py-4"><span className="whitespace-nowrap rounded-md bg-canvas px-2 py-1 text-xs font-medium">{edge.kind}</span></td><td className="break-all px-5 py-4"><Link className="text-xs font-medium text-accent-700 hover:underline" to={edge.kind==='upstream'?serverPath(edge.from):edge.href}>{edge.from}</Link></td><td className="break-all px-5 py-4"><Link className="text-xs font-medium text-accent-700 hover:underline" to={edge.kind==='upstream'?edge.href:serverPath(edge.to)}>{edge.to}</Link></td><td className="px-5 py-4"><Pill status={edge.state}/></td><td className="px-5 py-4"><button className={btn} aria-label={`Inspect ${edge.kind} ${edge.from} to ${edge.to}`} onClick={()=>setSelected(edge)}>Inspect relationship<ArrowRight className="size-3.5"/></button></td></tr>)}</tbody>
      </table></div>
      {!inventory.loading && shown.length===0 && <p className="px-5 py-10 text-center text-sm text-ink-muted">No matching relationships. <Link className="text-accent-700 underline" to="/setup">Follow the setup guide</Link> or adjust filters.</p>}
      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-line px-5 py-3"><p className="text-xs text-ink-muted">{Math.min(limit,shown.length)} of {shown.length} matching relationships · {edges.length} configured</p>{shown.length>limit && <button className={btn} onClick={()=>setLimit(n=>n+50)}>Show 50 more relationships</button>}</div>
    </ServerPanel>
    {selected && <ConnectionDetails relationship={selected} upstream={inventory.upstreams.find(u=>u.slug===selected.to)} onClose={()=>setSelected(null)}/>}
  </div>
}
