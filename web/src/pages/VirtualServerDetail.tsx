import { Link, useParams } from 'react-router-dom'
import { useInventory } from '../connections/useInventory'
import { capabilitiesOf, capabilityCounts, editorPath, relationships, serverPath } from '../connections/model'
import CapabilityTable from '../connections/CapabilityTable'
import Health from '../connections/Health'
import Connect from '../connections/Connect'
import RecentActivity from '../connections/RecentActivity'
import { ErrorBox } from '../ui'
import { btn } from '../styles'
import { useCapabilities } from '../interaction'

const views = ['overview','capabilities','access','protection','connect','activity'] as const
export default function VirtualServerDetail() {
  const {slug='',view='overview'} = useParams()
  const inventory = useInventory()
  const {edit} = useCapabilities()
  const server = inventory.servers.find(s=>s.slug===slug)
  const active = views.includes(view as typeof views[number]) ? view : 'overview'
  const grants = relationships(inventory.servers,inventory.upstreams,inventory.keys,inventory.bindings).filter(e=>e.server===slug && e.kind!=='upstream')
  if(!server)return <div className="p-10 space-y-4"><h1 className="text-2xl font-semibold">Virtual server</h1>{inventory.loading ? <p role="status">Loading saved configuration…</p> : <p>{inventory.loaded.includes('servers') ? `Virtual server “${slug}” was not found.` : 'Virtual server configuration is unavailable.'}</p>}{inventory.readError && <ErrorBox>{inventory.readError}</ErrorBox>}<Link className={btn} to="/virtual-servers">Back to virtual servers</Link></div>
  const objects = capabilitiesOf(server)
  return <div className="mx-auto max-w-7xl space-y-6 px-10 py-10">
    <header><Link className="text-sm text-accent-700 underline" to="/connections">Connections</Link><h1 className="mt-2 text-2xl font-semibold">{server.name}</h1><p className="text-sm text-ink-muted">Saved configuration · version {server.version} · {server.enabled?'enabled':'disabled'}. Open the editor to make a draft.</p></header>
    <div className="flex gap-3"><Link className={btn} to={editorPath('virtual-servers',slug)}>{edit?'Edit configuration':'Inspect configuration'}</Link><Link className={btn} to={`/setup?server=${encodeURIComponent(slug)}`}>Setup guide for this server</Link><button className={btn} disabled={inventory.loading} onClick={()=>inventory.load()}>Refresh saved configuration</button></div>
    {inventory.readError && <ErrorBox>{inventory.readError}. Related configuration may be incomplete or from an earlier refresh.</ErrorBox>}
    <nav aria-label="Virtual server views" className="flex flex-wrap gap-2 border-b border-line pb-3">{views.map(section=><Link key={section} aria-current={section===active?'page':undefined} className={`${btn} ${section===active?'bg-accent-50 text-accent-700':''}`} to={serverPath(slug,section)}>{section[0].toUpperCase()+section.slice(1)}</Link>)}</nav>
    {active==='overview' && <section className="space-y-4"><h2 className="text-lg font-semibold">Overview</h2><p>{server.description || 'No description provided.'}</p><p>{capabilityCounts(server)} selected</p><p>Authentication: {server.authRequired?'agent key or OAuth access token required':'anonymous access enabled'}</p><p>{objects.filter(o=>o.missing).length} missing capability objects · {grants.length} configured access relationships</p><p className="text-sm text-ink-muted">Counts are configuration selections, not successful calls or active clients. Disabled upstreams, missing objects and policies affect what is exposed.</p></section>}
    {active==='capabilities' && <section><h2 className="text-lg font-semibold">Capabilities</h2><CapabilityTable objects={objects}/><h2 className="mt-6 font-semibold">Upstream health observations</h2>{inventory.upstreams.filter(u=>objects.some(o=>o.upstream===u.slug)).map(upstream=><Health key={upstream.slug} upstream={upstream}/>)}</section>}
    {active==='access' && <section className="space-y-3"><h2 className="text-lg font-semibold">Access</h2><p>{server.authRequired?'Configured keys and conditional JWT bindings are listed below. IP restrictions, expiration, revocation, binding precedence and server state are still enforced.':'Anonymous access bypasses credential authentication. Anyone who can reach the gateway can access the selected capabilities, subject to server policies.'}</p>
      <div className="flex gap-3"><Link className={btn} to="/agent-keys">Manage agent keys</Link><Link className={btn} to="/identities">Manage identity bindings</Link></div>
      {grants.map(grant=><div key={grant.id} className="rounded border border-line p-3"><Link className="font-semibold text-accent-700 underline" to={grant.href}>{grant.from}</Link><p className="text-sm">{grant.kind} · {grant.state}</p><p className="text-sm">{grant.detail}</p></div>)}
      {!inventory.loading && grants.length===0 && <p>No configured access grants found. Related catalog errors may make this list incomplete.</p>}</section>}
    {active==='protection' && <section className="space-y-3"><h2 className="text-lg font-semibold">Protection</h2><p>Default tool policy: {server.policy?.default ?? 'allow'}. Per-tool rules appear in Capabilities.</p><p>Server rate limit: {server.rateLimitPerMinute ?? 'unlimited'} calls/minute; credential-specific limits may also apply.</p><p>PII masking: {server.pii?.enabled?'enabled':'disabled'} · argument masking: {server.pii?.maskArguments?'enabled':'disabled'}</p><p>Detectors: {server.pii?.detectors.join(', ') || 'none'}</p><p>Custom rules: {server.pii?.customRules.map(r=>r.name).join(', ') || 'none'}</p><p>Client feature permissions: {(['sampling','roots','elicitation'] as const).filter(f=>server.clientFeatures?.[f]).join(', ') || 'none'}. Client negotiation and approval also apply.</p><p className="text-sm text-ink-muted">PII masking is not a guarantee that all sensitive data is detected. Read-only policy relies on upstream annotations.</p></section>}
    {active==='connect' && <Connect key={`${slug}:${server.version}`} server={server}/>}
    {active==='activity' && <RecentActivity key={slug} slug={slug}/>}
  </div>
}
