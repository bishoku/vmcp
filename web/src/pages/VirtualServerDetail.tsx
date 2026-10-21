import AuditSettingsPanel from '../AuditSettingsPanel'
import { useEffect, useState } from 'react'
import { api, type VirtualServer } from '../api'
import GuidanceEditor from '../GuidanceEditor'
import Button from '../Button'
import { TagBadge } from '../Badge'
import { EmptyState, LoadingState } from '../Feedback'
import { BookOpen, Box, Cable, ChevronRight, Fingerprint, LayoutDashboard, Pencil, Radio, RefreshCw, Server, ShieldCheck, Wrench } from 'lucide-react'
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

const viewIcons = {guidance:BookOpen,overview:LayoutDashboard,capabilities:Wrench,access:Fingerprint,protection:ShieldCheck,connect:Cable,activity:Radio}
const views = ['overview','capabilities','guidance','access','protection','connect','activity'] as const
export default function VirtualServerDetail() {
  const {slug='',view='overview'} = useParams()
  const inventory = useInventory()
  const {edit} = useCapabilities()
  const server = inventory.servers.find(s=>s.slug===slug)
  const active = views.includes(view as typeof views[number]) ? view : 'overview'
  const grants = relationships(inventory.servers,inventory.upstreams,inventory.keys,inventory.bindings).filter(e=>e.server===slug && e.kind!=='upstream')
  if(!server)return <div className="page-layout"><h1 className="page-title">Virtual server</h1>{inventory.loading?<LoadingState label="Loading saved configuration…" detail="Reading saved servers and related configuration."/>:inventory.readError&&!inventory.loaded.includes('servers')?<ErrorBox title="Virtual server configuration is unavailable." onRetry={inventory.load}>{inventory.readError}</ErrorBox>:<div className="surface-card"><EmptyState title={inventory.loaded.includes('servers')?`Virtual server “${slug}” was not found.`:'Virtual server configuration is unavailable.'} description="Check the identifier or return to the virtual-server inventory." actions={<button className={btn} onClick={()=>void inventory.load()}>Retry</button>}/></div>}<Link className={btn} to="/virtual-servers">Back to virtual servers</Link></div>

  return <div className="page-layout">
    <header className="layout-stack">
      <div className="flex items-center gap-2 text-xs text-ink-muted"><Link className="hover:text-accent-700" to="/virtual-servers">Virtual servers</Link><ChevronRight aria-hidden="true" className="icon-inline"/><span className="font-mono">{server.slug}</span></div>
      <div className="flex flex-wrap items-start justify-between gap-5"><div className="flex min-w-0 flex-[1_1_280px] items-start gap-4"><div className="flex size-12 shrink-0 items-center justify-center rounded-xl border border-accent-500/15 bg-accent-50 text-accent-700"><Server aria-hidden="true" className="icon-hero"/></div><div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-3"><h1 className="page-title">{server.name}</h1><Pill status={server.enabled?'enabled':'disabled'}/><TagBadge label={`v${server.version}`}/></div><p className="page-description mt-1.5 max-w-2xl text-sm text-ink-muted">{server.description || 'A composed MCP endpoint for your agents.'}</p></div></div>
      <div className="flex flex-wrap gap-2"><Button loading={inventory.loading} className={btn} disabled={inventory.loading} onClick={()=>inventory.load()} aria-label="Refresh saved configuration"><RefreshCw aria-hidden="true" className={`icon-inline ${inventory.loading?'motion-spin':''}`}/>Refresh</Button>{edit && <Link className={primary} to={`/virtual-servers/${encodeURIComponent(slug)}/edit`}><Pencil aria-hidden="true" className="icon-inline"/>Edit configuration</Link>}</div></div>
      <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-ink-muted"><p>Saved configuration · version {server.version} · {server.enabled?'enabled':'disabled'}. Open the editor to make a draft.</p><Link className="inline-flex items-center gap-1 text-accent-700" to={`/setup?server=${encodeURIComponent(slug)}`}><Box aria-hidden="true" className="icon-inline"/>Setup guide for this server<ChevronRight aria-hidden="true" className="icon-inline"/></Link></div>
    </header>
    {inventory.readError && <ErrorBox title="Configuration could not be loaded" onRetry={inventory.load} busy={inventory.loading}>{inventory.readError}. Related configuration may be incomplete or from an earlier refresh.</ErrorBox>}
    <nav aria-label="Virtual server views" className="flex gap-5 overflow-x-auto border-b border-line">{views.map(section=>{const Icon=viewIcons[section];return <Link key={section} aria-current={section===active?'page':undefined} className={`inline-flex shrink-0 items-center gap-2 border-b-2 px-1 pb-3 pt-1 text-sm font-medium transition-colors ${section===active?'border-accent-500 text-accent-700':'border-transparent text-ink-muted hover:text-ink'}`} to={serverPath(slug,section)}><Icon aria-hidden="true" className="icon-control"/>{section[0].toUpperCase()+section.slice(1)}</Link>})}</nav>
    {active==='overview' && <ServerOverview server={server} upstreams={inventory.upstreams} grants={grants}/>}
    {active==='capabilities' && <SavedContent key={`${slug}:${server.version}:capabilities`} view="capabilities" server={server} upstreams={inventory.upstreams}/>}
    {active==='guidance' && <SavedContent view="guidance" key={`${slug}:${server.version}:guidance`} server={server} upstreams={inventory.upstreams}/>}
    {active==='access' && <ServerAccess key={slug} server={server} grants={grants} loading={inventory.loading}/>}
    {active==='protection' && <><ServerProtection server={server}/><AuditSettingsPanel scope="virtual" slug={slug}/></>}
    {active==='connect' && <Connect key={`${slug}:${server.version}`} server={server}/>}
    {active==='activity' && <RecentActivity key={slug} slug={slug}/>}
  </div>
}

function SavedContent({server,upstreams,view}:{view:'guidance'|'capabilities';server:VirtualServer;upstreams:import('../api').Upstream[]}) {
 const [full,setFull]=useState<VirtualServer|null>(server.hasGuidance?null:server),[error,setError]=useState(''),[retry,setRetry]=useState(0)
 useEffect(()=>{if(!server.hasGuidance)return;const c=new AbortController();api.getVirtualServer(server.slug,c.signal).then(v=>{if(!c.signal.aborted){setFull(v);setError('')}}).catch(e=>{if(!c.signal.aborted)setError(String(e))});return()=>c.abort()},[server.slug,server.hasGuidance,retry])
 if(error)return <ErrorBox onRetry={()=>setRetry(r=>r+1)}>{error}</ErrorBox>
 if(!full)return <LoadingState label="Loading agent guidance…"/>
 if(view==='capabilities')return <ServerCapabilities server={full} upstreams={upstreams}/>
 return <GuidanceEditor value={full.guidance} tools={full.tools} upstreams={upstreams}/>
}
