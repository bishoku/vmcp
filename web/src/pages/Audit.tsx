import { useCallback, useEffect, useRef, useState } from 'react'
import { api, type AuditEvent, type AuditQuery } from '../api'
import { ArrowUpRight, Clock3, FileClock, Layers3, ListFilter, RefreshCw, Search } from 'lucide-react'
import AuditFilters from '../connections/AuditFilters'
import AuditDetails from '../connections/AuditDetails'
import { ServerPanel, SummaryTile } from '../connections/ServerPanels'
import { btn } from '../styles'
import { fmt } from '../format'
import { ErrorBox, Pill } from '../ui'

export default function Audit({ initialServer, initialOperation, initialQuery }: { initialServer?: string; initialOperation?: string; initialQuery?: AuditQuery }) {
  const [filters, setFilters] = useState<AuditQuery>(()=>({...initialQuery, virtualServer:initialServer ?? initialQuery?.virtualServer, operationId:initialOperation ?? initialQuery?.operationId}))
  const [raw, setRaw] = useState(false)
  const [selected, setSelected] = useState<AuditEvent[] | null>(null)
  const [checked, setChecked] = useState('')
  const [items, setItems] = useState<AuditEvent[]>([])
  const [next, setNext] = useState<string | null>(null)
  const [error, setError] = useState('')

  const [loading, setLoading] = useState(false)
  const sequence = useRef(0)
  const controller = useRef<AbortController | null>(null)
  const pending = useRef(false)
  const load = useCallback(async (q: AuditQuery, append: boolean) => {
    if (append && pending.current) return
    controller.current?.abort()
    const abort = new AbortController()
    controller.current = abort
    const request = ++sequence.current
    pending.current = true
    setLoading(true)
    if (!append) setNext(null)
    try {
      const r = await api.listAudit(q, abort.signal)
      if (request !== sequence.current || abort.signal.aborted) return
      setItems(prev => append ? [...prev, ...r.items.filter(item => !prev.some(old => old.id === item.id))] : r.items)
      setChecked(new Date().toLocaleString())
      setNext(r.nextCursor ?? null)
      setError('')
    } catch (e) {
      if (request === sequence.current && !abort.signal.aborted) setError(e instanceof Error ? e.message : String(e))
    } finally {
      if (request === sequence.current) { pending.current = false; setLoading(false) }
    }
  }, [])
  const cancel = useCallback(() => { controller.current?.abort(); sequence.current++ }, [])
  useEffect(() => {
    const timer = setTimeout(() => { void load(filters, false) }, 150)
    return () => { clearTimeout(timer); cancel() }
  }, [filters, load, cancel])

  const set = (k: keyof AuditQuery, v: string) => {
    setItems([]); setNext(null); setChecked(''); setLoading(true)
    setFilters(f => ({...f, [k]:v || undefined}))
  }
  // datetime-local has no zone; interpret it in the browser's zone.
  const when = (k: 'from' | 'to', v: string) => set(k, v ? new Date(v).toISOString() : '')

  const groups = new Map<string, AuditEvent[]>()
  for (const event of items) {
    const key = raw ? `event:${event.id}` : event.operationId ? `operation:${event.operationId}` : `event:${event.id}`
    groups.set(key, [...(groups.get(key) ?? []), event])
  }
  const filterLink = `/audit?${new URLSearchParams(Object.entries(filters).filter((entry): entry is [string,string]=>typeof entry[1]==='string'))}`
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone
  const operationGroups=new Map<string,AuditEvent[]>()
  for(const event of items){const id=event.operationId?`operation:${event.operationId}`:`event:${event.id}`;operationGroups.set(id,[...(operationGroups.get(id) ?? []),event])}
  const missingGroups=[...operationGroups.values()].filter(events=>events.some(e=>e.stage==='admission') && !events.some(e=>e.stage==='outcome')).length
  const filterCount=Object.entries(filters).filter(([key,value])=>key!=='cursor' && value).length
  const reset=()=>{setItems([]);setNext(null);setChecked('');setLoading(true);setFilters({})}
  return <div className="mx-auto max-w-7xl space-y-6 px-4 py-6 sm:px-6 lg:px-10 lg:py-10">
    <header className="flex flex-wrap items-start justify-between gap-4"><div className="flex items-start gap-3"><span className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-accent-50 text-accent-700"><FileClock className="size-5"/></span><div><h1 className="text-2xl font-semibold tracking-tight">Audit Log</h1><p className="mt-1 text-sm text-ink-muted">Investigate tool calls and administrative operations from stored events.</p></div></div><button className={btn} disabled={loading} onClick={()=>void load(filters,false)}><RefreshCw className={`size-3.5 ${loading?'animate-spin':''}`}/>Refresh</button></header>
    <div className="grid grid-cols-2 gap-3 xl:grid-cols-4"><SummaryTile label="Loaded records" value={items.length} detail="Stored events in the current query" icon={FileClock}/><SummaryTile label="Displayed rows" value={groups.size} detail={raw?'Individual raw stored events':'Grouped by exact operation ID, where present'} icon={Layers3}/><SummaryTile label="Outcome not loaded" value={missingGroups} detail="Loaded groups with admission only · not failures" icon={Clock3}/><SummaryTile label="Last successful read" value={<span className="text-sm font-medium">{checked || 'Not loaded yet'}</span>} detail={error?'Latest query failed; displayed data may be older':loading?'Reading stored events…':'Manual refresh · no live subscription'} icon={RefreshCw}/></div>
    <AuditFilters filters={filters} set={set} when={when} reset={reset} link={new URL(filterLink,window.location.origin).href} zone={zone}/>
    {error && <ErrorBox>{error} Displayed events may be from the previous successful query.</ErrorBox>}
    <ServerPanel title="Stored activity" description={`${items.length} loaded records · ${filterCount} active filters`} icon={ListFilter} footer={<div className="flex flex-wrap items-center justify-between gap-3"><span>{groups.size} displayed rows from {items.length} loaded records{next?' · more records available':''}</span>{next && <button className={btn} disabled={loading} onClick={()=>load({...filters,cursor:next},true)}>Load more</button>}</div>}>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-4"><div className="inline-flex items-center gap-1 rounded-lg border border-line bg-canvas p-1"><button className={`rounded-md px-3 py-1.5 text-xs font-medium ${!raw?'bg-surface text-accent-700 shadow-sm':'text-ink-muted hover:text-ink'}`} aria-pressed={!raw} onClick={()=>setRaw(false)}>Operations</button><button className={`rounded-md px-3 py-1.5 text-xs font-medium ${raw?'bg-surface text-accent-700 shadow-sm':'text-ink-muted hover:text-ink'}`} aria-pressed={raw} onClick={()=>setRaw(true)}>Raw events</button></div><p className="text-xs text-ink-muted">{raw?'One row per stored event':'Operations group only loaded records'}</p></div>
      {loading && <p role="status" className="flex items-center gap-2 border-b border-line px-5 py-3 text-xs text-ink-muted"><RefreshCw className="size-3.5 animate-spin"/>Loading audit events…</p>}
      <div role="region" aria-label="Audit events" tabIndex={0} className="overflow-x-auto">
        {items.length>0?<table className="w-full min-w-[920px] text-left text-sm"><caption className="sr-only">{raw?'Raw audit events':'Loaded audit operations'}</caption><thead className="bg-canvas text-xs text-ink-muted"><tr>{['Time','Virtual server','Identity','Tool / action','Status','Redacted','Duration'].map(h=><th key={h} scope="col" className="px-4 py-3 font-medium">{h}</th>)}</tr></thead><tbody>{[...groups.values()].map(events=>{
          const e=events.find(event=>event.stage==='outcome') ?? events[0]
          const missing=!raw && events.some(event=>event.stage==='admission') && !events.some(event=>event.stage==='outcome')
          return <tr key={e.id} className="border-t border-line hover:bg-canvas/50" title={[e.clientIp,e.reason,e.operationId && `operation: ${e.operationId}`,e.requestId && `request: ${e.requestId}`,e.traceId && `trace: ${e.traceId}`].filter(Boolean).join(' · ')}>
            <td className="whitespace-nowrap px-4 py-4"><p className="text-xs text-ink-muted">{fmt(e.time)}</p><button className="mt-2 inline-flex items-center gap-1 text-xs font-medium text-accent-700 hover:underline" onClick={()=>setSelected(events)}>Details<ArrowUpRight className="size-3"/></button>{events.length>1 && <span className="ml-2 text-[11px] text-ink-muted">{events.length} records</span>}</td>
            <td className="max-w-40 break-all px-4 py-4 text-xs font-medium">{e.virtualServer || '—'}</td>
            <td className="max-w-48 px-4 py-4"><p className="break-all text-xs font-medium">{e.agentKey || e.actorId || '—'}</p>{e.subject && <p className="mt-1 break-all font-mono text-[11px] text-ink-muted">sub: {e.subject}</p>}{e.actorType && <p className="mt-1 text-[11px] text-ink-muted">{e.actorType}</p>}</td>
            <td className="max-w-60 px-4 py-4"><p className="break-all text-xs font-medium">{e.tool || e.action || e.method || '—'}</p><p className="mt-1 break-all text-[11px] text-ink-muted">{e.stage || 'event'}{e.upstream?` · ${e.upstream}`:e.target?` · ${e.target}`:''}</p></td>
            <td className="max-w-64 px-4 py-4"><Pill status={missing?'outcome not loaded':e.status}/>{e.reason && <p className="mt-2 break-words text-xs leading-relaxed text-ink-muted">{e.reason}</p>}</td>
            <td className="px-4 py-4 text-xs tabular-nums">{e.redactions?<span className="rounded-full bg-accent-50 px-2 py-0.5 font-medium text-accent-700">{e.redactions}</span>:'—'}</td><td className="whitespace-nowrap px-4 py-4 text-xs tabular-nums text-ink-muted">{e.stage==='outcome'?`${e.durationMs} ms`:'—'}</td>
          </tr>
        })}</tbody></table>:!loading && <div className="flex flex-col items-center px-5 py-12 text-center"><span className="flex size-12 items-center justify-center rounded-xl bg-accent-50 text-accent-700"><Search className="size-6"/></span><h3 className="mt-4 text-sm font-semibold">{error?'Audit events unavailable':'No events match.'}</h3><p className="mt-2 max-w-sm text-xs leading-relaxed text-ink-muted">{error?'Refresh to retry loading stored events.':'Adjust filters or refresh to read saved activity. No matching records does not prove that the endpoint is unused.'}</p></div>}
      </div>
    </ServerPanel>
    <p className="text-xs leading-relaxed text-ink-muted">Filters and pagination can split operations. An admission without a loaded outcome may be pending, outside this page or filter, or unavailable; it does not confirm failure. Shared request or trace IDs do not establish an operation relationship. Payloads are never stored.</p>
    {selected && <AuditDetails events={selected} zone={zone} close={()=>setSelected(null)}/>}
  </div>
}
