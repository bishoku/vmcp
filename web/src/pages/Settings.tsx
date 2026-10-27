import AuditSettingsPanel from '../AuditSettingsPanel'
import Button from '../Button'
import { TagBadge } from '../Badge'
import { filterItem } from '../search-filters'
import { SearchField, ActiveFilters, ResultCount } from '../SearchControls'
import DateTimeField from '../DateTimeField'
import Select from '../Select'
import { DataTable, LoadMore, RowActions } from '../DataTable'
import { useTableSort } from '../table-sorting'
import { FormField, FormScope, FormErrors } from '../FormField'
import { useFormValidation, slugError, dateError } from '../form-validation'
import { useNotifications } from '../notifications'
import { EmptyState, LoadingState } from '../Feedback'
import { useConfirm } from '../confirmation'
import { useSources, source } from '../requests'
import { useDraft, useOperation } from '../interaction'
import { useEffect, useState } from 'react'
import GitOps from './GitOps'
import { Ban, Clock3, Code2, Download, Fingerprint, KeyRound, Plus, RefreshCw, RotateCw, Server, Settings2, ShieldCheck, Trash2 } from 'lucide-react'
import { api, type AdminToken } from '../api'
import { btn, input, primary } from '../styles'
import { fmt } from '../format'
import { DetailRow, ServerPanel, SummaryTile } from '../connections/ServerPanels'
import { ErrorBox, Modal, Pill, SecretModal } from '../ui'

const slugify = (s: string) =>
  s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 63)

const tokenState=(token:AdminToken)=>token.revokedAt || token.status==='revoked'?'revoked':token.status==='expired' || (token.expiresAt && new Date(token.expiresAt).getTime()<=Date.now())?'expired':token.status

