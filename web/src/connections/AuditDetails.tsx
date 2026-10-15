import { Link } from 'react-router-dom'
import { ArrowUpRight, Clock3, FileClock, Fingerprint, Link2 } from 'lucide-react'
import type { AuditEvent } from '../api'
import { CopyButton, Modal, Pill } from '../ui'
import { fmt } from '../format'
import { DetailRow, ServerPanel } from './ServerPanels'

export default function AuditDetails({events,zone,close}:{events:AuditEvent[];zone:string;close:()=>void}) {
  const event=events.find(e=>e.stage==='outcome') ?? events[0]
  const missing=events.some(e=>e.stage==='admission') && !events.some(e=>e.stage==='outcome')
  return <Modal title="Audit details" variant="drawer" size="wide" onClose={close}>
    <div className="space-y-5"><p className="text-xs leading-relaxed text-ink-muted">{events.length} selected stored {events.length===1?'record':'records'} · Times shown in {zone}. Payloads are never stored.</p>
      {missing && <div className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs leading-relaxed text-amber-800"><Clock3 className="mt-0.5 size-4 shrink-0"/><p>This selection contains an admission without an outcome. Other records may be pending, outside the loaded page or filters, or unavailable; this does not confirm failure.</p></div>}
      <ServerPanel headingLevel={3} title="Event context" description="Context from the selected outcome, or first stored record" icon={Fingerprint}><dl className="divide-y divide-line px-5"><DetailRow label="Virtual server">{event.virtualServer || '—'}</DetailRow><DetailRow label="Identity"><span className="break-all">{event.agentKey || event.actorId || '—'}</span></DetailRow>{event.subject && <DetailRow label="Subject"><span className="break-all font-mono">{event.subject}</span></DetailRow>}<DetailRow label="Method / action">{event.method || event.action || '—'}</DetailRow>{event.tool && <DetailRow label="Tool"><span className="break-all font-mono">{event.tool}</span></DetailRow>}{event.upstream && <DetailRow label="Upstream">{event.upstream}</DetailRow>}{event.clientIp && <DetailRow label="Client IP">{event.clientIp}</DetailRow>}<DetailRow label="Recorded status"><Pill status={event.status}/></DetailRow></dl></ServerPanel>
      {event.operationId && <ServerPanel headingLevel={3} title="Operation correlation" description="Group stored records using the exact operation ID" icon={Link2}><div className="space-y-3 p-5"><p className="break-all rounded-lg bg-canvas p-3 font-mono text-xs">{event.operationId}</p><div className="flex flex-wrap items-center justify-between gap-3"><CopyButton label="Copy operation ID" value={event.operationId}/><Link className="inline-flex items-center gap-1 text-xs font-medium text-accent-700" to={`/audit?operationId=${encodeURIComponent(event.operationId)}`} onClick={close}>Open operation<ArrowUpRight className="size-3.5"/></Link></div></div></ServerPanel>}
      <div><h3 className="panel-title">Stored records</h3><p className="supporting-copy mt-1 text-xs text-ink-muted">Admission and outcome stages remain individually inspectable.</p></div>
      {events.map(record=><ServerPanel key={record.id} title={`${record.stage || 'event'} · ${fmt(record.time)}`} description={record.eventId || `Record ${record.id}`} icon={FileClock}>
        <dl className="divide-y divide-line px-5"><DetailRow label="Status"><Pill status={record.status}/></DetailRow>{record.reason && <DetailRow label="Reason"><span className="break-words">{record.reason}</span></DetailRow>}<DetailRow label="Duration">{record.stage==='outcome'?`${record.durationMs} ms`:'Not an outcome'}</DetailRow><DetailRow label="Redactions">{record.redactions ?? 0}</DetailRow><DetailRow label="Configuration revision">{record.configRevision ?? '—'}</DetailRow>{record.requestId && <DetailRow label="Request ID"><span className="mb-2 block break-all font-mono">{record.requestId}</span><CopyButton label="Copy request ID" value={record.requestId}/></DetailRow>}{record.traceId && <DetailRow label="Trace ID"><span className="mb-2 block break-all font-mono">{record.traceId}</span><CopyButton label="Copy trace ID" value={record.traceId}/></DetailRow>}</dl>
        <details className="border-t border-line px-5 py-4"><summary className="cursor-pointer text-xs font-medium text-accent-700">Raw stored metadata</summary><pre className="mt-3 overflow-x-auto whitespace-pre-wrap break-all rounded-lg border border-line bg-canvas p-3 font-mono text-[11px] leading-relaxed">{JSON.stringify(record,null,2)}</pre></details>
      </ServerPanel>)}
    </div>
  </Modal>
}
