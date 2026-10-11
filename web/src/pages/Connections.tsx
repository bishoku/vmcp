import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useInventory } from '../connections/useInventory'
import { capabilityCounts, filterRelationships, relationships, serverPath, type Relationship } from '../connections/model'
import CapabilityTable from '../connections/CapabilityTable'
import Health from '../connections/Health'
import { fmt } from '../format'
import { btn, input } from '../styles'
import { ErrorBox, Modal } from '../ui'

export default function Connections() {
  const inventory = useInventory()
  const [query, setQuery] = useState('')
  const [server, setServer] = useState('')
  const [limit, setLimit] = useState(50)
  const [selected, setSelected] = useState<Relationship | null>(null)
  const edges = useMemo(()=>relationships(inventory.servers,inventory.upstreams,inventory.keys,inventory.bindings),[inventory.servers,inventory.upstreams,inventory.keys,inventory.bindings])
  const shown = filterRelationships(edges,query,server)
  const focus = inventory.servers.find(s=>s.slug===server) ?? inventory.servers.find(s=>shown.some(edge=>edge.server===s.slug))
  const incoming = shown.filter(edge=>edge.server===focus?.slug && edge.kind!=='upstream')
  const outgoing = shown.filter(edge=>edge.server===focus?.slug && edge.kind==='upstream')
  const edgeButton = (edge:Relationship) => <button key={edge.id} className={`${btn} w-full justify-start text-left`} onClick={()=>setSelected(edge)}>
    <span><span className="block font-semibold">{edge.kind==='upstream'?edge.to:edge.from}</span><span className="block text-xs">{edge.kind} · {edge.state}</span>{edge.capabilities.length>0 && <span className="block text-xs">{edge.capabilities.length} selected capabilities · {edge.capabilities.filter(c=>c.missing).length} missing</span>}</span>
  </button>
  return <div className="mx-auto max-w-7xl space-y-6 px-4 py-6 sm:px-6 lg:px-10 lg:py-10">
    <div><h1 className="text-2xl font-semibold">Connections</h1><p className="mt-2 text-sm text-ink-muted">Configured access → virtual MCP → upstream MCP. These relationships are permissions, not active clients or live traffic.</p></div>
    <div className="flex flex-wrap items-center gap-3"><Link className={btn} to="/setup">Connection setup guide</Link><button className={btn} disabled={inventory.loading} onClick={()=>{setSelected(null);void inventory.load()}}>Refresh connections</button><span className="text-sm text-ink-muted">Last received configuration: {fmt(inventory.checkedAt)}. Refresh to update.</span></div>
    {inventory.loading && <p role="status">Loading connections…</p>}
    {inventory.readError && <ErrorBox>{inventory.readError}. The diagram may be incomplete or contain data from an earlier refresh.</ErrorBox>}
    <div className="flex gap-3"><label className="flex-1 text-sm">Filter relationships<input className={input} value={query} onChange={e=>{setQuery(e.target.value);setLimit(50)}} placeholder="Name, capability, alias, policy or status"/></label>
      <label className="text-sm">Virtual server<select className={input} value={server} onChange={e=>{setServer(e.target.value);setLimit(50)}}><option value="">All servers</option>{inventory.servers.map(s=><option key={s.slug} value={s.slug}>{s.name}</option>)}</select></label></div>
    {focus && <section aria-label="Connection flow" className="rounded-card border border-line bg-surface p-6">
      <div className="grid grid-cols-1 xl:grid-cols-[1fr_auto_1fr_auto_1fr] items-center gap-4">
        <div className="space-y-2"><h2 className="font-semibold">Configured identities</h2>{incoming.slice(0,8).map(edgeButton)}{incoming.length===0 && <p className="text-sm">No matching grants. Catalogs may still be loading.</p>}</div>
        <span aria-hidden="true" className="text-2xl text-accent-700">→</span>
        <Link className="rounded-xl border-2 border-accent-500 bg-accent-50 p-5" to={serverPath(focus.slug)}><h2 className="font-semibold">{focus.name}</h2><p className="text-sm">Virtual MCP · {focus.enabled?'enabled':'disabled'}</p><p className="mt-2 text-xs">{capabilityCounts(focus)}</p></Link>
        <span aria-hidden="true" className="text-2xl text-accent-700">→</span>
        <div className="space-y-2"><h2 className="font-semibold">Upstream MCP servers</h2>{outgoing.slice(0,8).map(edgeButton)}{outgoing.length===0 && <p className="text-sm">No matching selected upstreams.</p>}</div>
      </div>
      <p className="mt-4 text-xs text-ink-muted">Focused on {focus.slug}. Select a virtual server above to change the flow. Up to 8 relationships per side; all matches are available in the table.</p>
    </section>}
    <section aria-label="Relationships" className="overflow-x-auto rounded-card border border-line bg-surface p-4">
      <table className="w-full text-left text-sm"><caption className="pb-3 text-left font-semibold">Configured relationships ({shown.length})</caption><thead><tr>{['Kind','From','To','State','Details'].map(t=><th className="p-2" key={t}>{t}</th>)}</tr></thead>
        <tbody>{shown.slice(0,limit).map(edge=><tr key={edge.id} className="border-t border-line"><td className="p-2">{edge.kind}</td><td className="p-2 break-all"><Link className="text-accent-700 underline" to={edge.kind==='upstream'?serverPath(edge.from):edge.href}>{edge.from}</Link></td><td className="p-2 break-all"><Link className="text-accent-700 underline" to={edge.kind==='upstream'?edge.href:serverPath(edge.to)}>{edge.to}</Link></td><td className="p-2">{edge.state}</td><td className="p-2"><button className={btn} aria-label={`Inspect ${edge.kind} ${edge.from} to ${edge.to}`} onClick={()=>setSelected(edge)}>Inspect relationship</button></td></tr>)}</tbody>
      </table>{!inventory.loading && shown.length===0 && <p className="p-6">No matching relationships. <Link className="underline" to="/setup">Follow the setup guide</Link> or adjust filters.</p>}
      {shown.length>limit && <button className={btn} onClick={()=>setLimit(n=>n+50)}>Show 50 more relationships</button>}
    </section>
    {selected && <Modal title={`${selected.from} → ${selected.to}`} onClose={()=>setSelected(null)}><p>{selected.kind} · {selected.state}</p><p className="mt-3 break-words text-sm">{selected.detail}</p>
      {selected.kind==='upstream' && <><CapabilityTable objects={selected.capabilities}/>{inventory.upstreams.filter(u=>u.slug===selected.to).map(u=><Health key={u.slug} upstream={u}/>)}</>}
      <div className="mt-4 flex gap-3"><Link className={btn} to={selected.href}>Open configuration</Link><Link className={btn} to={serverPath(selected.server,'activity')}>Recent activity</Link></div>
    </Modal>}
  </div>
}
