import type { SelectionProps } from '../connections/ManagementRoute'
import { useInitialSelection } from '../connections/useInitialSelection'
import { useCapabilities } from '../interaction'
import { useSources, source } from '../requests'
import { StaleRecovery } from '../ui'
import { useDraft, useOperation } from '../interaction'
import { useEffect, useState } from 'react'
import { Ban, Clock3, Gauge, KeyRound, Layers3, Pencil, Plus, RefreshCw, RotateCw, Search, ShieldCheck, Trash2 } from 'lucide-react'
import { api, ApiError, type AgentKey, type VirtualServer } from '../api'
import { btn, input, primary } from '../styles'
import { AccessRestrictions, GrantedServers, ServerGrantPicker } from '../connections/IdentityPanels'
import { DetailRow, ServerPanel, SummaryTile } from '../connections/ServerPanels'
import { keyState } from '../connections/model'
import { fmt } from '../format'
import { ErrorBox, Modal, Pill, SecretModal } from '../ui'

function localDateTime(iso: string | null) {
  if (!iso) return ''
  const date = new Date(iso)
  return new Date(date.getTime() - date.getTimezoneOffset() * 60_000).toISOString().slice(0, 16)
}

const slugify = (s: string) =>
  s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 63)

interface Draft {
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
  const { edit: canEdit } = useCapabilities()
  const [keys, setKeys] = useState<AgentKey[]>([])
  const [servers, setServers] = useState<VirtualServer[]>([])
  const [error, setError] = useState('')
  const [editing,setEditing]=useState(false)
  const [query,setQuery]=useState('')
  const [status,setStatus]=useState('')
  const [limit,setLimit]=useState(50)
  const [draft, setDraft, resetDraft] = useDraft<Draft | null>(null, d => d ? d.isNew ? 'new' : d.slug : null)
  const [problems, setProblems] = useState<string[]>([])
  const [secret, setSecret] = useState<{ title: string; value: string } | null>(null)

  const { load, loading, readError } = useSources(() => [source('Agent keys', api.listAgentKeys, setKeys), source('Virtual servers', api.listVirtualServers, setServers)])
  useEffect(() => {
    const timer = setTimeout(() => { void load() }, 0)
    return () => clearTimeout(timer)
  }, [load])

