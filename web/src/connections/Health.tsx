import { StatusBadge } from '../Badge'
import { EmptyState } from '../Feedback'
import { Activity, Clock3 } from 'lucide-react'
import type { Upstream } from '../api'
import { fmt } from '../format'
import { Pill } from '../ui'
import { DetailRow } from './ServerPanels'

export default function Health({upstream}: {upstream: Upstream}) {
  return <section className="space-y-4 text-xs">
    <div className="flex flex-wrap items-center justify-between gap-3"><h3 className="panel-title inline-flex items-center gap-2"><Activity className="size-4 text-ink-muted"/>{upstream.name || upstream.slug}</h3><Pill status={upstream.enabled?upstream.health?.state??'unknown':'disabled'}/></div>
    <div className="rounded-lg border border-line bg-canvas px-4"><dl className="divide-y divide-line"><DetailRow label="Discovery">{fmt(upstream.lastRefreshAt)}</DetailRow><DetailRow label="Observations">{upstream.health?.nodes?.length??0} nodes</DetailRow></dl></div>
    {upstream.lastRefreshError&&<p className="break-words rounded-lg bg-red-50 p-3 leading-relaxed text-red-700">{upstream.lastRefreshError}</p>}
    {(!upstream.enabled || upstream.lastRefreshError) && <p className="leading-relaxed text-ink-muted">Check enabled state, upstream URL and credentials in configuration, then test and refresh discovery.</p>}
    <div className="space-y-3">{(upstream.health?.nodes ?? []).map(node=><div key={node.node} className="rounded-xl border border-line p-4">
      <div className="flex flex-wrap items-center justify-between gap-2"><p className="break-all font-medium">{node.node}</p><Pill status={node.state}/></div>
      <p className="mt-2 inline-flex items-center gap-1.5 text-ink-muted"><Clock3 className="size-3.5 shrink-0"/>Checked: {fmt(node.checkedAt)}</p><dl className="mt-2 divide-y divide-line"><DetailRow label="Circuit breaker"><StatusBadge status={`breaker ${node.breaker}`} label={node.breaker.replaceAll('_',' ')}/></DetailRow><DetailRow label="Recorded latency">{node.latencyMs} ms</DetailRow><DetailRow label="Endpoint"><span className="break-all font-mono">{node.url}</span></DetailRow></dl>{node.error&&<p className="mt-2 break-words rounded-lg bg-red-50 p-3 leading-relaxed text-red-700">{node.error}. Check node connectivity and credentials; retry the upstream test before changing routing.</p>}
    </div>)}</div>
    {!upstream.health?.nodes?.length && <EmptyState title="No node health observations available." compact/>}
  </section>
}
