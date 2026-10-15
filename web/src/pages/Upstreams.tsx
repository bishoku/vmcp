import { Link, useNavigate } from 'react-router-dom'
import { useCapabilities } from '../interaction'
import { ErrorBox, Pill } from '../ui'
import { useOperation } from '../interaction'
import { useSources, source } from '../requests'
import { useEffect, useMemo, useState } from 'react'
import { ArrowUpRight, Cable, Database, Plus, RefreshCw, Server, ShieldCheck, Trash2, Wrench } from 'lucide-react'
import clsx from 'clsx'
import { api, ApiError, type Upstream } from '../api'

import { btn, primary } from '../styles'

import { DetailRow, SummaryTile } from '../connections/ServerPanels'
import { ServerListFilters, ServerListResults, ServerListSelect, serverPageSize } from '../connections/ServerList'

function ago(iso: string | null) {
  if (!iso) return 'never'
  const s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000)
  if (s < 60) return 'just now'
  if (s < 3600) return `${Math.floor(s / 60)} min ago`
  if (s < 86400) return `${Math.floor(s / 3600)} hours ago`
  return `${Math.floor(s / 86400)} days ago`
}

function discoveryOf(u: Upstream) {
  return u.lastRefreshError ? 'failed' : u.lastRefreshAt ? 'ready' : 'pending'
}

function DiscoveryStatus({u}:{u:Upstream}) {
  const state=discoveryOf(u)
  return <span className={clsx('rounded-full px-2 py-1 text-xs font-medium',state==='failed'?'bg-red-50 text-red-700':state==='ready'?'bg-accent-50 text-accent-700':'bg-amber-50 text-amber-700')}>{state==='failed'?'Refresh failed':state==='ready'?'Snapshot available':'Not refreshed'}</span>
}

const errMsg = (e: unknown) => (e instanceof ApiError ? e.message : String(e))

