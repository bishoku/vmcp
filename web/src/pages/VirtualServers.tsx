import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Plus, Search, Trash2 } from 'lucide-react'
import { api, type VirtualServer } from '../api'
import { useCapabilities, useOperation } from '../interaction'
import { source, useSources } from '../requests'
import { serverPath, capabilityCounts } from '../connections/model'
import { ErrorBox, Pill } from '../ui'
import { btn, input, primary } from '../styles'

export default function VirtualServers() {
  const navigate = useNavigate()
  const { edit } = useCapabilities()
  const { busy, run } = useOperation()
  const [servers, setServers] = useState<VirtualServer[]>([])
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const { load, loading, readError } = useSources(() => [source('Virtual servers', api.listVirtualServers, setServers)])
  useEffect(() => { void load() }, [load])
  const shown = useMemo(() => servers.filter(server => `${server.name} ${server.slug} ${server.description}`.toLowerCase().includes(query.toLowerCase()) && (!status || (server.enabled ? 'enabled' : 'disabled') === status)), [servers, query, status])
  const remove = (server: VirtualServer) => run(async () => {
    if (!confirm(`Delete virtual server "${server.slug}"?`)) return
    try { await api.deleteVirtualServer(server.slug, server.version); await load() }
    catch (e) { setError(e instanceof Error ? e.message : String(e)) }
  })
  return <div className="mx-auto max-w-6xl px-4 py-6 sm:px-6 lg:px-10 lg:py-10">
    <header className="flex flex-wrap items-start justify-between gap-3"><div><h1 className="text-2xl font-semibold tracking-tight">Virtual Servers</h1><p className="mt-1 text-sm text-ink-muted">Compose capabilities into agent-specific endpoints. Select a server to view its configuration.</p></div>
      <div className="flex gap-2"><button className={btn} disabled={loading || busy} onClick={() => {setError(''); void load()}}>Refresh list</button>{edit ? <Link className={`${primary} inline-flex items-center gap-1.5`} to="/virtual-servers/new"><Plus className="size-4"/>New virtual server</Link> : <button className={primary} disabled>New virtual server</button>}</div></header>
    {loading && <p role="status" className="mt-4">Loading…</p>}{(readError || error) && <ErrorBox>{readError || error}</ErrorBox>}
    <div className="mt-6 rounded-card border border-line bg-surface shadow-card">
      <div className="flex flex-wrap justify-between gap-3 border-b border-line p-4"><div className="relative w-full max-w-xs"><Search className="absolute left-3 top-2.5 size-4 text-ink-muted"/><input className={`${input} pl-9`} aria-label="Search virtual servers" placeholder="Search" value={query} onChange={e => setQuery(e.target.value)}/></div><select className={`${input} w-44`} aria-label="Virtual server status" value={status} onChange={e=>setStatus(e.target.value)}><option value="">All statuses</option><option value="enabled">Enabled</option><option value="disabled">Disabled</option></select></div>
      <div className="overflow-x-auto"><table className="w-full min-w-[760px] text-left text-sm"><thead className="bg-canvas text-xs text-ink-muted"><tr>{['Name','Capabilities','Authentication','Rate limit','Status','Actions'].map(label=><th scope="col" className="px-4 py-3" key={label}>{label}</th>)}</tr></thead><tbody>
        {shown.map(server=><tr key={server.slug} className="cursor-pointer border-t border-line hover:bg-canvas" onClick={()=>navigate(serverPath(server.slug))}><td className="px-4 py-3"><Link className="font-medium text-accent-700 hover:underline" to={serverPath(server.slug)} onClick={e=>e.stopPropagation()}>{server.name}</Link><p className="text-xs text-ink-muted">{server.slug}</p></td><td className="px-4 py-3">{capabilityCounts(server)}</td><td className="px-4 py-3">{server.authRequired ? 'Required' : 'Anonymous'}</td><td className="px-4 py-3">{server.rateLimitPerMinute == null ? 'Unlimited' : `${server.rateLimitPerMinute}/min`}</td><td className="px-4 py-3"><Pill status={server.enabled?'enabled':'disabled'}/></td><td className="px-4 py-3"><button className={btn} disabled={!edit || busy} aria-label={`Delete ${server.name}`} onClick={e=>{e.stopPropagation();void remove(server)}}><Trash2 className="size-4 text-red-600"/></button></td></tr>)}
        {!loading && !readError && !shown.length && <tr><td colSpan={6} className="px-4 py-10 text-center text-ink-muted">{servers.length ? 'No virtual servers match your filters.' : 'No virtual servers yet. Register and refresh an upstream, then create a server to select its capabilities.'}</td></tr>}
      </tbody></table></div>
    </div>
  </div>
}
