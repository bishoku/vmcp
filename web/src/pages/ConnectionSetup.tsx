import { ArrowUpRight, BookOpen, Cable, GitBranch, KeyRound, Layers, ListChecks, PlugZap, RefreshCw, Server, ShieldCheck } from 'lucide-react'
import Connect from '../connections/Connect'
import { DetailRow, ServerPanel, SummaryTile } from '../connections/ServerPanels'
import { Link, useSearchParams } from 'react-router-dom'
import { useInventory } from '../connections/useInventory'
import { capabilityCounts, editorPath, keyState, serverPath } from '../connections/model'
import { useCapabilities } from '../interaction'
import { btn, input } from '../styles'
import { ErrorBox } from '../ui'

export default function ConnectionSetup() {
  const inventory = useInventory()
  const [params,setParams] = useSearchParams()
  const slug = params.get('server') ?? ''
  const {edit} = useCapabilities()
  const server = inventory.servers.find(s=>s.slug===slug)
  const referenced = new Set(server ? [...server.tools.map(t=>t.upstream),...(server.catalog ?? []).map(c=>c.upstream)] : [])
  const upstreams = inventory.upstreams.filter(u=>referenced.has(u.slug))
  const hasKey = inventory.keys.some(k=>k.virtualServers.includes(slug) && keyState(k)==='active')
  const hasBinding = inventory.bindings.some(b=>b.enabled && b.virtualServers.includes(slug))
  const steps = [
    {title:'Register upstreams',description:'Add physical MCP servers and their transport, credentials and TLS settings.',href:'/upstreams',status:inventory.upstreams.length?'Upstreams registered':'Registration needed'},
    {title:'Test connectivity',description:'Open an upstream and deliberately test its stored credentials. This checks the gateway-to-upstream hop only.',href:upstreams[0]?editorPath('upstreams',upstreams[0].slug):'/upstreams',status:'Explicit test required; not inferred from configuration'},
    {title:'Discover capabilities',description:'Refresh each upstream to discover tools, resources, templates and prompts. Check discovery errors before composing.',href:'/upstreams',status:referenced.size > 0 && upstreams.length === referenced.size && upstreams.every(u=>u.lastRefreshAt && !u.lastRefreshError)?'Referenced upstreams have discovery snapshots':'Discovery needed or not yet known'},
    {title:'Compose a virtual MCP server',description:'Select capabilities explicitly, set aliases, authentication and protection, then save the draft.',href:server?editorPath('virtual-servers',slug):'/virtual-servers',status:server?`${capabilityCounts(server)} selected`:'Choose or create a virtual server'},
    {title:'Grant client access',description:'Create a scoped agent key or configure a JWT identity binding. Copy new secrets immediately; they are shown once.',href:server?serverPath(slug,'access'):'/agent-keys',status:server?!server.authRequired?'Anonymous access enabled':hasKey||hasBinding?'Configured grants exist; runtime authorization still applies':'No active key or enabled binding found':'Select a virtual server first'},
    {title:'Connect your client',description:'Use the saved gateway endpoint and replace credential placeholders in your client settings.',href:server?serverPath(slug,'connect'):'/virtual-servers',status:'Client setup is a manual step'},
    {title:'Verify the path',description:'Explicitly initialize, ping and list capabilities from Connect. Inspect bounded recent activity separately. No tool is automatically invoked.',href:server?serverPath(slug,'connect'):'/virtual-servers',status:'Explicit client verification required'},
  ]
  const stepIcons = [Server, PlugZap, ListChecks, Layers, KeyRound, Cable, ShieldCheck]
  return <div className="mx-auto max-w-7xl space-y-6 px-4 py-6 sm:px-6 lg:px-10 lg:py-10">
    <header className="flex flex-wrap items-start justify-between gap-4">
      <div><div className="mb-3 flex items-center gap-2 text-xs font-medium text-accent-700"><BookOpen className="size-4"/>SETUP GUIDE</div><h1 className="text-2xl font-semibold tracking-tight">Connection setup guide</h1><p className="mt-2 max-w-2xl text-sm leading-relaxed text-ink-muted">Build the path from upstream capabilities to your client. Review saved configuration, prepare connection settings and explicitly verify discovery.</p></div>
      <div className="flex flex-wrap gap-2"><button className={btn} disabled={inventory.loading} onClick={()=>void inventory.load()}><RefreshCw className="size-3.5"/>Refresh context</button><Link className={btn} to="/connections"><GitBranch className="size-3.5"/>View configured connection flow</Link></div>
    </header>
    {!edit && <p role="status" className="rounded-xl border border-line bg-surface px-4 py-3 text-xs leading-relaxed text-ink-muted">Read-only access: inspect configuration and verify with a separate gateway credential. An editor must perform configuration changes.</p>}
    {inventory.loading && <p role="status" className="text-xs text-ink-muted">Loading setup context…</p>}
    {inventory.readError && <ErrorBox>{inventory.readError}. Step status may be incomplete; retained configuration is not proof of current access.</ErrorBox>}
    <ServerPanel title="Setup context" description="Choose the saved virtual server to guide connection settings and access checks" icon={Layers}>
      <div className="grid items-center gap-5 p-5 md:grid-cols-2"><label className="block text-xs font-medium">Guide for virtual server<select className={`${input} mt-2`} value={slug} onChange={e=>{const next=new URLSearchParams(params);if(e.target.value)next.set('server',e.target.value);else next.delete('server');setParams(next,{replace:true})}}><option value="">Choose a saved server</option>{slug && !server && <option value={slug}>{slug} · unavailable</option>}{inventory.servers.map(s=><option key={s.slug} value={s.slug}>{s.name}</option>)}</select></label><p className="text-xs leading-relaxed text-ink-muted">Step labels describe configuration and manual actions. They do not track completed tasks or certify a working end-to-end connection.</p></div>
    </ServerPanel>
    <div className="grid gap-3 sm:grid-cols-3"><SummaryTile label="Virtual server" value={server?server.enabled?'Enabled':'Disabled':'Not selected'} detail={server?`${server.name} · saved version ${server.version}`:'Choose a saved configuration above'} icon={Layers}/><SummaryTile label="Selected capabilities" value={server?capabilityCounts(server):'—'} detail="Tools, resources, templates and prompts" icon={ListChecks}/><SummaryTile label="Referenced upstreams" value={server?referenced.size:'—'} detail={server?`${upstreams.length} found in loaded inventory`:'Based on the selected server'} icon={Server}/></div>
    <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,1.7fr)_minmax(0,1fr)]">
      <ServerPanel title="Setup steps" description="Follow the configuration path, then verify each connection deliberately" icon={BookOpen}>
        <ol className="divide-y divide-line">{steps.map((step,index)=>{const Icon=stepIcons[index];return <li key={step.title} className="flex gap-3 p-5 sm:gap-4">
          <span aria-hidden="true" className="grid size-8 shrink-0 place-items-center rounded-xl border border-accent-100 bg-accent-50 text-xs font-semibold text-accent-700">{index+1}</span>
          <div className="min-w-0 flex-1"><div className="flex items-start justify-between gap-3"><h2 className="text-sm font-semibold">{step.title}</h2><Icon className="size-4 shrink-0 text-ink-muted" aria-hidden="true"/></div><p className="mt-2 text-xs leading-relaxed text-ink-muted">{step.description}</p><p className="mt-3 border-l-2 border-accent-200 pl-3 text-xs leading-relaxed">{step.status}</p><Link className="mt-3 inline-flex items-center gap-1.5 text-xs font-medium text-accent-700 hover:underline" to={step.href}>Open {step.title.toLowerCase()}<ArrowUpRight className="size-3.5"/></Link></div>
        </li>})}</ol>
      </ServerPanel>
      <div className="space-y-6 xl:sticky xl:top-6">
        <ServerPanel title="Saved connection path" description="Configuration for the selected virtual server" icon={GitBranch}>
          {server?<div className="p-5"><div className="rounded-xl border border-accent-100 bg-accent-50 p-4"><p className="text-xs text-accent-700">Virtual MCP server</p><Link className="mt-1 inline-flex items-center gap-2 break-all text-sm font-semibold text-accent-900 hover:underline" to={serverPath(slug)}>{server.name}<ArrowUpRight className="size-3.5 shrink-0"/></Link><p className="mt-2 text-xs leading-relaxed text-accent-700">{server.description || 'Saved capability composition'}</p></div><dl className="mt-2 divide-y divide-line"><DetailRow label="Authentication">{server.authRequired?'Agent key / OAuth token':'Anonymous'}</DetailRow><DetailRow label="Active scoped keys">{inventory.keys.filter(k=>k.virtualServers.includes(slug)&&keyState(k)==='active').length}</DetailRow><DetailRow label="Enabled identity bindings">{inventory.bindings.filter(b=>b.enabled&&b.virtualServers.includes(slug)).length}</DetailRow></dl><div className="mt-4 max-h-80 space-y-2 overflow-auto"><p className="text-xs font-medium">Referenced upstreams</p>{[...referenced].map(ref=>{const upstream=upstreams.find(u=>u.slug===ref);return <div key={ref} className="flex items-center justify-between gap-3 rounded-lg border border-line px-3 py-3 text-xs"><Link className="min-w-0 break-all font-medium text-accent-700 hover:underline" to={editorPath('upstreams',ref)}>{upstream?.name ?? ref}</Link><span className="shrink-0 text-ink-muted">{upstream?upstream.enabled?'Enabled':'Disabled':'Not loaded'}</span></div>})}{!referenced.size&&<p className="text-xs text-ink-muted">No upstream capabilities selected.</p>}</div></div>:<div className="p-5"><Layers className="mb-3 size-6 text-ink-muted"/><p className="text-sm font-medium">{slug?'Server unavailable':'Choose your virtual server'}</p><p className="mt-2 text-xs leading-relaxed text-ink-muted">{slug?'The selected server was not found in the loaded inventory. Refresh context or choose another saved server.':'The guide works without a selection. Select a saved server to see its upstream path and prepare a client connection below.'}</p><Link className={`${btn} mt-4`} to="/virtual-servers">Browse virtual servers</Link></div>}
        </ServerPanel>
        <ServerPanel title="Verification boundaries" description="Two explicit checks for two connection hops" icon={ShieldCheck}>
          <div className="space-y-4 p-5 text-xs leading-relaxed"><div><p className="font-medium">Gateway → upstream</p><p className="mt-1 text-ink-muted">Test stored upstream credentials on each upstream’s detail page. Discovery snapshots describe saved capabilities.</p></div><div className="border-t border-line pt-4"><p className="font-medium">Client → virtual server</p><p className="mt-1 text-ink-muted">Use the connection panel below to initialize, ping and list capabilities with your client identity. Configuration alone does not prove authorization.</p></div></div>
        </ServerPanel>
      </div>
    </div>
    {server?<Connect key={`${server.slug}:${server.version}`} server={server} showSummary={false}/>:<ServerPanel title="Client connection" description="Endpoint, client settings and explicit verification" icon={Cable}><p className="p-5 text-sm text-ink-muted">Select a saved virtual server to prepare and verify a client connection.</p></ServerPanel>}
  </div>
}
