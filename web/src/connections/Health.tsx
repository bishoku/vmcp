import type { Upstream } from '../api'
import { fmt } from '../format'
export default function Health({upstream}: {upstream: Upstream}) {
  return <section className="mt-3 space-y-2 text-sm">
    <h3 className="font-semibold">{upstream.name || upstream.slug} · {upstream.enabled ? upstream.health?.state ?? 'unknown' : 'disabled'}</h3>
    <p>Discovery: {fmt(upstream.lastRefreshAt)}{upstream.lastRefreshError && ` · ${upstream.lastRefreshError}`}</p>
    {(upstream.health?.nodes ?? []).map(node=><div key={node.node} className="rounded border border-line p-3">
      <p>{node.node}: {node.state} · breaker {node.breaker} · {node.latencyMs} ms</p>
      <p>Checked: {fmt(node.checkedAt)}</p><p className="break-all">{node.url}</p>{node.error && <p>{node.error}</p>}
    </div>)}
    {!upstream.health?.nodes?.length && <p>No node health observations available.</p>}
  </section>
}
