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
  return <section aria-label="Recent observed activity" className="space-y-6">
    <div className="flex flex-wrap items-start justify-between gap-4"><div><h2 className="text-lg font-semibold tracking-tight">Activity</h2><p className="mt-1 text-sm text-ink-muted">Recent stored events for this virtual server.</p></div><div className="flex flex-wrap gap-2"><button className={btn} disabled={loading} onClick={()=>load()}><RefreshCw className={`size-3.5 ${loading?'animate-spin':''}`} aria-hidden="true"/>Check recent activity</button><Link className={btn} to={`/audit?virtualServer=${encodeURIComponent(slug)}`}>Open audit log<ArrowUpRight className="size-3.5" aria-hidden="true"/></Link></div></div>
    <div className="grid gap-3 sm:grid-cols-3">
      <SummaryTile label="Time window" value="Last 15 minutes" detail="Recent events · refresh manually" icon={Clock3}/>
      <SummaryTile label="Loaded events" value={checkedAt?items.length:'—'} detail={more?'More events available in audit log':'Up to 50 stored records per check'} icon={FileClock}/>
      <SummaryTile label="Last successful check" value={<span className="text-sm font-medium">{checkedAt?fmt(checkedAt):'Not checked yet'}</span>} detail={readError?'Latest read failed; retained data may be older':loading?'Checking stored activity…':'Latest successful read of stored events'} icon={RefreshCw}/>
    </div>
    {readError && <ErrorBox title="Recent activity could not be loaded" onRetry={load} busy={loading}>{readError}. {checkedAt?'Showing events from the last successful check.':'Activity could not be loaded. Use Check recent activity to retry.'}</ErrorBox>}
    <ServerPanel title="Stored events" description="Identity, method and recorded outcome at a glance" icon={ListFilter} footer={<div className="flex flex-wrap items-center justify-between gap-3"><span>{items.length} loaded records{more?' · more events available':''}</span>{more && <Link className="inline-flex items-center gap-1 font-medium text-accent-700" to={`/audit?virtualServer=${encodeURIComponent(slug)}`}>Continue in audit log<ArrowUpRight className="size-3.5"/></Link>}</div>}>
      {loading && <LoadingState label="Loading recent activity…" detail={items.length?'Previously loaded events remain visible.':undefined} compact/>}
      {items.length>0?<div className="overflow-x-auto"><table className="w-full min-w-[760px] text-left text-sm"><caption className="sr-only">Recent events for {slug}</caption><thead className="bg-canvas text-xs text-ink-muted"><tr>{['Time','Identity','Method / action','Stage','Status'].map(t=><th scope="col" key={t} className="px-5 py-3 font-medium">{t}</th>)}</tr></thead><tbody>{items.map(event=><tr key={event.eventId || event.id} className="border-t border-line hover:bg-canvas/50"><td className="whitespace-nowrap px-5 py-4 text-xs text-ink-muted">{fmt(event.time)}</td><td className="px-5 py-4"><p className="max-w-48 break-all text-xs font-medium">{event.actorId || event.agentKey || event.subject || event.actorType || 'unknown'}</p>{event.actorType && <p className="mt-1 text-[11px] text-ink-muted">{event.actorType}</p>}</td><td className="px-5 py-4"><p className="text-xs font-medium">{event.method || event.action}</p>{event.tool && <p className="mt-1 max-w-60 break-all font-mono text-[11px] text-ink-muted">{event.upstream?`${event.upstream} / `:''}{event.tool}</p>}{event.operationId && <Link className="mt-1 inline-flex items-center gap-1 text-[11px] text-accent-700" to={`/audit?operationId=${encodeURIComponent(event.operationId)}`}>Inspect operation<ArrowUpRight className="size-3"/></Link>}</td><td className="px-5 py-4"><Pill status={event.stage}/></td><td className="px-5 py-4"><Pill status={event.status}/>{event.stage==='outcome' && event.durationMs>0 && <p className="mt-1.5 text-[11px] text-ink-muted">{event.durationMs} ms</p>}{event.reason && <p className="mt-1.5 max-w-56 break-words text-xs text-ink-muted">{event.reason}</p>}</td></tr>)}</tbody></table></div>:!loading && <EmptyState icon={Inbox} title={readError?'Activity unavailable':'No stored events in this window'} description={readError?'Retry the check to load recent events for this server.':'No events were returned for the last 15 minutes. This does not prove the endpoint is unused or unreachable.'}/>}

    </ServerPanel>
    <p className="text-xs leading-relaxed text-ink-muted">Loaded records are not unique calls or active sessions. Admissions and outcomes can be separate records; an admission without an outcome does not prove success. Retention and storage availability affect what is visible.</p>
  </section>
}
