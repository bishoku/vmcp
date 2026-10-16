import Button from '../Button'
import { ResultCount } from '../SearchControls'
import { DataTable } from '../DataTable'
import { useTableSort } from '../table-sorting'
import { EmptyState, LoadingState } from '../Feedback'
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowUpRight, Clock3, FileClock, Inbox, ListFilter, RefreshCw } from 'lucide-react'
import { api, type AuditEvent } from '../api'
import { source, useSources } from '../requests'
import { ErrorBox, Pill } from '../ui'
import { btn } from '../styles'
import { fmt } from '../format'
import { ServerPanel, SummaryTile } from './ServerPanels'

export default function RecentActivity({slug}: {slug:string}) {
  const [items,setItems] = useState<AuditEvent[]>([])
  const [more,setMore] = useState(false)
  const [checkedAt,setCheckedAt] = useState<string|null>(null)
  const {load,loading,readError} = useSources(()=>[source('Recent activity',signal=>api.listAudit({virtualServer:slug,from:new Date(Date.now()-15*60_000).toISOString()},signal),result=>{
    setItems(result.items);setMore(!!result.nextCursor);setCheckedAt(new Date().toISOString())
  })])
  useEffect(()=>{const timer=setTimeout(()=>{void load()},0);return()=>clearTimeout(timer)},[load])
  const sorting=useTableSort(items,{Time:e=>Date.parse(e.time),Identity:e=>e.actorId||e.agentKey||e.subject||e.actorType,'Method / action':e=>e.method||e.action,Stage:e=>e.stage,Status:e=>e.status})
  return <section aria-label="Recent observed activity" className="layout-stack">
    <div className="flex flex-wrap items-start justify-between gap-4"><div><h2 className="section-title">Activity</h2><p className="section-description mt-1 text-sm text-ink-muted">Recent stored events for this virtual server.</p></div><div className="flex flex-wrap gap-2"><Button loading={loading} className={btn} disabled={loading} onClick={()=>load()}><RefreshCw className={`icon-inline ${loading?'motion-spin':''}`} aria-hidden="true"/>Check recent activity</Button><Link className={btn} to={`/audit?virtualServer=${encodeURIComponent(slug)}`}>Open audit log<ArrowUpRight className="icon-inline" aria-hidden="true"/></Link></div></div>
    <div className="grid gap-3 sm:grid-cols-3">
      <SummaryTile label="Time window" value="Last 15 minutes" detail="Recent events · refresh manually" icon={Clock3}/>
      <SummaryTile label="Loaded events" value={checkedAt?items.length:'—'} detail={more?'More events available in audit log':'Up to 50 stored records per check'} icon={FileClock}/>
      <SummaryTile label="Last successful check" value={<span className="text-sm font-medium">{checkedAt?fmt(checkedAt):'Not checked yet'}</span>} detail={readError?'Latest read failed; retained data may be older':loading?'Checking stored activity…':'Latest successful read of stored events'} icon={RefreshCw}/>
    </div>
    {readError && <ErrorBox title="Recent activity could not be loaded" onRetry={load} busy={loading}>{readError}. {checkedAt?'Showing events from the last successful check.':'Activity could not be loaded. Use Check recent activity to retry.'}</ErrorBox>}
    <ServerPanel headingLevel={3} title="Stored events" description="Identity, method and recorded outcome at a glance" icon={ListFilter} footer={<div className="flex flex-wrap items-center justify-between gap-3"><ResultCount matched={items.length} noun="records" loaded note={more?'more events available':undefined}/>{more && <Link className="inline-flex items-center gap-1 font-medium text-accent-700" to={`/audit?virtualServer=${encodeURIComponent(slug)}`}>Continue in audit log<ArrowUpRight aria-hidden="true" className="icon-inline"/></Link>}</div>}>
      {loading && <LoadingState label="Loading recent activity…" detail={items.length?'Previously loaded events remain visible.':undefined} compact/>}
      {items.length>0?<DataTable caption={`Recent events for ${slug}`} columns={['Time','Identity','Method / action','Stage','Status']} sorting={sorting} minWidth={760} sortScope="Sorting applies to this loaded activity snapshot.">{sorting.rows.map(event=><tr key={event.eventId || event.id} className="border-t border-line hover:bg-canvas/50"><td className="whitespace-nowrap panel-inset py-4 text-xs text-ink-muted">{fmt(event.time)}</td><td className="panel-inset py-4"><p className="max-w-48 break-all text-xs font-medium">{event.actorId || event.agentKey || event.subject || event.actorType || 'unknown'}</p>{event.actorType && <p className="mt-1 text-[11px] text-ink-muted">{event.actorType}</p>}</td><td className="panel-inset py-4"><p className="text-xs font-medium">{event.method || event.action}</p>{event.tool && <p className="mt-1 max-w-60 break-all font-mono text-[11px] text-ink-muted">{event.upstream?`${event.upstream} / `:''}{event.tool}</p>}{event.operationId && <Link className="mt-1 inline-flex items-center gap-1 text-[11px] text-accent-700" to={`/audit?operationId=${encodeURIComponent(event.operationId)}`}>Inspect operation<ArrowUpRight aria-hidden="true" className="icon-inline"/></Link>}</td><td className="panel-inset py-4"><Pill status={event.stage}/></td><td className="panel-inset py-4"><Pill status={event.status}/>{event.stage==='outcome' && event.durationMs>0 && <p className="mt-1.5 text-[11px] text-ink-muted">{event.durationMs} ms</p>}{event.reason && <p className="mt-1.5 max-w-56 break-words text-xs text-ink-muted">{event.reason}</p>}</td></tr>)}</DataTable>:!loading && <EmptyState icon={Inbox} title={readError?'Activity unavailable':'No stored events in this window'} description={readError?'Retry the check to load recent events for this server.':'No events were returned for the last 15 minutes. This does not prove the endpoint is unused or unreachable.'}/>}

    </ServerPanel>
    <p className="text-xs leading-relaxed text-ink-muted">Loaded records are not unique calls or active sessions. Admissions and outcomes can be separate records; an admission without an outcome does not prove success. Retention and storage availability affect what is visible.</p>
  </section>
}
