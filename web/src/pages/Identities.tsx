import type { SelectionProps } from '../connections/ManagementRoute'
import { useInitialSelection } from '../connections/useInitialSelection'
import { useCapabilities } from '../interaction'
import { useSources, source } from '../requests'
import { StaleRecovery } from '../ui'
import { useDraft, useOperation } from '../interaction'
import { useEffect, useState } from 'react'
import { Fingerprint, Gauge, Layers3, ListFilter, Pencil, Plus, RefreshCw, Search, ShieldCheck, UserCheck } from 'lucide-react'
import { api, ApiError, type BindingEvaluation, type Condition, type IdentityBinding, type VirtualServer } from '../api'
import { btn, input, primary } from '../styles'
import { AccessRestrictions, GrantedServers, ServerGrantPicker } from '../connections/IdentityPanels'
import { ServerPanel, SummaryTile } from '../connections/ServerPanels'
import { ErrorBox, Modal, Pill } from '../ui'

const slugify = (s: string) =>
  s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 63)

interface Draft {
 version?: number
  slug: string
  isNew: boolean
  description: string
  conditions: Condition[]
  virtualServers: string[]
  ips: string
  rateLimit: string
  enabled: boolean
}

const blank: Draft = {
  slug: '',
  isNew: true,
  description: '',
  conditions: [{ claim: '', op: 'equals', value: '' }],
  virtualServers: [],
  ips: '',
  rateLimit: '',
  enabled: true,
}

