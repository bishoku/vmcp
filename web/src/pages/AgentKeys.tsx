import { useCapabilities } from '../interaction'
import { useSources, source } from '../requests'
import { StaleRecovery } from '../ui'
import { useDraft, useOperation } from '../interaction'
import { useEffect, useState } from 'react'
import { KeyRound, Plus } from 'lucide-react'
import { api, ApiError, type AgentKey, type VirtualServer } from '../api'
import { btn, input, primary } from '../styles'
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

export default function AgentKeys() {
  const { edit: canEdit } = useCapabilities()
  const [keys, setKeys] = useState<AgentKey[]>([])
  const [servers, setServers] = useState<VirtualServer[]>([])
  const [error, setError] = useState('')
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
    setDraft(
      k
        ? { version:k.version, slug: k.slug, isNew: false, description: k.description, virtualServers: k.virtualServers, ips: k.ipAllowlist.join('\n'), rateLimit: k.rateLimitPerMinute?.toString() ?? '', expiresAt: localDateTime(k.expiresAt), originalExpiry: k.expiresAt, expiryEdited: false }
        : { slug: '', isNew: true, description: '', virtualServers: [], ips: '', rateLimit: '', expiresAt: '', originalExpiry: null, expiryEdited: false },
    )
  }

  const { busy, run } = useOperation()
  const save = () => run(async () => {
    if (!draft) return
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

  return (
    <div className="mx-auto max-w-6xl px-10 py-10">
      <div className="flex items-start justify-between">
        <div>
          {readError && <ErrorBox>{readError}</ErrorBox>}
      {loading && <p role="status">Loading…</p>}
      <button className={btn} disabled={loading} onClick={()=>{setError('');void load()}}>Refresh list</button>
      <h1 className="text-2xl font-semibold tracking-tight">Agent Keys</h1>
          <p className="mt-1 text-sm text-ink-muted">Credentials agents use to call virtual servers.</p>
        </div>
        <button disabled={!canEdit || busy} className={primary} onClick={() => open()}>
          <Plus className="size-4" /> New key
        </button>
      </div>
      {error && <ErrorBox>{error}</ErrorBox>}

      <div className="mt-6 overflow-hidden rounded-card border border-line bg-surface shadow-card">
        <table className="w-full text-left text-sm">
          <thead className="border-b border-line bg-canvas text-xs uppercase tracking-wide text-ink-muted">
            <tr>
              <th className="px-4 py-3">Key</th>
              <th className="px-4 py-3">Virtual servers</th>
              <th className="px-4 py-3">IP allowlist</th>
              <th className="px-4 py-3">Rate limit</th>
              <th className="px-4 py-3">Expires</th>
              <th className="px-4 py-3">Status</th>
              <th className="px-4 py-3" />
            </tr>
          </thead>
          <tbody>
            {keys.map((k) => (
              <tr key={k.slug} className="border-b border-line last:border-0">
                <td className="px-4 py-3">
                  <button className="font-medium hover:underline" onClick={() => open(k)}>{k.slug}</button>
                  <div className="font-mono text-xs text-ink-muted">{k.keyPrefix}…</div>
                </td>
                <td className="px-4 py-3">{k.virtualServers.join(', ') || '—'}</td>
                <td className="px-4 py-3">{k.ipAllowlist.join(', ') || 'any'}</td>
                <td className="px-4 py-3">{k.rateLimitPerMinute ? `${k.rateLimitPerMinute}/min` : '—'}</td>
                <td className="px-4 py-3">{fmt(k.expiresAt)}</td>
                <td className="px-4 py-3"><Pill status={k.status} /></td>
                <td className="space-x-2 px-4 py-3 text-right">
                  <button disabled={!canEdit || busy} className={btn} onClick={() => confirm(`Rotate "${k.slug}"? The old secret stops working.`) && act(async () => { const r = await api.rotateAgentKey(k.slug,k.version); setSecret({ title: 'Key rotated', value: r.key }) })}>Rotate</button>
                  {k.status !== 'revoked' && <button disabled={!canEdit || busy} className={btn} onClick={() => confirm(`Revoke "${k.slug}"?`) && act(() => api.revokeAgentKey(k.slug,k.version))}>Revoke</button>}
                  <button disabled={!canEdit || busy} className={btn} onClick={() => confirm(`Delete "${k.slug}"?`) && act(() => api.deleteAgentKey(k.slug,k.version))}>Delete</button>
                </td>
              </tr>
            ))}
            {!loading && !readError && keys.length === 0 && (
              <tr>
                <td colSpan={7} className="p-10 text-center text-ink-muted">
                  <KeyRound className="mx-auto mb-2 size-5" /> No agent keys yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {draft && (
        <Modal readOnly={!canEdit} busy={busy} title={draft.isNew ? 'New agent key' : `Edit ${draft.slug}`} onClose={() => setDraft(null)}>
      <StaleRecovery message={problems.join('; ')} reload={async () => {
        const latest = (await api.listAgentKeys()).find(item => item.slug === draft.slug)
        if (!latest) throw new Error('This item was deleted. Cancel the draft and refresh the list.')
        resetDraft(null); open(latest); setProblems([])
      }} />
          <div className="mt-4 space-y-4">
            {draft.isNew && (
              <label className="block text-sm font-medium">
                Name
                <input className={`${input} mt-1`} value={draft.slug} onChange={(e) => setDraft({ ...draft, slug: slugify(e.target.value) })} placeholder="support-agent" />
              </label>
            )}
            <label className="block text-sm font-medium">
              Description
              <input className={`${input} mt-1`} value={draft.description} onChange={(e) => setDraft({ ...draft, description: e.target.value })} />
            </label>
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
            <label className="block text-sm font-medium">
              Expires <span className="font-normal text-ink-muted">(optional; local time: {Intl.DateTimeFormat().resolvedOptions().timeZone})</span>
              <input type="datetime-local" className={`${input} mt-1`} value={draft.expiresAt} onChange={(e) => setDraft({ ...draft, expiresAt: e.target.value, expiryEdited: true })} />
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
      {secret && <SecretModal title={secret.title} secret={secret.value} onClose={() => setSecret(null)} />}
    </div>
  )
}
