import Button from '../Button'
import { FormField } from '../FormField'
import { useNotifications } from '../notifications'
import { Cable, CheckCircle2, Code2, Globe, KeyRound, PlugZap, ShieldCheck } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import type { VirtualServer } from '../api'
import { connectionEndpoint } from '../endpoint'
import { btn, input, primary } from '../styles'
import { CopyButton, ErrorBox } from '../ui'
import { useOperation } from '../interaction'
import { connectionSettings } from './model'
import { DetailRow, ServerPanel, SummaryTile } from './ServerPanels'
import type { VerificationResult } from './verify'

export default function Connect({server,showSummary=true}: {server:VirtualServer;showSummary?:boolean}) {
  const {notify}=useNotifications()
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
      if(!control.signal.aborted){setResult(result);setVerifiedAt(new Date().toLocaleString());notify({title:'Connection verified',description:'Gateway initialization and capability listing completed.'})}
    } catch(e) {
      setError(control.signal.aborted ? 'Verification cancelled or timed out. No tool was invoked.' : `Verification failed: ${e instanceof Error ? e.message : String(e)}. Check gateway URL, credential, network access and allowed browser Origin/CORS configuration.`)
    } finally { clearTimeout(timeout);controller.current=null }
  })
  return <section aria-label="Client connection" className="space-y-6">
    <div><h2 className="section-title">Connect a client</h2><p className="section-description mt-1 text-sm text-ink-muted">Copy your endpoint, configure a client and verify discovery with a separate gateway credential.</p></div>
    {showSummary && <div className="grid gap-3 sm:grid-cols-3"><SummaryTile label="Transport" value="Streamable HTTP" detail="Saved MCP endpoint" icon={Cable}/><SummaryTile label="Authentication" value={server.authRequired?'Required':'Anonymous'} detail={server.authRequired?'Agent key or OAuth access token':'Credentials are bypassed'} icon={KeyRound}/><SummaryTile label="Server configuration" value={server.enabled?'Enabled':'Disabled'} detail={endpoint?'Public gateway URL configured':'Public gateway URL not configured'} icon={Globe}/></div>}
    <ServerPanel headingLevel={3} title="Gateway endpoint" description="Streamable HTTP endpoint from the saved server configuration" icon={Cable}>
      <div className="flex items-center gap-3 px-5 py-5"><code className="min-w-0 flex-1 break-all rounded-lg bg-canvas px-3 py-3 text-xs">{endpoint ?? server.endpoint}</code><CopyButton label="Copy endpoint" value={endpoint}/></div>
      {!endpoint && <div className="px-5 pb-5"><ErrorBox>Public gateway URL is not configured. Configure the public gateway URL in the deployment; the path alone is not a client connection URL.</ErrorBox></div>}
      {!server.authRequired && <p className="mx-5 mb-5 rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800">Anonymous access is enabled. Credentials are bypassed on this server.</p>}
    </ServerPanel>
    <div className="grid items-start gap-6 xl:grid-cols-2">
      <ServerPanel headingLevel={3} title="Client configuration" description="HTTP settings to adapt to your client’s configuration format" icon={Code2} footer="Use an agent key or verified OAuth access token, never an admin API token.">
        <div className="space-y-4 p-5"><p className="text-xs leading-relaxed text-ink-muted">{server.authRequired?'Replace the credential placeholder before connecting.':'This endpoint accepts anonymous requests; no authorization header is included.'}</p>{settings?<><pre className="overflow-x-auto rounded-lg border border-line bg-canvas p-4 text-xs leading-relaxed">{settings}</pre><CopyButton label="Copy client settings" value={settings}/></>:<p className="rounded-lg bg-canvas p-4 text-xs text-ink-muted">Client settings become available once the public gateway URL is configured.</p>}</div>
      </ServerPanel>
      <ServerPanel headingLevel={3} title="Verify this connection" description="Check initialization, ping and negotiated discovery" icon={PlugZap} footer="The browser must reach the gateway and its Origin must be allowed. The verification session has no sampling, roots or elicitation permissions.">
        <div className="space-y-4 p-5"><p className="text-xs leading-relaxed text-ink-muted">Only initialize, ping and list negotiated tools, resources, templates and prompts. No tools are called, resources read, or prompts executed. Results cover the first page of each list.</p>
          {server.authRequired && <FormField name="credential" label="Agent key or OAuth access token" required hint="Use a credential with access to this server; admin API tokens cannot be used here."><input className={`${input} mt-2`} autoComplete="off" type="password" value={credential} disabled={busy} onChange={e=>setCredential(e.target.value)}/></FormField>}
          <div className="flex items-start gap-2 rounded-lg bg-canvas p-3 text-xs leading-relaxed text-ink-muted"><ShieldCheck className="mt-0.5 size-4 shrink-0"/><p>The credential stays in memory and is cleared from the input when verification starts. It is sent only to the configured gateway.</p></div>
          <div className="flex flex-wrap items-center gap-3"><Button loading={busy} className={primary} disabled={busy || !endpoint || !server.enabled || (server.authRequired && !credential.trim())} onClick={verify}><PlugZap className="size-3.5"/>Verify connection</Button>{busy && <><span role="status" className="text-xs text-ink-muted">Verifying…</span><button className={btn} onClick={()=>controller.current?.abort()}>Cancel verification</button></>}</div>
          {!server.enabled && <p className="text-xs text-amber-700">Enable this virtual server before verifying.</p>}
          {error && <ErrorBox>{error}</ErrorBox>}
          {result && <div role="status" className="rounded-xl border border-green-200 bg-green-50 p-4"><div className="flex items-start gap-2"><CheckCircle2 className="mt-0.5 size-4 shrink-0 text-green-700"/><p className="text-xs font-medium text-green-800">Verified initialization and ping with {result.server} at {verifiedAt}.</p></div><dl className="mt-3 divide-y divide-green-200">{result.lists.map(list=><DetailRow key={list.kind} label={list.kind}>{list.count} on the first page{list.more?' (more pages available)':''}</DetailRow>)}</dl><p className="mt-3 text-xs leading-relaxed text-green-800">This verifies discovery for the supplied identity, not permission to execute every capability.</p></div>}
        </div>
      </ServerPanel>
    </div>
  </section>
}
