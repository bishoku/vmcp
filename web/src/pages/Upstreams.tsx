import { useCallback, useEffect, useMemo, useState, type ChangeEvent } from 'react'
import { CheckCircle2, Plus, RefreshCw, Search, Trash2, X, XCircle } from 'lucide-react'
import clsx from 'clsx'
import { Modal } from '../ui'
import { api, ApiError, type ConnectionTestResult, type Tool, type Upstream, type UpstreamTLSInput, type UpstreamAuthInput } from '../api'

const btn =
  'inline-flex items-center justify-center gap-1.5 rounded-lg border border-line bg-surface px-3 py-1.5 text-sm font-medium text-ink hover:bg-canvas disabled:opacity-50'
const input =
  'w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-accent-500 focus:ring-2 focus:ring-accent-500/20'

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
  const [items, setItems] = useState<Upstream[]>([])
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [selected, setSelected] = useState<Upstream | 'new' | null>(null)

  const load = useCallback(async () => {
    try {
      setItems(await api.listUpstreams())
      setError('')
    } catch (e) {
      setError(errMsg(e))
    }
  }, [])
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

  const refresh = async (u: Upstream) => {
    try {
      await api.refreshTools(u.slug)
    } catch (e) {
      setError(errMsg(e))
    }
    void load()
  }
  const remove = async (u: Upstream) => {
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
  }

  return (
    <div className="mx-auto max-w-6xl px-10 py-10">
      <div className="flex items-start justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Upstreams</h1>
          <p className="mt-1 text-sm text-ink-muted">
            Register physical MCP servers and inspect their tools.
          </p>
        </div>
        <button
          className="inline-flex items-center gap-1.5 rounded-lg bg-accent-500 px-4 py-2 text-sm font-medium text-white hover:bg-accent-700"
          onClick={() => setSelected('new')}
        >
          <Plus className="size-4" /> Add upstream
        </button>
      </div>

      {error && (
        <div className="mt-6 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {error}
        </div>
      )}

      <div className="mt-6 rounded-card border border-line bg-surface shadow-card">
        <div className="flex items-center justify-between gap-3 border-b border-line p-4">
          <div className="relative w-full max-w-xs">
            <Search className="absolute left-3 top-2.5 size-4 text-ink-muted" />
            <input
              className={clsx(input, 'pl-9')}
              placeholder="Search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </div>
          <select className={clsx(input, 'w-44')} value={status} onChange={(e) => setStatus(e.target.value)}>
            <option value="">All statuses</option>
            <option value="healthy">Healthy</option>
            <option value="failed">Refresh failed</option>
            <option value="pending">Not refreshed</option>
            <option value="disabled">Disabled</option>
          </select>
        </div>
        <table className="w-full text-left text-sm">
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
                onClick={() => setSelected(u)}
              >
                <td className="px-4 py-3">
                  <button type="button" className="font-medium hover:underline" onClick={() => setSelected(u)}>{u.name}</button>
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
                    <button className={btn} title="Refresh tools" onClick={() => refresh(u)}>
                      <RefreshCw className="size-3.5" />
                    </button>
                    <button className={btn} title="Delete" onClick={() => remove(u)}>
                      <Trash2 className="size-3.5 text-red-600" />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
            {shown.length === 0 && (
              <tr>
                <td colSpan={7} className="px-4 py-10 text-center text-ink-muted">
                  No upstreams yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {selected && (
        <Panel
          key={selected === 'new' ? 'new' : selected.slug}
          upstream={selected === 'new' ? null : selected}
          onClose={() => setSelected(null)}
          onSaved={() => {
            setSelected(null)
            void load()
          }}
        />
      )}
    </div>
  )
}

function Panel({
  upstream,
  onClose,
  onSaved,
}: {
  upstream: Upstream | null
  onClose: () => void
  onSaved: () => void
}) {
  const [slug, setSlug] = useState(upstream?.slug ?? '')
  const [name, setName] = useState(upstream?.name ?? '')
  const [url, setUrl] = useState(upstream?.url ?? '')
  const [fallbacks, setFallbacks] = useState((upstream?.fallbackUrls ?? []).join('\n'))
  const fallbackUrls = () => fallbacks.split(/[\s,]+/).filter(Boolean)
  const [enabled, setEnabled] = useState(upstream?.enabled ?? true)
  const [transport, setTransport] = useState<'streamable_http' | 'sse' | 'stdio_worker'>(upstream?.transport ?? 'streamable_http')
  // Headers: existing names are masked; only entries the user touches are sent.
  const [existing, setExisting] = useState<string[]>(upstream?.headerNames ?? [])
  const [replaced, setReplaced] = useState<Record<string, string>>({})
  const [removed, setRemoved] = useState<string[]>([])
  const [newName, setNewName] = useState('')
  const [newValue, setNewValue] = useState('')
  // TLS: the key is write-only; an empty key field keeps the stored one.
  const [caPem, setCaPem] = useState(upstream?.tls?.caPem ?? '')
  const [serverName, setServerName] = useState(upstream?.tls?.serverName ?? '')
  const [clientCert, setClientCert] = useState(upstream?.tls?.clientCertPem ?? '')
  const [clientKey, setClientKey] = useState('')
  const [authType, setAuthType] = useState<'none' | 'oauth_client_credentials'>(
    upstream?.auth?.type === 'oauth_client_credentials' ? 'oauth_client_credentials' : 'none',
  )
  const [tokenUrl, setTokenUrl] = useState(upstream?.auth?.tokenUrl ?? '')
  const [clientId, setClientId] = useState(upstream?.auth?.clientId ?? '')
  const [clientSecret, setClientSecret] = useState('')
  const [scopes, setScopes] = useState((upstream?.auth?.scopes ?? []).join(' '))
  const [audience, setAudience] = useState(upstream?.auth?.audience ?? '')

  const authInput = (): UpstreamAuthInput => {
    if (authType === 'none') {
      return { type: 'none' }
    }
    return {
      type: 'oauth_client_credentials',
      tokenUrl,
      clientId,
      ...(clientSecret && { clientSecret }),
      scopes: scopes.split(/[\s,]+/).filter(Boolean),
      audience: audience || undefined,
    }
  }

  const tlsInput = (): UpstreamTLSInput => ({
    caPem,
    serverName,
    clientCertPem: clientCert,
    ...(clientKey && { clientKeyPem: clientKey }),
  })
  const readFile = (set: (v: string) => void) => (e: ChangeEvent<HTMLInputElement>) => {
    e.target.files?.[0]?.text().then(set)
    e.target.value = ''
  }
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const [test, setTest] = useState<ConnectionTestResult | null>(null)
  const [tools, setTools] = useState<Tool[]>([])

  useEffect(() => {
    if (upstream) api.listTools(upstream.slug).then(setTools).catch(() => setTools([]))
  }, [upstream])

  const addHeader = () => {
    if (!newName.trim()) return
    setReplaced((r) => ({ ...r, [newName.trim()]: newValue }))
    if (!existing.includes(newName.trim())) setExisting((e) => [...e, newName.trim()])
    setNewName('')
    setNewValue('')
  }

  const [openedAt] = useState(() => Date.now())

  const save = async () => {
    setBusy(true)
    setErr('')
    try {
      const headers: Record<string, string | null> = { ...replaced }
      removed.forEach((h) => (headers[h] = null))
      if (upstream) {
        await api.patchUpstream(upstream.slug, {
          name,
          url,
          fallbackUrls: fallbackUrls(),
          transport,
          enabled,
          tls: tlsInput(),
          auth: authInput(),
          ...(Object.keys(headers).length && { headers }),
        },upstream.version)
      } else {
        await api.createUpstream(slug, {
          name: name || slug,
          url,
          fallbackUrls: fallbackUrls(),
          transport,
          enabled,
          headers: replaced,
          tls: tlsInput(),
          auth: authInput(),
        })
      }
      onSaved()
    } catch (e) {
      setErr(errMsg(e))
    } finally {
      setBusy(false)
    }
  }

  const runTest = async () => {
    setBusy(true)
    try {
      // Stored secrets are never readable, so the test uses only headers typed in this session.
      setTest(await api.testConnection({ url, transport, headers: replaced, tls: tlsInput(), auth: authInput() }))
    } catch (e) {
      setTest({ ok: false, error: errMsg(e) })
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal title={upstream ? upstream.slug : 'New upstream'} onClose={onClose} variant="drawer">
        {upstream && <button className={btn} disabled={busy} onClick={async()=>{setBusy(true);try{setTest(await api.testStoredConnection(upstream.slug))}catch(e){setErr(errMsg(e))}finally{setBusy(false)}}}>Test stored credentials</button>}
        <div className="space-y-4">
          {!upstream && (
            <label className="block text-sm font-medium">
              Slug
              <input className={clsx(input, 'mt-1')} value={slug} onChange={(e) => setSlug(e.target.value)} placeholder="github" />
            </label>
          )}
          <label className="block text-sm font-medium">
            Name
            <input className={clsx(input, 'mt-1')} value={name} onChange={(e) => setName(e.target.value)} />
          </label>
          <label className="block text-sm font-medium">
            URL
            <input className={clsx(input, 'mt-1')} value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://host/mcp" />
          </label>
          <label className="block text-sm font-medium">
            Fallback URLs <span className="font-normal text-ink-muted">(optional, one per line, tried in order)</span>
            <textarea className={clsx(input, 'mt-1 font-mono')} rows={2} value={fallbacks} onChange={(e) => setFallbacks(e.target.value)} />
          </label>
          {upstream && upstream.health?.nodes.length > 0 && (
            <div className="rounded-lg border border-line p-3 text-xs">
              <div className="mb-1 font-medium text-sm">Health by node</div>
              {upstream.health.nodes.map((n) => (
                <div key={n.node} className="flex items-center justify-between gap-2 py-0.5" title={n.error ?? n.url}>
                  <span className="truncate">{n.node}</span>
                  <span className="text-ink-muted">{n.latencyMs} ms · breaker {n.breaker.replace('_', ' ')}</span>
                  <span className={n.state === 'healthy' ? 'text-green-700' : 'text-red-700'}>{n.state}</span>
                </div>
              ))}
            </div>
          )}
          <label className="block text-sm font-medium">
            Transport
            <select className={clsx(input, 'mt-1')} value={transport} onChange={(e) => setTransport(e.target.value as 'streamable_http' | 'sse' | 'stdio_worker')}>
              <option value="streamable_http">Streamable HTTP</option>
              <option value="sse">Legacy SSE</option>
              <option value="stdio_worker">Isolated stdio worker (HTTPS/mTLS)</option>
            </select>
          </label>
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} /> Enabled
          </label>

          <details className="rounded-lg border border-line p-3" open={!!(caPem || serverName || clientCert)}>
            <summary className="cursor-pointer text-sm font-medium">TLS (custom CA, server name, mTLS)</summary>
            <div className="mt-3 space-y-3">
              <label className="block text-sm font-medium">
                CA certificate(s) <span className="font-normal text-ink-muted">(PEM; trusted only for this upstream)</span>
                <textarea className={clsx(input, 'mt-1 font-mono text-xs')} rows={4} value={caPem} onChange={(e) => setCaPem(e.target.value)} placeholder="-----BEGIN CERTIFICATE-----" />
              </label>
              <input type="file" accept=".pem,.crt,.cer" onChange={readFile(setCaPem)} className="text-xs" aria-label="Upload CA PEM" />
              {upstream && (upstream.tls?.caCerts?.length ?? 0) > 0 && caPem === upstream.tls.caPem && (
                <ul className="text-xs text-ink-muted">
                  {upstream.tls.caCerts.map((c, i) => (
                    <li key={i} className="truncate" title={c.subject}>
                      {c.subject} · expires {new Date(c.notAfter).toLocaleDateString()}
                      {Date.parse(c.notAfter) < openedAt && <span className="ml-1 text-red-700">expired</span>}
                    </li>
                  ))}
                </ul>
              )}
              <label className="block text-sm font-medium">
                Server name <span className="font-normal text-ink-muted">(optional; SNI and verification when reaching it by IP/alias)</span>
                <input className={clsx(input, 'mt-1')} value={serverName} onChange={(e) => setServerName(e.target.value)} placeholder="mcp.internal.example" />
              </label>
              <label className="block text-sm font-medium">
                Client certificate <span className="font-normal text-ink-muted">(optional, mTLS)</span>
                <textarea className={clsx(input, 'mt-1 font-mono text-xs')} rows={3} value={clientCert} onChange={(e) => setClientCert(e.target.value)} />
              </label>
              <input type="file" accept=".pem,.crt,.cer" onChange={readFile(setClientCert)} className="text-xs" aria-label="Upload client certificate" />
              <label className="block text-sm font-medium">
                Client key <span className="font-normal text-ink-muted">(write-only)</span>
                <textarea
                  className={clsx(input, 'mt-1 font-mono text-xs')}
                  rows={3}
                  value={clientKey}
                  onChange={(e) => setClientKey(e.target.value)}
                  placeholder={upstream?.tls?.hasClientKey ? '•••••••• stored — paste to replace' : '-----BEGIN PRIVATE KEY-----'}
                />
              </label>
              <p className="text-xs text-ink-muted">Certificate verification can't be disabled. Using the self-signed server certificate as the CA is fine.</p>
            </div>
          </details>

          <details className="rounded-lg border border-line p-3" open={authType === 'oauth_client_credentials'}>
            <summary className="cursor-pointer text-sm font-medium">OAuth 2.0 Client Credentials</summary>
            <div className="mt-3 space-y-3">
              <label className="flex items-center gap-2 text-sm font-medium">
                <input
                  type="checkbox"
                  checked={authType === 'oauth_client_credentials'}
                  onChange={(e) => setAuthType(e.target.checked ? 'oauth_client_credentials' : 'none')}
                />
                Enable OAuth client credentials
              </label>
              {authType === 'oauth_client_credentials' && (
                <>
                  <label className="block text-sm font-medium">
                    Token URL
                    <input
                      className={clsx(input, 'mt-1')}
                      value={tokenUrl}
                      onChange={(e) => setTokenUrl(e.target.value)}
                      placeholder="https://idp.example.com/oauth/token"
                    />
                  </label>
                  <label className="block text-sm font-medium">
                    Client ID
                    <input
                      className={clsx(input, 'mt-1')}
                      value={clientId}
                      onChange={(e) => setClientId(e.target.value)}
                    />
                  </label>
                  <label className="block text-sm font-medium">
                    Client Secret <span className="font-normal text-ink-muted">(write-only)</span>
                    <input
                      type="password"
                      className={clsx(input, 'mt-1')}
                      value={clientSecret}
                      onChange={(e) => setClientSecret(e.target.value)}
                      placeholder={upstream?.auth?.hasClientSecret ? '•••••••• stored — type to replace' : 'Secret'}
                    />
                  </label>
                  <label className="block text-sm font-medium">
                    Scopes <span className="font-normal text-ink-muted">(optional, space-separated)</span>
                    <input
                      className={clsx(input, 'mt-1 font-mono text-xs')}
                      value={scopes}
                      onChange={(e) => setScopes(e.target.value)}
                      placeholder="mcp.read mcp.tools"
                    />
                  </label>
                  <label className="block text-sm font-medium">
                    Audience <span className="font-normal text-ink-muted">(optional)</span>
                    <input
                      className={clsx(input, 'mt-1')}
                      value={audience}
                      onChange={(e) => setAudience(e.target.value)}
                      placeholder="fixture"
                    />
                  </label>
                  <p className="text-xs text-ink-muted">
                    The gateway fetches and caches access tokens from the token endpoint and automatically retries once on 401. Cannot be combined with a static Authorization header.
                  </p>
                </>
              )}
            </div>
          </details>

          <div>
            <div className="mb-2 text-sm font-medium">Headers (write-only)</div>
            <div className="space-y-2">
              {existing
                .filter((h) => !removed.includes(h))
                .map((h) => (
                  <div key={h} className="flex items-center gap-2 text-sm">
                    <span className="flex-1 truncate rounded-lg bg-canvas px-3 py-2">
                      {h} <span className="text-ink-muted">{replaced[h] !== undefined ? '(new value)' : '••••••••'}</span>
                    </span>
                    <button
                      className={btn}
                      onClick={() => {
                        const v = prompt(`New value for ${h}`)
                        if (v !== null) setReplaced((r) => ({ ...r, [h]: v }))
                      }}
                    >
                      Replace
                    </button>
                    <button className={btn} aria-label="Remove header" onClick={() => setRemoved((r) => [...r, h])}>
                      <X className="size-3.5" />
                    </button>
                  </div>
                ))}
              <div className="flex gap-2">
                <input className={input} placeholder="Header" value={newName} onChange={(e) => setNewName(e.target.value)} />
                <input className={input} type="password" placeholder="Value" value={newValue} onChange={(e) => setNewValue(e.target.value)} />
                <button className={btn} onClick={addHeader}>
                  Add
                </button>
              </div>
            </div>
          </div>

          <button className={clsx(btn, 'w-full')} disabled={busy || !url} onClick={runTest}>
            Test edited connection
          </button>
          {test && (
            <div className={clsx('flex items-start gap-2 text-sm', test.ok ? 'text-green-700' : 'text-red-700')}>
              {test.ok ? <CheckCircle2 className="size-4 shrink-0" /> : <XCircle className="size-4 shrink-0" />}
              {test.ok ? `Connected${test.toolCount != null ? ` · ${test.toolCount} tools` : ''}` : (test.error ?? 'Connection failed')}
            </div>
          )}
          {upstream && (
            <p className="text-xs text-ink-muted">
              The test only uses headers entered here; stored header values can't be read back.
            </p>
          )}

          {upstream && (
            <div>
              <div className="mb-2 text-sm font-medium">Discovered tools ({tools.length})</div>
              <ul className="max-h-60 divide-y divide-line overflow-y-auto rounded-lg border border-line">
                {tools.map((t) => (
                  <li key={t.name} className="px-3 py-2">
                    <div className="text-sm font-medium">{t.name}</div>
                    <div className="truncate text-xs text-ink-muted">{t.description}</div>
                  </li>
                ))}
                {tools.length === 0 && <li className="px-3 py-4 text-center text-xs text-ink-muted">Refresh tools to discover them.</li>}
              </ul>
            </div>
          )}
        </div>

        {err && <div className="mt-4 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>}
        <div className="mt-6 flex justify-end gap-2">
          <button className={btn} onClick={onClose}>
            Cancel
          </button>
          <button
            className="rounded-lg bg-accent-500 px-4 py-1.5 text-sm font-medium text-white hover:bg-accent-700 disabled:opacity-50"
            disabled={busy || !url || (!upstream && !slug)}
            onClick={save}
          >
            Save
          </button>
        </div>
    </Modal>
  )
}
