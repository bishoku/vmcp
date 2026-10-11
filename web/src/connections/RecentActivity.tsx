import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type AuditEvent } from '../api'
import { source, useSources } from '../requests'
import { ErrorBox, Pill } from '../ui'
import { btn } from '../styles'
import { fmt } from '../format'

export default function RecentActivity({slug}: {slug:string}) {
  const [items,setItems] = useState<AuditEvent[]>([])
  const [more,setMore] = useState(false)
  const [checkedAt,setCheckedAt] = useState<string|null>(null)
  const {load,loading,readError} = useSources(()=>[source('Recent activity',signal=>api.listAudit({virtualServer:slug,from:new Date(Date.now()-15*60_000).toISOString()},signal),result=>{
    setItems(result.items);setMore(!!result.nextCursor);setCheckedAt(new Date().toISOString())
  })])
  useEffect(()=>{const timer=setTimeout(()=>{void load()},0);return()=>clearTimeout(timer)},[load])
  return <section className="space-y-3"><h2 className="text-lg font-semibold">Recent observed activity</h2>
    <p className="text-sm text-ink-muted">Up to 50 stored events from the last 15 minutes, checked {fmt(checkedAt)}. No active-session or traffic-rate inference. An admission without an outcome does not prove success.</p>
    <button className={btn} disabled={loading} onClick={()=>load()}>Check recent activity</button> <Link className={btn} to={`/audit?virtualServer=${encodeURIComponent(slug)}`}>Open audit log</Link>
    {loading && <p role="status">Loading recent activity…</p>}{readError && <ErrorBox>{readError}</ErrorBox>}
    {!loading && !readError && items.length===0 && <p>No stored events in this window. This does not prove the endpoint is unused or unreachable.</p>}
    <div className="overflow-x-auto"><table className="w-full text-left text-sm"><caption className="sr-only">Recent events for {slug}</caption><thead><tr>{['Time','Actor','Method / action','Stage','Status'].map(t=><th key={t} className="p-2">{t}</th>)}</tr></thead><tbody>{items.map(event=><tr key={event.eventId || event.id} className="border-t border-line"><td className="p-2">{fmt(event.time)}</td><td className="p-2">{event.actorId || event.agentKey || event.subject || event.actorType || 'unknown'}</td><td className="p-2">{event.method || event.action}</td><td className="p-2">{event.stage}</td><td className="p-2"><Pill status={event.status}/></td></tr>)}</tbody></table></div>
    {more && <p className="text-sm">More events exist in this window. Open the audit log to continue.</p>}
  </section>
}
