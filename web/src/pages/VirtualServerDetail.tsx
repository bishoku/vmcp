import { Box, Cable, ChevronRight, Fingerprint, LayoutDashboard, Pencil, Radio, RefreshCw, Server, ShieldCheck, Wrench } from 'lucide-react'
import { Link, useParams } from 'react-router-dom'
import { useInventory } from '../connections/useInventory'
import { relationships, serverPath } from '../connections/model'
import ServerProtection from '../connections/ServerProtection'
import ServerOverview from '../connections/ServerOverview'
import ServerCapabilities from '../connections/ServerCapabilities'
import ServerAccess from '../connections/ServerAccess'
import Connect from '../connections/Connect'
import RecentActivity from '../connections/RecentActivity'
import { ErrorBox, Pill } from '../ui'
import { btn, primary } from '../styles'
import { useCapabilities } from '../interaction'

const viewIcons = {overview:LayoutDashboard,capabilities:Wrench,access:Fingerprint,protection:ShieldCheck,connect:Cable,activity:Radio}
const views = ['overview','capabilities','access','protection','connect','activity'] as const
export default function VirtualServerDetail() {
  const {slug='',view='overview'} = useParams()
  const inventory = useInventory()
  const {edit} = useCapabilities()
  const server = inventory.servers.find(s=>s.slug===slug)
  const active = views.includes(view as typeof views[number]) ? view : 'overview'
  const grants = relationships(inventory.servers,inventory.upstreams,inventory.keys,inventory.bindings).filter(e=>e.server===slug && e.kind!=='upstream')
  if(!server)return <div className="p-10 space-y-4"><h1 className="text-2xl font-semibold">Virtual server</h1>{inventory.loading ? <p role="status">Loading saved configuration…</p> : <p>{inventory.loaded.includes('servers') ? `Virtual server “${slug}” was not found.` : 'Virtual server configuration is unavailable.'}</p>}{inventory.readError && <ErrorBox>{inventory.readError}</ErrorBox>}<Link className={btn} to="/virtual-servers">Back to virtual servers</Link></div>
  return <div className="mx-auto max-w-7xl space-y-6 px-4 py-6 sm:px-6 lg:px-10 lg:py-10">
    <header className="space-y-5">
      <div className="flex items-center gap-2 text-xs text-ink-muted"><Link className="hover:text-accent-700" to="/virtual-servers">Virtual servers</Link><ChevronRight className="size-3"/><span className="font-mono">{server.slug}</span></div>
      <div className="flex flex-wrap items-start justify-between gap-5"><div className="flex items-start gap-4"><div className="flex size-12 shrink-0 items-center justify-center rounded-xl border border-accent-500/15 bg-accent-50 text-accent-700"><Server className="size-6"/></div><div><div className="flex flex-wrap items-center gap-3"><h1 className="text-2xl font-semibold tracking-tight">{server.name}</h1><Pill status={server.enabled?'enabled':'disabled'}/><span className="rounded-md border border-line bg-surface px-2 py-0.5 text-[11px] font-medium text-ink-muted">v{server.version}</span></div><p className="mt-1.5 max-w-2xl text-sm text-ink-muted">{server.description || 'A composed MCP endpoint for your agents.'}</p></div></div>
      <div className="flex flex-wrap gap-2"><button className={btn} disabled={inventory.loading} onClick={()=>inventory.load()} aria-label="Refresh saved configuration"><RefreshCw className={`size-3.5 ${inventory.loading?'animate-spin':''}`}/>Refresh</button>{edit && <Link className={primary} to={`/virtual-servers/${encodeURIComponent(slug)}/edit`}><Pencil className="size-3.5"/>Edit configuration</Link>}</div></div>
      <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-ink-muted"><p>Saved configuration · version {server.version} · {server.enabled?'enabled':'disabled'}. Open the editor to make a draft.</p><Link className="inline-flex items-center gap-1 text-accent-700" to={`/setup?server=${encodeURIComponent(slug)}`}><Box className="size-3"/>Setup guide for this server<ChevronRight className="size-3"/></Link></div>
    </header>
    {inventory.readError && <ErrorBox>{inventory.readError}. Related configuration may be incomplete or from an earlier refresh.</ErrorBox>}
    <nav aria-label="Virtual server views" className="flex gap-5 overflow-x-auto border-b border-line">{views.map(section=>{const Icon=viewIcons[section];return <Link key={section} aria-current={section===active?'page':undefined} className={`inline-flex shrink-0 items-center gap-2 border-b-2 px-1 pb-3 pt-1 text-sm font-medium transition-colors ${section===active?'border-accent-500 text-accent-700':'border-transparent text-ink-muted hover:text-ink'}`} to={serverPath(slug,section)}><Icon className="size-4"/>{section[0].toUpperCase()+section.slice(1)}</Link>})}</nav>
    {active==='overview' && <ServerOverview server={server} upstreams={inventory.upstreams} grants={grants}/>}
    {active==='capabilities' && <ServerCapabilities key={slug} server={server} upstreams={inventory.upstreams}/>}
    {active==='access' && <ServerAccess key={slug} server={server} grants={grants} loading={inventory.loading}/>}
    {active==='protection' && <ServerProtection server={server}/>}
    {active==='connect' && <Connect key={`${slug}:${server.version}`} server={server}/>}
    {active==='activity' && <RecentActivity key={slug} slug={slug}/>}
  </div>
}
