import Button from '../Button'
import { StatusBadge } from '../Badge'
import { filterItem } from '../search-filters'
import { useTableSort } from '../table-sorting'
import { useNotifications } from '../notifications'
import { LoadingState } from '../Feedback'
import { useConfirm } from '../confirmation'
import { Link, useNavigate } from 'react-router-dom'
import { useCapabilities } from '../interaction'
import { ErrorBox, Pill } from '../ui'
import { useOperation } from '../interaction'
import { useSources, source } from '../requests'
import { useEffect, useMemo, useState } from 'react'
import { ArrowUpRight, Cable, Database, Plus, RefreshCw, Server, ShieldCheck, Trash2, Wrench } from 'lucide-react'
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
  if(u.kind==='rest')return 'ready'
  return u.lastRefreshError ? 'failed' : u.lastRefreshAt ? 'ready' : 'pending'
}

function DiscoveryStatus({u}:{u:Upstream}) {
  if(u.kind==='rest')return <StatusBadge status="enabled" label={u.rest?.mock?"Mock operations":"Configured operations"}/>
  const state=discoveryOf(u)
  return <StatusBadge status={state==='failed'?'refresh failed':state==='ready'?'snapshot available':'not refreshed'} label={state==='failed'?'Refresh failed':state==='ready'?'Snapshot available':'Not refreshed'}/>
}

const errMsg = (e: unknown) => (e instanceof ApiError ? e.message : String(e))

