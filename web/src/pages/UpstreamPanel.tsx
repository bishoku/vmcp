import { useCapabilities } from '../interaction'
import { useOperation, useUnsaved } from '../interaction'
import { useEffect, useState, type ChangeEvent } from 'react'
import { CheckCircle2, X, XCircle } from 'lucide-react'
import clsx from 'clsx'
import { Modal, StaleRecovery } from '../ui'
import { api, ApiError, type ConnectionTestResult, type Tool, type Upstream, type UpstreamTLSInput, type UpstreamAuthInput } from '../api'

import { btn, input } from '../styles'

const errMsg = (e: unknown) => (e instanceof ApiError ? e.message : String(e))


export default function UpstreamPanel({
  upstream,
  onClose,
  onSaved,
  onReload,
}: {
  upstream: Upstream | null
  onClose: () => void
  onReload: (upstream: Upstream) => void
  onSaved: () => void
}) {
  const { edit: canEdit } = useCapabilities()
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
  const { busy, run } = useOperation()
  const unsaved = useUnsaved({slug,name,url,fallbacks,enabled,transport,replaced,removed,newName,newValue,caPem,serverName,clientCert,clientKey,authType,tokenUrl,clientId,clientSecret,scopes,audience})
  const close = () => { if (!busy) unsaved.guard(onClose) }
  const [test, setTest] = useState<ConnectionTestResult | null>(null)
  const [tools, setTools] = useState<Tool[]>([])

  useEffect(() => {
    if (upstream) api.listTools(upstream.slug).then(setTools).catch(e => setErr(`Tool discovery unavailable: ${errMsg(e)}`))
  }, [upstream])

  const addHeader = () => {
    if (!newName.trim()) return
    setReplaced((r) => ({ ...r, [newName.trim()]: newValue }))
    if (!existing.includes(newName.trim())) setExisting((e) => [...e, newName.trim()])
    setNewName('')
    setNewValue('')
  }

  const [openedAt] = useState(() => Date.now())

  const save = () => run(async () => {
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
      unsaved.clean()
      onSaved()
    } catch (e) {
      setErr(errMsg(e))
    }
  })

  const runTest = () => run(async () => {
    try {
      // Stored secrets are never readable, so the test uses only headers typed in this session.
      setTest(await api.testConnection({ url, transport, headers: replaced, tls: tlsInput(), auth: authInput() }))
    } catch (e) {
      setTest({ ok: false, error: errMsg(e) })
    }
  })

  return (
    <Modal readOnly={!canEdit} busy={busy} title={upstream ? upstream.slug : 'New upstream'} onClose={close} variant="drawer">
        <StaleRecovery message={err} reload={async()=>{
          const latest=(await api.listUpstreams()).find(item=>item.slug===upstream?.slug)
          if(!latest)throw new Error('This upstream was deleted. Cancel the draft and refresh the list.')
          unsaved.clean();onReload(latest)
        }}/>
        {upstream && <button className={btn} disabled={busy} onClick={()=>run(async()=>{try{setTest(await api.testStoredConnection(upstream.slug))}catch(e){setErr(errMsg(e))}})}>Test stored credentials</button>}
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
          <button className={btn} onClick={close}>
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