  const act = (fn: () => Promise<unknown>) => run(async () => {
    try {
      await fn()
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  })

  const open = (k?: AgentKey) => {
    setProblems([])
    setEditing(!k)
    setDraft(
      k
        ? { version:k.version, slug: k.slug, isNew: false, description: k.description, virtualServers: k.virtualServers, ips: k.ipAllowlist.join('\n'), rateLimit: k.rateLimitPerMinute?.toString() ?? '', expiresAt: localDateTime(k.expiresAt), originalExpiry: k.expiresAt, expiryEdited: false }
        : { slug: '', isNew: true, description: '', virtualServers: [], ips: '', rateLimit: '', expiresAt: '', originalExpiry: null, expiryEdited: false },
    )
  }

  const { busy, run } = useOperation()
  const save = () => run(async () => {
    if (!draft || !canEdit || !editing) return
    const body = {
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
      resetDraft(null)
      await load()
    } catch (e) {
      if (e instanceof ApiError) setProblems(e.errors.length ? e.errors.map((x) => `${x.field ?? ''} ${x.message}`.trim()) : [e.message])
      else setProblems([String(e)])
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
  return <div className="mx-auto max-w-7xl space-y-6 px-4 py-6 sm:px-6 lg:px-10 lg:py-10">
    <header className="flex flex-wrap items-start justify-between gap-4"><div className="flex items-start gap-3"><span className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-accent-50 text-accent-700"><KeyRound className="size-5"/></span><div><h1 className="text-2xl font-semibold tracking-tight">Agent Keys</h1><p className="mt-1 text-sm text-ink-muted">Manage agent credentials, virtual-server grants and access restrictions.</p></div></div><div className="flex flex-wrap gap-2"><button className={btn} disabled={loading || busy} onClick={()=>{setError('');void load()}}><RefreshCw className="size-3.5"/>Refresh list</button><button disabled={!canEdit || busy} className={primary} onClick={()=>open()}><Plus className="size-4"/>New key</button></div></header>
    {loading && <p role="status" className="text-sm text-ink-muted">Loading…</p>}{(readError || error) && <ErrorBox>{readError || error}</ErrorBox>}
    <div className="grid grid-cols-2 gap-3 xl:grid-cols-4"><SummaryTile label="Configured keys" value={keys.length} detail="All saved credentials" icon={KeyRound}/><SummaryTile label="Active keys" value={keys.filter(k=>keyState(k)==='active').length} detail="Unexpired & unrevoked · not sessions" icon={ShieldCheck}/><SummaryTile label="With expiry" value={keys.filter(k=>k.expiresAt).length} detail="Explicit credential lifetimes" icon={Clock3}/><SummaryTile label="Virtual servers granted" value={new Set(keys.flatMap(k=>k.virtualServers)).size} detail="Distinct configured server grants" icon={Layers3}/></div>
    <ServerPanel title="Agent credentials" description={`${keys.length} saved keys · select a key to inspect its configuration`} icon={KeyRound} footer={`${Math.min(limit,shown.length)} of ${shown.length} matching keys shown`}>
      <div className="flex flex-wrap gap-3 border-b border-line p-4"><label className="relative w-full sm:max-w-sm"><span className="sr-only">Search agent keys</span><Search className="absolute left-3 top-2.5 size-4 text-ink-muted"/><input className={`${input} pl-9`} value={query} onChange={e=>{setQuery(e.target.value);setLimit(50)}} placeholder="Search key, server or restriction"/></label><select aria-label="Agent key status" className={`${input} w-full sm:w-44`} value={status} onChange={e=>{setStatus(e.target.value);setLimit(50)}}><option value="">All statuses</option>{['active','expired','revoked'].map(s=><option key={s} value={s}>{s[0].toUpperCase()+s.slice(1)}</option>)}</select></div>
      <div className="overflow-x-auto"><table className="w-full min-w-[880px] text-left text-sm"><caption className="sr-only">Configured agent keys</caption><thead className="bg-canvas text-xs text-ink-muted"><tr>{['Key','Virtual servers','Restrictions','Expires','Status','Actions'].map(h=><th key={h} scope="col" className="px-5 py-3 font-medium">{h}</th>)}</tr></thead><tbody>{shown.slice(0,limit).map(k=><tr key={k.slug} className="border-t border-line hover:bg-canvas/50"><td className="max-w-60 px-5 py-4"><button className="break-all text-left text-sm font-medium text-accent-700 hover:underline" onClick={()=>open(k)}>{k.slug}</button><p className="mt-1 text-[11px] font-mono text-ink-muted">{k.keyPrefix}…</p>{k.description && <p className="mt-1 break-words text-xs text-ink-muted">{k.description}</p>}</td><td className="max-w-48 break-all px-5 py-4 text-xs">{k.virtualServers.join(', ') || 'No grants'}</td><td className="px-5 py-4 text-xs text-ink-muted"><p>{k.ipAllowlist.length?`${k.ipAllowlist.length} IP restrictions`:'Any IP'}</p><p className="mt-1">{k.rateLimitPerMinute==null?'Unlimited':`${k.rateLimitPerMinute} / min`}</p></td><td className="whitespace-nowrap px-5 py-4 text-xs text-ink-muted">{k.expiresAt?fmt(k.expiresAt):'No expiry'}</td><td className="px-5 py-4"><Pill status={keyState(k)}/></td><td className="px-5 py-4"><div className="flex gap-2"><button disabled={!canEdit || busy} className={btn} aria-label="Rotate" title="Rotate key" onClick={()=>confirm(`Rotate "${k.slug}"? The old secret stops working.`) && act(async()=>{const r=await api.rotateAgentKey(k.slug,k.version);setSecret({title:'Key rotated',value:r.key})})}><RotateCw className="size-3.5"/></button>{keyState(k)!=='revoked' && <button disabled={!canEdit || busy} className={btn} aria-label="Revoke" title="Revoke key" onClick={()=>confirm(`Revoke "${k.slug}"?`) && act(()=>api.revokeAgentKey(k.slug,k.version))}><Ban className="size-3.5 text-amber-700"/></button>}<button disabled={!canEdit || busy} className={btn} aria-label="Delete" title="Delete key" onClick={()=>confirm(`Delete "${k.slug}"?`) && act(()=>api.deleteAgentKey(k.slug,k.version))}><Trash2 className="size-3.5 text-red-600"/></button></div></td></tr>)}</tbody></table></div>
      {!loading && shown.length===0 && <div className="px-5 py-10 text-center"><KeyRound className="mx-auto size-6 text-accent-700"/><h3 className="mt-3 text-sm font-medium">{keys.length?'No matching agent keys':readError?'Agent keys unavailable':'No agent keys yet.'}</h3><p className="mt-2 text-xs text-ink-muted">{keys.length?'Try another search or status.':readError?'Refresh the list to retry.':'Create a key and grant access to selected virtual servers.'}</p></div>}{shown.length>limit && <div className="p-5"><button className={btn} onClick={()=>setLimit(n=>n+50)}>Show 50 more keys</button></div>}
    </ServerPanel>
    {draft && <Modal size="wide" readOnly={editing && !canEdit} busy={busy} title={draft.isNew?'New agent key':editing?`Edit ${draft.slug}`:draft.slug} onClose={()=>setDraft(null)}>
      {!editing && !draft.isNew?<div className="space-y-5"><div className="flex flex-wrap items-start justify-between gap-3"><div><p className="text-sm text-ink-muted">{draft.description || 'No description provided.'}</p><p className="mt-2 text-xs text-ink-muted">Saved configuration · version {draft.version}</p></div>{canEdit && <button className={primary} onClick={()=>setEditing(true)}><Pencil className="size-3.5"/>Edit key</button>}</div><div className="grid gap-3 sm:grid-cols-3"><SummaryTile label="Credential status" value={selected?keyState(selected):'Unavailable'} detail="Configured credential state" icon={ShieldCheck}/><SummaryTile label="Virtual servers" value={draft.virtualServers.length} detail="Saved access grants" icon={Layers3}/><SummaryTile label="Rate limit" value={draft.rateLimit?`${draft.rateLimit} / min`:'Unlimited'} detail="Server limits may also apply" icon={Gauge}/></div><div className="grid items-start gap-5 md:grid-cols-2"><GrantedServers slugs={draft.virtualServers} servers={servers}/><div className="space-y-5"><AccessRestrictions ips={draft.ips.split(/[\s,]+/).filter(Boolean)} rate={draft.rateLimit?Number(draft.rateLimit):null} expires={draft.originalExpiry?fmt(draft.originalExpiry):'No expiry'}/><ServerPanel title="Credential metadata" description="Secret values cannot be read back" icon={KeyRound}><dl className="divide-y divide-line px-5"><DetailRow label="Key prefix"><span className="font-mono">{selected?.keyPrefix ?? 'Unavailable'}…</span></DetailRow><DetailRow label="Created">{fmt(selected?.createdAt)}</DetailRow>{selected?.revokedAt && <DetailRow label="Revoked">{fmt(selected.revokedAt)}</DetailRow>}</dl></ServerPanel></div></div><p className="text-xs text-ink-muted">Saved grants do not guarantee runtime authorization. IP restrictions, expiry, revocation and server policies still apply.</p></div>:<>
        <p className="mb-5 text-sm text-ink-muted">{draft.isNew?'Issue a credential with explicit virtual-server grants. The secret will be shown once.':'Edit grants and restrictions. Changes take effect only after Save.'}</p>
        <StaleRecovery message={problems.join('; ')} reload={async()=>{const latest=(await api.listAgentKeys()).find(item=>item.slug===draft.slug);if(!latest)throw new Error('This item was deleted. Cancel the draft and refresh the list.');resetDraft(null);open(latest);setProblems([])}}/>
        <div className="grid items-start gap-5 md:grid-cols-2"><div className="space-y-5"><ServerPanel title="Key details" description="Name and purpose of this credential" icon={KeyRound}><div className="space-y-4 p-5">{draft.isNew && <label className="block text-xs font-medium">Name<input className={`${input} mt-2`} value={draft.slug} onChange={e=>setDraft({...draft,slug:slugify(e.target.value)})} placeholder="support-agent"/></label>}<label className="block text-xs font-medium">Description<input className={`${input} mt-2`} value={draft.description} onChange={e=>setDraft({...draft,description:e.target.value})}/></label></div></ServerPanel><ServerPanel title="Virtual servers" description="Grant only the endpoints this agent needs" icon={Layers3}><ServerGrantPicker servers={servers} selected={draft.virtualServers} toggle={toggle}/></ServerPanel></div>
        <ServerPanel title="Restrictions & lifetime" description="Optional network, rate and expiry controls" icon={ShieldCheck}><div className="space-y-4 p-5"><label className="block text-xs font-medium">IP allowlist <span className="font-normal text-ink-muted">(CIDR, one per line; empty = any)</span><textarea className={`${input} mt-2 font-mono`} rows={3} value={draft.ips} onChange={e=>setDraft({...draft,ips:e.target.value})}/></label><label className="block text-xs font-medium">Rate limit <span className="font-normal text-ink-muted">(calls per minute, empty = unlimited)</span><input type="number" min={1} className={`${input} mt-2`} value={draft.rateLimit} onChange={e=>setDraft({...draft,rateLimit:e.target.value})}/></label><label className="block text-xs font-medium">Expires <span className="font-normal text-ink-muted">(optional; local time: {Intl.DateTimeFormat().resolvedOptions().timeZone})</span><input type="datetime-local" className={`${input} mt-2`} value={draft.expiresAt} onChange={e=>setDraft({...draft,expiresAt:e.target.value,expiryEdited:true})}/></label></div></ServerPanel></div>
        {problems.length>0 && <ul role="alert" className="mt-4 list-disc rounded-lg bg-red-50 py-3 pl-7 pr-3 text-xs text-red-700">{problems.map((p,i)=><li key={i}>{p}</li>)}</ul>}
        <div className="mt-6 flex justify-end gap-2 border-t border-line pt-4"><button className={btn} onClick={()=>setDraft(null)}>Cancel</button><button className={primary} disabled={busy || !draft.slug} onClick={save}>{draft.isNew?'Create':'Save'}</button></div>
      </>}
    </Modal>}
    {secret && <SecretModal title={secret.title} secret={secret.value} onClose={()=>setSecret(null)}/>}
  </div>
}
