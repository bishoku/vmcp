import { Link, useNavigate } from 'react-router-dom'
import { useCapabilities } from '../interaction'
import { ErrorBox } from '../ui'
import { useOperation } from '../interaction'
import { useSources, source } from '../requests'
import { useEffect, useMemo, useState } from 'react'
import { Plus, RefreshCw, Search, Trash2 } from 'lucide-react'
import clsx from 'clsx'
import { api, ApiError, type Upstream } from '../api'

import { btn, input } from '../styles'

function ago(iso: string | null) {
  if (!iso) return 'never'
  const s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000)
  if (s < 60) return 'just now'
  if (s < 3600) return `${Math.floor(s / 60)} min ago`
  if (s < 86400) return `${Math.floor(s / 3600)} hours ago`
  return `${Math.floor(s / 86400)} days ago`
}

function statusOf(u: Upstream) {
  return !u.enabled ? 'disabled' : u.lastRefreshError ? 'failed' : u.lastRefreshAt ? 'healthy' : 'pending'
}

function Status({ u }: { u: Upstream }) {
  const h = u.health?.state
  const [label, cls] = !u.enabled
    ? ['Disabled', 'bg-gray-100 text-gray-600']
    : h === 'unhealthy'
      ? ['Unreachable', 'bg-red-50 text-red-700']
      : h === 'degraded'
        ? ['Degraded', 'bg-amber-50 text-amber-700']
        : u.lastRefreshError
          ? ['Refresh failed', 'bg-red-50 text-red-700']
          : u.lastRefreshAt
            ? ['Healthy', 'bg-green-50 text-green-700']
            : ['Not refreshed', 'bg-amber-50 text-amber-700']
  return <span className={clsx('rounded-full px-2 py-0.5 text-xs font-medium', cls)}>{label}</span>
}

const errMsg = (e: unknown) => (e instanceof ApiError ? e.message : String(e))

