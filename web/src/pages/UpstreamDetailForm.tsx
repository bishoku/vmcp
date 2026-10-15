import { useNotifications } from '../notifications'
import { EmptyState } from '../Feedback'
import { source, useSources } from '../requests'
import { useCapabilities } from '../interaction'
import { useOperation, useUnsaved } from '../interaction'
import { useEffect, useState, type ChangeEvent } from 'react'
import { Activity, Cable, CheckCircle2, ChevronRight, Code2, Fingerprint, KeyRound, Layers3, Pencil, PlugZap, ShieldCheck, Wrench, X, XCircle } from 'lucide-react'
import clsx from 'clsx'
import { ErrorBox, Modal, Pill, StaleRecovery } from '../ui'
import { api, ApiError, type Catalog, type ConnectionTestResult, type Tool, type Upstream, type UpstreamTLSInput, type UpstreamAuthInput } from '../api'

import EditorSaveBar from '../connections/EditorSaveBar'
import UpstreamDiscovery from '../connections/UpstreamDiscovery'
import { DetailRow, ServerPanel, SummaryTile } from '../connections/ServerPanels'
import { fmt } from '../format'
import { btn, input, primary } from '../styles'

const errMsg = (e: unknown) => (e instanceof ApiError ? e.message : String(e))


export default function UpstreamDetailForm({
  upstream,
  onClose,
  onSaved,
  onReload,
}: {
  upstream: Upstream | null
  onClose: () => void
  onReload: (upstream: Upstream) => void
  onSaved: (saved: Upstream) => void
}) {
  const { edit: canEdit } = useCapabilities()
  const [editing, setEditing] = useState(!upstream)
  const editable = canEdit && editing
  const [slug, setSlug] = useState(upstream?.slug ?? '')
  const [name, setName] = useState(upstream?.name ?? '')
  const [url, setUrl] = useState(upstream?.url ?? '')
  const [fallbacks, setFallbacks] = useState((upstream?.fallbackUrls ?? []).join('\n'))
  const fallbackUrls = () => fallbacks.split(/[\s,]+/).filter(Boolean)
  const [enabled, setEnabled] = useState(upstream?.enabled ?? true)
  const [transport, setTransport] = useState<'streamable_http' | 'sse'>(upstream?.transport ?? 'streamable_http')
  // Headers: existing names are masked; only entries the user touches are sent.
  const [existing, setExisting] = useState<string[]>(upstream?.headerNames ?? [])
  const [replaced, setReplaced] = useState<Record<string, string>>({})
  const [removed, setRemoved] = useState<string[]>([])
  const [newName, setNewName] = useState('')
  const [newValue, setNewValue] = useState('')
  const [headerToReplace,setHeaderToReplace]=useState<string|null>(null)
  const [headerReplacement,setHeaderReplacement]=useState('')
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
  const unsaved = useUnsaved({slug,name,url,fallbacks,enabled,transport,replaced,removed,newName,newValue,headerReplacement,caPem,serverName,clientCert,clientKey,authType,tokenUrl,clientId,clientSecret,scopes,audience})
  const close = () => { if (!busy) onClose() }
  const [test, setTest] = useState<ConnectionTestResult | null>(null)
  const [tools, setTools] = useState<Tool[]>([])

  const [catalog,setCatalog] = useState<Catalog|null>(null)
  const {load:loadDiscovery,loading:discoveryLoading,readError:discoveryError} = useSources(()=>upstream ? [
    source('Tools',signal=>api.listTools(upstream.slug,signal),setTools),
    source('Catalog',signal=>api.listCatalog(upstream.slug,signal),setCatalog),
  ] : [])
  useEffect(()=>{const timer=setTimeout(()=>{void loadDiscovery()},0);return()=>clearTimeout(timer)},[loadDiscovery])

  const addHeader = () => {
    if (!newName.trim()) return
    setReplaced((r) => ({ ...r, [newName.trim()]: newValue }))
    if (!existing.includes(newName.trim())) setExisting((e) => [...e, newName.trim()])
    setNewName('')
    setNewValue('')
  }

  const [openedAt] = useState(() => Date.now())

  const {notify}=useNotifications()
  const save = async () => {
    let saved: Upstream | undefined
    await run(async () => {
      if (!editable) return
      setErr('')
      try {
        const headers: Record<string, string | null> = { ...replaced }
        removed.forEach((h) => (headers[h] = null))
        if (upstream) {
          saved = await api.patchUpstream(upstream.slug, {
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
          saved = await api.createUpstream(slug, {
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
      } catch (e) {
        setErr(errMsg(e))
      }
    })
    if (saved) { notify({title:upstream?'Upstream saved':'Upstream created',description:saved.name}); onSaved(saved) }
  }

  const runTest = () => run(async () => {
    try {
      // Stored secrets are never readable, so the test uses only headers typed in this session.
      const result=await api.testConnection({ url, transport, headers: replaced, tls: tlsInput(), auth: authInput() })
      setTest(result)
      if(result.ok)notify({title:'Connection test passed',description:'The supplied upstream settings were verified.'})
    } catch (e) {
      setTest({ ok: false, error: errMsg(e) })
    }
  })

  return <div className="mx-auto max-w-7xl space-y-6 px-4 py-6 sm:px-6 lg:px-10 lg:py-10">
    <header className="space-y-5">
      <div className="flex items-center gap-2 text-xs text-ink-muted"><button className="hover:text-accent-700" onClick={close}>Back to upstreams</button><ChevronRight className="size-3"/><span className="font-mono">{upstream?.slug || 'new'}</span></div>
      <div className="flex flex-wrap items-start justify-between gap-5"><div className="flex min-w-0 items-start gap-4"><div className="flex size-12 shrink-0 items-center justify-center rounded-xl border border-accent-500/15 bg-accent-50 text-accent-700"><Layers3 className="size-6"/></div><div className="min-w-0"><div className="flex flex-wrap items-center gap-3"><h1 className="break-words text-2xl font-semibold tracking-tight">{upstream ? upstream.name : 'New upstream'}</h1><Pill status={enabled?'enabled':'disabled'}/>{upstream && <span className="rounded-md border border-line bg-surface px-2 py-0.5 text-[11px] font-medium text-ink-muted">v{upstream.version}</span>}</div><p className="mt-1.5 max-w-2xl text-sm text-ink-muted">{upstream?.description || 'An upstream MCP server for your virtual endpoints.'}</p></div></div>
      <div className="flex flex-wrap gap-2">{!editing && canEdit && <button className={primary} onClick={()=>setEditing(true)}><Pencil className="size-3.5"/>Edit upstream</button>}</div></div>
      <p role="status" className="text-xs text-ink-muted">{editing ? 'Editing configuration. Changes apply only after Save.' : canEdit ? 'Read-only configuration. Choose Edit to make changes.' : 'Read-only configuration. Editor access is required to make changes.'}</p>
    </header>
    {editing && <EditorSaveBar status={upstream?`Editing draft of saved version ${upstream.version}`:'New upstream draft'} detail={newName||newValue?'Add or clear the pending header before saving.':!url||(!upstream&&!slug)?'Enter an identifier and connection URL before saving.':'Changes apply only after Save. Stored secrets are kept unless explicitly replaced.'} busy={busy} busyLabel="Working…" disabled={!editable||!url||(!upstream&&!slug)||!!(newName||newValue)} onSave={()=>void save()} onCancel={()=>upstream?unsaved.guard(()=>{unsaved.clean();setEditing(false);onReload(upstream)}):close()}/>}
    {err && <ErrorBox>{err}</ErrorBox>}
    <StaleRecovery message={err} reload={async()=>{
      const latest=(await api.listUpstreams()).find(item=>item.slug===upstream?.slug)
      if(!latest)throw new Error('This upstream was deleted. Cancel the draft and refresh the list.')
      unsaved.clean();onReload(latest)
    }}/>
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <SummaryTile label="Transport" value={transport==='sse'?'Legacy SSE':'Streamable HTTP'} detail={editing?'Current draft transport':'Saved endpoint transport'} icon={Cable}/>
      <SummaryTile label="Authentication" value={authType==='none'?(existing.some(h=>!removed.includes(h))?'Headers':'None'):'OAuth 2.0'} detail={authType==='none'?`${existing.filter(h=>!removed.includes(h)).length} configured headers`:'Client credentials grant'} icon={KeyRound}/>
      <SummaryTile label="Discovered tools" value={upstream?.toolCount ?? '—'} detail={upstream?'Last saved discovery count':'Available after saving & discovery'} icon={Wrench}/>
      <SummaryTile label="Recorded health" value={upstream?.enabled===false?'Disabled':upstream?.health?.state ?? 'Unknown'} detail="Stored observation · not a live check" icon={Activity}/>
    </div>
    <div className="grid items-start gap-6 xl:grid-cols-[1.4fr_1fr]">
      <ServerPanel title="Connection settings" description={editing?'Edit the endpoint and transport for this upstream':'Saved endpoint and routing configuration'} icon={Cable}>
        <fieldset disabled={!editable || busy} className="min-w-0 space-y-4 border-0 p-5 m-0">
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
          <label className="block text-sm font-medium">
            Transport
            <select className={clsx(input, 'mt-1')} value={transport} onChange={(e) => setTransport(e.target.value as 'streamable_http' | 'sse')}>
              <option value="streamable_http">Streamable HTTP</option>
              <option value="sse">Legacy SSE</option>
            </select>
          </label>
          <label className="flex min-w-0 flex-wrap items-center gap-2 text-sm">
            <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} /> Enabled
          </label>

        </fieldset>
      </ServerPanel>
      <div className="min-w-0 space-y-6">
        <ServerPanel title="Connection verification" description="Explicit tests for saved or edited settings" icon={PlugZap} footer="A test checks the gateway-to-upstream connection. It does not verify a client’s access to virtual servers.">
          <div className="space-y-4 p-5">
            {upstream && <div><button className={btn} disabled={busy || !canEdit} onClick={()=>run(async()=>{setErr('');try{const result=await api.testStoredConnection(upstream.slug);setTest(result);if(result.ok)notify({title:'Connection test passed',description:'The saved upstream settings were verified.'})}catch(e){setErr(errMsg(e))}})}>Test stored credentials</button><p className="mt-2 text-xs leading-relaxed text-ink-muted">Uses the saved upstream settings and stored credentials.</p></div>}
            {editing && <div className={upstream?'border-t border-line pt-4':''}><button className={btn} disabled={!editable || busy || !url} onClick={runTest}>Test edited connection</button><p className="mt-2 text-xs leading-relaxed text-ink-muted">The edited test only uses headers entered here; stored header values cannot be read back.</p></div>}
            {!canEdit && <p className="text-xs text-ink-muted">Connection tests require editor access.</p>}
            {busy && <p className="text-xs text-ink-muted">An operation is in progress…</p>}
            {test && <div role="status" className={clsx('flex items-start gap-2 rounded-lg border p-3 text-xs leading-relaxed',test.ok?'border-green-200 bg-green-50 text-green-700':'border-red-200 bg-red-50 text-red-700')}>{test.ok?<CheckCircle2 className="mt-0.5 size-4 shrink-0"/>:<XCircle className="mt-0.5 size-4 shrink-0"/>}<span>{test.ok?`Connected${test.toolCount!=null?` · ${test.toolCount} tools`:''}`:test.error ?? 'Connection failed'}</span></div>}
          </div>
        </ServerPanel>
        {upstream && <ServerPanel title="Recorded node health" description="Last received health and discovery observations" icon={Activity} footer="Stored observations may be stale. Use an explicit connection test to check connectivity.">
          <dl className="divide-y divide-line px-5"><DetailRow label="Health"><Pill status={upstream.enabled?upstream.health?.state ?? 'unknown':'disabled'}/></DetailRow><DetailRow label="Last discovery">{fmt(upstream.lastRefreshAt)}</DetailRow></dl>
          {upstream.lastRefreshError && <p className="mx-5 mb-4 break-words rounded-lg bg-red-50 p-3 text-xs text-red-700">{upstream.lastRefreshError}</p>}
          <div className="divide-y divide-line border-t border-line">{(upstream.health?.nodes ?? []).map(node=><details key={node.node} className="px-5 py-4"><summary className="cursor-pointer text-xs"><span className="font-medium">{node.node}</span><span className="ml-2 text-ink-muted">{node.state} · {node.latencyMs} ms</span></summary><dl className="mt-2 divide-y divide-line"><DetailRow label="Checked">{fmt(node.checkedAt)}</DetailRow><DetailRow label="Circuit breaker">{node.breaker.replaceAll('_',' ')}</DetailRow><DetailRow label="URL"><span className="break-all font-mono">{node.url}</span></DetailRow></dl>{node.error && <p className="mt-2 break-words text-xs text-red-700">{node.error}</p>}</details>)}{!upstream.health?.nodes?.length && <EmptyState title="No node health observations available." compact/>}</div>
        </ServerPanel>}
      </div>
    </div>
    <div><h2 className="text-lg font-semibold tracking-tight">Security & credentials</h2><p className="mt-1 text-sm text-ink-muted">Transport trust, upstream authentication and write-only secrets.</p></div>
    <div className="grid items-start gap-6 xl:grid-cols-2">
      <ServerPanel title="Transport security" description="Custom trust and mutual TLS settings" icon={ShieldCheck}>
        <fieldset disabled={!editable || busy} className="min-w-0 border-0 p-5 m-0">
          <>{!editing && !(caPem || serverName || clientCert) && <p className="mb-3 text-xs leading-relaxed text-ink-muted">Uses system trust. No custom CA, server name or client certificate is configured.</p>}<details className="min-w-0" open={!!(caPem || serverName || clientCert)}>
            <summary className="cursor-pointer text-sm font-medium">TLS (custom CA, server name, mTLS)</summary>
            <div className="mt-3 space-y-3">
              <label className="block text-sm font-medium">
                CA certificate(s) <span className="font-normal text-ink-muted">(PEM; trusted only for this upstream)</span>
                <textarea className={clsx(input, 'mt-1 font-mono text-xs')} rows={4} value={caPem} onChange={(e) => setCaPem(e.target.value)} placeholder="-----BEGIN CERTIFICATE-----" />
              </label>
              <input type="file" accept=".pem,.crt,.cer" onChange={readFile(setCaPem)} className="max-w-full text-xs" aria-label="Upload CA PEM" />
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
              <input type="file" accept=".pem,.crt,.cer" onChange={readFile(setClientCert)} className="max-w-full text-xs" aria-label="Upload client certificate" />
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
          </details></>

        </fieldset>
      </ServerPanel>
      <ServerPanel title="OAuth authentication" description="Token endpoint and client credentials" icon={Fingerprint}>
        <fieldset disabled={!editable || busy} className="min-w-0 border-0 p-5 m-0">
          <details className="min-w-0" open={authType === 'oauth_client_credentials'}>
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

        </fieldset>
      </ServerPanel>
    </div>
    <ServerPanel title="Request headers" description="Stored values remain masked; only explicit changes are sent" icon={Code2} footer="Header values are write-only. Replacing or removing a header takes effect after Save.">
      {editing ? (
      <fieldset disabled={!editable || busy} className="min-w-0 border-0 p-5 m-0">
          <div>

            <div className="space-y-2">
              {existing
                .filter((h) => !removed.includes(h))
                .map((h) => (
                  <div key={h} className="flex min-w-0 flex-wrap items-center gap-2 text-sm">
                    <span className="min-w-0 flex-1 truncate rounded-lg bg-canvas px-3 py-2">
                      {h} <span className="text-ink-muted">{replaced[h] !== undefined ? '(new value)' : '••••••••'}</span>
                    </span>
                    <button
                      className={btn}
                      onClick={() => {
                        setHeaderToReplace(h);setHeaderReplacement('')
                      }}
                    >
                      Replace
                    </button>
                    <button className={btn} aria-label="Remove header" onClick={() => setRemoved((r) => [...r, h])}>
                      <X className="size-3.5" />
                    </button>
                  </div>
                ))}
              <div className="flex flex-col gap-2 sm:flex-row">
                <input className={input} aria-label="New header name" placeholder="Header" value={newName} onChange={(e) => setNewName(e.target.value)} />
                <input className={input} type="password" aria-label="New header value" placeholder="Value" value={newValue} onChange={(e) => setNewValue(e.target.value)} />
                <button className={btn} disabled={!newName.trim()} onClick={addHeader}>
                  Add
                </button>
              </div>
            </div>
          </div>

      </fieldset>
      ) : <div className="divide-y divide-line px-5">{existing.length?existing.map(header=><div key={header} className="flex min-w-0 items-center justify-between gap-4 py-4"><span className="min-w-0 break-all font-mono text-xs">{header}</span><span className="shrink-0 text-xs text-ink-muted">Value stored · masked</span></div>):<EmptyState title="No request headers configured." compact/>}</div>}
    </ServerPanel>
    {headerToReplace&&<Modal title={`Replace ${headerToReplace}`} description="The new header value stays in this draft until you save the upstream." onClose={()=>{setHeaderToReplace(null);setHeaderReplacement('')}}><div className="space-y-5"><label className="block text-xs font-medium">Replacement header value<input type="password" autoComplete="off" className={`${input} mt-2`} value={headerReplacement} onChange={e=>setHeaderReplacement(e.target.value)}/></label><div className="flex flex-wrap justify-end gap-3"><button className={btn} onClick={()=>{setHeaderToReplace(null);setHeaderReplacement('')}}>Cancel</button><button className={primary} onClick={()=>{setReplaced(r=>({...r,[headerToReplace]:headerReplacement}));setHeaderToReplace(null);setHeaderReplacement('')}}>Use replacement</button></div></div></Modal>}
    {editing && !upstream && <ServerPanel title="Capability discovery" description="Discover tools, resources and prompts after saving the upstream" icon={Wrench}><div className="space-y-2 p-5 text-xs leading-relaxed text-ink-muted"><p>Save the connection settings, test stored credentials, then refresh discovery from the upstream list.</p><p>Capability selections and aliases are configured separately in a virtual server.</p></div></ServerPanel>}
    {upstream && <UpstreamDiscovery tools={tools} catalog={catalog} loading={discoveryLoading} error={discoveryError} reload={loadDiscovery}/>}
  </div>
}
