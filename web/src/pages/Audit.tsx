import Button from '../Button'
import { Badge } from '../Badge'
import { ResultCount } from '../SearchControls'
import { auditDateErrors, parseLocalDateTime } from '../date-time'
import { RadioGroup } from '../SelectionControls'
import { DataTable, LoadMore } from '../DataTable'
import { useTableSort } from '../table-sorting'
import { EmptyState, LoadingState } from '../Feedback'
import { useCallback, useEffect, useRef, useState } from 'react'
import { api, type AuditEvent, type AuditQuery } from '../api'
import { ArrowUpRight, Clock3, FileClock, Layers3, ListFilter, RefreshCw, Search } from 'lucide-react'
import AuditFilters from '../connections/AuditFilters'
import AuditDetails from '../connections/AuditDetails'
import { auditOperationSummary, groupAuditOperations } from '../connections/audit-operations'
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
    if (Object.keys(auditDateErrors(q)).length) { setLoading(false); setNext(null); return }
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
  const when = (k: 'from' | 'to', v: string) => set(k, v ? parseLocalDateTime(v)?.toISOString() ?? v : '')

  const operations = groupAuditOperations(items)
  const groups = raw ? items.map(event => [event]) : operations
  const rows=groups.map(events=>{
    const summary = auditOperationSummary(events)
    return {...summary, events, missing: !raw && summary.missing}
  })
  const sorting=useTableSort(rows,{Time:r=>Date.parse(r.event.time),'Virtual server':r=>r.event.virtualServer,Identity:r=>r.event.agentKey||r.event.actorId,'Tool / action':r=>r.event.tool||r.event.action||r.event.method,Status:r=>r.missing?'outcome not loaded':r.event.status,Redacted:r=>r.event.redactions??0,Duration:r=>!r.missing&&r.event.stage==='outcome'?r.event.durationMs:null})
  const filterLink = `/audit?${new URLSearchParams(Object.entries(filters).filter((entry): entry is [string,string]=>typeof entry[1]==='string'))}`
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone
  const missingGroups=operations.filter(events=>auditOperationSummary(events).incomplete).length
  const filterCount=Object.entries(filters).filter(([key,value])=>key!=='cursor' && value).length
  const reset=()=>{setItems([]);setNext(null);setChecked('');setLoading(true);setFilters({})}
  return <div className="page-layout">
    <header className="flex flex-wrap items-start justify-between gap-4"><div className="flex items-start gap-3"><span className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-accent-50 text-accent-700"><FileClock aria-hidden="true" className="icon-feature"/></span><div><h1 className="page-title">Audit Log</h1><p className="page-description mt-1 text-sm text-ink-muted">Investigate tool calls and administrative operations from stored events.</p></div></div><Button loading={loading} className={btn} disabled={loading} onClick={()=>void load(filters,false)}><RefreshCw aria-hidden="true" className={`icon-inline ${loading?'motion-spin':''}`}/>Refresh</Button></header>
    <div className="grid grid-cols-2 gap-3 xl:grid-cols-4"><SummaryTile label="Loaded records" value={items.length} detail="Stored events in the current query" icon={FileClock}/><SummaryTile label="Displayed rows" value={groups.length} detail={raw?'Individual raw stored events':'Tool calls with explicitly linked upstream operations'} icon={Layers3}/><SummaryTile label="Outcome not loaded" value={missingGroups} detail="Loaded groups with missing outcomes · not failures" icon={Clock3}/><SummaryTile label="Last successful read" value={<span className="text-sm font-medium">{checked || 'Not loaded yet'}</span>} detail={error?'Latest query failed; displayed data may be older':loading?'Reading stored events…':'Manual refresh · no live subscription'} icon={RefreshCw}/></div>
    <AuditFilters filters={filters} set={set} when={when} reset={reset} link={new URL(filterLink,window.location.origin).href} zone={zone}/>
    {error && <ErrorBox title="Audit events could not be loaded" onRetry={()=>load(filters,false)} busy={loading}>{error} Displayed events may be from the previous successful query.</ErrorBox>}
    <ServerPanel title="Stored activity" description={`${items.length} loaded records · ${filterCount} active filters`} icon={ListFilter}>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-4"><RadioGroup label="Audit view" value={raw?'raw':'operations'} onChange={value=>setRaw(value==='raw')} options={[{value:'operations',label:'Operations'},{value:'raw',label:'Raw events'}]}/><div className="flex flex-wrap items-center gap-3"><ResultCount matched={items.length} noun="records" loaded busy={loading}/><p className="text-xs text-ink-muted">{raw?'One row per stored event':'Operations group only loaded records'}</p></div></div>
      {loading && <LoadingState label="Loading audit events…" detail={items.length?'Previously loaded events remain visible.':undefined} compact/>}
      <div role="region" aria-label="Audit events">
        {items.length>0?<DataTable caption={raw?'Raw audit events':'Loaded audit operations'} columns={['Time','Virtual server','Identity','Tool / action','Status','Redacted','Duration']} sorting={sorting} minWidth={920} sortScope="Sorting applies to loaded records only; more events may be available.">{sorting.rows.map(({events,event:e,missing})=>{
          return <tr key={e.id} className="border-t border-line hover:bg-canvas/50" data-tooltip={[e.clientIp,e.reason,e.operationId && `operation: ${e.operationId}`,e.requestId && `request: ${e.requestId}`,e.traceId && `trace: ${e.traceId}`].filter(Boolean).join(' · ')}>
            <td className="whitespace-nowrap px-4 py-4"><p className="text-xs text-ink-muted">{fmt(e.time)}</p><button className="mt-2 inline-flex items-center gap-1 text-xs font-medium text-accent-700 hover:underline" onClick={()=>setSelected(events)}>Details<ArrowUpRight aria-hidden="true" className="icon-inline"/></button>{events.length>1 && <span className="ml-2 text-[11px] text-ink-muted">{events.length} records</span>}</td>
            <td className="max-w-40 break-all px-4 py-4 text-xs font-medium">{e.virtualServer || '—'}</td>
            <td className="max-w-48 px-4 py-4"><p className="break-all text-xs font-medium">{e.agentKey || e.actorId || '—'}</p>{e.subject && <p className="mt-1 break-all font-mono text-[11px] text-ink-muted">sub: {e.subject}</p>}{e.actorType && <p className="mt-1 text-[11px] text-ink-muted">{e.actorType}</p>}</td>
            <td className="max-w-60 px-4 py-4"><p className="break-all text-xs font-medium">{e.tool || e.action || e.method || '—'}</p><p className="mt-1 break-all text-[11px] text-ink-muted">{e.action==='omni/tools/call'?'Omni MCP · ':''}{e.stage || 'event'}{e.upstream?` · ${e.upstream}`:e.target?` · ${e.target}`:''}</p></td>
            <td className="max-w-64 px-4 py-4"><Pill status={missing?'outcome not loaded':e.status}/>{e.reason && <p className="mt-2 break-words text-xs leading-relaxed text-ink-muted">{e.reason}</p>}</td>
            <td className="px-4 py-4 text-xs tabular-nums">{e.redactions?<Badge tone="info">{e.redactions}</Badge>:'—'}</td><td className="whitespace-nowrap px-4 py-4 text-xs tabular-nums text-ink-muted">{!missing&&e.stage==='outcome'?`${e.durationMs} ms`:'—'}</td>
          </tr>
        })}</DataTable>:!loading && <EmptyState icon={Search} title={Object.keys(auditDateErrors(filters)).length?'Choose a valid date range':error?'Audit events unavailable':'No events match.'} description={Object.keys(auditDateErrors(filters)).length?'Correct the highlighted dates to load stored events.':error?'Refresh to retry loading stored events.':'Adjust filters or refresh to read saved activity. No matching records does not prove that the endpoint is unused.'}/>}

      </div>
      <LoadMore shown={items.length} hasMore={!!next} busy={loading} label="Load more" onLoad={()=>{if(next)void load({...filters,cursor:next},true)}}/>
    </ServerPanel>
    <p className="text-xs leading-relaxed text-ink-muted">Filters and pagination can split operations. An admission without a loaded outcome may be pending, outside this page or filter, or unavailable; it does not confirm failure. Shared request or trace IDs do not establish an operation relationship. Payload capture follows the recorded audit policy.</p>
    {selected && <AuditDetails events={selected} zone={zone} close={()=>setSelected(null)}/>}
  </div>
}
