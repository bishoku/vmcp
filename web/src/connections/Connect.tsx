import { useEffect, useRef, useState } from 'react'
import type { VirtualServer } from '../api'
import { connectionEndpoint } from '../endpoint'
import { btn, input, primary } from '../styles'
import { CopyButton, ErrorBox } from '../ui'
import { useOperation } from '../interaction'
import { connectionSettings } from './model'
import type { VerificationResult } from './verify'

export default function Connect({server}: {server:VirtualServer}) {
  const endpoint = connectionEndpoint(server.endpoint)
  const settings = endpoint ? connectionSettings(server,endpoint) : null
  const [credential,setCredential] = useState('')
  const [result,setResult] = useState<VerificationResult|null>(null)
  const [verifiedAt,setVerifiedAt] = useState('')
  const [error,setError] = useState('')
  const controller = useRef<AbortController|null>(null)
  const {busy,run} = useOperation()
  useEffect(()=>()=>controller.current?.abort(),[])
  const verify = () => run(async()=>{
    const control = new AbortController()
    controller.current = control
    const timeout = setTimeout(()=>control.abort(),20_000)
    setError('');setResult(null)
    const secret=credential.trim()
    setCredential('')
    try {
      if(!endpoint)throw new Error('Public gateway URL is not configured.')
      const {verifyConnection}=await import('./verify')
      const result=await verifyConnection(endpoint,secret,control.signal)
      if(!control.signal.aborted){setResult(result);setVerifiedAt(new Date().toLocaleString())}
    } catch(e) {
      setError(control.signal.aborted ? 'Verification cancelled or timed out. No tool was invoked.' : `Verification failed: ${e instanceof Error ? e.message : String(e)}. Check gateway URL, credential, network access and allowed browser Origin/CORS configuration.`)
    } finally { clearTimeout(timeout);controller.current=null }
  })
  return <section className="space-y-4"><h2 className="text-lg font-semibold">Connect a client</h2>
    <p className="text-sm">Streamable HTTP endpoint from the saved server configuration:</p><div className="flex items-center gap-3 rounded-lg bg-canvas p-3"><code className="min-w-0 flex-1 break-all">{endpoint ?? server.endpoint}</code><CopyButton label="Copy endpoint" value={endpoint}/></div>
    {!endpoint && <ErrorBox>Public gateway URL is not configured. Set VITE_PUBLIC_GATEWAY_URL and rebuild the UI; the path alone is not a client connection URL.</ErrorBox>}
    {!server.authRequired && <p className="text-amber-700">Anonymous access is enabled. Credentials are bypassed on this server.</p>}
    <p className="text-sm text-ink-muted">HTTP client settings: adapt this example to your client's configuration format. Replace the credential placeholder with an agent key or verified OAuth access token, never an admin API token.</p>
    {settings && <><pre className="overflow-x-auto rounded-lg bg-canvas p-4 text-xs">{settings}</pre><CopyButton label="Copy client settings" value={settings}/></>}
    <section className="space-y-3 rounded-lg border border-line p-4"><h3 className="font-semibold">Verify this connection</h3>
      <p className="text-sm">Only initialize, ping and list negotiated tools, resources, templates and prompts. No tools are called, resources read, or prompts executed. Results cover the first page of each list.</p>
      <p className="text-xs text-ink-muted">The browser must reach the gateway and its Origin must be allowed. Verification uses a separate MCP session with no sampling, roots or elicitation permissions.</p>
      {server.authRequired && <label className="block text-sm">Agent key or OAuth access token<input className={input} autoComplete="off" type="password" value={credential} disabled={busy} onChange={e=>setCredential(e.target.value)}/></label>}
      <p className="text-xs text-ink-muted">The credential stays in memory and is cleared from the input when verification starts. It is sent only to the configured gateway.</p>
      <button className={primary} disabled={busy || !endpoint || !server.enabled || (server.authRequired && !credential.trim())} onClick={verify}>Verify connection</button>
      {busy && <><span role="status" className="ml-3 text-sm">Verifying…</span><button className={btn} onClick={()=>controller.current?.abort()}>Cancel verification</button></>}
      {!server.enabled && <p className="text-amber-700">Enable this virtual server before verifying.</p>}
      {error && <ErrorBox>{error}</ErrorBox>}
      {result && <div role="status"><p>Verified initialization and ping with {result.server} at {verifiedAt}.</p><ul>{result.lists.map(list=><li key={list.kind}>{list.count} {list.kind} on the first page{list.more ? ' (more pages available)' : ''}</li>)}</ul><p className="text-sm">This verifies discovery for the supplied identity, not permission to execute every capability.</p></div>}
    </section>
  </section>
}