export default function Upstreams() {
  const navigate = useNavigate()
  const { edit: canEdit } = useCapabilities()
  const [items, setItems] = useState<Upstream[]>([])
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [orphanedWarning, setOrphanedWarning] = useState('')

  const { load, loading, readError } = useSources(() => [source('Upstreams', api.listUpstreams, setItems)])
  useEffect(() => {
    const timer = setTimeout(() => { void load() }, 0)
    return () => clearTimeout(timer)
  }, [load])

  const shown = useMemo(
    () =>
      items.filter(
        (u) =>
          (u.slug + u.name + u.url).toLowerCase().includes(query.toLowerCase()) &&
          (!status || statusOf(u) === status),
      ),
    [items, query, status],
  )

  const { run } = useOperation()
  const refresh = (u: Upstream) => run(async () => {
    try {
      setOrphanedWarning('')
      const res = await api.refreshTools(u.slug)
      if (res.orphanedSelections && res.orphanedSelections.length > 0) {
        const list = res.orphanedSelections.map((o) => `${o.virtualServer}:${o.tool}`).join(', ')
        setOrphanedWarning(`Upstream "${u.slug}" removed tools still selected by virtual servers: ${list}`)
      }
    } catch (e) {
      setError(errMsg(e))
    }
    void load()
  })
  const remove = (u: Upstream) => run(async () => {
    if (!confirm(`Delete upstream "${u.slug}"?`)) return
    try {
      await api.deleteUpstream(u.slug,false,u.version)
    } catch (e) {
      if (e instanceof ApiError && e.code === 'in_use') {
        if (confirm(`${e.message}\n\nDelete anyway and remove it from those virtual servers?`))
          await api.deleteUpstream(u.slug, true,u.version).catch((x) => setError(errMsg(x)))
      } else setError(errMsg(e))
    }
    void load()
  })


  return (
    <div className="mx-auto max-w-6xl px-4 py-6 sm:px-6 lg:px-10 lg:py-10">
      <div className="flex flex-wrap gap-3 items-start justify-between">
        <div>
          {readError && <ErrorBox>{readError}</ErrorBox>}
      {loading && <p role="status">Loading…</p>}
      <button className={btn} disabled={loading} onClick={()=>{setError('');setOrphanedWarning('');void load()}}>Refresh list</button>
      <h1 className="text-2xl font-semibold tracking-tight">Upstreams</h1>
          <p className="mt-1 text-sm text-ink-muted">
            Register physical MCP servers and inspect their tools.
          </p>
        </div>
        <button
          className="inline-flex items-center gap-1.5 rounded-lg bg-accent-500 px-4 py-2 text-sm font-medium text-white hover:bg-accent-700"
          disabled={!canEdit}
          onClick={() => navigate('/upstreams/new')}
        >
          <Plus className="size-4" /> Add upstream
        </button>
      </div>

      {error && (
        <div className="mt-6 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {error}
        </div>
      )}
      {orphanedWarning && (
        <div role="status" className="mt-6 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
          {orphanedWarning}
        </div>
      )}

      <div className="mt-6 rounded-card border border-line bg-surface shadow-card">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line p-4">
          <div className="relative w-full max-w-xs">
            <Search className="absolute left-3 top-2.5 size-4 text-ink-muted" />
            <input
              className={clsx(input, 'pl-9')}
              aria-label="Search upstreams" placeholder="Search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </div>
          <select aria-label="Upstream status" className={clsx(input, 'w-44')} value={status} onChange={(e) => setStatus(e.target.value)}>
            <option value="">All statuses</option>
            <option value="healthy">Healthy</option>
            <option value="failed">Refresh failed</option>
            <option value="pending">Not refreshed</option>
            <option value="disabled">Disabled</option>
          </select>
        </div>
        <div className="overflow-x-auto"><table className="min-w-[760px] w-full text-left text-sm">
          <thead className="bg-canvas text-xs font-medium text-ink-muted">
            <tr>
              {['Name', 'URL', 'Transport', 'Tools', 'Last refresh', 'Status', ''].map((h) => (
                <th key={h} className="px-4 py-2.5">
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {shown.map((u) => (
              <tr
                key={u.slug}
                className="cursor-pointer border-t border-line hover:bg-canvas"
                onClick={() => navigate(`/upstreams/${encodeURIComponent(u.slug)}`)}
              >
                <td className="px-4 py-3">
                  <Link className="font-medium text-accent-700 hover:underline" to={`/upstreams/${encodeURIComponent(u.slug)}`} onClick={e=>e.stopPropagation()}>{u.name}</Link>
                  <div className="text-xs text-ink-muted">{u.slug}</div>
                </td>
                <td className="max-w-56 truncate px-4 py-3">{u.url}</td>
                <td className="px-4 py-3">
                  <div className="flex flex-wrap gap-1">
                    <span className="rounded-md bg-accent-50 px-2 py-0.5 text-xs text-accent-700">
                      {u.transport === 'sse' ? 'SSE' : 'Streamable HTTP'}
                    </span>
                    {u.auth?.type === 'oauth_client_credentials' && (
                      <span className="rounded-md bg-purple-50 px-2 py-0.5 text-xs font-medium text-purple-700">
                        OAuth
                      </span>
                    )}
                  </div>
                </td>
                <td className="px-4 py-3">{u.toolCount}</td>
                <td className="px-4 py-3 text-ink-muted">{ago(u.lastRefreshAt)}</td>
                <td className="px-4 py-3">
                  <Status u={u} />
                </td>
                <td className="px-4 py-3" onClick={(e) => e.stopPropagation()}>
                  <div className="flex justify-end gap-1">
                    <button disabled={!canEdit} className={btn} title="Refresh capabilities" onClick={() => refresh(u)}>
                      <RefreshCw className="size-3.5" />
                    </button>
                    <button disabled={!canEdit} className={btn} title="Delete" onClick={() => remove(u)}>
                      <Trash2 className="size-3.5 text-red-600" />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
            {!loading && !readError && shown.length === 0 && (
              <tr>
                <td colSpan={7} className="px-4 py-10 text-center text-ink-muted">
                  {items.length ? 'No upstreams match your filters.' : 'No upstreams yet. Add an MCP endpoint, test its stored credentials, then refresh discovery.'}
                </td>
              </tr>
            )}
          </tbody>
        </table></div>
      </div>


    </div>
  )
}
