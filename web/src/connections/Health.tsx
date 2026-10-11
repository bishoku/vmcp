import type { Upstream } from '../api'
import { fmt } from '../format'
export default function Health({upstream}: {upstream: Upstream}) {
  return <section className="mt-3 space-y-2 text-sm">
    <h3 className="font-semibold">{upstream.name || upstream.slug} · {upstream.enabled ? upstream.health?.state ?? 'unknown' : 'disabled'}</h3>
    <p>Discovery: {fmt(upstream.lastRefreshAt)}{upstream.lastRefreshError && ` · ${upstream.lastRefreshError}`}</p>
    {(!upstream.enabled || upstream.lastRefreshError) && <p>Check enabled state, upstream URL and credentials in configuration, then test and refresh discovery.</p>}
    {(upstream.health?.nodes ?? []).map(node=><div key={node.node} className="rounded border border-line p-3">
      <p>{node.node}: {node.state} · breaker {node.breaker} · {node.latencyMs} ms</p>
      <p>Checked: {fmt(node.checkedAt)}</p><p className="break-all">{node.url}</p>{node.error && <p>{node.error}. Check node connectivity and credentials; retry the upstream test before changing routing.</p>}
    </div>)}
    {!upstream.health?.nodes?.length && <p>No node health observations available.</p>}
  </section>
}