export default function Upstreams() {
  const {notify}=useNotifications()
  const confirm=useConfirm()
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
          (!status || (u.enabled?'enabled':'disabled') === status) && (!transport || (u.rest?.mock?'mock':u.kind==='rest'?'rest':u.transport)===transport) && (!discovery || discoveryOf(u)===discovery),
      ),
    [items, query, status, transport,discovery],
  )

  const sorting=useTableSort(shown,{Name:u=>u.name,Identifier:u=>u.slug,Tools:u=>u.toolCount,Status:u=>u.enabled?'enabled':'disabled',Discovery:u=>u.lastRefreshAt?Date.parse(u.lastRefreshAt):null})
  const currentPage=Math.min(page,Math.max(0,Math.ceil(shown.length/serverPageSize)-1))
  const reset=()=>{setQuery('');setStatus('');setTransport('');setDiscovery('');setPage(0)}
  const { busy, run } = useOperation()
  const refresh = (u: Upstream) => run(async () => {
    try {
      setError('');setOrphanedWarning('')
      const res = await api.refreshTools(u.slug)
      notify({title:'Discovery refreshed',description:u.name})
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
    if (!await confirm(`Delete upstream "${u.slug}"?`)) return
    setError('')
    try {
      await api.deleteUpstream(u.slug,false,u.version)
      notify({title:'Upstream deleted',description:u.name})
    } catch (e) {
      if (e instanceof ApiError && e.code === 'in_use') {
        if (await confirm({title:'Delete upstream and remove its selections?',description:e.message,confirmLabel:'Delete anyway',tone:'danger'}))
          await api.deleteUpstream(u.slug, true,u.version).then(()=>notify({title:'Upstream deleted',description:u.name})).catch((x) => setError(errMsg(x)))
      } else setError(errMsg(e))
    }
    void load()
  })

  return <div className="page-layout">
    <header className="flex flex-wrap items-start justify-between gap-4"><div><h1 className="page-title">Upstreams</h1><p className="page-description mt-2 max-w-2xl text-sm leading-relaxed text-ink-muted">Register MCP servers and REST APIs, then review saved connections and available tools.</p></div><div className="flex flex-wrap gap-2"><Button loading={loading} className={btn} disabled={loading||busy} onClick={()=>{setError('');setOrphanedWarning('');void load()}}><RefreshCw aria-hidden="true" className="icon-inline"/>Refresh list</Button><button className={primary} disabled={!canEdit} onClick={()=>navigate('/upstreams/new')}><Plus aria-hidden="true" className="icon-control"/>Add upstream</button></div></header>
    {loading&&items.length>0&&<LoadingState label="Refreshing upstream inventory…" compact/>}{readError&&<ErrorBox title="Saved configuration could not be loaded" onRetry={load} busy={loading}>{readError}. Showing any last loaded configuration.</ErrorBox>}{error&&<ErrorBox>{error}</ErrorBox>}{orphanedWarning&&<div role="status" className="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-xs leading-relaxed text-amber-800">{orphanedWarning}</div>}
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4"><SummaryTile label="Upstream servers" value={items.length} detail="Saved MCP and REST connections" icon={Server}/><SummaryTile label="Enabled servers" value={items.filter(u=>u.enabled).length} detail="Enabled in the saved configuration" icon={Cable}/><SummaryTile label="Discovery snapshots" value={items.filter(u=>discoveryOf(u)==='ready').length} detail="Stored snapshots without refresh errors" icon={Database}/><SummaryTile label="Discovered tools" value={items.reduce((sum,u)=>sum+u.toolCount,0)} detail="Tool definitions in stored discovery" icon={Wrench}/></div>
    <ServerListFilters title="Find upstreams" searchLabel="Search upstreams" query={query} onQuery={value=>{setQuery(value);setPage(0)}} activeFilters={[filterItem('Status',status,()=>{setStatus('');setPage(0)}),filterItem('Transport',transport==='streamable_http'?'Streamable HTTP':transport.toUpperCase(),()=>{setTransport('');setPage(0)}),filterItem('Discovery',({ready:'Snapshot available',failed:'Refresh failed',pending:'Not refreshed'} as Record<string,string>)[discovery]??'',()=>{setDiscovery('');setPage(0)})]} onReset={reset}>
      <ServerListSelect label="Upstream status" value={status} onChange={value=>{setStatus(value);setPage(0)}}><option value="">All statuses</option><option value="enabled">Enabled</option><option value="disabled">Disabled</option></ServerListSelect>
      <ServerListSelect label="Upstream transport" value={transport} onChange={value=>{setTransport(value);setPage(0)}}><option value="">All transports</option><option value="streamable_http">Streamable HTTP</option><option value="sse">SSE</option><option value="rest">REST API</option><option value="mock">Mock / WireMock</option></ServerListSelect>
      <ServerListSelect label="Upstream discovery" value={discovery} onChange={value=>{setDiscovery(value);setPage(0)}}><option value="">All discovery states</option><option value="ready">Snapshot available</option><option value="failed">Refresh failed</option><option value="pending">Not refreshed</option></ServerListSelect>
    </ServerListFilters>
    <ServerListResults sorting={sorting} title="Configured upstreams" total={items.length} matched={shown.length} page={currentPage} onPage={setPage} onClear={reset} emptyAction={canEdit?<button className={btn} onClick={()=>navigate('/upstreams/new')}>Create an upstream</button>:undefined} loading={loading} readError={readError} empty={items.length?'No upstreams match your filters.':'No upstreams yet. Add an MCP endpoint, test its stored credentials, then refresh discovery.'}>
      {sorting.rows.slice(currentPage*serverPageSize,(currentPage+1)*serverPageSize).map(u=><article key={u.slug} aria-label={u.name} className="min-w-0 flex flex-col overflow-hidden surface-card surface-interactive">
        <div className="flex items-start gap-3 panel-body"><div className="grid size-10 shrink-0 place-items-center rounded-xl bg-accent-50 text-accent-700"><Server aria-hidden="true" className="icon-feature"/></div><div className="min-w-0 flex-1"><h2 className="panel-title break-words"><Link className="text-accent-700 hover:underline" to={`/upstreams/${encodeURIComponent(u.slug)}`}>{u.name}</Link></h2><p className="meta-copy mt-1 break-all font-mono text-xs text-ink-muted">{u.slug}</p></div><Pill status={u.enabled?'enabled':'disabled'}/></div>
        <div className="flex-1 panel-inset pb-5"><p className="line-clamp-2 min-h-10 text-xs leading-relaxed text-ink-muted">{u.description||'Upstream service supplying capabilities to virtual servers.'}</p><div className="mt-4 surface-inset p-3"><p className="text-xs font-medium">Connection endpoint</p><p className="mt-1 break-all font-mono text-xs leading-relaxed text-ink-muted">{u.url}</p></div><dl className="mt-2 divide-y divide-line"><DetailRow label="Transport">{u.rest?.mock?'Mock / WireMock':u.kind==='rest'?'REST API':u.transport==='sse'?'SSE':'Streamable HTTP'}</DetailRow><DetailRow label="Discovered tools">{u.toolCount}</DetailRow><DetailRow label="Last discovery refresh"><span tabIndex={u.lastRefreshAt?0:undefined} data-tooltip={u.lastRefreshAt??undefined}>{ago(u.lastRefreshAt)}</span></DetailRow><DetailRow label="Recorded health"><Pill status={u.health?.state??'unknown'}/></DetailRow></dl><div className="mt-2 flex flex-wrap items-center justify-between gap-2"><DiscoveryStatus u={u}/><span className="inline-flex items-center gap-1.5 text-xs text-ink-muted"><ShieldCheck aria-hidden="true" className="icon-inline"/>{u.kind==='rest'?u.rest?.auth.type:u.auth?.type==='oauth_client_credentials'?'OAuth':u.headerNames.length?'Configured headers':'No auth headers'}</span></div>{u.lastRefreshError&&<p className="mt-3 line-clamp-2 text-xs leading-relaxed text-red-700">{u.lastRefreshError}</p>}</div>
        <footer className="flex flex-wrap items-center justify-between gap-3 border-t border-line panel-footer"><span className="text-xs text-ink-muted">Saved version {u.version}</span><div className="flex items-center gap-2"><Link className={btn} aria-label={`View ${u.name}`} to={`/upstreams/${encodeURIComponent(u.slug)}`}>View server<ArrowUpRight aria-hidden="true" className="icon-inline"/></Link><button disabled={!canEdit||busy} className={`${btn} action-icon`} hidden={u.kind==='rest'} data-tooltip="Refresh capabilities" aria-label={`Refresh capabilities for ${u.name}`} onClick={()=>void refresh(u)}><RefreshCw aria-hidden="true" className="icon-inline"/></button><button disabled={!canEdit||busy} className={`${btn} action-danger-outline action-icon`} data-tooltip="Delete" aria-label={`Delete ${u.name}`} onClick={()=>void remove(u)}><Trash2 aria-hidden="true" className="icon-inline text-red-600"/></button></div></footer>
      </article>)}
    </ServerListResults>
    <p className="text-xs leading-relaxed text-ink-muted">Health and discovery describe recorded observations. Reloading this list does not test upstream connectivity or refresh discovery.</p>
  </div>
}
