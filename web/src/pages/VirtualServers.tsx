import { filterItem } from '../search-filters'
import { useTableSort } from '../table-sorting'
import { useNotifications } from '../notifications'
import { LoadingState } from '../Feedback'
import { useConfirm } from '../confirmation'
import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowUpRight, Cable, KeyRound, Layers, Plus, RefreshCw, ShieldCheck, Trash2, Wrench } from 'lucide-react'
import { api, type VirtualServer } from '../api'
import { useCapabilities, useOperation } from '../interaction'
import { source, useSources } from '../requests'
import { serverPath, capabilityCounts } from '../connections/model'
import { ErrorBox, Pill } from '../ui'
import { btn, primary } from '../styles'

import { DetailRow, SummaryTile } from '../connections/ServerPanels'
import { ServerListFilters, ServerListResults, ServerListSelect, serverPageSize } from '../connections/ServerList'

export default function VirtualServers() {
  const {notify}=useNotifications()
  const confirm=useConfirm()
  const { edit } = useCapabilities()
  const { busy, run } = useOperation()
  const [servers, setServers] = useState<VirtualServer[]>([])
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('')
  const [auth,setAuth] = useState('')
  const [page,setPage] = useState(0)
  const [error, setError] = useState('')
  const { load, loading, readError } = useSources(() => [source('Virtual servers', api.listVirtualServers, setServers)])
  useEffect(() => { void load() }, [load])
  const shown = useMemo(() => servers.filter(server => `${server.name} ${server.slug} ${server.description}`.toLowerCase().includes(query.trim().toLowerCase()) && (!status || (server.enabled ? 'enabled' : 'disabled') === status) && (!auth || (server.authRequired?'required':'anonymous')===auth)), [servers, query, status,auth])
  const sorting=useTableSort(shown,{Name:v=>v.name,Identifier:v=>v.slug,Tools:v=>v.tools.length,Status:v=>v.enabled?'enabled':'disabled'})
  const currentPage=Math.min(page,Math.max(0,Math.ceil(shown.length/serverPageSize)-1))
  const reset=()=>{setQuery('');setStatus('');setAuth('');setPage(0)}
  const remove = (server: VirtualServer) => run(async () => {
    if (!await confirm(`Delete virtual server "${server.slug}"?`)) return
    try { await api.deleteVirtualServer(server.slug, server.version); notify({title:'Virtual server deleted',description:server.name}); await load() }
    catch (e) { setError(e instanceof Error ? e.message : String(e)) }
  })
  return <div className="page-layout">
    <header className="flex flex-wrap items-start justify-between gap-4"><div><h1 className="page-title">Virtual Servers</h1><p className="page-description mt-2 max-w-2xl text-sm leading-relaxed text-ink-muted">Compose upstream capabilities into client endpoints. Review saved configuration, access requirements and selected tools.</p></div>
      <div className="flex flex-wrap gap-2"><button className={btn} disabled={loading || busy} onClick={() => {setError(''); void load()}}><RefreshCw aria-hidden="true" className="icon-inline"/>Refresh list</button>{edit ? <Link className={primary} to="/virtual-servers/new"><Plus aria-hidden="true" className="icon-control"/>New virtual server</Link> : <button className={primary} disabled>New virtual server</button>}</div></header>
    {loading&&servers.length>0&&<LoadingState label="Refreshing virtual servers…" compact/>}{readError && <ErrorBox title="Saved configuration could not be loaded" onRetry={load} busy={loading}>{readError}. Showing any last loaded configuration.</ErrorBox>}{error && <ErrorBox>{error}</ErrorBox>}
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4"><SummaryTile label="Virtual servers" value={servers.length} detail="Saved configurations in this inventory" icon={Layers}/><SummaryTile label="Enabled servers" value={servers.filter(s=>s.enabled).length} detail="Configured to accept client connections" icon={Cable}/><SummaryTile label="Authentication required" value={servers.filter(s=>s.authRequired).length} detail="Agent key or OAuth access token" icon={KeyRound}/><SummaryTile label="Selected tools" value={servers.reduce((sum,s)=>sum+s.tools.length,0)} detail="Configured selections across all servers" icon={Wrench}/></div>
    <ServerListFilters title="Find virtual servers" searchLabel="Search virtual servers" query={query} onQuery={value=>{setQuery(value);setPage(0)}} activeFilters={[filterItem('Status',status,()=>{setStatus('');setPage(0)}),filterItem('Authentication',auth==='required'?'Required':auth==='anonymous'?'Anonymous':'',()=>{setAuth('');setPage(0)})]} onReset={reset}>
      <ServerListSelect label="Virtual server status" value={status} onChange={value=>{setStatus(value);setPage(0)}}><option value="">All statuses</option><option value="enabled">Enabled</option><option value="disabled">Disabled</option></ServerListSelect>
      <ServerListSelect label="Virtual server authentication" value={auth} onChange={value=>{setAuth(value);setPage(0)}}><option value="">All authentication</option><option value="required">Required</option><option value="anonymous">Anonymous</option></ServerListSelect>
    </ServerListFilters>
    <ServerListResults sorting={sorting} title="Configured virtual servers" total={servers.length} matched={shown.length} page={currentPage} onPage={setPage} onClear={reset} emptyAction={edit?<Link className={btn} to="/virtual-servers/new">Create a virtual server</Link>:undefined} loading={loading} readError={readError} empty={servers.length?'No virtual servers match your filters.':'No virtual servers yet. Register and refresh an upstream, then create a server to select its capabilities.'}>
      {sorting.rows.slice(currentPage*serverPageSize,(currentPage+1)*serverPageSize).map(server=><article key={server.slug} aria-label={server.name} className="min-w-0 flex flex-col overflow-hidden surface-card surface-interactive">
        <div className="flex items-start gap-3 panel-body"><div className="grid size-10 shrink-0 place-items-center rounded-xl bg-accent-50 text-accent-700"><Layers aria-hidden="true" className="icon-feature"/></div><div className="min-w-0 flex-1"><h2 className="panel-title break-words"><Link className="text-accent-700 hover:underline" to={serverPath(server.slug)}>{server.name}</Link></h2><p className="meta-copy mt-1 break-all font-mono text-xs text-ink-muted">{server.slug}</p></div><Pill status={server.enabled?'enabled':'disabled'}/></div>
        <div className="flex-1 panel-inset pb-5"><p className="line-clamp-2 min-h-10 text-xs leading-relaxed text-ink-muted">{server.description || 'Virtual MCP endpoint with explicitly selected upstream capabilities.'}</p><div className="mt-4 surface-inset p-3"><p className="text-xs font-medium">Selected capabilities</p><p className="mt-1 text-xs leading-relaxed text-ink-muted">{capabilityCounts(server)}</p></div><dl className="mt-2 divide-y divide-line"><DetailRow label="Authentication">{server.authRequired?'Required':'Anonymous'}</DetailRow><DetailRow label="Upstream references">{new Set([...server.tools,...(server.catalog??[])].map(c=>c.upstream)).size}</DetailRow><DetailRow label="Rate limit">{server.rateLimitPerMinute == null ? 'Unlimited' : `${server.rateLimitPerMinute}/min`}</DetailRow></dl><div className="mt-2 flex items-start gap-2 surface-inset p-3 text-xs text-ink-muted"><ShieldCheck aria-hidden="true" className="icon-control shrink-0"/><span>{server.authRequired?'Access depends on configured grants and policy.':'Anonymous access bypasses credential authentication.'}</span></div></div>
        <footer className="flex flex-wrap items-center justify-between gap-3 border-t border-line panel-footer"><span className="text-xs text-ink-muted">Saved version {server.version}</span><div className="flex items-center gap-2"><Link className={btn} aria-label={`View ${server.name}`} to={serverPath(server.slug)}>View server<ArrowUpRight aria-hidden="true" className="icon-inline"/></Link><button className={`${btn} action-danger-outline action-icon`} disabled={!edit || busy} aria-label={`Delete ${server.name}`} onClick={()=>void remove(server)}><Trash2 aria-hidden="true" className="icon-inline text-red-600"/></button></div></footer>
      </article>)}
    </ServerListResults>
  </div>
}
