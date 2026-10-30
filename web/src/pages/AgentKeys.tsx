import Button from '../Button'
import { filterItem } from '../search-filters'
import { SearchField, ActiveFilters, ResultCount } from '../SearchControls'
import DateTimeField from '../DateTimeField'
import { localDateTime } from '../date-time'
import Select from '../Select'
import { DataTable, LoadMore, RowActions } from '../DataTable'
import { useTableSort } from '../table-sorting'
import { FormField, FormGroup, FormScope, FormErrors } from '../FormField'
import { useFormValidation, slugError, rateError, dateError } from '../form-validation'
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
import { Ban, Clock3, Gauge, KeyRound, Layers3, Pencil, Plus, RefreshCw, RotateCw, ShieldCheck, Trash2 } from 'lucide-react'
import { api, type AgentKey, type AuditCorrelation, type VirtualServer } from '../api'
import { btn, input, primary } from '../styles'
import { AccessRestrictions, GrantedServers, ServerGrantPicker } from '../connections/IdentityPanels'
import { DetailRow, ServerPanel, SummaryTile } from '../connections/ServerPanels'
import { keyState } from '../connections/model'
import { fmt } from '../format'
import { ErrorBox, Modal, Pill, SecretModal } from '../ui'


const slugify = (s: string) =>
  s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 63)

interface Draft {
 auditCorrelation:AuditCorrelation
 version?: number
  slug: string
  isNew: boolean
  description: string
  virtualServers: string[]
  ips: string
  rateLimit: string // calls per minute, '' = unlimited
  expiresAt: string // local datetime for editing
  originalExpiry: string | null
  expiryEdited: boolean
}

