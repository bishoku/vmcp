import { useState } from 'react'
import { api } from '../api'
import { useCapabilities } from '../interaction'
import { Link } from 'react-router-dom'
import { ArrowUpRight, Clock3, FileClock, Fingerprint, Link2 } from 'lucide-react'
import type { AuditEvent } from '../api'
import { CopyButton, Modal, Pill } from '../ui'
import { fmt } from '../format'
import { DetailRow, ServerPanel } from './ServerPanels'
import { auditOperationSummary } from './audit-operations'

export default function AuditDetails({events,zone,close}:{events:AuditEvent[];zone:string;close:()=>void}) {
  const {event, operationId, incomplete: missing, missing: rootMissing}=auditOperationSummary(events)
  const steps = new Map<string, AuditEvent[]>()
  for (const record of events) {
    const key = record.operationId || `event:${record.id}`
    steps.set(key, [...(steps.get(key) || []), record])
  }
  const ordered = [...steps.entries()].sort(([a, ar], [b, br]) =>
    a === operationId ? -1 : b === operationId ? 1 : Math.min(...ar.map(r=>Date.parse(r.time))) - Math.min(...br.map(r=>Date.parse(r.time))))
  const [selection,setSelection]=useState<string>()
  const selected=ordered.find(([id])=>id===selection) || ordered[0]
  const selectedRecords=selected[1]
  const selectedEvent=selectedRecords.find(r=>r.stage==='outcome') || selectedRecords[0]
  const request=selectedRecords.find(r=>r.direction==='request') || selectedRecords.find(r=>r.stage==='admission')
  const response=selectedRecords.find(r=>r.direction==='response') || selectedRecords.find(r=>r.stage==='outcome')
  return <Modal title="Audit details" size="audit" onClose={close}>
    <div className="layout-stack">
      <section aria-label="Operation summary" className="space-y-3">
        <h3 className="text-lg font-semibold break-all">{event.tool || event.action || event.method || 'Audit operation'}</h3>
        <div className="flex flex-wrap items-center gap-3 text-xs"><span>{rootMissing?'Parent outcome not loaded':<Pill status={event.status}/>}</span>{!rootMissing && event.stage==='outcome' && <span>{event.durationMs} ms total</span>}<span className="text-ink-muted">{event.virtualServer || 'Control plane'} · {fmt(event.time)} · {zone}</span></div>
        {!rootMissing && event.reason && <p className="text-xs break-words">{event.reason}</p>}
      </section>
      {missing && <div className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs leading-relaxed text-amber-800"><Clock3 aria-hidden="true" className="mt-0.5 icon-control shrink-0"/><p>This selection is missing a parent or lifecycle outcome. Other records may be pending, outside the loaded page or filters, or unavailable; this does not confirm failure.</p></div>}
      <section aria-label="Operation flow" className="space-y-3">
        <h3 className="panel-title">Operation flow</h3>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {ordered.map(([id,records])=>{
            const outcome=records.find(r=>r.stage==='outcome')
            const record=outcome || records[0]
            return <button type="button" key={id} aria-pressed={selected[0]===id} onClick={()=>setSelection(id)} className={`min-w-0 rounded-xl border p-4 text-left ${selected[0]===id?'border-accent-400 bg-accent-50':'border-line bg-surface'}`}>
              <span className="block text-xs text-ink-muted">{id===operationId?'Root operation':record.parentOperationId?'Linked child operation':'Stored event'} · {record.boundary || record.scope || 'Boundary not recorded'}</span>
              <span className="mt-2 block break-all font-medium">{(record.parentOperationId ? record.upstream : record.tool) || record.action || record.method || 'Event'}</span>
              {record.parentOperationId && <span className="mt-1 block break-all text-xs text-ink-muted">Parent: {record.parentOperationId}</span>}
              <span className="mt-3 flex flex-wrap items-center gap-3 text-xs"><Pill status={record.status}/><span>{outcome?`${outcome.durationMs} ms`:record.stage==='admission'?'Outcome not loaded':'Event'}</span></span>
            </button>
          })}
        </div>
      </section>
      <section aria-label="Selected step payloads" className="space-y-3">
        <div aria-live="polite" className="space-y-1"><h3 className="panel-title break-all">{(selectedEvent.parentOperationId ? selectedEvent.upstream : selectedEvent.tool) || selectedEvent.action || 'Selected event'} · Payloads</h3><p className="text-xs break-all text-ink-muted">{selected[0]}</p>{selectedEvent.reason && <p className="text-xs break-words">{selectedEvent.reason}</p>}</div>
        <div className="grid gap-4 lg:grid-cols-2">
          <PayloadView key={`request:${selected[0]}`} event={request} label="Request"/>
          <PayloadView key={`response:${selected[0]}`} event={response} label="Response"/>
        </div>
        {!request && !response && selectedRecords.map(record=><PayloadView key={record.id} event={record} label="Event payload"/>)}
      </section>
      <details><summary className="cursor-pointer text-xs font-medium text-accent-700">Technical context &amp; raw events · {events.length}</summary><div className="layout-stack mt-4">
      <ServerPanel headingLevel={3} title="Event context" description="Parent operation context when loaded" icon={Fingerprint}><dl className="divide-y divide-line panel-inset"><DetailRow label="Virtual server">{event.virtualServer || '—'}</DetailRow><DetailRow label="Identity"><span className="break-all">{event.agentKey || event.actorId || '—'}</span></DetailRow>{event.subject && <DetailRow label="Subject"><span className="break-all font-mono">{event.subject}</span></DetailRow>}<DetailRow label="Method / action">{event.method || event.action || '—'}</DetailRow>{event.tool && <DetailRow label="Tool"><span className="break-all font-mono">{event.tool}</span></DetailRow>}{event.upstream && <DetailRow label="Upstream">{event.upstream}</DetailRow>}{event.clientIp && <DetailRow label="Client IP">{event.clientIp}</DetailRow>}<DetailRow label="Recorded status"><Pill status={event.status}/></DetailRow></dl></ServerPanel>
      {operationId && <ServerPanel headingLevel={3} title="Operation correlation" description="Parent operation and explicitly linked upstream calls" icon={Link2}><div className="space-y-3 panel-body"><p className="break-all surface-inset p-3 font-mono text-xs">{operationId}</p><div className="flex flex-wrap items-center justify-between gap-3"><CopyButton label="Copy operation ID" value={operationId}/><Link className="inline-flex items-center gap-1 text-xs font-medium text-accent-700" to={`/audit?operationId=${encodeURIComponent(operationId)}`} onClick={close}>Open operation<ArrowUpRight aria-hidden="true" className="icon-inline"/></Link></div></div></ServerPanel>}
      <div><h3 className="panel-title">Stored records</h3><p className="supporting-copy mt-1 text-xs text-ink-muted">Admission and outcome stages remain individually inspectable.</p></div>
      {events.map(record=><ServerPanel key={record.id} title={`${record.stage || 'event'} · ${fmt(record.time)}`} description={record.eventId || `Record ${record.id}`} icon={FileClock}>
        <dl className="divide-y divide-line panel-inset"><DetailRow label="Status"><Pill status={record.status}/></DetailRow><DetailRow label="Boundary">{record.boundary || record.scope || '—'}</DetailRow>{record.operationId && <DetailRow label="Operation ID"><span className="break-all font-mono">{record.operationId}</span></DetailRow>}{record.parentOperationId && <DetailRow label="Parent operation ID"><span className="break-all font-mono">{record.parentOperationId}</span></DetailRow>}{record.reason && <DetailRow label="Reason"><span className="break-words">{record.reason}</span></DetailRow>}<DetailRow label="Duration">{record.stage==='outcome'?`${record.durationMs} ms`:'Not an outcome'}</DetailRow><DetailRow label="Redactions">{record.redactions ?? 0}</DetailRow><DetailRow label="Audit mode">{record.auditMode??'strict'}</DetailRow><DetailRow label="Payload state">{record.payloadState??'disabled'}</DetailRow><DetailRow label="Configuration revision">{record.configRevision ?? '—'}</DetailRow>{record.requestId && <DetailRow label="Request ID"><span className="mb-2 block break-all font-mono">{record.requestId}</span><CopyButton label="Copy request ID" value={record.requestId}/></DetailRow>}{record.traceId && <DetailRow label="Trace ID"><span className="mb-2 block break-all font-mono">{record.traceId}</span><CopyButton label="Copy trace ID" value={record.traceId}/></DetailRow>}</dl>
        <details className="border-t border-line panel-inset py-4"><summary className="cursor-pointer text-xs font-medium text-accent-700">Raw stored metadata</summary><pre className="mt-3 overflow-x-auto whitespace-pre-wrap break-all surface-inset p-3 font-mono text-[11px] leading-relaxed">{JSON.stringify(record,null,2)}</pre></details>
      </ServerPanel>)}
      </div></details>
    </div>
  </Modal>
}

function PayloadView({event,label}:{event?:AuditEvent;label:string}){
 const {manageTokens}=useCapabilities()
 const [body,setBody]=useState<unknown>(),[state,setState]=useState(''),[busy,setBusy]=useState(false)
 const load=async()=>{
   if(!event || !manageTokens)return
   setBusy(true);setBody(undefined)
   try{const p=await api.auditPayload(event.eventId);setState(p.state);setBody(p.content)}
   catch{setState('Payload could not be loaded')}
   finally{setBusy(false)}
 }
 const recorded=event?.payloadState || 'disabled'
 return <section aria-label={`${label} payload`} className="min-w-0 rounded-xl border border-line bg-surface">
   <div className="flex flex-wrap items-center justify-between gap-2 border-b border-line p-3"><h4 className="text-xs font-semibold">{label}</h4><span className="text-xs text-ink-muted">{event?(state || recorded):'Record not loaded'}</span></div>
   <div className="space-y-3 p-4 text-xs">
     {!event?<p className="text-ink-muted">No {label.toLowerCase()} record in this selection.</p>:!manageTokens?<p className="text-ink-muted">Administrator access is required to view payloads.</p>:recorded==='disabled'?<p className="text-ink-muted">Payload capture was disabled for this record.</p>:<button className="text-xs font-medium text-accent-700" disabled={busy} onClick={()=>void load()}>{busy?'Loading payload…':'Load audit payload'}</button>}
     {state&&<p role="status">{state}</p>}
     {manageTokens&&body!==undefined&&<pre className="max-h-[32rem] overflow-auto whitespace-pre-wrap break-all surface-inset p-3 font-mono text-xs">{JSON.stringify(body,null,2)}</pre>}
   </div>
 </section>
}
