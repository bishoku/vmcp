import Button from '../Button'
import { TagBadge } from '../Badge'
import { filterItem } from '../search-filters'
import { SearchField, ActiveFilters, ResultCount } from '../SearchControls'
import { Switch } from '../SelectionControls'
import Select from '../Select'
import { DataTable, LoadMore, RowActions } from '../DataTable'
import { useTableSort } from '../table-sorting'
import { FormField, FormGroup, FormScope, FormErrors } from '../FormField'
import { useFormValidation, requiredText, slugError, rateError } from '../form-validation'
import { useNotifications } from '../notifications'
import { EmptyState, LoadingState } from '../Feedback'
import { useConfirm } from '../confirmation'
import type { SelectionProps } from '../connections/ManagementRoute'
import { useInitialSelection } from '../connections/useInitialSelection'
import { useCapabilities } from '../interaction'
import { useSources, source } from '../requests'
import { StaleRecovery } from '../ui'
import { useDraft, useOperation } from '../interaction'
import { useEffect, useState } from 'react'
import { Fingerprint, Gauge, Layers3, ListFilter, Pencil, Plus, RefreshCw, ShieldCheck, UserCheck, X } from 'lucide-react'
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
  const {notify}=useNotifications()
  const confirm=useConfirm()
  const { edit: canEdit } = useCapabilities()
  const [items, setItems] = useState<IdentityBinding[]>([])
  const [servers, setServers] = useState<VirtualServer[]>([])
  const [error, setError] = useState('')
  const [editing,setEditing]=useState(false)
  const [query,setQuery]=useState('')
  const [status,setStatus]=useState('')
  const [limit,setLimit]=useState(50)
  const [draft, setDraft, resetDraft] = useDraft<Draft | null>(null, d => d ? d.isNew ? 'new' : d.slug : null)
  const validation=useFormValidation()
  const evaluationValidation=useFormValidation()
  const problems=validation.issues.map(issue=>issue.message)
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
    validation.reset()
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
    if(!validation.check({slug:slugError(draft.slug),description:draft.description.length>1000?'Use 1,000 characters or fewer.':undefined,rateLimitPerMinute:rateError(draft.rateLimit),...Object.fromEntries(draft.conditions.flatMap((c,i)=>[[`conditions.${i}.claim`,requiredText(c.claim,'Claim',200) || (/\s/.test(c.claim)?'Claim paths cannot contain spaces.':undefined)],[`conditions.${i}.value`,c.op==='exists'?undefined:requiredText(c.value??'','Value',500)]]))}))return
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
      notify({title:draft.isNew?'Identity binding created':'Identity binding saved',description:draft.slug})
      resetDraft(null)
      await load()
    } catch (e) {
      validation.fail(e)
    }
  })

  const evaluate = () => run(async () => {
    setError('');setResult(null)
    evaluationValidation.reset()
    try {
      const r = await api.evaluateBindings({
        claims: JSON.parse(claims),
        virtualServer: evalVS || undefined,
        clientIp: evalIP || undefined,
      })
      setResult(r)
    } catch (e) {
      setResult(null)
      evaluationValidation.fail(e instanceof SyntaxError?new ApiError(0,'Invalid claims',undefined,[{field:'/claims',message:'Claims is not valid JSON. Enter an object with claim names and values.'}]):e)
    }
  })

  const setCond = (i: number, c: Partial<Condition>) => {
    if(c.op)validation.clear(`conditions.${i}.value`)
    if(draft)setDraft({ ...draft, conditions: draft.conditions.map((x, j) => (j === i ? { ...x, ...c } : x)) })
  }
  const toggle = (slug: string) =>
    draft &&
    setDraft({
      ...draft,
      virtualServers: draft.virtualServers.includes(slug) ? draft.virtualServers.filter((s) => s !== slug) : [...draft.virtualServers, slug],
    })

  useInitialSelection(initialSelection, items, open)

  const shown=items.filter(b=>(!status || (b.enabled?'enabled':'disabled')===status) && `${b.slug} ${b.description} ${b.virtualServers.join(' ')} ${b.conditions.map(c=>`${c.claim} ${c.op} ${c.value ?? ''}`).join(' ')}`.toLowerCase().includes(query.toLowerCase()))
  const sorting=useTableSort(shown,{Binding:b=>b.slug,'Claim conditions':b=>b.conditions.length,'Virtual servers':b=>b.virtualServers.length,Status:b=>b.enabled?'enabled':'disabled'})
  return <FormScope validation={validation}><div className="mx-auto max-w-7xl space-y-6 px-4 py-6 sm:px-6 lg:px-10 lg:py-10">
    <header className="flex flex-wrap items-start justify-between gap-4"><div className="flex items-start gap-3"><span className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-accent-50 text-accent-700"><Fingerprint className="size-5"/></span><div><h1 className="page-title">Identities</h1><p className="page-description mt-1 text-sm text-ink-muted">Map verified access-token claims to virtual-server grants.</p></div></div><div className="flex flex-wrap gap-2"><button className={btn} disabled={loading || busy} onClick={()=>{setError('');void load()}}><RefreshCw className="size-3.5"/>Refresh list</button><button disabled={!canEdit || busy} className={btn} onClick={()=>setShowEval(v=>!v)}>Evaluate claims</button><button disabled={!canEdit || busy} className={primary} onClick={()=>open()}><Plus className="size-4"/>New binding</button></div></header>
    {loading && <LoadingState label="Loading identity bindings…" compact/>}{readError && <ErrorBox title="Saved data could not be loaded" onRetry={load} busy={loading}>{readError}. Available data remains visible; some sections may be incomplete.</ErrorBox>}{error && <ErrorBox>{error}</ErrorBox>}
    <div className="grid grid-cols-2 gap-3 xl:grid-cols-4"><SummaryTile label="Configured bindings" value={items.length} detail="Saved identity mappings" icon={Fingerprint}/><SummaryTile label="Enabled bindings" value={items.filter(b=>b.enabled).length} detail="Conditional on verified claims" icon={ShieldCheck}/><SummaryTile label="Claim conditions" value={items.reduce((n,b)=>n+b.conditions.length,0)} detail="All conditions in a binding must match" icon={ListFilter}/><SummaryTile label="Virtual servers granted" value={new Set(items.flatMap(b=>b.virtualServers)).size} detail="Distinct configured server grants" icon={Layers3}/></div>
    <p className="rounded-lg border border-line bg-surface px-4 py-3 text-xs leading-relaxed text-ink-muted">Requires <code className="break-all">VMCP_AGENT_OIDC_ISSUER</code> on the data plane. Verified tokens must match a binding; precedence and network restrictions still apply.</p>
      {showEval && (
        <FormScope validation={evaluationValidation}><ServerPanel title="Evaluate claims" description="Check provided claims against saved identity bindings" icon={ListFilter} footer="This evaluates supplied claims; it does not validate a JWT or establish a client session."><fieldset disabled={!canEdit || busy} className="min-w-0 border-0 p-5 m-0">
          <FormErrors validation={evaluationValidation} title="Claims could not be evaluated"/><div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <FormField name="claims" label="Token claims (JSON)" required hint="Use a JSON object containing claims to evaluate; this does not verify a JWT."><textarea className={`${input} mt-1 font-mono`} rows={6} value={claims} onChange={(e) => {setClaims(e.target.value);setResult(null)}} /></FormField>
            <div className="space-y-3">
              <FormField name="virtualServer" label="Virtual server" hint="Optional. Limit the evaluation to a saved virtual server."><Select className={`${input} mt-1`} value={evalVS} onChange={(e) => {setEvalVS(e.target.value);setResult(null)}}>
                  <option value="">(any)</option>
                  {servers.map((s) => <option key={s.slug} value={s.slug}>{s.slug}</option>)}
                </Select></FormField>
              <FormField name="clientIp" label="Client IP" hint="Optional. Check network restrictions for this address."><input className={`${input} mt-1`} value={evalIP} onChange={(e) => {setEvalIP(e.target.value);setResult(null)}} /></FormField>
              <Button disabled={!canEdit || busy} variant="primary" loading={busy} onClick={evaluate}>Evaluate</Button>
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
        </fieldset></ServerPanel></FormScope>
      )}

    <ServerPanel title="Identity bindings" description={`${items.length} saved mappings · select a binding to inspect its conditions`} icon={Fingerprint} footer={`${Math.min(limit,shown.length)} of ${shown.length} matching bindings shown`}>
      <div className="flex flex-wrap gap-3 border-b border-line p-4"><SearchField label="Search identity bindings" value={query} onChange={value=>{setQuery(value);setLimit(50)}} placeholder="Search identity, claim or server" className="relative w-full sm:max-w-sm"/><Select aria-label="Identity binding status" className={`${input} w-full sm:w-44`} value={status} onChange={e=>{setStatus(e.target.value);setLimit(50)}}><option value="">All statuses</option><option value="enabled">Enabled</option><option value="disabled">Disabled</option></Select></div>
      <ActiveFilters className="border-b border-line px-4 py-3" label="Active bindings filters" items={[filterItem("Search",query,()=>{setQuery('');setLimit(50)}),filterItem("Status",status,()=>{setStatus('');setLimit(50)})]} onReset={()=>{setQuery('');setStatus('');setLimit(50)}}><ResultCount matched={shown.length} total={items.length} noun="bindings" busy={loading} note={readError?'last loaded configuration':undefined}/></ActiveFilters>
      <DataTable caption="Configured identity bindings" columns={['Binding', 'Claim conditions', 'Virtual servers', 'Restrictions', 'Status', 'Actions']} sorting={sorting} actions={5} minWidth={880}>{sorting.rows.slice(0,limit).map(b=><tr key={b.slug} className="border-t border-line hover:bg-canvas/50"><td className="max-w-60 px-5 py-4"><button className="break-all text-left text-sm font-medium text-accent-700 hover:underline" onClick={()=>open(b)}>{b.slug}</button><p className="mt-1 break-words text-xs text-ink-muted">{b.description}</p></td><td className="max-w-64 px-5 py-4 text-xs">{b.conditions.slice(0,3).map((c,i)=><p className="break-all font-mono" key={i}>{c.claim} <span className="text-ink-muted">{c.op}</span>{c.op!=='exists'?` ${c.value ?? ''}`:''}</p>)}{b.conditions.length>3 && <p className="mt-1 text-ink-muted">+{b.conditions.length-3} more conditions</p>}</td><td className="max-w-48 break-all px-5 py-4 text-xs">{b.virtualServers.join(', ') || 'No grants'}</td><td className="px-5 py-4 text-xs text-ink-muted"><p>{b.ipAllowlist.length?`${b.ipAllowlist.length} IP restrictions`:'Any IP'}</p><p className="mt-1">{b.rateLimitPerMinute==null?'Unlimited':`${b.rateLimitPerMinute} / min`}</p></td><td className="px-5 py-4"><Pill status={b.enabled?'enabled':'disabled'}/></td><td className="px-5 py-4"><RowActions label={`Actions for ${b.slug}`}> <button disabled={!canEdit || busy} className={`${btn} action-danger-outline`} onClick={async()=>await confirm(`Delete "${b.slug}"?`) && run(async()=>{try{await api.deleteBinding(b.slug,b.version);notify({title:'Identity binding deleted',description:b.slug});await load()}catch(e){setError(e instanceof Error?e.message:String(e))}})}>Delete</button></RowActions></td></tr>)}</DataTable>
      {!loading && shown.length===0 && <EmptyState icon={UserCheck} title={items.length?'No matching identity bindings':readError.includes('Identity bindings:')?'Identity bindings unavailable':'No identity bindings yet.'} description={items.length?'Try another search or status.':readError.includes('Identity bindings:')?'Refresh the list to retry.':'Create a binding with verified claim conditions and explicit grants.'}/>}<LoadMore shown={Math.min(limit,shown.length)} total={shown.length} noun="bindings" label="Show 50 more bindings" onLoad={()=>setLimit(n=>n+50)}/>
    </ServerPanel>
    {draft && <Modal size="wide" readOnly={editing && !canEdit} busy={busy} title={draft.isNew?'New identity binding':editing?`Edit ${draft.slug}`:draft.slug} onClose={()=>setDraft(null)}>
      {!editing && !draft.isNew?<div className="space-y-5"><div className="flex flex-wrap items-start justify-between gap-3"><div><p className="text-sm text-ink-muted">{draft.description || 'No description provided.'}</p><p className="mt-2 text-xs text-ink-muted">Saved configuration · version {draft.version}</p></div>{canEdit && <button className={primary} onClick={()=>setEditing(true)}><Pencil className="size-3.5"/>Edit binding</button>}</div><div className="grid gap-3 sm:grid-cols-3"><SummaryTile label="Binding status" value={draft.enabled?'Enabled':'Disabled'} detail="Still conditional on verified claims" icon={ShieldCheck}/><SummaryTile label="Claim conditions" value={draft.conditions.length} detail="All conditions must match" icon={ListFilter}/><SummaryTile label="Rate limit" value={draft.rateLimit?`${draft.rateLimit} / min`:'Unlimited'} detail="Server limits may also apply" icon={Gauge}/></div><ServerPanel headingLevel={3} title="Claim conditions" description="Every condition must match a verified access token" icon={Fingerprint}><div className="divide-y divide-line">{draft.conditions.map((c,i)=><div key={i} className="flex flex-wrap items-start gap-3 px-5 py-3 text-xs"><span className="min-w-0 break-all font-mono font-medium">{c.claim}</span><TagBadge label={c.op}/>{c.op!=='exists' && <span className="min-w-0 break-all font-mono">{c.value}</span>}</div>)}</div></ServerPanel><div className="grid items-start gap-5 md:grid-cols-2"><GrantedServers slugs={draft.virtualServers} servers={servers}/><AccessRestrictions ips={draft.ips.split(/[\s,]+/).filter(Boolean)} rate={draft.rateLimit?Number(draft.rateLimit):null}/></div><p className="text-xs text-ink-muted">Verified JWT claims, binding precedence, IP restrictions and server policies affect runtime authorization. This configuration is not an active session.</p></div>:<>
        <p className="mb-5 text-sm text-ink-muted">{draft.isNew?'Define the claims an agent must match and the virtual servers it may access.':'Edit conditions and grants. Changes take effect only after Save.'}</p>
        <FormErrors validation={validation}/>
        <StaleRecovery message={problems.join('; ')} reload={async()=>{const latest=(await api.listBindings()).find(item=>item.slug===draft.slug);if(!latest)throw new Error('This item was deleted. Cancel the draft and refresh the list.');resetDraft(null);open(latest);validation.reset()}}/>
        <div className="grid items-start gap-5 md:grid-cols-2"><div className="space-y-5"><ServerPanel headingLevel={3} title="Binding details" description="Purpose and enabled state of this mapping" icon={Fingerprint}><div className="space-y-4 p-5">{draft.isNew && <FormField name="slug" label="Name" required hint="Stable identifier: lowercase letters, numbers and hyphens, up to 63 characters."><input className={`${input} mt-2`} value={draft.slug} onChange={e=>setDraft({...draft,slug:slugify(e.target.value)})} placeholder="billing-agents"/></FormField>}<FormField name="description" label="Description" hint="Optional. Explain the purpose in up to 1,000 characters."><input className={`${input} mt-2`} value={draft.description} onChange={e=>setDraft({...draft,description:e.target.value})}/></FormField><Switch label="Enabled" field="enabled" checked={draft.enabled} onChange={e=>setDraft({...draft,enabled:e.target.checked})}/></div></ServerPanel><ServerPanel headingLevel={3} title="Virtual servers" description="Explicit access grants for matching identities" icon={Layers3}><FormGroup name="virtualServers" label="Virtual server grants"><ServerGrantPicker servers={servers} selected={draft.virtualServers} toggle={toggle}/></FormGroup></ServerPanel></div>
        <div className="space-y-5"><ServerPanel headingLevel={3} title="Claim conditions" description="All conditions must match a verified token" icon={ListFilter}><div className="space-y-3 p-5">{draft.conditions.map((c,i)=><div key={i} className="space-y-2 rounded-lg border border-line bg-canvas/50 p-3"><p className="text-[11px] font-medium text-ink-muted">Condition {i+1}</p><FormField name={`conditions.${i}.claim`} label={`Claim ${i+1}`} required hint="Verified claim name or dotted path, up to 200 characters."><input className={`${input} font-mono text-xs`} aria-label={`Claim ${i+1}`} placeholder="claim, e.g. azp or realm_access.roles" value={c.claim} onChange={e=>setCond(i,{claim:e.target.value})}/></FormField><div className="flex gap-2"><FormField name={`conditions.${i}.op`} label={`Operator ${i+1}`} required className="flex-1"><Select aria-label={`Operator ${i+1}`} className={`${input} min-w-0 flex-1 text-xs`} value={c.op} onChange={e=>setCond(i,{op:e.target.value as Condition['op']})}><option value="equals">equals</option><option value="contains">contains</option><option value="exists">exists</option></Select></FormField><button aria-label={`Remove condition ${i+1}`} className={`${btn} action-danger-outline action-icon`} disabled={draft.conditions.length===1} onClick={()=>{validation.clear('conditions');setDraft({...draft,conditions:draft.conditions.filter((_,j)=>j!==i)})}}><X aria-hidden="true" className="size-4"/></button></div><FormField name={`conditions.${i}.value`} label={`Value ${i+1}`} required={c.op!=='exists'} hint={c.op==='exists'?'No value is needed for exists.':'Value to match, up to 500 characters.'}><input className={`${input} text-xs`} aria-label={`Value ${i+1}`} placeholder={c.op==='exists'?'No value needed':'value'} disabled={c.op==='exists'} value={c.op==='exists'?'':c.value ?? ''} onChange={e=>setCond(i,{value:e.target.value})}/></FormField></div>)}<button className={btn} onClick={()=>setDraft({...draft,conditions:[...draft.conditions,{claim:'',op:'equals',value:''}]})}>+ Condition</button></div></ServerPanel>
        <ServerPanel title="Access restrictions" description="Optional network and rate controls" icon={ShieldCheck}><div className="space-y-4 p-5"><FormField name="ipAllowlist" label="IP allowlist" hint="Optional. One IP address or CIDR per line; empty allows any IP."><textarea className={`${input} mt-2 font-mono`} rows={3} value={draft.ips} onChange={e=>setDraft({...draft,ips:e.target.value})}/></FormField><FormField name="rateLimitPerMinute" label="Rate limit" help="Server and credential limits can both apply. Empty means this configuration does not add a rate limit." hint="Optional. 1–1,000,000 calls per minute; empty means unlimited."><input type="number" min={1} className={`${input} mt-2`} value={draft.rateLimit} onChange={e=>setDraft({...draft,rateLimit:e.target.value})}/></FormField></div></ServerPanel></div></div>

        <div className="mt-6 flex justify-end gap-2 border-t border-line pt-4"><button className={btn} onClick={()=>setDraft(null)}>Cancel</button><Button variant="primary" loading={busy} disabled={busy} onClick={save}>{draft.isNew?'Create':'Save'}</Button></div>
      </>}
    </Modal>}
  </div></FormScope>
}
