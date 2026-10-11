import { useSources, source } from '../requests'
import { useDraft, useOperation } from '../interaction'
import { useEffect, useState } from 'react'
import GitOps from './GitOps'
import { Plus } from 'lucide-react'
import { api, ApiError, type AdminToken } from '../api'
import { btn, input, primary } from '../styles'
import { fmt } from '../format'
import { ErrorBox, Modal, Pill, SecretModal } from '../ui'

const slugify = (s: string) =>
  s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 63)

export default function SettingsPage({ role }: { role: 'viewer' | 'editor' | 'admin' }) {
  const canManage = role === 'admin'
  const [tokens, setTokens] = useState<AdminToken[]>([])
  const [info, setInfo] = useState<{ version: string; configRevision: number; tenantId?: string } | null>(null)
  const [error, setError] = useState('')
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
      await fn()
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  })

  const { busy, run } = useOperation()
  const create = () => run(async () => {
    if (!form) return
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

  return (
    <div className="mx-auto max-w-6xl px-4 py-6 sm:px-6 lg:px-10 lg:py-10">
      {readError && <ErrorBox>{readError}</ErrorBox>}
      {loading && <p role="status">Loading…</p>}
      <button className={btn} disabled={loading} onClick={()=>{setError('');void load()}}>Refresh list</button>
      <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
      <p className="mt-1 text-sm text-ink-muted">Gateway configuration.</p>
      {error && <ErrorBox>{error}</ErrorBox>}

      <section className="mt-8">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-lg font-semibold">Admin API tokens</h2>
            <p className="text-sm text-ink-muted">For integrations that manage vMCP over the API. <code>read</code> allows GET only.</p>
          </div>
          {canManage && (
            <button className={primary} onClick={() => setForm({ slug: '', description: '', scope: 'read', expiresAt:'' })}>
              <Plus className="size-4" /> New token
            </button>
          )}
        </div>
        <div className="mt-4 overflow-x-auto rounded-card border border-line bg-surface shadow-card">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-line bg-canvas text-xs uppercase tracking-wide text-ink-muted">
              <tr>
                <th className="px-4 py-3">Token</th>
                <th className="px-4 py-3">Scope</th>
                <th className="px-4 py-3">Last used</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3" />
              </tr>
            </thead>
            <tbody>
              {tokens.map((t) => (
                <tr key={t.slug} className="border-b border-line last:border-0">
                  <td className="px-4 py-3">
                    <div className="font-medium">{t.slug}</div>
                    <div className="font-mono text-xs text-ink-muted">{t.tokenPrefix}…</div><div className="text-xs text-ink-muted">{t.description} · expires {fmt(t.expiresAt)}</div>
                  </td>
                  <td className="px-4 py-3">{t.scope}</td>
                  <td className="px-4 py-3">{fmt(t.lastUsedAt)}</td>
                  <td className="px-4 py-3"><Pill status={t.status} /></td>
                  <td className="space-x-2 px-4 py-3 text-right">
                    {canManage && <>
                    <button className={btn} onClick={() => confirm(`Rotate "${t.slug}"?`) && act(async () => { const r = await api.rotateAdminToken(t.slug,t.version); setSecret({ title: 'Token rotated', value: r.token }) })}>Rotate</button>
                    {t.status !== 'revoked' && <button className={btn} onClick={() => confirm(`Revoke "${t.slug}"?`) && act(() => api.revokeAdminToken(t.slug,t.version))}>Revoke</button>}
                    <button className={btn} onClick={() => confirm(`Delete "${t.slug}"?`) && act(() => api.deleteAdminToken(t.slug,t.version))}>Delete</button>
                    </>}
                  </td>
                </tr>
              ))}
              {tokens.length === 0 && <tr><td colSpan={5} className="p-8 text-center text-ink-muted">No admin tokens. The bootstrap token from the environment still works.</td></tr>}
            </tbody>
          </table>
        </div>
      </section>

      <section className="mt-10">
        <h2 className="text-lg font-semibold">About</h2>
        <dl className="mt-3 grid max-w-sm grid-cols-1 sm:grid-cols-2 gap-y-2 rounded-card border border-line bg-surface p-5 text-sm shadow-card">
          <dt className="text-ink-muted">Version</dt><dd>{info?.version ?? '—'}</dd>
          <dt className="text-ink-muted">Tenant</dt><dd>{info?.tenantId ?? 'default'}</dd>
          <dt className="text-ink-muted">Config revision</dt><dd>{info?.configRevision ?? '—'}</dd>
        </dl>
      </section>

      <GitOps canManage={role !== 'viewer'} />
      {form && (
        <Modal busy={busy} title="New admin token" onClose={() => setForm(null)}>
          <div className="mt-4 space-y-4">
            <label className="block text-sm font-medium">
              Name
              <input className={`${input} mt-1`} value={form.slug} onChange={(e) => setForm({ ...form, slug: slugify(e.target.value) })} placeholder="ci-pipeline" />
            </label>
            <label className="block text-sm font-medium">
              Description
              <input className={`${input} mt-1`} value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
            </label>
            <label className="block text-sm font-medium">
              Expires at (optional)
              <input type="datetime-local" className={`${input} mt-1`} value={form.expiresAt} onChange={(e) => setForm({...form,expiresAt:e.target.value})} />
            </label>
            <label className="block text-sm font-medium">
              Scope
              <select className={`${input} mt-1`} value={form.scope} onChange={(e) => setForm({ ...form, scope: e.target.value as 'admin' | 'read' })}>
                <option value="read">read (GET only)</option>
                <option value="admin">admin (full access)</option>
              </select>
            </label>
          </div>
          {problem && <div className="mt-4 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700">{problem}</div>}
          <div className="mt-6 flex justify-end gap-2">
            <button className={btn} onClick={() => setForm(null)}>Cancel</button>
            <button className={primary} disabled={busy || !form.slug} onClick={create}>Create</button>
          </div>
        </Modal>
      )}
      {secret && <SecretModal title={secret.title} secret={secret.value} onClose={() => setSecret(null)} />}
    </div>
  )
}
