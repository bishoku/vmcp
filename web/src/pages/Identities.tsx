import type { SelectionProps } from '../connections/ManagementRoute'
import { useInitialSelection } from '../connections/useInitialSelection'
import { useCapabilities } from '../interaction'
import { useSources, source } from '../requests'
import { StaleRecovery } from '../ui'
import { useDraft, useOperation } from '../interaction'
import { useEffect, useState } from 'react'
import { Plus, UserCheck } from 'lucide-react'
import { api, ApiError, type BindingEvaluation, type Condition, type IdentityBinding, type VirtualServer } from '../api'
import { btn, input, primary } from '../styles'
import { ErrorBox, Modal } from '../ui'

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
    if (!draft) return
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

  return (
    <div className="mx-auto max-w-6xl px-4 py-6 sm:px-6 lg:px-10 lg:py-10">
      <div className="flex flex-wrap gap-3 items-start justify-between">
        <div>
          {readError && <ErrorBox>{readError}</ErrorBox>}
      {loading && <p role="status">Loading…</p>}
      <button className={btn} disabled={loading} onClick={()=>{setError('');void load()}}>Refresh list</button>
      <h1 className="text-2xl font-semibold tracking-tight">Identities</h1>
          <p className="mt-1 text-sm text-ink-muted">
            Map claims of IdP-issued access tokens to virtual servers. Requires <code>VMCP_AGENT_OIDC_ISSUER</code> on the data plane; access is denied unless a binding matches.
          </p>
        </div>
        <div className="flex gap-2">
          <button disabled={!canEdit || busy} className={btn} onClick={() => setShowEval((v) => !v)}>Evaluate claims</button>
          <button disabled={!canEdit || busy} className={primary} onClick={() => open()}>
            <Plus className="size-4" /> New binding
          </button>
        </div>
      </div>
      {error && <ErrorBox>{error}</ErrorBox>}

      {showEval && (
        <div className="mt-6 rounded-card border border-line bg-surface p-4 shadow-card">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <label className="block text-sm font-medium">
              Token claims (JSON)
              <textarea className={`${input} mt-1 font-mono`} rows={6} value={claims} onChange={(e) => setClaims(e.target.value)} />
            </label>
            <div className="space-y-3">
              <label className="block text-sm font-medium">
                Virtual server
                <select className={`${input} mt-1`} value={evalVS} onChange={(e) => setEvalVS(e.target.value)}>
                  <option value="">(any)</option>
                  {servers.map((s) => <option key={s.slug} value={s.slug}>{s.slug}</option>)}
                </select>
              </label>
              <label className="block text-sm font-medium">
                Client IP <span className="font-normal text-ink-muted">(optional)</span>
                <input className={`${input} mt-1`} value={evalIP} onChange={(e) => setEvalIP(e.target.value)} />
              </label>
              <button disabled={!canEdit || busy} className={primary} onClick={evaluate}>Evaluate</button>
            </div>
          </div>
          {result && (
            <div className={`mt-4 rounded-lg p-3 text-sm ${result.decision ? (result.decision.allowed ? 'bg-green-50 text-green-800' : 'bg-red-50 text-red-700') : 'bg-canvas text-ink'}`}>
              {result.decision
                ? (result.decision.allowed ? `Allowed as ${result.decision.principal}` : `Denied${result.decision.reason ? `: ${result.decision.reason}` : ''}`)
                : (result.matches && result.matches.length > 0 ? `${result.matches.length} matching binding(s)` : 'No matching bindings')}
              {result.matches && result.matches.length > 0 && <div className="mt-1 text-xs">Matching bindings: {result.matches.map((m) => m.slug).join(', ')}</div>}
            </div>
          )}
        </div>
      )}

      <div className="mt-6 overflow-x-auto rounded-card border border-line bg-surface shadow-card">
        <table className="w-full text-left text-sm">
          <thead className="border-b border-line bg-canvas text-xs uppercase tracking-wide text-ink-muted">
            <tr>
              <th className="px-4 py-3">Binding</th>
              <th className="px-4 py-3">Conditions</th>
              <th className="px-4 py-3">Virtual servers</th>
              <th className="px-4 py-3">IP allowlist</th>
              <th className="px-4 py-3">Rate limit</th>
              <th className="px-4 py-3">Enabled</th>
              <th className="px-4 py-3" />
            </tr>
          </thead>
          <tbody>
            {items.map((b) => (
              <tr key={b.slug} className="border-b border-line last:border-0">
                <td className="px-4 py-3">
                  <button className="font-medium hover:underline" onClick={() => open(b)}>{b.slug}</button>
                  <div className="text-xs text-ink-muted">{b.description}</div>
                </td>
                <td className="px-4 py-3 font-mono text-xs">
                  {b.conditions.map((c, i) => <div key={i}>{c.claim} {c.op}{c.op !== 'exists' ? ` ${c.value ?? ''}` : ''}</div>)}
                </td>
                <td className="px-4 py-3">{b.virtualServers.join(', ') || '—'}</td>
                <td className="px-4 py-3">{b.ipAllowlist.join(', ') || 'any'}</td>
                <td className="px-4 py-3">{b.rateLimitPerMinute ? `${b.rateLimitPerMinute}/min` : '—'}</td>
                <td className="px-4 py-3">{b.enabled ? 'yes' : 'no'}</td>
                <td className="px-4 py-3 text-right">
                  <button disabled={!canEdit || busy} className={btn} onClick={() => confirm(`Delete "${b.slug}"?`) && run(async()=>{try{await api.deleteBinding(b.slug,b.version);await load()}catch(e){setError(e instanceof Error?e.message:String(e))}})}>Delete</button>
                </td>
              </tr>
            ))}
            {!loading && !readError && items.length === 0 && (
              <tr>
                <td colSpan={7} className="p-10 text-center text-ink-muted">
                  <UserCheck className="mx-auto mb-2 size-5" /> No identity bindings yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {draft && (
        <Modal readOnly={!canEdit} busy={busy} title={draft.isNew ? 'New identity binding' : `Edit ${draft.slug}`} onClose={() => setDraft(null)}>
      <StaleRecovery message={problems.join('; ')} reload={async () => {
        const latest = (await api.listBindings()).find(item => item.slug === draft.slug)
        if (!latest) throw new Error('This item was deleted. Cancel the draft and refresh the list.')
        resetDraft(null); open(latest); setProblems([])
      }} />
          <div className="mt-4 space-y-4">
            {draft.isNew && (
              <label className="block text-sm font-medium">
                Name
                <input className={`${input} mt-1`} value={draft.slug} onChange={(e) => setDraft({ ...draft, slug: slugify(e.target.value) })} placeholder="billing-agents" />
              </label>
            )}
            <label className="block text-sm font-medium">
              Description
              <input className={`${input} mt-1`} value={draft.description} onChange={(e) => setDraft({ ...draft, description: e.target.value })} />
            </label>
            <fieldset>
              <legend className="text-sm font-medium">Conditions <span className="font-normal text-ink-muted">(all must match)</span></legend>
              <div className="mt-1 space-y-2">
                {draft.conditions.map((c, i) => (
                  <div key={i} className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                    <input className={`${input} flex-1 font-mono`} aria-label={`Claim ${i+1}`} placeholder="claim, e.g. azp or realm_access.roles" value={c.claim} onChange={(e) => setCond(i, { claim: e.target.value })} />
                    <select aria-label={`Operator ${i+1}`} className={input} value={c.op} onChange={(e) => setCond(i, { op: e.target.value as Condition['op'] })}>
                      <option value="equals">equals</option>
                      <option value="contains">contains</option>
                      <option value="exists">exists</option>
                    </select>
                    <input className={`${input} flex-1`} aria-label={`Value ${i+1}`} placeholder="value" disabled={c.op === 'exists'} value={c.value ?? ''} onChange={(e) => setCond(i, { value: e.target.value })} />
                    <button aria-label={`Remove condition ${i+1}`} className={btn} disabled={draft.conditions.length === 1} onClick={() => setDraft({ ...draft, conditions: draft.conditions.filter((_, j) => j !== i) })}>×</button>
                  </div>
                ))}
                <button className={btn} onClick={() => setDraft({ ...draft, conditions: [...draft.conditions, { claim: '', op: 'equals', value: '' }] })}>+ Condition</button>
              </div>
            </fieldset>
            <fieldset>
              <legend className="text-sm font-medium">Virtual servers</legend>
              <div className="mt-1 max-h-32 space-y-1 overflow-y-auto rounded-lg border border-line p-2">
                {servers.map((s) => (
                  <label key={s.slug} className="flex items-center gap-2 text-sm">
                    <input type="checkbox" checked={draft.virtualServers.includes(s.slug)} onChange={() => toggle(s.slug)} /> {s.name}
                  </label>
                ))}
                {servers.length === 0 && <span className="text-xs text-ink-muted">No virtual servers.</span>}
              </div>
            </fieldset>
            <label className="block text-sm font-medium">
              IP allowlist <span className="font-normal text-ink-muted">(CIDR, one per line; empty = any)</span>
              <textarea className={`${input} mt-1 font-mono`} rows={3} value={draft.ips} onChange={(e) => setDraft({ ...draft, ips: e.target.value })} />
            </label>
            <label className="block text-sm font-medium">
              Rate limit <span className="font-normal text-ink-muted">(calls per minute, empty = unlimited)</span>
              <input type="number" min={1} className={`${input} mt-1`} value={draft.rateLimit} onChange={(e) => setDraft({ ...draft, rateLimit: e.target.value })} />
            </label>
            <label className="flex items-center gap-2 text-sm font-medium">
              <input type="checkbox" checked={draft.enabled} onChange={(e) => setDraft({ ...draft, enabled: e.target.checked })} /> Enabled
            </label>
          </div>
          {problems.length > 0 && (
            <ul className="mt-4 list-disc rounded-lg bg-red-50 py-2 pl-7 pr-3 text-sm text-red-700">
              {problems.map((p, i) => <li key={i}>{p}</li>)}
            </ul>
          )}
          <div className="mt-6 flex justify-end gap-2">
            <button className={btn} onClick={() => setDraft(null)}>Cancel</button>
            <button className={primary} disabled={busy || !draft.slug} onClick={save}>{draft.isNew ? 'Create' : 'Save'}</button>
          </div>
        </Modal>
      )}
    </div>
  )
}