export default function Upstreams() {
  const navigate = useNavigate()
  const { edit: canEdit } = useCapabilities()
  const [items, setItems] = useState<Upstream[]>([])
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('')
  const [transport,setTransport] = useState('')
  const [discovery,setDiscovery] = useState('')
  const [page,setPage] = useState(0)
  const [error, setError] = useState('')
  const [orphanedWarning, setOrphanedWarning] = useState('')

  const { load, loading, readError } = useSources(() => [source('Upstreams', api.listUpstreams, setItems)])
  useEffect(() => {
    const timer = setTimeout(() => { void load() }, 0)
    return () => clearTimeout(timer)
  }, [load])

  const shown = useMemo(
    () =>
      items.filter(
        (u) =>
          ` ${u.slug} ${u.name} ${u.url} ${u.description}`.toLowerCase().includes(query.trim().toLowerCase()) &&
          (!status || (u.enabled?'enabled':'disabled') === status) && (!transport || u.transport===transport) && (!discovery || discoveryOf(u)===discovery),
      ),
    [items, query, status, transport,discovery],
  )

  const currentPage=Math.min(page,Math.max(0,Math.ceil(shown.length/serverPageSize)-1))
  const reset=()=>{setQuery('');setStatus('');setTransport('');setDiscovery('');setPage(0)}
  const { busy, run } = useOperation()
  const refresh = (u: Upstream) => run(async () => {
    try {
      setError('');setOrphanedWarning('')
      const res = await api.refreshTools(u.slug)
      if (res.orphanedSelections && res.orphanedSelections.length > 0) {
        const list = res.orphanedSelections.map((o) => `${o.virtualServer}:${o.tool}`).join(', ')
        setOrphanedWarning(`Upstream "${u.slug}" removed tools still selected by virtual servers: ${list}`)
      }
    } catch (e) {
      setError(errMsg(e))
    }
    void load()
  })
  const remove = (u: Upstream) => run(async () => {
    if (!confirm(`Delete upstream "${u.slug}"?`)) return
    setError('')
    try {
      await api.deleteUpstream(u.slug,false,u.version)
    } catch (e) {
      if (e instanceof ApiError && e.code === 'in_use') {
        if (confirm(`${e.message}\n\nDelete anyway and remove it from those virtual servers?`))
          await api.deleteUpstream(u.slug, true,u.version).catch((x) => setError(errMsg(x)))
      } else setError(errMsg(e))
    }
    void load()
  })

  return <div className="mx-auto max-w-7xl space-y-6 px-4 py-6 sm:px-6 lg:px-10 lg:py-10">
    <header className="flex flex-wrap items-start justify-between gap-4"><div><h1 className="text-2xl font-semibold tracking-tight">Upstreams</h1><p className="mt-2 max-w-2xl text-sm leading-relaxed text-ink-muted">Register physical MCP servers and review saved connection settings, discovery snapshots and recorded health.</p></div><div className="flex flex-wrap gap-2"><button className={btn} disabled={loading||busy} onClick={()=>{setError('');setOrphanedWarning('');void load()}}><RefreshCw className="size-3.5"/>Refresh list</button><button className={primary} disabled={!canEdit} onClick={()=>navigate('/upstreams/new')}><Plus className="size-4"/>Add upstream</button></div></header>
    {loading&&<p role="status" className="text-xs text-ink-muted">Loading…</p>}{readError&&<ErrorBox>{readError}. Showing any last loaded configuration.</ErrorBox>}{error&&<ErrorBox>{error}</ErrorBox>}{orphanedWarning&&<div role="status" className="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-xs leading-relaxed text-amber-800">{orphanedWarning}</div>}
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4"><SummaryTile label="Upstream servers" value={items.length} detail="Saved physical MCP connections" icon={Server}/><SummaryTile label="Enabled servers" value={items.filter(u=>u.enabled).length} detail="Enabled in the saved configuration" icon={Cable}/><SummaryTile label="Discovery snapshots" value={items.filter(u=>discoveryOf(u)==='ready').length} detail="Stored snapshots without refresh errors" icon={Database}/><SummaryTile label="Discovered tools" value={items.reduce((sum,u)=>sum+u.toolCount,0)} detail="Tool definitions in stored discovery" icon={Wrench}/></div>
    <ServerListFilters title="Find upstreams" searchLabel="Search upstreams" query={query} onQuery={value=>{setQuery(value);setPage(0)}} filtered={!!(query||status||transport||discovery)} onReset={reset}>
      <ServerListSelect label="Upstream status" value={status} onChange={value=>{setStatus(value);setPage(0)}}><option value="">All statuses</option><option value="enabled">Enabled</option><option value="disabled">Disabled</option></ServerListSelect>
      <ServerListSelect label="Upstream transport" value={transport} onChange={value=>{setTransport(value);setPage(0)}}><option value="">All transports</option><option value="streamable_http">Streamable HTTP</option><option value="sse">SSE</option></ServerListSelect>
      <ServerListSelect label="Upstream discovery" value={discovery} onChange={value=>{setDiscovery(value);setPage(0)}}><option value="">All discovery states</option><option value="ready">Snapshot available</option><option value="failed">Refresh failed</option><option value="pending">Not refreshed</option></ServerListSelect>
    </ServerListFilters>
    <ServerListResults title="Configured upstreams" total={items.length} matched={shown.length} page={currentPage} onPage={setPage} loading={loading} readError={readError} empty={items.length?'No upstreams match your filters.':'No upstreams yet. Add an MCP endpoint, test its stored credentials, then refresh discovery.'}>
      {shown.slice(currentPage*serverPageSize,(currentPage+1)*serverPageSize).map(u=><article key={u.slug} aria-label={u.name} className="min-w-0 flex flex-col overflow-hidden rounded-card border border-line bg-surface shadow-card transition-shadow hover:shadow-md">
        <div className="flex items-start gap-3 p-5"><div className="grid size-10 shrink-0 place-items-center rounded-xl bg-accent-50 text-accent-700"><Server className="size-5"/></div><div className="min-w-0 flex-1"><h2 className="break-words text-sm font-semibold"><Link className="text-accent-700 hover:underline" to={`/upstreams/${encodeURIComponent(u.slug)}`}>{u.name}</Link></h2><p className="mt-1 break-all font-mono text-xs text-ink-muted">{u.slug}</p></div><Pill status={u.enabled?'enabled':'disabled'}/></div>
        <div className="flex-1 px-5 pb-5"><p className="line-clamp-2 min-h-10 text-xs leading-relaxed text-ink-muted">{u.description||'Physical MCP connection supplying capabilities to virtual servers.'}</p><div className="mt-4 rounded-lg border border-line bg-canvas p-3"><p className="text-xs font-medium">Connection endpoint</p><p className="mt-1 break-all font-mono text-xs leading-relaxed text-ink-muted">{u.url}</p></div><dl className="mt-2 divide-y divide-line"><DetailRow label="Transport">{u.transport==='sse'?'SSE':'Streamable HTTP'}</DetailRow><DetailRow label="Discovered tools">{u.toolCount}</DetailRow><DetailRow label="Last discovery refresh"><span title={u.lastRefreshAt??undefined}>{ago(u.lastRefreshAt)}</span></DetailRow><DetailRow label="Recorded health"><Pill status={u.health?.state??'unknown'}/></DetailRow></dl><div className="mt-2 flex flex-wrap items-center justify-between gap-2"><DiscoveryStatus u={u}/><span className="inline-flex items-center gap-1.5 text-xs text-ink-muted"><ShieldCheck className="size-3.5"/>{u.auth?.type==='oauth_client_credentials'?'OAuth':u.headerNames.length?'Configured headers':'No auth headers'}</span></div>{u.lastRefreshError&&<p className="mt-3 line-clamp-2 text-xs leading-relaxed text-red-700">{u.lastRefreshError}</p>}</div>
        <footer className="flex flex-wrap items-center justify-between gap-3 border-t border-line px-5 py-3"><span className="text-xs text-ink-muted">Saved version {u.version}</span><div className="flex items-center gap-2"><Link className={btn} aria-label={`View ${u.name}`} to={`/upstreams/${encodeURIComponent(u.slug)}`}>View server<ArrowUpRight className="size-3.5"/></Link><button disabled={!canEdit||busy} className={btn} title="Refresh capabilities" aria-label={`Refresh capabilities for ${u.name}`} onClick={()=>void refresh(u)}><RefreshCw className="size-3.5"/></button><button disabled={!canEdit||busy} className={btn} title="Delete" aria-label={`Delete ${u.name}`} onClick={()=>void remove(u)}><Trash2 className="size-3.5 text-red-600"/></button></div></footer>
      </article>)}
    </ServerListResults>
    <p className="text-xs leading-relaxed text-ink-muted">Health and discovery describe recorded observations. Reloading this list does not test upstream connectivity or refresh discovery.</p>
  </div>
}
