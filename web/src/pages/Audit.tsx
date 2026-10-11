import { useCallback, useEffect, useRef, useState } from 'react'
import { api, type AuditEvent, type AuditQuery } from '../api'
import { btn, input } from '../styles'
import { fmt } from '../format'
import { ErrorBox, Pill } from '../ui'

export default function Audit({ initialServer }: { initialServer?: string }) {
  const [filters, setFilters] = useState<AuditQuery>(()=>initialServer?{virtualServer:initialServer}:{})
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

  const set = (k: keyof AuditQuery, v: string) => setFilters((f) => ({ ...f, [k]: v || undefined }))
  // datetime-local has no zone; interpret it in the browser's zone.
  const when = (k: 'from' | 'to', v: string) => set(k, v ? new Date(v).toISOString() : '')

  return (
    <div className="mx-auto max-w-7xl px-10 py-10">
      <h1 className="text-2xl font-semibold tracking-tight">Audit Log</h1>
      <p className="mt-1 text-sm text-ink-muted">Tool calls and administrative operations. Admissions without outcomes identify in-flight or incomplete work. Payloads are never stored.</p>

      <div className="mt-6 grid grid-cols-6 gap-3">
        <input className={input} placeholder="Virtual server" value={filters.virtualServer ?? ''} onChange={(e) => set('virtualServer', e.target.value.trim())} />
        <input className={input} placeholder="Agent key / oauth:binding" onChange={(e) => set('agentKey', e.target.value.trim())} />
        <input className={input} placeholder="Subject" onChange={(e) => set('subject', e.target.value.trim())} />
        <input className={input} placeholder="Tool" onChange={(e) => set('tool', e.target.value.trim())} />
        <select className={input} onChange={(e) => set('status', e.target.value)}>
          <option value="">Any status</option>
          <option value="started">started</option>
          <option value="ok">ok</option>
          <option value="tool_error">tool error</option>
          <option value="error">error</option>
          <option value="denied">denied</option>
        </select>
        <input type="datetime-local" className={input} aria-label="From" onChange={(e) => when('from', e.target.value)} />
        <input type="datetime-local" className={input} aria-label="To" onChange={(e) => when('to', e.target.value)} />
      </div>
      <label className="mt-3 flex items-center gap-2 text-sm text-ink-muted">
        <input type="checkbox" onChange={(e) => set('incomplete', e.target.checked ? 'true' : '')} />
        Incomplete operations only
      </label>
      {loading && <p role="status">Loading audit events…</p>}
      {error && <ErrorBox>{error}</ErrorBox>}

      <div className="mt-4 overflow-hidden rounded-card border border-line bg-surface shadow-card">
        <table className="w-full text-left text-sm">
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
            {items.map((e) => (
              <tr key={e.id} className="border-b border-line last:border-0" title={[e.clientIp, e.reason, e.operationId && `operation: ${e.operationId}`, e.requestId && `request: ${e.requestId}`, e.traceId && `trace: ${e.traceId}`].filter(Boolean).join(' · ')}>
                <td className="whitespace-nowrap px-4 py-2.5">{fmt(e.time)}</td>
                <td className="px-4 py-2.5">{e.virtualServer}</td>
                <td className="px-4 py-2.5">
                  {e.agentKey || e.actorId || '—'}
                  {e.subject && <div className="font-mono text-xs text-ink-muted">sub: {e.subject}</div>}
                </td>
                <td className="px-4 py-2.5">{e.tool || e.action || e.method}<div className="text-xs text-ink-muted">{e.stage} · {e.target}</div></td>
                <td className="px-4 py-2.5"><Pill status={e.status} />{e.reason && <span className="ml-2 text-xs text-ink-muted">{e.reason}</span>}</td>
                <td className="px-4 py-2.5">
                  {e.redactions ? <span className="rounded-full bg-accent-50 px-2 py-0.5 text-xs font-medium text-accent-700">{e.redactions}</span> : '—'}
                </td>
                <td className="px-4 py-2.5 text-right tabular-nums">{e.durationMs} ms</td>
              </tr>
            ))}
            {!loading && items.length === 0 && (
              <tr><td colSpan={7} className="p-10 text-center text-ink-muted">No events match.</td></tr>
            )}
          </tbody>
        </table>
      </div>
      {next && (
        <div className="mt-4 text-center">
          <button className={btn} disabled={loading} onClick={() => load({ ...filters, cursor: next }, true)}>Load more</button>
        </div>
      )}
    </div>
  )
}
