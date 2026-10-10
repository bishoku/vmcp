import { useCallback, useEffect, useState } from 'react'
import { api, type AuditEvent, type AuditQuery } from '../api'
import { btn, ErrorBox, fmt, input, Pill } from '../ui'

export default function Audit() {
  const [filters, setFilters] = useState<AuditQuery>({})
  const [items, setItems] = useState<AuditEvent[]>([])
  const [next, setNext] = useState<string | null>(null)
  const [error, setError] = useState('')

  const load = useCallback(async (q: AuditQuery, append: boolean) => {
    try {
      const r = await api.listAudit(q)
      setItems((prev) => (append ? [...prev, ...r.items] : r.items))
      setNext(r.nextCursor ?? null)
      setError('')
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }, [])
  useEffect(() => {
    void load(filters, false)
  }, [filters, load])

  const set = (k: keyof AuditQuery, v: string) => setFilters((f) => ({ ...f, [k]: v || undefined }))
  // datetime-local has no zone; interpret it in the browser's zone.
  const when = (k: 'from' | 'to', v: string) => set(k, v ? new Date(v).toISOString() : '')

  return (
    <div className="mx-auto max-w-7xl px-10 py-10">
      <h1 className="text-2xl font-semibold tracking-tight">Audit Log</h1>
      <p className="mt-1 text-sm text-ink-muted">Every tool call through the data plane. Payloads are never stored.</p>

      <div className="mt-6 grid grid-cols-6 gap-3">
        <input className={input} placeholder="Virtual server" onChange={(e) => set('virtualServer', e.target.value.trim())} />
        <input className={input} placeholder="Agent key / oauth:binding" onChange={(e) => set('agentKey', e.target.value.trim())} />
        <input className={input} placeholder="Tool" onChange={(e) => set('tool', e.target.value.trim())} />
        <select className={input} onChange={(e) => set('status', e.target.value)}>
          <option value="">Any status</option>
          <option value="ok">ok</option>
          <option value="tool_error">tool error</option>
          <option value="error">error</option>
          <option value="denied">denied</option>
        </select>
        <input type="datetime-local" className={input} aria-label="From" onChange={(e) => when('from', e.target.value)} />
        <input type="datetime-local" className={input} aria-label="To" onChange={(e) => when('to', e.target.value)} />
      </div>
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
              <tr key={e.id} className="border-b border-line last:border-0" title={[e.clientIp, e.reason].filter(Boolean).join(' · ')}>
                <td className="whitespace-nowrap px-4 py-2.5">{fmt(e.time)}</td>
                <td className="px-4 py-2.5">{e.virtualServer}</td>
                <td className="px-4 py-2.5">
                  {e.agentKey ?? '—'}
                  {e.subject && <div className="font-mono text-xs text-ink-muted">sub: {e.subject}</div>}
                </td>
                <td className="px-4 py-2.5">{e.tool ?? e.method}</td>
                <td className="px-4 py-2.5"><Pill status={e.status} />{e.reason && <span className="ml-2 text-xs text-ink-muted">{e.reason}</span>}</td>
                <td className="px-4 py-2.5">
                  {e.redactions ? <span className="rounded-full bg-accent-50 px-2 py-0.5 text-xs font-medium text-accent-700">{e.redactions}</span> : '—'}
                </td>
                <td className="px-4 py-2.5 text-right tabular-nums">{e.durationMs} ms</td>
              </tr>
            ))}
            {items.length === 0 && (
              <tr><td colSpan={7} className="p-10 text-center text-ink-muted">No events match.</td></tr>
            )}
          </tbody>
        </table>
      </div>
      {next && (
        <div className="mt-4 text-center">
          <button className={btn} onClick={() => load({ ...filters, cursor: next }, true)}>Load more</button>
        </div>
      )}
    </div>
  )
}