export default function AgentKeys({ initialSelection }: SelectionProps) {
  const {notify}=useNotifications()
  const confirm=useConfirm()
  const { edit: canEdit } = useCapabilities()
  const [keys, setKeys] = useState<AgentKey[]>([])
  const [servers, setServers] = useState<VirtualServer[]>([])
  const [error, setError] = useState('')
  const [editing,setEditing]=useState(false)
  const [query,setQuery]=useState('')
  const [status,setStatus]=useState('')
  const [limit,setLimit]=useState(50)
  const [draft, setDraft, resetDraft] = useDraft<Draft | null>(null, d => d ? d.isNew ? 'new' : d.slug : null)
  const validation=useFormValidation()
  const problems=validation.issues.map(issue=>issue.message)
  const [secret, setSecret] = useState<{ title: string; value: string } | null>(null)

  const { load, loading, readError } = useSources(() => [source('Agent keys', api.listAgentKeys, setKeys), source('Virtual servers', api.listVirtualServers, setServers)])
  useEffect(() => {
    const timer = setTimeout(() => { void load() }, 0)
    return () => clearTimeout(timer)
  }, [load])

  const act = (fn: () => Promise<unknown>, title: string) => run(async () => {
    try {
      await fn()
      notify({title})
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  })

  const open = (k?: AgentKey) => {
    validation.reset()
    setEditing(!k)
    setDraft(
      k
        ? { auditCorrelation:k.auditCorrelation??{}, version:k.version, slug: k.slug, isNew: false, description: k.description, virtualServers: k.virtualServers, ips: k.ipAllowlist.join('\n'), rateLimit: k.rateLimitPerMinute?.toString() ?? '', expiresAt: localDateTime(k.expiresAt), originalExpiry: k.expiresAt, expiryEdited: false }
        : { auditCorrelation:{}, slug: '', isNew: true, description: '', virtualServers: [], ips: '', rateLimit: '', expiresAt: '', originalExpiry: null, expiryEdited: false },
    )
  }

  const { busy, run } = useOperation()
  const save = () => run(async () => {
    if (!draft || !canEdit || !editing) return
    if(!validation.check({slug:slugError(draft.slug),description:draft.description.length>1000?'Use 1,000 characters or fewer.':undefined,rateLimitPerMinute:rateError(draft.rateLimit),expiresAt:dateError(draft.expiresAt)}))return
    const body = {
      auditCorrelation:draft.auditCorrelation,
 description: draft.description,
      virtualServers: draft.virtualServers,
      ipAllowlist: draft.ips.split(/[\s,]+/).filter(Boolean),
      rateLimitPerMinute: draft.rateLimit ? Number(draft.rateLimit) : null,
      expiresAt: draft.expiryEdited ? (draft.expiresAt ? new Date(draft.expiresAt).toISOString() : null) : draft.originalExpiry,
    }
    try {
      if (draft.isNew) {
        const created = await api.createAgentKey({ slug: draft.slug, ...body })
        setSecret({ title: 'Agent key created', value: created.key })
      } else {
        await api.patchAgentKey(draft.slug, body, draft.version)
      }
      notify({title:draft.isNew?'Agent key created':'Agent key saved',description:draft.slug})
      resetDraft(null)
      await load()
    } catch (e) {
      validation.fail(e)
    }
  })

  const toggle = (slug: string) =>
    draft &&
    setDraft({
      ...draft,
      virtualServers: draft.virtualServers.includes(slug) ? draft.virtualServers.filter((s) => s !== slug) : [...draft.virtualServers, slug],
    })

  useInitialSelection(initialSelection, keys, open)

  const selected=keys.find(k=>k.slug===draft?.slug)
  const shown=keys.filter(k=>(!status || keyState(k)===status) && `${k.slug} ${k.description} ${k.virtualServers.join(' ')} ${k.ipAllowlist.join(' ')}`.toLowerCase().includes(query.toLowerCase()))
  const sorting=useTableSort(shown,{Key:k=>k.slug,'Virtual servers':k=>k.virtualServers.length,Expires:k=>k.expiresAt?Date.parse(k.expiresAt):null,Status:k=>keyState(k)})
  return <FormScope validation={validation}><div className="page-layout">
    <header className="flex flex-wrap items-start justify-between gap-4"><div className="flex items-start gap-3"><span className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-accent-50 text-accent-700"><KeyRound aria-hidden="true" className="icon-feature"/></span><div><h1 className="page-title">Agent Keys</h1><p className="page-description mt-1 text-sm text-ink-muted">Manage agent credentials, virtual-server grants and access restrictions.</p></div></div><div className="flex flex-wrap gap-2"><button className={btn} disabled={loading || busy} onClick={()=>{setError('');void load()}}><RefreshCw aria-hidden="true" className="icon-inline"/>Refresh list</button><button disabled={!canEdit || busy} className={primary} onClick={()=>open()}><Plus aria-hidden="true" className="icon-control"/>New key</button></div></header>
    {loading && <LoadingState label="Loading agent keys…" compact/>}{readError && <ErrorBox title="Saved data could not be loaded" onRetry={load} busy={loading}>{readError}. Available data remains visible; some sections may be incomplete.</ErrorBox>}{error && <ErrorBox>{error}</ErrorBox>}
    <div className="grid grid-cols-2 gap-3 xl:grid-cols-4"><SummaryTile label="Configured keys" value={keys.length} detail="All saved credentials" icon={KeyRound}/><SummaryTile label="Active keys" value={keys.filter(k=>keyState(k)==='active').length} detail="Unexpired & unrevoked · not sessions" icon={ShieldCheck}/><SummaryTile label="With expiry" value={keys.filter(k=>k.expiresAt).length} detail="Explicit credential lifetimes" icon={Clock3}/><SummaryTile label="Virtual servers granted" value={new Set(keys.flatMap(k=>k.virtualServers)).size} detail="Distinct configured server grants" icon={Layers3}/></div>
    <ServerPanel title="Agent credentials" description={`${keys.length} saved keys · select a key to inspect its configuration`} icon={KeyRound} footer={`${Math.min(limit,shown.length)} of ${shown.length} matching keys shown`}>
      <div className="flex flex-wrap gap-3 border-b border-line panel-body"><SearchField label="Search agent keys" value={query} onChange={value=>{setQuery(value);setLimit(50)}} placeholder="Search key, server or restriction" className="relative w-full sm:max-w-sm"/><Select aria-label="Agent key status" className={`${input} w-full sm:w-44`} value={status} onChange={e=>{setStatus(e.target.value);setLimit(50)}}><option value="">All statuses</option>{['active','expired','revoked'].map(s=><option key={s} value={s}>{s[0].toUpperCase()+s.slice(1)}</option>)}</Select></div>
      <ActiveFilters className="border-b border-line panel-footer" label="Active keys filters" items={[filterItem("Search",query,()=>{setQuery('');setLimit(50)}),filterItem("Status",status,()=>{setStatus('');setLimit(50)})]} onReset={()=>{setQuery('');setStatus('');setLimit(50)}}><ResultCount matched={shown.length} total={keys.length} noun="keys" busy={loading} note={readError?'last loaded configuration':undefined}/></ActiveFilters>
      <DataTable caption="Configured agent keys" columns={['Key', 'Virtual servers', 'Restrictions', 'Expires', 'Status', 'Actions']} sorting={sorting} actions={5} minWidth={880}>{sorting.rows.slice(0,limit).map(k=><tr key={k.slug} className="border-t border-line hover:bg-canvas/50"><td className="max-w-60 panel-inset py-4"><button className="break-all text-left text-sm font-medium text-accent-700 hover:underline" onClick={()=>open(k)}>{k.slug}</button><p className="mt-1 text-[11px] font-mono text-ink-muted">{k.keyPrefix}…</p>{k.description && <p className="mt-1 break-words text-xs text-ink-muted">{k.description}</p>}</td><td className="max-w-48 break-all panel-inset py-4 text-xs">{k.virtualServers.join(', ') || 'No grants'}</td><td className="panel-inset py-4 text-xs text-ink-muted"><p>{k.ipAllowlist.length?`${k.ipAllowlist.length} IP restrictions`:'Any IP'}</p><p className="mt-1">{k.rateLimitPerMinute==null?'Unlimited':`${k.rateLimitPerMinute} / min`}</p></td><td className="whitespace-nowrap panel-inset py-4 text-xs text-ink-muted">{k.expiresAt?fmt(k.expiresAt):'No expiry'}</td><td className="panel-inset py-4"><Pill status={keyState(k)}/></td><td className="panel-inset py-4"><RowActions label={`Actions for ${k.slug}`}><button disabled={!canEdit || busy} className={`${btn} action-icon`} aria-label="Rotate" data-tooltip={!canEdit?'Editor access is required to rotate keys.':busy?'Wait for the current operation to finish.':'Rotate key'} onClick={async()=>await confirm(`Rotate "${k.slug}"? The old secret stops working.`) && act(async()=>{const r=await api.rotateAgentKey(k.slug,k.version);setSecret({title:'Key rotated',value:r.key})},'Agent key rotated')}><RotateCw aria-hidden="true" className="icon-inline"/></button>{keyState(k)!=='revoked' && <button disabled={!canEdit || busy} className={`${btn} action-danger-outline action-icon`} aria-label="Revoke" data-tooltip={!canEdit?'Editor access is required to revoke keys.':busy?'Wait for the current operation to finish.':'Revoke key'} onClick={async()=>await confirm(`Revoke "${k.slug}"?`) && act(()=>api.revokeAgentKey(k.slug,k.version),'Agent key revoked')}><Ban aria-hidden="true" className="icon-inline text-amber-700"/></button>}<button disabled={!canEdit || busy} className={`${btn} action-danger-outline action-icon`} aria-label="Delete" data-tooltip={!canEdit?'Editor access is required to delete keys.':busy?'Wait for the current operation to finish.':'Delete key'} onClick={async()=>await confirm(`Delete "${k.slug}"?`) && act(()=>api.deleteAgentKey(k.slug,k.version),'Agent key deleted')}><Trash2 aria-hidden="true" className="icon-inline text-red-600"/></button></RowActions></td></tr>)}</DataTable>
      {!loading && shown.length===0 && <EmptyState icon={KeyRound} title={keys.length?'No matching agent keys':readError.includes('Agent keys:')?'Agent keys unavailable':'No agent keys yet.'} description={keys.length?'Try another search or status.':readError.includes('Agent keys:')?'Refresh the list to retry.':'Create a key and grant access to selected virtual servers.'}/>}<LoadMore shown={Math.min(limit,shown.length)} total={shown.length} noun="keys" label="Show 50 more keys" onLoad={()=>setLimit(n=>n+50)}/>
    </ServerPanel>
    {draft && <Modal size="wide" readOnly={editing && !canEdit} busy={busy} title={draft.isNew?'New agent key':editing?`Edit ${draft.slug}`:draft.slug} onClose={()=>setDraft(null)}>
      {!editing && !draft.isNew?<div className="layout-stack"><div className="flex flex-wrap items-start justify-between gap-3"><div><p className="text-sm text-ink-muted">{draft.description || 'No description provided.'}</p><p className="mt-2 text-xs text-ink-muted">Saved configuration · version {draft.version}</p></div>{canEdit && <button className={primary} onClick={()=>setEditing(true)}><Pencil aria-hidden="true" className="icon-inline"/>Edit key</button>}</div><div className="grid gap-3 sm:grid-cols-3"><SummaryTile label="Credential status" value={selected?keyState(selected):'Unavailable'} detail="Configured credential state" icon={ShieldCheck}/><SummaryTile label="Virtual servers" value={draft.virtualServers.length} detail="Saved access grants" icon={Layers3}/><SummaryTile label="Rate limit" value={draft.rateLimit?`${draft.rateLimit} / min`:'Unlimited'} detail="Server limits may also apply" icon={Gauge}/></div><div className="grid items-start layout-grid md:grid-cols-2"><GrantedServers slugs={draft.virtualServers} servers={servers}/><div className="layout-stack"><AccessRestrictions ips={draft.ips.split(/[\s,]+/).filter(Boolean)} rate={draft.rateLimit?Number(draft.rateLimit):null} expires={draft.originalExpiry?fmt(draft.originalExpiry):'No expiry'}/><ServerPanel title="Credential metadata" description="Secret values cannot be read back" icon={KeyRound}><dl className="divide-y divide-line panel-inset"><DetailRow label="Key prefix"><span className="font-mono">{selected?.keyPrefix ?? 'Unavailable'}…</span></DetailRow><DetailRow label="Created">{fmt(selected?.createdAt)}</DetailRow>{selected?.revokedAt && <DetailRow label="Revoked">{fmt(selected.revokedAt)}</DetailRow>}</dl></ServerPanel></div></div><p className="text-xs text-ink-muted">Audit correlation: {Object.values(draft.auditCorrelation).some(Boolean)?JSON.stringify(draft.auditCorrelation):'Disabled'}. Saved grants do not guarantee runtime authorization. IP restrictions, expiry, revocation and server policies still apply.</p></div>:<>
        <p className="mb-5 text-sm text-ink-muted">{draft.isNew?'Issue a credential with explicit virtual-server grants. The secret will be shown once.':'Edit grants and restrictions. Changes take effect only after Save.'}</p>
        <ServerPanel headingLevel={3} title="Audit correlation" description="Optional client metadata mapping; does not grant access or change upstream forwarding" icon={Layers3}><div className="space-y-4 panel-body"><p className="text-xs text-ink-muted">Paths are JSON Pointers relative to params._meta. Use ~1 for a slash in a metadata key. Empty paths disable extraction. Missing or invalid metadata never blocks a tool call. IDs are stored as searchable audit metadata even when payload capture is off; use non-sensitive identifiers.</p><div className="flex flex-wrap gap-2"><button type="button" className={btn} onClick={()=>setDraft({...draft,auditCorrelation:{conversationId:'/antigravity.google~1conversation_id',rootConversationId:'/antigravity.google~1root_conversation_id'}})}>Use Antigravity mapping</button><button type="button" className={btn} onClick={()=>setDraft({...draft,auditCorrelation:{}})}>Disable correlation</button></div>{(['conversationId','rootConversationId','turnId'] as const).map(field=><FormField key={field} name={`auditCorrelation.${field}`} label={field==='conversationId'?'Conversation ID path':field==='rootConversationId'?'Root conversation ID path':'Turn ID path'}><input maxLength={512} className={`${input} mt-2`} value={draft.auditCorrelation[field]??''} onChange={e=>setDraft({...draft,auditCorrelation:{...draft.auditCorrelation,[field]:e.target.value}})}/></FormField>)}</div></ServerPanel><FormErrors validation={validation}/>
        <StaleRecovery message={problems.join('; ')} reload={async()=>{const latest=(await api.listAgentKeys()).find(item=>item.slug===draft.slug);if(!latest)throw new Error('This item was deleted. Cancel the draft and refresh the list.');resetDraft(null);open(latest);validation.reset()}}/>
        <div className="grid items-start layout-grid md:grid-cols-2"><div className="layout-stack"><ServerPanel headingLevel={3} title="Key details" description="Name and purpose of this credential" icon={KeyRound}><div className="space-y-4 panel-body">{draft.isNew && <FormField name="slug" label="Name" required hint="Stable identifier: lowercase letters, numbers and hyphens, up to 63 characters."><input className={`${input} mt-2`} value={draft.slug} onChange={e=>setDraft({...draft,slug:slugify(e.target.value)})} placeholder="support-agent"/></FormField>}<FormField name="description" label="Description" hint="Optional. Explain the purpose in up to 1,000 characters."><input className={`${input} mt-2`} value={draft.description} onChange={e=>setDraft({...draft,description:e.target.value})}/></FormField></div></ServerPanel><ServerPanel headingLevel={3} title="Virtual servers" description="Grant only the endpoints this agent needs" icon={Layers3}><FormGroup name="virtualServers" label="Virtual server grants"><ServerGrantPicker servers={servers} selected={draft.virtualServers} toggle={toggle}/></FormGroup></ServerPanel></div>
        <ServerPanel headingLevel={3} title="Restrictions & lifetime" description="Optional network, rate and expiry controls" icon={ShieldCheck}><div className="space-y-4 panel-body"><FormField name="ipAllowlist" label="IP allowlist" hint="Optional. One IP address or CIDR per line; empty allows any IP."><textarea className={`${input} mt-2 font-mono`} rows={3} value={draft.ips} onChange={e=>setDraft({...draft,ips:e.target.value})}/></FormField><FormField name="rateLimitPerMinute" label="Rate limit" help="Server and credential limits can both apply. Empty means this configuration does not add a rate limit." hint="Optional. 1–1,000,000 calls per minute; empty means unlimited."><input type="number" min={1} className={`${input} mt-2`} value={draft.rateLimit} onChange={e=>setDraft({...draft,rateLimit:e.target.value})}/></FormField><DateTimeField name="expiresAt" label="Expires" value={draft.expiresAt} onChange={value=>setDraft({...draft,expiresAt:value,expiryEdited:true})}/></div></ServerPanel></div>

        <div className="mt-6 flex justify-end gap-2 border-t border-line pt-4"><button className={btn} onClick={()=>setDraft(null)}>Cancel</button><Button variant="primary" loading={busy} disabled={busy} onClick={save}>{draft.isNew?'Create':'Save'}</Button></div>
      </>}
    </Modal>}
    {secret && <SecretModal title={secret.title} secret={secret.value} onClose={()=>setSecret(null)}/>}
  </div></FormScope>
}