export default function SettingsPage({ role }: { role: 'viewer' | 'editor' | 'admin' | 'team' }) {
  const {notify}=useNotifications()
  const confirm=useConfirm()
  const canManage = role === 'admin'
  const [tokens, setTokens] = useState<AdminToken[]>([])
  const [info, setInfo] = useState<{ version: string; configRevision: number; tenantId?: string } | null>(null)
  const [error, setError] = useState('')
  const [query,setQuery]=useState('')
  const [scope,setScope]=useState('')
  const [status,setStatus]=useState('')
  const [limit,setLimit]=useState(50)
  const [selected,setSelected]=useState<AdminToken|null>(null)
  const [form, setForm, resetForm] = useDraft<{ slug: string; description: string; scope: 'admin' | 'read'; expiresAt: string } | null>(null, f => f ? 'new' : null)
  const validation=useFormValidation()
  const [secret, setSecret] = useState<{ title: string; value: string } | null>(null)

  const { load, loading, readError } = useSources(() => [source('Admin tokens', api.listAdminTokens, setTokens), source('Gateway information', api.info, setInfo)])
  useEffect(() => {
    const timer = setTimeout(() => { void load() }, 0)
    return () => clearTimeout(timer)
  }, [load])

  const act = (fn: () => Promise<unknown>, title: string) => run(async () => {
    try {
      setError('')
      await fn()
      notify({title})
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  })

  const { busy, run } = useOperation()
  const downloadSpec = (format: 'json' | 'yaml') => run(async () => {
    setError('')
    try {
      const blob = await api.downloadOpenAPI(format)
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = `vmcp-openapi.${format}`
      document.body.appendChild(link)
      link.click()
      link.remove()
      notify({title:'Download started',description:`OpenAPI specification · ${format.toUpperCase()}`})
      setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  })

  const create = () => run(async () => {
    if (!form || !canManage) return
    if(!validation.check({slug:slugError(form.slug),description:form.description.length>1000?'Use 1,000 characters or fewer.':undefined,expiresAt:dateError(form.expiresAt)}))return
    try {
      const r = await api.createAdminToken({...form,expiresAt:form.expiresAt ? new Date(form.expiresAt).toISOString():null})
      setSecret({ title: 'Admin token created', value: r.token })
      notify({title:'Admin token created',description:form.slug})
      resetForm(null)
      validation.reset()
      await load()
    } catch (e) {
      validation.fail(e)
    }
  })

  const shown=tokens.filter(token=>(!scope || token.scope===scope) && (!status || tokenState(token)===status) && `${token.slug} ${token.description} ${token.tokenPrefix}`.toLowerCase().includes(query.toLowerCase()))
  const sorting=useTableSort(shown,{Token:t=>t.slug,Scope:t=>t.scope,Expiry:t=>t.expiresAt?Date.parse(t.expiresAt):null,'Last used':t=>t.lastUsedAt?Date.parse(t.lastUsedAt):null,Status:t=>tokenState(t)})
  return <FormScope validation={validation}><div className="page-layout">
    <header className="flex flex-wrap items-start justify-between gap-4"><div className="flex items-start gap-3"><span className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-accent-50 text-accent-700"><Settings2 aria-hidden="true" className="icon-feature"/></span><div><h1 className="page-title">Settings</h1><p className="page-description mt-1 text-sm text-ink-muted">Gateway information, control-plane credentials and configuration workflows.</p></div></div><button className={btn} disabled={loading || busy} onClick={()=>{setError('');void load()}}><RefreshCw aria-hidden="true" className={`icon-inline ${loading?'motion-spin':''}`}/>Refresh list</button></header>
    {role==='admin'&&<AuditSettingsPanel/>}{loading && <LoadingState label="Loading settings…" compact/>}{readError && <ErrorBox title="Saved data could not be loaded" onRetry={load} busy={loading}>{readError}. Available data remains visible; some sections may be incomplete.</ErrorBox>}{error && <ErrorBox>{error}</ErrorBox>}
    <div className="grid grid-cols-2 gap-3 xl:grid-cols-4"><SummaryTile label="Gateway version" value={info?.version ?? '—'} detail="Last received gateway information" icon={Server}/><SummaryTile label="Configuration revision" value={info?.configRevision ?? '—'} detail="Saved control-plane revision" icon={Code2}/><SummaryTile label="Admin API tokens" value={tokens.length} detail="Loaded credential metadata" icon={KeyRound}/><SummaryTile label="Your role" value={role[0].toUpperCase()+role.slice(1)} detail={canManage?'Admin token management available':'Admin token changes require an admin'} icon={Fingerprint}/></div>
    <div className="grid items-start layout-grid xl:grid-cols-2">
      <ServerPanel title="Gateway information" description="Version, tenant and saved configuration metadata" icon={Server}><dl className="divide-y divide-line panel-inset"><DetailRow label="Version">{info?.version ?? '—'}</DetailRow><DetailRow label="Tenant"><span className="break-all font-mono">{info?info.tenantId ?? 'default':'—'}</span></DetailRow><DetailRow label="Config revision">{info?.configRevision ?? '—'}</DetailRow><DetailRow label="Signed-in role">{role}</DetailRow></dl></ServerPanel>
      <ServerPanel title="Control plane API" description="OpenAPI 3.1 specification for this server version" icon={Code2} footer="Downloads use the authenticated control-plane API."><div className="space-y-4 panel-body"><p className="text-xs leading-relaxed text-ink-muted">Manage upstream MCP servers, virtual servers and gateway configuration through the API. Download the specification for your integrations.</p><div className="flex flex-wrap gap-2"><button className={btn} disabled={busy} onClick={()=>void downloadSpec('json')}><Download aria-hidden="true" className="icon-inline"/>Download OpenAPI JSON</button><button className={btn} disabled={busy} onClick={()=>void downloadSpec('yaml')}><Download aria-hidden="true" className="icon-inline"/>Download OpenAPI YAML</button></div></div></ServerPanel>
    </div>
    <section aria-label="Admin token management" className="layout-stack"><div className="flex flex-wrap items-start justify-between gap-4"><div><h2 className="section-title">Admin API tokens</h2><p className="section-description mt-1 text-sm text-ink-muted">Credentials for integrations that manage vMCP through the control-plane API.</p></div>{canManage && <button className={primary} disabled={busy} onClick={()=>{validation.reset();setForm({slug:'',description:'',scope:'read',expiresAt:''})}}><Plus aria-hidden="true" className="icon-control"/>New token</button>}</div>
      <ServerPanel headingLevel={3} title="Stored tokens" description={`${tokens.length} saved credentials · secret values cannot be read back`} icon={KeyRound} footer={<div className="flex flex-wrap items-center justify-between gap-3"><span>{Math.min(limit,shown.length)} of {shown.length} matching tokens shown</span><span>{canManage?'Read scope permits GET only.':'Token creation, rotation and revocation require an admin.'}</span></div>}>
        <div className="flex flex-wrap gap-3 border-b border-line panel-body"><SearchField label="Search admin tokens" value={query} onChange={value=>{setQuery(value);setLimit(50)}} placeholder="Search token, purpose or prefix" className="relative w-full sm:max-w-sm"/><Select aria-label="Admin token scope" className={`${input} w-full sm:w-40`} value={scope} onChange={e=>{setScope(e.target.value);setLimit(50)}}><option value="">All scopes</option><option value="read">Read</option><option value="admin">Admin</option></Select><Select aria-label="Admin token status" className={`${input} w-full sm:w-40`} value={status} onChange={e=>{setStatus(e.target.value);setLimit(50)}}><option value="">All statuses</option>{['active','expired','revoked'].map(state=><option key={state} value={state}>{state[0].toUpperCase()+state.slice(1)}</option>)}</Select></div>
      <ActiveFilters className="border-b border-line panel-footer" label="Active tokens filters" items={[filterItem("Search",query,()=>{setQuery('');setLimit(50)}),filterItem("Scope",scope,()=>{setScope('');setLimit(50)}),filterItem("Status",status,()=>{setStatus('');setLimit(50)})]} onReset={()=>{setQuery('');setScope('');setStatus('');setLimit(50)}}><ResultCount matched={shown.length} total={tokens.length} noun="tokens" busy={loading} note={readError?'last loaded configuration':undefined}/></ActiveFilters>
        <DataTable caption="Configured admin API tokens" columns={['Token', 'Scope', 'Expiry', 'Last used', 'Status', 'Actions']} sorting={sorting} actions={5} minWidth={840}>{sorting.rows.slice(0,limit).map(token=><tr key={token.slug} className="border-t border-line hover:bg-canvas/50"><td className="max-w-60 px-4 py-4"><button className="break-all text-left text-sm font-medium text-accent-700 hover:underline" onClick={()=>setSelected(token)}>{token.slug}</button><p className="mt-1 font-mono text-[11px] text-ink-muted">{token.tokenPrefix}…</p>{token.description && <p className="mt-1 break-words text-xs text-ink-muted">{token.description}</p>}</td><td className="px-4 py-4"><TagBadge label={token.scope}/></td><td className="whitespace-nowrap px-4 py-4 text-xs text-ink-muted">{token.expiresAt?fmt(token.expiresAt):'No expiry'}</td><td className="whitespace-nowrap px-4 py-4 text-xs text-ink-muted">{token.lastUsedAt?fmt(token.lastUsedAt):'Not recorded'}</td><td className="px-4 py-4"><Pill status={tokenState(token)}/></td><td className="px-4 py-4">{canManage?<RowActions label={`Actions for ${token.slug}`}><button className={`${btn} action-icon`} disabled={busy} aria-label="Rotate" data-tooltip="Rotate token" onClick={async()=>await confirm(`Rotate "${token.slug}"? The old secret stops working.`) && act(async()=>{const result=await api.rotateAdminToken(token.slug,token.version);setSecret({title:'Token rotated',value:result.token})},'Admin token rotated')}><RotateCw aria-hidden="true" className="icon-inline"/></button>{tokenState(token)!=='revoked' && <button className={`${btn} action-danger-outline action-icon`} disabled={busy} aria-label="Revoke" data-tooltip="Revoke token" onClick={async()=>await confirm(`Revoke "${token.slug}"?`) && act(()=>api.revokeAdminToken(token.slug,token.version),'Admin token revoked')}><Ban aria-hidden="true" className="icon-inline text-amber-700"/></button>}<button className={`${btn} action-danger-outline action-icon`} disabled={busy} aria-label="Delete" data-tooltip="Delete token" onClick={async()=>await confirm(`Delete "${token.slug}"?`) && act(()=>api.deleteAdminToken(token.slug,token.version),'Admin token deleted')}><Trash2 aria-hidden="true" className="icon-inline text-red-600"/></button></RowActions>:<span className="text-xs text-ink-muted">Read only</span>}</td></tr>)}</DataTable>
        {!loading && shown.length===0 && <EmptyState icon={KeyRound} title={tokens.length?'No matching admin tokens':readError.includes('Admin tokens:')?'Admin tokens unavailable':'No admin tokens'} description={tokens.length?'Try another search, scope or status.':readError.includes('Admin tokens:')?'Refresh the list to retry.':'Bootstrap credentials are managed separately in deployment configuration.'}/>}<LoadMore shown={Math.min(limit,shown.length)} total={shown.length} noun="tokens" label="Show 50 more tokens" onLoad={()=>setLimit(n=>n+50)}/>
      </ServerPanel>
    </section>
    <GitOps canManage={role!=='viewer'}/>
    {selected && <Modal size="wide" title={selected.slug} onClose={()=>setSelected(null)}><div className="layout-stack"><p className="text-sm text-ink-muted">{selected.description || 'No description provided.'}</p><div className="grid gap-3 sm:grid-cols-3"><SummaryTile label="Scope" value={selected.scope} detail={selected.scope==='read'?'GET requests only':'Full control-plane access'} icon={ShieldCheck}/><SummaryTile label="Credential status" value={tokenState(selected)} detail="Expiry and revocation are enforced" icon={KeyRound}/><SummaryTile label="Expiry" value={<span className="text-sm">{selected.expiresAt?fmt(selected.expiresAt):'No expiry'}</span>} detail="Configured token lifetime" icon={Clock3}/></div><ServerPanel headingLevel={3} title="Token metadata" description="Stored metadata only · secret values remain write-only" icon={KeyRound}><dl className="divide-y divide-line panel-inset"><DetailRow label="Prefix"><span className="font-mono">{selected.tokenPrefix}…</span></DetailRow><DetailRow label="Created">{fmt(selected.createdAt)}</DetailRow><DetailRow label="Last used">{selected.lastUsedAt?fmt(selected.lastUsedAt):'Not recorded'}</DetailRow><DetailRow label="Version">{selected.version}</DetailRow>{selected.revokedAt && <DetailRow label="Revoked">{fmt(selected.revokedAt)}</DetailRow>}</dl></ServerPanel><p className="text-xs text-ink-muted">Admin API tokens authenticate control-plane integrations. Use agent keys or OAuth access tokens to connect to virtual-server endpoints.</p></div></Modal>}
    {form && <Modal size="wide" busy={busy} title="New admin token" onClose={()=>setForm(null)}><FormErrors validation={validation}/><p className="mb-5 text-sm text-ink-muted">Create a control-plane credential. The secret will be shown once after creation.</p><div className="grid items-start layout-grid md:grid-cols-2"><ServerPanel headingLevel={3} title="Token details" description="Name and purpose of this integration" icon={KeyRound}><div className="space-y-4 panel-body"><FormField name="slug" label="Name" required hint="Stable identifier: lowercase letters, numbers and hyphens, up to 63 characters."><input className={`${input} mt-2`} value={form.slug} onChange={e=>setForm({...form,slug:slugify(e.target.value)})} placeholder="ci-pipeline"/></FormField><FormField name="description" label="Description" hint="Optional. Explain the purpose in up to 1,000 characters."><input className={`${input} mt-2`} value={form.description} onChange={e=>setForm({...form,description:e.target.value})}/></FormField></div></ServerPanel><ServerPanel headingLevel={3} title="Scope & lifetime" description="Choose the access level and optional expiry" icon={ShieldCheck}><div className="space-y-4 panel-body"><FormField name="scope" label="Scope" required help="Read tokens permit GET requests only. Admin tokens permit full control-plane management. Use agent credentials for virtual-server endpoints." hint="Read permits GET requests. Admin permits full control-plane management."><Select className={`${input} mt-2`} value={form.scope} onChange={e=>setForm({...form,scope:e.target.value as 'admin'|'read'})}><option value="read">read (GET only)</option><option value="admin">admin (full access)</option></Select></FormField><p className="text-xs leading-relaxed text-ink-muted">{form.scope==='read'?'Read scope permits GET requests only.':'Admin scope permits full control-plane management, including credential changes.'}</p><DateTimeField name="expiresAt" label="Expires at" value={form.expiresAt} onChange={value=>setForm({...form,expiresAt:value})}/></div></ServerPanel></div><div className="mt-6 flex justify-end gap-2 border-t border-line pt-4"><button className={btn} onClick={()=>setForm(null)}>Cancel</button><Button variant="primary" loading={busy} disabled={busy} onClick={create}>Create</Button></div></Modal>}
    {secret && <SecretModal title={secret.title} secret={secret.value} onClose={()=>setSecret(null)}/>}
  </div></FormScope>
}
