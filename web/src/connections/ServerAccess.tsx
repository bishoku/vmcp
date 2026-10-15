import { filterItem } from '../search-filters'
import { SearchField, ActiveFilters, ResultCount } from '../SearchControls'
import { LoadMore } from '../DataTable'
import { EmptyState, LoadingState } from '../Feedback'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowUpRight, Fingerprint, Globe, KeyRound, ShieldCheck, UsersRound } from 'lucide-react'
import type { VirtualServer } from '../api'
import type { Relationship } from './model'
import { Pill } from '../ui'
import { btn } from '../styles'
import { ServerPanel, SummaryTile } from './ServerPanels'

export default function ServerAccess({server,grants,loading}:{server:VirtualServer;grants:Relationship[];loading:boolean}) {
  const [query,setQuery]=useState('')
  const [limit,setLimit]=useState(30)
  const shown=grants.filter(g=>`${g.from} ${g.kind} ${g.state} ${g.detail}`.toLowerCase().includes(query.toLowerCase()))
  return <section aria-label="Server access" className="space-y-6">
    <div className="flex flex-wrap items-start justify-between gap-4"><div><h2 className="section-title">Access</h2><p className="section-description mt-1 text-sm text-ink-muted">Configured identities and authentication requirements for this endpoint.</p></div><div className="flex flex-wrap gap-2"><Link className={btn} to="/agent-keys">Manage agent keys<ArrowUpRight className="size-3.5"/></Link><Link className={btn} to="/identities">Manage identity bindings<ArrowUpRight className="size-3.5"/></Link></div></div>
    <div className="grid gap-3 sm:grid-cols-3"><SummaryTile label="Authentication" value={server.authRequired?'Required':'Anonymous'} detail={server.authRequired?'Agent key or verified OAuth access token':'Credentials are bypassed on this server'} icon={server.authRequired?ShieldCheck:Globe}/><SummaryTile label="Agent keys" value={grants.filter(g=>g.kind==='agent-key').length} detail="Includes expired and revoked grants" icon={KeyRound}/><SummaryTile label="JWT bindings" value={grants.filter(g=>g.kind==='binding').length} detail="Conditional on verified matching claims" icon={Fingerprint}/></div>
    <div className="grid items-start gap-6 xl:grid-cols-[1.5fr_1fr]">
      <ServerPanel headingLevel={3} title="Configured access relationships" description={`${grants.length} saved grants · configuration, not active clients`} icon={UsersRound} footer={`${Math.min(limit,shown.length)} of ${shown.length} matching relationships shown`}>
        <div className="border-b border-line p-4"><SearchField label="Search access relationships" value={query} onChange={value=>{setQuery(value);setLimit(30)}} placeholder="Search identity, status or restriction" className="relative block"/></div>
      <ActiveFilters className="border-b border-line px-4 py-3" label="Active relationships filters" items={[filterItem("Search",query,()=>{setQuery('');setLimit(30)})]} onReset={()=>{setQuery('');setLimit(30)}}><ResultCount matched={shown.length} total={grants.length} noun="relationships" busy={loading}/></ActiveFilters>
        <div className="divide-y divide-line">{shown.slice(0,limit).map(grant=><div key={grant.id} className="px-5 py-4"><div className="flex flex-wrap items-start justify-between gap-3"><div className="flex min-w-0 items-center gap-3"><span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-accent-50 text-accent-700">{grant.kind==='agent-key'?<KeyRound className="size-4"/>:grant.kind==='binding'?<Fingerprint className="size-4"/>:<Globe className="size-4"/>}</span><div className="min-w-0"><p className="break-all text-sm font-medium">{grant.kind==='anonymous'?grant.from:<Link className="text-accent-700 hover:underline" to={grant.href}>{grant.from}</Link>}</p><p className="mt-1 text-xs text-ink-muted">{grant.kind==='agent-key'?'Agent key':grant.kind==='binding'?'JWT identity binding':'Public endpoint'}</p></div></div><span className="max-w-full break-words"><Pill status={grant.state}/></span></div><details className="mt-3 text-xs"><summary className="cursor-pointer font-medium text-ink-muted">Restrictions & conditions</summary><p className="mt-2 break-words rounded-lg bg-canvas px-3 py-3 leading-relaxed text-ink-muted">{grant.detail}</p></details></div>)}</div>
        {loading && <LoadingState label="Loading configured access…" compact/>}
        {!loading && shown.length===0 && <EmptyState icon={UsersRound} title={grants.length?'No matching access relationships':'No configured access grants found'} description={grants.length?'Try another identity, status or restriction.':'Related catalog errors may make this list incomplete.'}/>}
        {shown.length>limit && <LoadMore shown={Math.min(limit,shown.length)} total={shown.length} noun="relationships" label="Show 30 more access relationships" onLoad={()=>setLimit(n=>n+30)}/>}
      </ServerPanel>
      <ServerPanel headingLevel={3} title="Authentication behavior" description={server.authRequired?'Credential verification is required':'Anonymous access is enabled'} icon={ShieldCheck}>
        <div className="space-y-4 px-5 py-5 text-xs leading-relaxed text-ink-muted">{server.authRequired?<><p>Agent keys and OAuth access tokens identify callers. Saved grants do not guarantee runtime authorization.</p><div className="border-t border-line pt-4"><h4 className="panel-title font-medium text-ink">Agent key restrictions</h4><p className="supporting-copy mt-2">IP allowlists, expiry, revocation, rate limits and virtual-server state still apply.</p></div><div className="border-t border-line pt-4"><h4 className="panel-title font-medium text-ink">Conditional JWT bindings</h4><p className="supporting-copy mt-2">Verified claims must match. Binding precedence and IP restrictions also apply.</p></div></>:<><p className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-amber-800">Anonymous access bypasses credential authentication. Anyone who can reach the gateway can access the selected capabilities, subject to server policies.</p><p>Saved keys and JWT bindings remain listed for inspection; their credential restrictions do not protect an anonymous endpoint.</p></>}</div>
      </ServerPanel>
    </div>
  </section>
}
