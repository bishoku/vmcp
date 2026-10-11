import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useInventory } from '../connections/useInventory'
import { capabilityCounts, editorPath, keyState, serverPath } from '../connections/model'
import { useCapabilities } from '../interaction'
import { btn, input } from '../styles'
import { ErrorBox } from '../ui'

export default function ConnectionSetup() {
  const inventory = useInventory()
  const [params] = useSearchParams()
  const [slug,setSlug] = useState(params.get('server') ?? '')
  const {edit} = useCapabilities()
  const server = inventory.servers.find(s=>s.slug===slug)
  const referenced = new Set(server ? [...server.tools.map(t=>t.upstream),...(server.catalog ?? []).map(c=>c.upstream)] : [])
  const upstreams = inventory.upstreams.filter(u=>referenced.has(u.slug))
  const hasKey = inventory.keys.some(k=>k.virtualServers.includes(slug) && keyState(k)==='active')
  const hasBinding = inventory.bindings.some(b=>b.enabled && b.virtualServers.includes(slug))
  const steps = [
    {title:'Register upstreams',description:'Add physical MCP servers and their transport, credentials and TLS settings.',href:'/upstreams',status:inventory.upstreams.length?'Upstreams registered':'Registration needed'},
    {title:'Test connectivity',description:'Open an upstream and deliberately test its stored credentials. This checks the gateway-to-upstream hop only.',href:upstreams[0]?editorPath('upstreams',upstreams[0].slug):'/upstreams',status:'Explicit test required; not inferred from configuration'},
    {title:'Discover capabilities',description:'Refresh each upstream to discover tools, resources, templates and prompts. Check discovery errors before composing.',href:'/upstreams',status:upstreams.length && upstreams.every(u=>u.lastRefreshAt && !u.lastRefreshError)?'Referenced upstreams have discovery snapshots':'Discovery needed or not yet known'},
    {title:'Compose a virtual MCP server',description:'Select capabilities explicitly, set aliases, authentication and protection, then save the draft.',href:server?editorPath('virtual-servers',slug):'/virtual-servers',status:server?`${capabilityCounts(server)} selected`:'Choose or create a virtual server'},
    {title:'Grant client access',description:'Create a scoped agent key or configure a JWT identity binding. Copy new secrets immediately; they are shown once.',href:server?serverPath(slug,'access'):'/agent-keys',status:server?!server.authRequired?'Anonymous access enabled':hasKey||hasBinding?'Configured grants exist; runtime authorization still applies':'No active key or enabled binding found':'Select a virtual server first'},
    {title:'Connect your client',description:'Use the saved gateway endpoint and replace credential placeholders in your client settings.',href:server?serverPath(slug,'connect'):'/virtual-servers',status:'Client setup is a manual step'},
    {title:'Verify the path',description:'Explicitly initialize, ping and list capabilities from Connect. Inspect bounded recent activity separately. No tool is automatically invoked.',href:server?serverPath(slug,'connect'):'/virtual-servers',status:'Explicit client verification required'},
  ]
  return <div className="mx-auto max-w-5xl space-y-6 px-4 py-6 sm:px-6 lg:px-10 lg:py-10"><h1 className="text-2xl font-semibold">Connection setup guide</h1><p>Register → test → discover → compose → grant access → connect → verify.</p>
    {!edit && <p role="status">Read-only access: inspect configuration and verify with a separate gateway credential. An editor must perform configuration changes.</p>}
    {inventory.loading && <p role="status">Loading setup context…</p>}{inventory.readError && <ErrorBox>{inventory.readError}. Step status may be incomplete.</ErrorBox>}
    <label className="block text-sm">Guide for virtual server<select className={input} value={slug} onChange={e=>setSlug(e.target.value)}><option value="">Choose a saved server</option>{inventory.servers.map(s=><option key={s.slug} value={s.slug}>{s.name}</option>)}</select></label>
    <ol className="space-y-4">{steps.map((step,index)=><li key={step.title} className="flex gap-4 rounded-card border border-line bg-surface p-5"><span aria-hidden="true" className="grid size-8 shrink-0 place-items-center rounded-full bg-accent-50 text-accent-700">{index+1}</span><div><h2 className="font-semibold">{step.title}</h2><p className="mt-1 text-sm">{step.description}</p><p className="mt-2 text-xs text-ink-muted">{step.status}</p><Link className={`${btn} mt-3`} to={step.href}>Open {step.title.toLowerCase()}</Link></div></li>)}</ol>
    <Link className={btn} to="/connections">View configured connection flow</Link>
  </div>
}
