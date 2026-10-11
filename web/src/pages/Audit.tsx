import { useCallback, useEffect, useRef, useState } from 'react'
import { api, type AuditEvent, type AuditQuery } from '../api'
import { btn, input } from '../styles'
import { fmt } from '../format'
import { CopyButton, Modal, ErrorBox, Pill } from '../ui'

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
  const localTime = (value?:string) => {
    if (!value) return ''
    const date = new Date(value)
    if (!Number.isFinite(date.getTime())) return ''
    return new Date(date.getTime()-date.getTimezoneOffset()*60000).toISOString().slice(0,16)
  }
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone
  return (
    <div className="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-10 lg:py-10">
      <h1 className="text-2xl font-semibold tracking-tight">Audit Log</h1>
      <p className="mt-1 text-sm text-ink-muted">Tool calls and administrative operations. Operations group only loaded events. A missing outcome may be pending, outside this page or filter, or lost; it does not confirm failure. Payloads are never stored.</p>

      <div className="mt-6 grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-3">
        <input className={input} aria-label="Virtual server" placeholder="Virtual server" value={filters.virtualServer ?? ''} onChange={(e) => set('virtualServer', e.target.value.trim())} />
        <input className={input} aria-label="Agent key / oauth:binding" placeholder="Agent key / oauth:binding" value={filters.agentKey ?? ''} onChange={(e) => set('agentKey', e.target.value.trim())} />
        <input className={input} aria-label="Subject" placeholder="Subject" value={filters.subject ?? ''} onChange={(e) => set('subject', e.target.value.trim())} />
        <input className={input} aria-label="Tool" placeholder="Tool" value={filters.tool ?? ''} onChange={(e) => set('tool', e.target.value.trim())} />
        <select aria-label="Status" value={filters.status ?? ''} className={input} onChange={(e) => set('status', e.target.value)}>
          <option value="">Any status</option>
          <option value="started">started</option>
          <option value="ok">ok</option>
          <option value="tool_error">tool error</option>
          <option value="error">error</option>
          <option value="denied">denied</option>
        </select>
        <input type="datetime-local" className={input} aria-label="From" value={localTime(filters.from)} onChange={(e) => when('from', e.target.value)} />
        <input type="datetime-local" className={input} aria-label="To" value={localTime(filters.to)} onChange={(e) => when('to', e.target.value)} />
      </div>
      <p className="mt-3 text-sm text-ink-muted">Times: {zone}. Manual refresh; last loaded: {checked || 'not yet loaded'}. Filters and pagination can split operations.</p>
      <div className="mt-3 flex flex-wrap gap-3"><button className={btn} aria-pressed={!raw} onClick={()=>setRaw(false)}>Operations</button><button className={btn} aria-pressed={raw} onClick={()=>setRaw(true)}>Raw events</button><button className={btn} disabled={loading} onClick={()=>void load(filters,false)}>Refresh</button><CopyButton label="Copy audit filter link" value={new URL(filterLink,window.location.origin).href}/></div>
      <label className="mt-3 flex items-center gap-2 text-sm text-ink-muted">
        <input type="checkbox" checked={filters.incomplete==='true'} onChange={(e) => set('incomplete', e.target.checked ? 'true' : '')} />
        Incomplete operations only
      </label>
      <div className="mt-3 grid gap-3 sm:grid-cols-2"><input className={input} aria-label="Operation ID" placeholder="Operation ID" value={filters.operationId ?? ''} onChange={e=>set('operationId',e.target.value.trim())}/><input className={input} aria-label="Actor ID" placeholder="Actor ID" value={filters.actorId ?? ''} onChange={e=>set('actorId',e.target.value.trim())}/></div>
      {loading && <p role="status">Loading audit events…</p>}
      {error && <ErrorBox>{error} Displayed events may be from the previous successful query.</ErrorBox>}

      <div role="region" aria-label="Audit events" tabIndex={0} className="mt-4 overflow-x-auto rounded-card border border-line bg-surface shadow-card">
        <table className="min-w-[800px] w-full text-left text-sm">
          <thead className="border-b border-line bg-canvas text-xs uppercase tracking-wide text-ink-muted">
            <tr>
              <th className="px-4 py-3">Time</th>
              <th className="px-4 py-3">Virtual server</th>
              <th className="px-4 py-3">Key</th>
              <th className="px-4 py-3">Tool</th>
              <th className="px-4 py-3">Status</th>
              <th className="px-4 py-3">Redacted</th>
              <th className="px-4 py-3 text-right">Duration</th>
            </tr>
          </thead>
          <tbody>
            {[...groups.values()].map((events) => {
              const e = events.find(event=>event.stage==='outcome') ?? events[0]
              const missing = !raw && events.some(event=>event.stage==='admission') && !events.some(event=>event.stage==='outcome')
              return (
              <tr key={e.id} className="border-b border-line last:border-0" title={[e.clientIp, e.reason, e.operationId && `operation: ${e.operationId}`, e.requestId && `request: ${e.requestId}`, e.traceId && `trace: ${e.traceId}`].filter(Boolean).join(' · ')}>
                <td className="whitespace-nowrap px-4 py-2.5">{fmt(e.time)}<button className={btn+' block mt-1'} onClick={()=>setSelected(events)}>Details</button></td>
                <td className="px-4 py-2.5">{e.virtualServer}</td>
                <td className="px-4 py-2.5">
                  {e.agentKey || e.actorId || '—'}
                  {e.subject && <div className="font-mono text-xs text-ink-muted">sub: {e.subject}</div>}
                </td>
                <td className="px-4 py-2.5">{e.tool || e.action || e.method}<div className="text-xs text-ink-muted">{e.stage} · {e.target}</div></td>
                <td className="px-4 py-2.5"><Pill status={missing ? 'outcome not loaded' : e.status} />{e.reason && <span className="ml-2 text-xs text-ink-muted">{e.reason}</span>}</td>
                <td className="px-4 py-2.5">
                  {e.redactions ? <span className="rounded-full bg-accent-50 px-2 py-0.5 text-xs font-medium text-accent-700">{e.redactions}</span> : '—'}
                </td>
                <td className="px-4 py-2.5 text-right tabular-nums">{e.durationMs} ms</td>
              </tr>
            )})}
            {!loading && items.length === 0 && (
              <tr><td colSpan={7} className="p-10 text-center text-ink-muted">No events match. Adjust filters or generate traffic through a virtual server.</td></tr>
            )}
          </tbody>
        </table>
      </div>
      {selected && <Modal title="Audit details" variant="drawer" onClose={()=>setSelected(null)}>
        <p className="text-sm">Raw stored metadata; times shown in {zone}.</p>
        {selected[0].operationId && <div className="my-3 break-all"><p>{selected[0].operationId}</p><CopyButton label="Copy operation ID" value={selected[0].operationId}/><a className="block text-accent-700 underline" href={`/audit?operationId=${encodeURIComponent(selected[0].operationId)}`}>Open operation</a></div>}
        {selected.map(event=><section key={event.id} className="mt-4"><h3>{event.stage} · {fmt(event.time)}</h3>{event.requestId && <CopyButton label="Copy request ID" value={event.requestId}/>} {event.traceId && <CopyButton label="Copy trace ID" value={event.traceId}/>}<pre className="whitespace-pre-wrap break-all rounded bg-canvas p-3 text-xs">{JSON.stringify(event,null,2)}</pre></section>)}
      </Modal>}
      {next && (
        <div className="mt-4 text-center">
          <button className={btn} disabled={loading} onClick={() => load({ ...filters, cursor: next }, true)}>Load more</button>
        </div>
      )}
    </div>
  )
}
