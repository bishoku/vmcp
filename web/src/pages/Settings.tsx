import { useSources, source } from '../requests'
import { useDraft, useOperation } from '../interaction'
import { useEffect, useState } from 'react'
import GitOps from './GitOps'
import { Ban, Clock3, Code2, Download, Fingerprint, KeyRound, Plus, RefreshCw, RotateCw, Search, Server, Settings2, ShieldCheck, Trash2 } from 'lucide-react'
import { api, ApiError, type AdminToken } from '../api'
import { btn, input, primary } from '../styles'
import { fmt } from '../format'
import { DetailRow, ServerPanel, SummaryTile } from '../connections/ServerPanels'
import { ErrorBox, Modal, Pill, SecretModal } from '../ui'

const slugify = (s: string) =>
  s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 63)

const tokenState=(token:AdminToken)=>token.revokedAt || token.status==='revoked'?'revoked':token.status==='expired' || (token.expiresAt && new Date(token.expiresAt).getTime()<=Date.now())?'expired':token.status

export default function SettingsPage({ role }: { role: 'viewer' | 'editor' | 'admin' }) {
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
  const [problem, setProblem] = useState('')
  const [secret, setSecret] = useState<{ title: string; value: string } | null>(null)

  const { load, loading, readError } = useSources(() => [source('Admin tokens', api.listAdminTokens, setTokens), source('Gateway information', api.info, setInfo)])
  useEffect(() => {
    const timer = setTimeout(() => { void load() }, 0)
    return () => clearTimeout(timer)
  }, [load])

  const act = (fn: () => Promise<unknown>) => run(async () => {
    try {
      setError('')
      await fn()
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
      setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  })

  const create = () => run(async () => {
    if (!form || !canManage) return
    try {
      const r = await api.createAdminToken({...form,expiresAt:form.expiresAt ? new Date(form.expiresAt).toISOString():null})
      setSecret({ title: 'Admin token created', value: r.token })
      resetForm(null)
      setProblem('')
      await load()
    } catch (e) {
      setProblem(e instanceof ApiError && e.errors.length ? e.errors.map((x) => x.message).join('; ') : String(e instanceof Error ? e.message : e))
    }
  })

  const shown=tokens.filter(token=>(!scope || token.scope===scope) && (!status || tokenState(token)===status) && `${token.slug} ${token.description} ${token.tokenPrefix}`.toLowerCase().includes(query.toLowerCase()))
  const zone=Intl.DateTimeFormat().resolvedOptions().timeZone
  return <div className="mx-auto max-w-7xl space-y-6 px-4 py-6 sm:px-6 lg:px-10 lg:py-10">
    <header className="flex flex-wrap items-start justify-between gap-4"><div className="flex items-start gap-3"><span className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-accent-50 text-accent-700"><Settings2 className="size-5"/></span><div><h1 className="text-2xl font-semibold tracking-tight">Settings</h1><p className="mt-1 text-sm text-ink-muted">Gateway information, control-plane credentials and configuration workflows.</p></div></div><button className={btn} disabled={loading || busy} onClick={()=>{setError('');void load()}}><RefreshCw className={`size-3.5 ${loading?'animate-spin':''}`}/>Refresh list</button></header>
    {loading && <p role="status" className="text-sm text-ink-muted">Loading…</p>}{(readError || error) && <ErrorBox>{readError || error}</ErrorBox>}
    <div className="grid grid-cols-2 gap-3 xl:grid-cols-4"><SummaryTile label="Gateway version" value={info?.version ?? '—'} detail="Last received gateway information" icon={Server}/><SummaryTile label="Configuration revision" value={info?.configRevision ?? '—'} detail="Saved control-plane revision" icon={Code2}/><SummaryTile label="Admin API tokens" value={tokens.length} detail="Loaded credential metadata" icon={KeyRound}/><SummaryTile label="Your role" value={role[0].toUpperCase()+role.slice(1)} detail={canManage?'Admin token management available':'Admin token changes require an admin'} icon={Fingerprint}/></div>
    <div className="grid items-start gap-6 xl:grid-cols-2">
      <ServerPanel title="Gateway information" description="Version, tenant and saved configuration metadata" icon={Server}><dl className="divide-y divide-line px-5"><DetailRow label="Version">{info?.version ?? '—'}</DetailRow><DetailRow label="Tenant"><span className="break-all font-mono">{info?info.tenantId ?? 'default':'—'}</span></DetailRow><DetailRow label="Config revision">{info?.configRevision ?? '—'}</DetailRow><DetailRow label="Signed-in role">{role}</DetailRow></dl></ServerPanel>
      <ServerPanel title="Control plane API" description="OpenAPI 3.1 specification for this server version" icon={Code2} footer="Downloads use the authenticated control-plane API."><div className="space-y-4 p-5"><p className="text-xs leading-relaxed text-ink-muted">Manage upstream MCP servers, virtual servers and gateway configuration through the API. Download the specification for your integrations.</p><div className="flex flex-wrap gap-2"><button className={btn} disabled={busy} onClick={()=>void downloadSpec('json')}><Download className="size-3.5"/>Download OpenAPI JSON</button><button className={btn} disabled={busy} onClick={()=>void downloadSpec('yaml')}><Download className="size-3.5"/>Download OpenAPI YAML</button></div></div></ServerPanel>
    </div>
    <section aria-label="Admin token management" className="space-y-5"><div className="flex flex-wrap items-start justify-between gap-4"><div><h2 className="text-lg font-semibold tracking-tight">Admin API tokens</h2><p className="mt-1 text-sm text-ink-muted">Credentials for integrations that manage vMCP through the control-plane API.</p></div>{canManage && <button className={primary} disabled={busy} onClick={()=>{setProblem('');setForm({slug:'',description:'',scope:'read',expiresAt:''})}}><Plus className="size-4"/>New token</button>}</div>
      <ServerPanel title="Stored tokens" description={`${tokens.length} saved credentials · secret values cannot be read back`} icon={KeyRound} footer={<div className="flex flex-wrap items-center justify-between gap-3"><span>{Math.min(limit,shown.length)} of {shown.length} matching tokens shown</span><span>{canManage?'Read scope permits GET only.':'Token creation, rotation and revocation require an admin.'}</span></div>}>
        <div className="flex flex-wrap gap-3 border-b border-line p-4"><label className="relative w-full sm:max-w-sm"><span className="sr-only">Search admin tokens</span><Search className="absolute left-3 top-2.5 size-4 text-ink-muted"/><input className={`${input} pl-9`} value={query} onChange={e=>{setQuery(e.target.value);setLimit(50)}} placeholder="Search token, purpose or prefix"/></label><select aria-label="Admin token scope" className={`${input} w-full sm:w-40`} value={scope} onChange={e=>{setScope(e.target.value);setLimit(50)}}><option value="">All scopes</option><option value="read">Read</option><option value="admin">Admin</option></select><select aria-label="Admin token status" className={`${input} w-full sm:w-40`} value={status} onChange={e=>{setStatus(e.target.value);setLimit(50)}}><option value="">All statuses</option>{['active','expired','revoked'].map(state=><option key={state} value={state}>{state[0].toUpperCase()+state.slice(1)}</option>)}</select></div>
        <div className="overflow-x-auto"><table className="w-full min-w-[840px] text-left text-sm"><caption className="sr-only">Configured admin API tokens</caption><thead className="bg-canvas text-xs text-ink-muted"><tr>{['Token','Scope','Expiry','Last used','Status','Actions'].map(label=><th scope="col" key={label} className="px-4 py-3 font-medium">{label}</th>)}</tr></thead><tbody>{shown.slice(0,limit).map(token=><tr key={token.slug} className="border-t border-line hover:bg-canvas/50"><td className="max-w-60 px-4 py-4"><button className="break-all text-left text-sm font-medium text-accent-700 hover:underline" onClick={()=>setSelected(token)}>{token.slug}</button><p className="mt-1 font-mono text-[11px] text-ink-muted">{token.tokenPrefix}…</p>{token.description && <p className="mt-1 break-words text-xs text-ink-muted">{token.description}</p>}</td><td className="px-4 py-4"><span className="rounded-md bg-canvas px-2 py-1 text-xs font-medium">{token.scope}</span></td><td className="whitespace-nowrap px-4 py-4 text-xs text-ink-muted">{token.expiresAt?fmt(token.expiresAt):'No expiry'}</td><td className="whitespace-nowrap px-4 py-4 text-xs text-ink-muted">{token.lastUsedAt?fmt(token.lastUsedAt):'Not recorded'}</td><td className="px-4 py-4"><Pill status={tokenState(token)}/></td><td className="px-4 py-4">{canManage?<div className="flex gap-2"><button className={btn} disabled={busy} aria-label="Rotate" title="Rotate token" onClick={()=>confirm(`Rotate "${token.slug}"? The old secret stops working.`) && act(async()=>{const result=await api.rotateAdminToken(token.slug,token.version);setSecret({title:'Token rotated',value:result.token})})}><RotateCw className="size-3.5"/></button>{tokenState(token)!=='revoked' && <button className={btn} disabled={busy} aria-label="Revoke" title="Revoke token" onClick={()=>confirm(`Revoke "${token.slug}"?`) && act(()=>api.revokeAdminToken(token.slug,token.version))}><Ban className="size-3.5 text-amber-700"/></button>}<button className={btn} disabled={busy} aria-label="Delete" title="Delete token" onClick={()=>confirm(`Delete "${token.slug}"?`) && act(()=>api.deleteAdminToken(token.slug,token.version))}><Trash2 className="size-3.5 text-red-600"/></button></div>:<span className="text-xs text-ink-muted">Read only</span>}</td></tr>)}</tbody></table></div>
        {!loading && shown.length===0 && <div className="px-5 py-10 text-center"><KeyRound className="mx-auto size-6 text-accent-700"/><h3 className="mt-3 text-sm font-medium">{tokens.length?'No matching admin tokens':readError?'Admin tokens unavailable':'No admin tokens'}</h3><p className="mt-2 text-xs text-ink-muted">{tokens.length?'Try another search, scope or status.':readError?'Refresh the list to retry.':'Bootstrap credentials are managed separately in deployment configuration.'}</p></div>}{shown.length>limit && <div className="p-5"><button className={btn} onClick={()=>setLimit(n=>n+50)}>Show 50 more tokens</button></div>}
      </ServerPanel>
    </section>
    <GitOps canManage={role!=='viewer'}/>
    {selected && <Modal size="wide" title={selected.slug} onClose={()=>setSelected(null)}><div className="space-y-5"><p className="text-sm text-ink-muted">{selected.description || 'No description provided.'}</p><div className="grid gap-3 sm:grid-cols-3"><SummaryTile label="Scope" value={selected.scope} detail={selected.scope==='read'?'GET requests only':'Full control-plane access'} icon={ShieldCheck}/><SummaryTile label="Credential status" value={tokenState(selected)} detail="Expiry and revocation are enforced" icon={KeyRound}/><SummaryTile label="Expiry" value={<span className="text-sm">{selected.expiresAt?fmt(selected.expiresAt):'No expiry'}</span>} detail="Configured token lifetime" icon={Clock3}/></div><ServerPanel title="Token metadata" description="Stored metadata only · secret values remain write-only" icon={KeyRound}><dl className="divide-y divide-line px-5"><DetailRow label="Prefix"><span className="font-mono">{selected.tokenPrefix}…</span></DetailRow><DetailRow label="Created">{fmt(selected.createdAt)}</DetailRow><DetailRow label="Last used">{selected.lastUsedAt?fmt(selected.lastUsedAt):'Not recorded'}</DetailRow><DetailRow label="Version">{selected.version}</DetailRow>{selected.revokedAt && <DetailRow label="Revoked">{fmt(selected.revokedAt)}</DetailRow>}</dl></ServerPanel><p className="text-xs text-ink-muted">Admin API tokens authenticate control-plane integrations. Use agent keys or OAuth access tokens to connect to virtual-server endpoints.</p></div></Modal>}
    {form && <Modal size="wide" busy={busy} title="New admin token" onClose={()=>setForm(null)}><p className="mb-5 text-sm text-ink-muted">Create a control-plane credential. The secret will be shown once after creation.</p><div className="grid items-start gap-5 md:grid-cols-2"><ServerPanel title="Token details" description="Name and purpose of this integration" icon={KeyRound}><div className="space-y-4 p-5"><label className="block text-xs font-medium">Name<input className={`${input} mt-2`} value={form.slug} onChange={e=>setForm({...form,slug:slugify(e.target.value)})} placeholder="ci-pipeline"/></label><label className="block text-xs font-medium">Description<input className={`${input} mt-2`} value={form.description} onChange={e=>setForm({...form,description:e.target.value})}/></label></div></ServerPanel><ServerPanel title="Scope & lifetime" description="Choose the access level and optional expiry" icon={ShieldCheck}><div className="space-y-4 p-5"><label className="block text-xs font-medium">Scope<select className={`${input} mt-2`} value={form.scope} onChange={e=>setForm({...form,scope:e.target.value as 'admin'|'read'})}><option value="read">read (GET only)</option><option value="admin">admin (full access)</option></select></label><p className="text-xs leading-relaxed text-ink-muted">{form.scope==='read'?'Read scope permits GET requests only.':'Admin scope permits full control-plane management, including credential changes.'}</p><label className="block text-xs font-medium">Expires at (optional)<input type="datetime-local" className={`${input} mt-2`} value={form.expiresAt} onChange={e=>setForm({...form,expiresAt:e.target.value})}/></label><p className="text-[11px] text-ink-muted">Local time: {zone}. Empty means no expiry.</p></div></ServerPanel></div>{problem && <div role="alert" className="mt-4 rounded-lg bg-red-50 px-3 py-3 text-xs text-red-700">{problem}</div>}<div className="mt-6 flex justify-end gap-2 border-t border-line pt-4"><button className={btn} onClick={()=>setForm(null)}>Cancel</button><button className={primary} disabled={busy || !form.slug} onClick={create}>Create</button></div></Modal>}
    {secret && <SecretModal title={secret.title} secret={secret.value} onClose={()=>setSecret(null)}/>}
  </div>
}
