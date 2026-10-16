import { EmptyState } from '../Feedback'
import { Link } from 'react-router-dom'
import { ArrowRight, ArrowUpRight, Box, Cable, Fingerprint, KeyRound, Layers3, ShieldCheck, ShieldOff, Wrench } from 'lucide-react'
import type { Upstream, VirtualServer } from '../api'
import { capabilitiesOf, editorPath, serverPath, type Relationship } from './model'
import { Pill } from '../ui'

export default function ServerOverview({server,upstreams,grants}:{server:VirtualServer;upstreams:Upstream[];grants:Relationship[]}) {
  const objects=capabilitiesOf(server)
  const sources=[...new Set(objects.map(o=>o.upstream))]
  const missing=objects.filter(o=>o.missing).length
  const stats=[
    {label:'Selected tools',value:server.tools.length,detail:`${objects.length-server.tools.length} other capabilities`,icon:Wrench,view:'capabilities'},
    {label:'Upstream servers',value:sources.length,detail:'Composed into this endpoint',icon:Layers3,view:'capabilities'},
    {label:'Access relationships',value:grants.length,detail:server.authRequired?'Keys & conditional identities':'Includes anonymous access',icon:Fingerprint,view:'access'},
    {label:'Missing selections',value:missing,detail:missing?'Review upstream discovery':'All selections discovered',icon:Box,view:'capabilities'},
  ]
  return <section aria-label="Server overview" className="layout-stack">
    <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">{stats.map(({label,value,detail,icon:Icon,view})=><Link key={label} to={serverPath(server.slug,view)} className="group surface-card surface-interactive p-4 sm:p-5"><div className="flex items-center justify-between gap-2"><span className="text-xs font-medium text-ink-muted">{label}</span><Icon aria-hidden="true" className={`icon-control ${label==='Missing selections'&&missing?'text-amber-600':'text-accent-700'}`}/></div><p className="summary-value mt-3">{value}</p><p className="mt-2 text-xs text-ink-muted">{detail}</p></Link>)}</div>
    <div className="grid layout-grid xl:grid-cols-[1.4fr_1fr]">
      <section aria-label="Upstream composition" className="flex flex-col overflow-hidden surface-card">
        <div className="flex items-center justify-between gap-3 border-b border-line panel-inset py-4"><div><h2 className="panel-title">Upstream composition</h2><p className="panel-description mt-1 text-xs text-ink-muted">Capabilities selected for this virtual server</p></div><Cable aria-hidden="true" className="icon-control text-ink-muted"/></div>
        <div className="overflow-x-auto divide-y divide-line">{sources.map(slug=>{
          const upstream=upstreams.find(u=>u.slug===slug),selected=objects.filter(o=>o.upstream===slug),missingCount=selected.filter(o=>o.missing).length
          return <Link key={slug} to={editorPath('upstreams',slug)} className="flex items-center gap-3 panel-inset py-4 hover:bg-canvas transition-colors"><span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-accent-50 text-accent-700"><Layers3 aria-hidden="true" className="icon-control"/></span><div className="min-w-0 flex-1"><p className="truncate text-sm font-medium">{upstream?.name ?? slug}</p><p className="mt-1 text-xs text-ink-muted">{selected.filter(o=>o.kind==='tool').length} tools · {selected.filter(o=>o.kind!=='tool').length} other capabilities{missingCount?` · ${missingCount} missing`:''}</p></div><Pill status={!upstream?'missing':upstream.enabled?'enabled':'disabled'}/><ArrowUpRight aria-hidden="true" className="icon-inline shrink-0 text-ink-muted"/></Link>
        })}{sources.length===0 && <EmptyState title="No upstream capabilities selected yet." compact/>}</div>
        <Link to={serverPath(server.slug,'capabilities')} className="mt-auto flex items-center justify-between border-t border-line panel-inset py-3 text-xs font-medium text-accent-700">Explore capabilities<ArrowRight aria-hidden="true" className="icon-inline"/></Link>
      </section>
      <section aria-label="Access and protection" className="flex flex-col overflow-hidden surface-card">
        <div className="flex items-center justify-between gap-3 border-b border-line panel-inset py-4"><div><h2 className="panel-title">Access & protection</h2><p className="panel-description mt-1 text-xs text-ink-muted">Configured controls for this endpoint</p></div><ShieldCheck aria-hidden="true" className="icon-control text-ink-muted"/></div>
        <dl className="divide-y divide-line panel-inset text-sm">{[
          ['Authentication',server.authRequired?'Agent key / OAuth token':'Anonymous access'],
          ['Default tool policy',server.policy?.default ?? 'allow'],
          ['Server rate limit',server.rateLimitPerMinute==null?'Unlimited':`${server.rateLimitPerMinute} / min`],
          ['PII masking',server.pii?.enabled?'Enabled':'Disabled'],
        ].map(([label,value])=><div key={label} className="flex items-center justify-between gap-4 py-3.5"><dt className="text-xs text-ink-muted">{label}</dt><dd className="text-xs font-medium text-right">{value}</dd></div>)}</dl>
        <Link to={serverPath(server.slug,'protection')} className="mt-auto flex items-center justify-between border-t border-line panel-inset py-3 text-xs font-medium text-accent-700">Review protection<ArrowRight aria-hidden="true" className="icon-inline"/></Link>
      </section>
    </div>
    <section className="flex flex-wrap items-center justify-between gap-4 surface-card panel-body" aria-label="Gateway endpoint"><div className="flex items-center gap-3 min-w-0"><span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-accent-50 text-accent-700">{server.authRequired?<KeyRound aria-hidden="true" className="icon-feature"/>:<ShieldOff aria-hidden="true" className="icon-feature"/>}</span><div className="min-w-0"><h2 className="panel-title">Gateway endpoint</h2><p className="meta-copy mt-1 break-all font-mono text-xs text-ink-muted">{server.endpoint}</p></div></div><Link to={serverPath(server.slug,'connect')} className="inline-flex items-center gap-2 text-sm font-medium text-accent-700">Connection settings<ArrowRight aria-hidden="true" className="icon-control"/></Link></section>
    <p className="text-xs text-ink-muted">Counts describe saved configuration. Missing objects, disabled upstreams and tool policies affect what is exposed; access relationships do not indicate active clients.</p>
  </section>
}