export default function Identities({ initialSelection }: SelectionProps) {
  const { edit: canEdit } = useCapabilities()
  const [items, setItems] = useState<IdentityBinding[]>([])
  const [servers, setServers] = useState<VirtualServer[]>([])
  const [error, setError] = useState('')
  const [editing,setEditing]=useState(false)
  const [query,setQuery]=useState('')
  const [status,setStatus]=useState('')
  const [limit,setLimit]=useState(50)
  const [draft, setDraft, resetDraft] = useDraft<Draft | null>(null, d => d ? d.isNew ? 'new' : d.slug : null)
  const [problems, setProblems] = useState<string[]>([])
  const [showEval, setShowEval] = useState(false)
  const [claims, setClaims] = useState('{\n  "azp": "billing-bot"\n}')
  const [evalVS, setEvalVS] = useState('')
  const [evalIP, setEvalIP] = useState('')
  const [result, setResult] = useState<BindingEvaluation | null>(null)

  const { load, loading, readError } = useSources(() => [source('Identity bindings', api.listBindings, setItems), source('Virtual servers', api.listVirtualServers, setServers)])
  useEffect(() => {
    const timer = setTimeout(() => { void load() }, 0)
    return () => clearTimeout(timer)
  }, [load])

  const open = (b?: IdentityBinding) => {
    setProblems([])
    setEditing(!b)
    setDraft(
      b
        ? {
            version:b.version,
 slug: b.slug,
            isNew: false,
            description: b.description,
            conditions: b.conditions.map((c) => ({ ...c, value: c.value ?? '' })),
            virtualServers: b.virtualServers,
            ips: b.ipAllowlist.join('\n'),
            rateLimit: b.rateLimitPerMinute?.toString() ?? '',
            enabled: b.enabled,
          }
        : blank,
    )
  }

  const { busy, run } = useOperation()
  const save = () => run(async () => {
    if (!draft || !canEdit || !editing) return
    const body = {
      description: draft.description,
      conditions: draft.conditions.map((c) => (c.op === 'exists' ? { claim: c.claim, op: c.op } : c)),
      virtualServers: draft.virtualServers,
      ipAllowlist: draft.ips.split(/[\s,]+/).filter(Boolean),
      rateLimitPerMinute: draft.rateLimit ? Number(draft.rateLimit) : null,
      enabled: draft.enabled,
    }
    try {
      if (draft.isNew) await api.createBinding({ slug: draft.slug, ...body })
      else await api.patchBinding(draft.slug, body,draft.version)
      resetDraft(null)
      await load()
    } catch (e) {
      if (e instanceof ApiError) setProblems(e.errors.length ? e.errors.map((x) => `${x.field ?? ''} ${x.message}`.trim()) : [e.message])
      else setProblems([String(e)])
    }
  })

  const evaluate = () => run(async () => {
    setError('');setResult(null)
    try {
      const r = await api.evaluateBindings({
        claims: JSON.parse(claims),
        virtualServer: evalVS || undefined,
        clientIp: evalIP || undefined,
      })
      setResult(r)
    } catch (e) {
      setResult(null)
      setError(e instanceof SyntaxError ? `Claims is not valid JSON: ${e.message}` : e instanceof Error ? e.message : String(e))
    }
  })

  const setCond = (i: number, c: Partial<Condition>) =>
    draft && setDraft({ ...draft, conditions: draft.conditions.map((x, j) => (j === i ? { ...x, ...c } : x)) })
  const toggle = (slug: string) =>
    draft &&
    setDraft({
      ...draft,
      virtualServers: draft.virtualServers.includes(slug) ? draft.virtualServers.filter((s) => s !== slug) : [...draft.virtualServers, slug],
    })

  useInitialSelection(initialSelection, items, open)

  const shown=items.filter(b=>(!status || (b.enabled?'enabled':'disabled')===status) && `${b.slug} ${b.description} ${b.virtualServers.join(' ')} ${b.conditions.map(c=>`${c.claim} ${c.op} ${c.value ?? ''}`).join(' ')}`.toLowerCase().includes(query.toLowerCase()))
  return <div className="mx-auto max-w-7xl space-y-6 px-4 py-6 sm:px-6 lg:px-10 lg:py-10">
    <header className="flex flex-wrap items-start justify-between gap-4"><div className="flex items-start gap-3"><span className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-accent-50 text-accent-700"><Fingerprint className="size-5"/></span><div><h1 className="text-2xl font-semibold tracking-tight">Identities</h1><p className="mt-1 text-sm text-ink-muted">Map verified access-token claims to virtual-server grants.</p></div></div><div className="flex flex-wrap gap-2"><button className={btn} disabled={loading || busy} onClick={()=>{setError('');void load()}}><RefreshCw className="size-3.5"/>Refresh list</button><button disabled={!canEdit || busy} className={btn} onClick={()=>setShowEval(v=>!v)}>Evaluate claims</button><button disabled={!canEdit || busy} className={primary} onClick={()=>open()}><Plus className="size-4"/>New binding</button></div></header>
    {loading && <p role="status" className="text-sm text-ink-muted">Loading…</p>}{(readError || error) && <ErrorBox>{readError || error}</ErrorBox>}
    <div className="grid grid-cols-2 gap-3 xl:grid-cols-4"><SummaryTile label="Configured bindings" value={items.length} detail="Saved identity mappings" icon={Fingerprint}/><SummaryTile label="Enabled bindings" value={items.filter(b=>b.enabled).length} detail="Conditional on verified claims" icon={ShieldCheck}/><SummaryTile label="Claim conditions" value={items.reduce((n,b)=>n+b.conditions.length,0)} detail="All conditions in a binding must match" icon={ListFilter}/><SummaryTile label="Virtual servers granted" value={new Set(items.flatMap(b=>b.virtualServers)).size} detail="Distinct configured server grants" icon={Layers3}/></div>
    <p className="rounded-lg border border-line bg-surface px-4 py-3 text-xs leading-relaxed text-ink-muted">Requires <code className="break-all">VMCP_AGENT_OIDC_ISSUER</code> on the data plane. Verified tokens must match a binding; precedence and network restrictions still apply.</p>
      {showEval && (
        <ServerPanel title="Evaluate claims" description="Check provided claims against saved identity bindings" icon={ListFilter} footer="This evaluates supplied claims; it does not validate a JWT or establish a client session."><fieldset disabled={!canEdit || busy} className="min-w-0 border-0 p-5 m-0">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <label className="block text-sm font-medium">
              Token claims (JSON)
              <textarea className={`${input} mt-1 font-mono`} rows={6} value={claims} onChange={(e) => {setClaims(e.target.value);setResult(null)}} />
            </label>
            <div className="space-y-3">
              <label className="block text-sm font-medium">
                Virtual server
                <select className={`${input} mt-1`} value={evalVS} onChange={(e) => {setEvalVS(e.target.value);setResult(null)}}>
                  <option value="">(any)</option>
                  {servers.map((s) => <option key={s.slug} value={s.slug}>{s.slug}</option>)}
                </select>
              </label>
              <label className="block text-sm font-medium">
                Client IP <span className="font-normal text-ink-muted">(optional)</span>
                <input className={`${input} mt-1`} value={evalIP} onChange={(e) => {setEvalIP(e.target.value);setResult(null)}} />
              </label>
              <button disabled={!canEdit || busy} className={primary} onClick={evaluate}>Evaluate</button>
            </div>
          </div>
          {result && (
            <div role="status" className={`mt-4 rounded-lg p-3 text-sm ${result.decision ? (result.decision.allowed ? 'bg-green-50 text-green-800' : 'bg-red-50 text-red-700') : 'bg-canvas text-ink'}`}>
              <p className="font-medium">{result.decision
                ? (result.decision.allowed ? `Allowed as ${result.decision.principal}` : `Denied${result.decision.reason ? `: ${result.decision.reason}` : ''}`)
                : (result.matches && result.matches.length > 0 ? `${result.matches.length} matching binding(s)` : 'No matching bindings')}</p>
              {result.matches && result.matches.length > 0 && <div className="mt-1 text-xs">Matching bindings: {result.matches.map((m) => m.slug).join(', ')}</div>}
            </div>
          )}
        </fieldset></ServerPanel>
      )}

    <ServerPanel title="Identity bindings" description={`${items.length} saved mappings · select a binding to inspect its conditions`} icon={Fingerprint} footer={`${Math.min(limit,shown.length)} of ${shown.length} matching bindings shown`}>
      <div className="flex flex-wrap gap-3 border-b border-line p-4"><label className="relative w-full sm:max-w-sm"><span className="sr-only">Search identity bindings</span><Search className="absolute left-3 top-2.5 size-4 text-ink-muted"/><input className={`${input} pl-9`} value={query} onChange={e=>{setQuery(e.target.value);setLimit(50)}} placeholder="Search identity, claim or server"/></label><select aria-label="Identity binding status" className={`${input} w-full sm:w-44`} value={status} onChange={e=>{setStatus(e.target.value);setLimit(50)}}><option value="">All statuses</option><option value="enabled">Enabled</option><option value="disabled">Disabled</option></select></div>
      <div className="overflow-x-auto"><table className="w-full min-w-[900px] text-left text-sm"><caption className="sr-only">Configured identity bindings</caption><thead className="bg-canvas text-xs text-ink-muted"><tr>{['Binding','Claim conditions','Virtual servers','Restrictions','Status','Actions'].map(h=><th key={h} scope="col" className="px-5 py-3 font-medium">{h}</th>)}</tr></thead><tbody>{shown.slice(0,limit).map(b=><tr key={b.slug} className="border-t border-line hover:bg-canvas/50"><td className="max-w-60 px-5 py-4"><button className="break-all text-left text-sm font-medium text-accent-700 hover:underline" onClick={()=>open(b)}>{b.slug}</button><p className="mt-1 break-words text-xs text-ink-muted">{b.description}</p></td><td className="max-w-64 px-5 py-4 text-xs">{b.conditions.slice(0,3).map((c,i)=><p className="break-all font-mono" key={i}>{c.claim} <span className="text-ink-muted">{c.op}</span>{c.op!=='exists'?` ${c.value ?? ''}`:''}</p>)}{b.conditions.length>3 && <p className="mt-1 text-ink-muted">+{b.conditions.length-3} more conditions</p>}</td><td className="max-w-48 break-all px-5 py-4 text-xs">{b.virtualServers.join(', ') || 'No grants'}</td><td className="px-5 py-4 text-xs text-ink-muted"><p>{b.ipAllowlist.length?`${b.ipAllowlist.length} IP restrictions`:'Any IP'}</p><p className="mt-1">{b.rateLimitPerMinute==null?'Unlimited':`${b.rateLimitPerMinute} / min`}</p></td><td className="px-5 py-4"><Pill status={b.enabled?'enabled':'disabled'}/></td><td className="px-5 py-4"><button disabled={!canEdit || busy} className={btn} onClick={()=>confirm(`Delete "${b.slug}"?`) && run(async()=>{try{await api.deleteBinding(b.slug,b.version);await load()}catch(e){setError(e instanceof Error?e.message:String(e))}})}>Delete</button></td></tr>)}</tbody></table></div>
      {!loading && shown.length===0 && <div className="px-5 py-10 text-center"><UserCheck className="mx-auto size-6 text-accent-700"/><h3 className="mt-3 text-sm font-medium">{items.length?'No matching identity bindings':readError?'Identity bindings unavailable':'No identity bindings yet.'}</h3><p className="mt-2 text-xs text-ink-muted">{items.length?'Try another search or status.':readError?'Refresh the list to retry.':'Create a binding with verified claim conditions and explicit grants.'}</p></div>}{shown.length>limit && <div className="p-5"><button className={btn} onClick={()=>setLimit(n=>n+50)}>Show 50 more bindings</button></div>}
    </ServerPanel>
    {draft && <Modal size="wide" readOnly={editing && !canEdit} busy={busy} title={draft.isNew?'New identity binding':editing?`Edit ${draft.slug}`:draft.slug} onClose={()=>setDraft(null)}>
      {!editing && !draft.isNew?<div className="space-y-5"><div className="flex flex-wrap items-start justify-between gap-3"><div><p className="text-sm text-ink-muted">{draft.description || 'No description provided.'}</p><p className="mt-2 text-xs text-ink-muted">Saved configuration · version {draft.version}</p></div>{canEdit && <button className={primary} onClick={()=>setEditing(true)}><Pencil className="size-3.5"/>Edit binding</button>}</div><div className="grid gap-3 sm:grid-cols-3"><SummaryTile label="Binding status" value={draft.enabled?'Enabled':'Disabled'} detail="Still conditional on verified claims" icon={ShieldCheck}/><SummaryTile label="Claim conditions" value={draft.conditions.length} detail="All conditions must match" icon={ListFilter}/><SummaryTile label="Rate limit" value={draft.rateLimit?`${draft.rateLimit} / min`:'Unlimited'} detail="Server limits may also apply" icon={Gauge}/></div><ServerPanel title="Claim conditions" description="Every condition must match a verified access token" icon={Fingerprint}><div className="max-h-64 overflow-auto divide-y divide-line">{draft.conditions.map((c,i)=><div key={i} className="flex flex-wrap items-start gap-3 px-5 py-3 text-xs"><span className="min-w-0 break-all font-mono font-medium">{c.claim}</span><span className="rounded-md bg-canvas px-2 py-0.5 text-ink-muted">{c.op}</span>{c.op!=='exists' && <span className="min-w-0 break-all font-mono">{c.value}</span>}</div>)}</div></ServerPanel><div className="grid items-start gap-5 md:grid-cols-2"><GrantedServers slugs={draft.virtualServers} servers={servers}/><AccessRestrictions ips={draft.ips.split(/[\s,]+/).filter(Boolean)} rate={draft.rateLimit?Number(draft.rateLimit):null}/></div><p className="text-xs text-ink-muted">Verified JWT claims, binding precedence, IP restrictions and server policies affect runtime authorization. This configuration is not an active session.</p></div>:<>
        <p className="mb-5 text-sm text-ink-muted">{draft.isNew?'Define the claims an agent must match and the virtual servers it may access.':'Edit conditions and grants. Changes take effect only after Save.'}</p>
        <StaleRecovery message={problems.join('; ')} reload={async()=>{const latest=(await api.listBindings()).find(item=>item.slug===draft.slug);if(!latest)throw new Error('This item was deleted. Cancel the draft and refresh the list.');resetDraft(null);open(latest);setProblems([])}}/>
        <div className="grid items-start gap-5 md:grid-cols-2"><div className="space-y-5"><ServerPanel title="Binding details" description="Purpose and enabled state of this mapping" icon={Fingerprint}><div className="space-y-4 p-5">{draft.isNew && <label className="block text-xs font-medium">Name<input className={`${input} mt-2`} value={draft.slug} onChange={e=>setDraft({...draft,slug:slugify(e.target.value)})} placeholder="billing-agents"/></label>}<label className="block text-xs font-medium">Description<input className={`${input} mt-2`} value={draft.description} onChange={e=>setDraft({...draft,description:e.target.value})}/></label><label className="flex items-center gap-2 text-xs font-medium"><input type="checkbox" checked={draft.enabled} onChange={e=>setDraft({...draft,enabled:e.target.checked})}/>Enabled</label></div></ServerPanel><ServerPanel title="Virtual servers" description="Explicit access grants for matching identities" icon={Layers3}><ServerGrantPicker servers={servers} selected={draft.virtualServers} toggle={toggle}/></ServerPanel></div>
        <div className="space-y-5"><ServerPanel title="Claim conditions" description="All conditions must match a verified token" icon={ListFilter}><div className="space-y-3 p-5">{draft.conditions.map((c,i)=><div key={i} className="space-y-2 rounded-lg border border-line bg-canvas/50 p-3"><p className="text-[11px] font-medium text-ink-muted">Condition {i+1}</p><input className={`${input} font-mono text-xs`} aria-label={`Claim ${i+1}`} placeholder="claim, e.g. azp or realm_access.roles" value={c.claim} onChange={e=>setCond(i,{claim:e.target.value})}/><div className="flex gap-2"><select aria-label={`Operator ${i+1}`} className={`${input} min-w-0 flex-1 text-xs`} value={c.op} onChange={e=>setCond(i,{op:e.target.value as Condition['op']})}><option value="equals">equals</option><option value="contains">contains</option><option value="exists">exists</option></select><button aria-label={`Remove condition ${i+1}`} className={btn} disabled={draft.conditions.length===1} onClick={()=>setDraft({...draft,conditions:draft.conditions.filter((_,j)=>j!==i)})}>×</button></div><input className={`${input} text-xs`} aria-label={`Value ${i+1}`} placeholder={c.op==='exists'?'No value needed':'value'} disabled={c.op==='exists'} value={c.op==='exists'?'':c.value ?? ''} onChange={e=>setCond(i,{value:e.target.value})}/></div>)}<button className={btn} onClick={()=>setDraft({...draft,conditions:[...draft.conditions,{claim:'',op:'equals',value:''}]})}>+ Condition</button></div></ServerPanel>
        <ServerPanel title="Access restrictions" description="Optional network and rate controls" icon={ShieldCheck}><div className="space-y-4 p-5"><label className="block text-xs font-medium">IP allowlist <span className="font-normal text-ink-muted">(CIDR, one per line; empty = any)</span><textarea className={`${input} mt-2 font-mono`} rows={3} value={draft.ips} onChange={e=>setDraft({...draft,ips:e.target.value})}/></label><label className="block text-xs font-medium">Rate limit <span className="font-normal text-ink-muted">(calls per minute, empty = unlimited)</span><input type="number" min={1} className={`${input} mt-2`} value={draft.rateLimit} onChange={e=>setDraft({...draft,rateLimit:e.target.value})}/></label></div></ServerPanel></div></div>
        {problems.length>0 && <ul role="alert" className="mt-4 list-disc rounded-lg bg-red-50 py-3 pl-7 pr-3 text-xs text-red-700">{problems.map((p,i)=><li key={i}>{p}</li>)}</ul>}
        <div className="mt-6 flex justify-end gap-2 border-t border-line pt-4"><button className={btn} onClick={()=>setDraft(null)}>Cancel</button><button className={primary} disabled={busy || !draft.slug} onClick={save}>{draft.isNew?'Create':'Save'}</button></div>
      </>}
    </Modal>}
  </div>
}
