import { Link } from 'react-router-dom'
import { ArrowRight, Fingerprint, Gauge, ScanEye, ShieldCheck, SlidersHorizontal, Wrench } from 'lucide-react'
import type { VirtualServer } from '../api'
import { Pill } from '../ui'
import { serverPath } from './model'
import { DetailRow, ServerPanel, SummaryTile } from './ServerPanels'

const features=[
  {key:'sampling',label:'Sampling',description:'Allow the server to request model completions from clients.'},
  {key:'roots',label:'Roots',description:'Allow the server to request the client’s shared root locations.'},
  {key:'elicitation',label:'Elicitation',description:'Allow the server to request additional input from clients.'},
] as const

export default function ServerProtection({server}:{server:VirtualServer}) {
  const policy=server.policy?.default ?? 'allow'
  const rate=server.rateLimitPerMinute
  const pii=server.pii
  const detectors=pii?.detectors ?? []
  const rules=pii?.customRules ?? []
  const enabledFeatures=features.filter(f=>server.clientFeatures?.[f.key]).length
  const overrides=Object.entries(server.policy?.tools ?? {})
  return <section aria-label="Server protection" className="space-y-6">
    <div><h2 className="text-lg font-semibold tracking-tight">Protection</h2><p className="mt-1 text-sm text-ink-muted">Tool access, data masking and client permissions for this endpoint.</p></div>
    <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
      <SummaryTile label="Default tool policy" value={policy.replace('_',' ')} detail="Per-tool overrides take precedence" icon={Wrench}/>
      <SummaryTile label="Server rate limit" value={rate==null?'Unlimited':`${rate} / min`} detail="Credential limits may also apply" icon={Gauge}/>
      <SummaryTile label="PII masking" value={pii?.enabled?'Enabled':'Disabled'} detail={pii?.maskArguments?'Responses & tool arguments':'Tool responses when enabled'} icon={ScanEye}/>
      <SummaryTile label="Client permissions" value={`${enabledFeatures} of 3`} detail="Subject to negotiation & approval" icon={Fingerprint}/>
    </div>
    <div className="grid gap-6 xl:grid-cols-2">
      <ServerPanel title="Tool access & limits" description="Default behavior and explicit policy overrides" icon={ShieldCheck} footer={<Link to={serverPath(server.slug,'capabilities')} className="flex items-center justify-between font-medium text-accent-700">Review per-tool capabilities<ArrowRight className="size-3.5"/></Link>}>
        <dl className="divide-y divide-line px-5"><DetailRow label="Default tool policy"><span className="rounded-md bg-canvas px-2 py-1 font-mono">{policy}</span></DetailRow><DetailRow label="Server rate limit">{rate==null?'Unlimited':`${rate} calls / minute`}</DetailRow><DetailRow label="Per-tool overrides">{overrides.length} configured</DetailRow></dl>
        {overrides.length>0 && <div className="mx-5 mb-4 max-h-44 overflow-auto rounded-lg border border-line divide-y divide-line">{overrides.map(([tool,mode])=><div key={tool} className="flex items-start justify-between gap-3 px-3 py-2.5 text-xs"><span className="min-w-0 break-all font-mono text-ink-muted">{tool}</span><span className="shrink-0 font-medium">{mode.replace('_',' ')}</span></div>)}</div>}
        <p className="px-5 pb-4 text-xs leading-relaxed text-ink-muted">Read-only policy relies on upstream annotations. Credential-specific rate limits can further restrict requests.</p>
      </ServerPanel>
      <ServerPanel title="Data masking" description="Built-in detectors and custom redaction rules" icon={ScanEye} footer="Masking does not guarantee that every sensitive value is detected.">
        <dl className="divide-y divide-line px-5"><DetailRow label="PII masking"><Pill status={pii?.enabled?'enabled':'disabled'}/></DetailRow><DetailRow label="Argument masking"><Pill status={pii?.maskArguments?'enabled':'disabled'}/></DetailRow><DetailRow label="Detectors">{detectors.length?detectors.join(', '):'all built-in (default)'}</DetailRow><DetailRow label="Custom rules">{rules.length?`${rules.length} configured`:'none'}</DetailRow></dl>
        {rules.length>0 && <div className="flex flex-wrap gap-2 px-5 pb-5">{rules.map((rule,i)=><span key={i} className="max-w-full break-all rounded-md border border-line bg-canvas px-2 py-1 font-mono text-xs">{rule.name}</span>)}</div>}
        {!pii?.enabled && <p className="px-5 pb-4 text-xs text-ink-muted">Masking is disabled. Saved detector and argument settings take effect when masking is enabled.</p>}
      </ServerPanel>
    </div>
    <ServerPanel title="Client feature permissions" description="Capabilities this virtual server may request from connected clients" icon={SlidersHorizontal} footer="Client negotiation and approval still apply to enabled permissions.">
      <div className="grid divide-y divide-line lg:grid-cols-3 lg:divide-y-0 lg:divide-x">{features.map(f=><div key={f.key} className="p-5"><div className="flex items-center justify-between gap-3"><h4 className="text-sm font-medium">{f.label}</h4><Pill status={server.clientFeatures?.[f.key]?'enabled':'disabled'}/></div><p className="mt-3 text-xs leading-relaxed text-ink-muted">{f.description}</p></div>)}</div>
    </ServerPanel>
  </section>
}
