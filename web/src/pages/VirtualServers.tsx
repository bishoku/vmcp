import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  arrayMove,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { Copy, GripVertical, Plus, Trash2, X } from 'lucide-react'
import clsx from 'clsx'
import { api, ApiError, type PiiConfig, type PolicyMode, type ToolPolicy, type Tool, type ToolSelection, type Upstream, type VirtualServer } from '../api'

const btn =
  'inline-flex items-center justify-center gap-1.5 rounded-lg border border-line bg-surface px-3 py-1.5 text-sm font-medium text-ink hover:bg-canvas disabled:opacity-50'
const input =
  'w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-accent-500 focus:ring-2 focus:ring-accent-500/20'
const primary =
  'rounded-lg bg-accent-500 px-4 py-2 text-sm font-medium text-white hover:bg-accent-700 disabled:opacity-50'

const key = (s: ToolSelection) => `${s.upstream}\u0000${s.tool}`
const slugify = (s: string) =>
  s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 63)

interface Draft {
 version?: number
  isNew: boolean
  slug: string
  name: string
  description: string
  enabled: boolean
  authRequired: boolean
  pii: PiiConfig
  policy: { default: PolicyMode; tools: Record<string, PolicyMode> }
  rateLimit: string // calls per minute, '' = unlimited
  tools: ToolSelection[]
}

const noPii: PiiConfig = { enabled: false, detectors: [], customRules: [], maskArguments: false }
const DETECTORS = [
  ['email', 'Email'],
  ['phone', 'Phone'],
  ['iban', 'IBAN'],
  ['credit_card', 'Credit card'],
  ['tckn', 'TC Kimlik No'],
  ['us_ssn', 'US SSN'],
]

const blank: Draft = { isNew: true, slug: '', name: '', description: '', enabled: true, authRequired: true, pii: noPii, policy: { default: 'allow', tools: {} }, rateLimit: '', tools: [] }
const fromServer = (v: VirtualServer): Draft => ({
  isNew: false,
  version:v.version,
 slug: v.slug,
  name: v.name,
  description: v.description,
  enabled: v.enabled,
  authRequired: v.authRequired,
  pii: { ...noPii, ...v.pii },
  policy: { default: v.policy?.default ?? 'allow', tools: { ...v.policy?.tools } },
  rateLimit: v.rateLimitPerMinute?.toString() ?? '',
  tools: v.tools.map(({ upstream, tool, alias, description }) => ({ upstream, tool, ...(alias && { alias }), ...(description && { description }) })),
})

const MODES: [PolicyMode, string][] = [
  ['allow', 'Allow'],
  ['read_only', 'Read-only'],
  ['deny', 'Deny'],
]

/** Drops overrides for tools no longer exposed or equal to nothing useful. */
function cleanPolicy(d: Draft): ToolPolicy {
  const names = new Set(d.tools.map((t) => t.alias || `${t.upstream}__${t.tool}`))
  return { default: d.policy.default, tools: Object.fromEntries(Object.entries(d.policy.tools).filter(([n]) => names.has(n))) }
}

export default function VirtualServers() {
  const [servers, setServers] = useState<VirtualServer[]>([])
  const [upstreams, setUpstreams] = useState<Upstream[]>([])
  const [catalog, setCatalog] = useState<Record<string, Tool[]>>({})
  const [draft, setDraft] = useState<Draft | null>(null)
  const [problems, setProblems] = useState<string[]>([])
  const [error, setError] = useState('')
  const [filter, setFilter] = useState('')
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    try {
      const [vs, us] = await Promise.all([api.listVirtualServers(), api.listUpstreams()])
      setServers(vs)
      setUpstreams(us)
      const entries = await Promise.all(
        us.map(async (u) => [u.slug, await api.listTools(u.slug).catch(() => [])] as const),
      )
      setCatalog(Object.fromEntries(entries))
      setError('')
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }, [])
  useEffect(() => {
    const timer = setTimeout(() => { void load() }, 0)
    return () => clearTimeout(timer)
  }, [load])

  const selected = useMemo(() => new Set(draft?.tools.map(key)), [draft])
  const exposed = (s: ToolSelection) => s.alias || `${s.upstream}__${s.tool}`
  const dupes = useMemo(() => {
    const seen = new Map<string, number>()
    draft?.tools.forEach((t) => seen.set(exposed(t), (seen.get(exposed(t)) ?? 0) + 1))
    return new Set([...seen].filter(([, n]) => n > 1).map(([k]) => k))
  }, [draft])

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )
  const onDragEnd = (e: DragEndEvent) => {
    if (!draft || !e.over || e.active.id === e.over.id) return
    const keys = draft.tools.map(key)
    const from = keys.indexOf(String(e.active.id))
    const to = keys.indexOf(String(e.over.id))
    setDraft({ ...draft, tools: arrayMove(draft.tools, from, to) })
  }

  const setPii = (p: Partial<PiiConfig>) => draft && setDraft({ ...draft, pii: { ...draft.pii, ...p } })
  const setMode = (name: string, m: PolicyMode) =>
    draft && setDraft({ ...draft, policy: { ...draft.policy, tools: { ...draft.policy.tools, [name]: m } } })
  const add = (upstream: string, tool: string) =>
    draft && setDraft({ ...draft, tools: [...draft.tools, { upstream, tool }] })
  const patchTool = (k: string, p: Partial<ToolSelection>) =>
    draft &&
    setDraft({
      ...draft,
      tools: draft.tools.map((t) => {
        if (key(t) !== k) return t
        const next = { ...t, ...p }
        if (!next.alias) delete next.alias
        if (!next.description) delete next.description
        return next
      }),
    })
  const removeTool = (k: string) =>
    draft && setDraft({ ...draft, tools: draft.tools.filter((t) => key(t) !== k) })

  const save = async () => {
    if (!draft) return
    setBusy(true)
    setProblems([])
    try {
      await api.putVirtualServer(draft.slug, {
        name: draft.name,
        description: draft.description,
        enabled: draft.enabled,
        authRequired: draft.authRequired,
        pii: draft.pii,
        policy: cleanPolicy(draft),
        rateLimitPerMinute: draft.rateLimit ? Number(draft.rateLimit) : null,
        tools: draft.tools,
      },draft.version)
      setDraft(null)
      await load()
    } catch (e) {
      if (e instanceof ApiError) {
        setProblems(e.errors.length ? e.errors.map((x) => `${x.field ?? ''} ${x.message}`.trim()) : [e.message])
      } else setProblems([String(e)])
    } finally {
      setBusy(false)
    }
  }

  const remove = async (v: VirtualServer) => {
    if (!confirm(`Delete virtual server "${v.slug}"?`)) return
    await api.deleteVirtualServer(v.slug,v.version).catch((e) => setError(String(e.message ?? e)))
    if (draft?.slug === v.slug) setDraft(null)
    await load()
  }

  const endpoint = draft && !draft.isNew ? `${location.protocol}//${location.hostname}:8080/mcp/v1/virtual/${draft.slug}` : ''

  return (
    <div className="mx-auto max-w-7xl px-10 py-10">
      <div className="flex items-start justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Virtual Servers</h1>
          <p className="mt-1 text-sm text-ink-muted">Compose tools into agent-specific endpoints.</p>
        </div>
        <button className={clsx(primary, 'inline-flex items-center gap-1.5')} onClick={() => { setDraft(blank); setProblems([]) }}>
          <Plus className="size-4" /> New virtual server
        </button>
      </div>
      {error && <div className="mt-6 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>}

      <div className="mt-6 grid grid-cols-[260px_1fr] gap-6">
        <div className="rounded-card border border-line bg-surface shadow-card">
          {servers.map((v) => (
            <div
              key={v.slug}
              onClick={() => { setDraft(fromServer(v)); setProblems([]) }}
              className={clsx(
                'flex cursor-pointer items-center justify-between border-b border-line px-4 py-3 last:border-0 hover:bg-canvas',
                draft?.slug === v.slug && !draft.isNew && 'bg-accent-50',
              )}
            >
              <div className="min-w-0">
                <div className="truncate text-sm font-medium">{v.name}</div>
                <div className="text-xs text-ink-muted">{v.tools.length} tools{v.enabled ? '' : ' · disabled'}</div>
              </div>
              <button className="p-1" aria-label="Delete" onClick={(e) => { e.stopPropagation(); void remove(v) }}>
                <Trash2 className="size-4 text-red-600" />
              </button>
            </div>
          ))}
          {servers.length === 0 && <div className="p-6 text-center text-sm text-ink-muted">No virtual servers yet.</div>}
        </div>

        {draft ? (
          <div className="rounded-card border border-line bg-surface p-6 shadow-card">
            <div className="grid grid-cols-3 gap-4">
              <label className="text-sm font-medium">
                Name
                <input
                  className={clsx(input, 'mt-1')}
                  value={draft.name}
                  onChange={(e) =>
                    setDraft({ ...draft, name: e.target.value, ...(draft.isNew && { slug: slugify(e.target.value) }) })
                  }
                />
              </label>
              <label className="text-sm font-medium">
                Slug
                <input className={clsx(input, 'mt-1')} disabled={!draft.isNew} value={draft.slug} onChange={(e) => setDraft({ ...draft, slug: e.target.value })} />
              </label>
              <label className="flex items-end gap-2 pb-2 text-sm">
                <input type="checkbox" checked={draft.enabled} onChange={(e) => setDraft({ ...draft, enabled: e.target.checked })} /> Enabled
              </label>
              <label className="col-span-3 flex items-center gap-2 text-sm">
                <input type="checkbox" checked={draft.authRequired} onChange={(e) => setDraft({ ...draft, authRequired: e.target.checked })} />
                Require an agent key
                {!draft.authRequired && <span className="text-xs text-amber-700">Anyone who can reach the data plane can call these tools.</span>}
              </label>
            </div>
            <label className="mt-4 block text-sm font-medium">
              Description
              <input className={clsx(input, 'mt-1')} value={draft.description} onChange={(e) => setDraft({ ...draft, description: e.target.value })} />
            </label>
            {endpoint && (
              <div className="mt-4 flex items-center gap-2 rounded-lg bg-canvas px-3 py-2 text-xs">
                <code className="flex-1 truncate">{endpoint}</code>
                <button aria-label="Copy endpoint" onClick={() => navigator.clipboard.writeText(endpoint)}>
                  <Copy className="size-3.5" />
                </button>
              </div>
            )}

            <div className="mt-6 grid grid-cols-2 gap-6">
              <section>
                <div className="mb-2 flex items-center justify-between">
                  <h3 className="text-sm font-semibold">Available tools</h3>
                  <input className="w-40 rounded-lg border border-line px-2 py-1 text-xs" placeholder="Filter" value={filter} onChange={(e) => setFilter(e.target.value)} />
                </div>
                <div className="max-h-[420px] space-y-4 overflow-y-auto rounded-lg border border-line p-3">
                  {upstreams.map((u) => {
                    const tools = (catalog[u.slug] ?? []).filter((t) => t.name.toLowerCase().includes(filter.toLowerCase()))
                    return (
                      <div key={u.slug}>
                        <div className="mb-1 text-xs font-semibold uppercase tracking-wide text-ink-muted">{u.name}</div>
                        {tools.map((t) => {
                          const used = selected.has(key({ upstream: u.slug, tool: t.name }))
                          return (
                            <button
                              key={t.name}
                              disabled={used}
                              onClick={() => add(u.slug, t.name)}
                              className="flex w-full items-center justify-between rounded-lg px-2 py-1.5 text-left hover:bg-canvas disabled:opacity-40"
                            >
                              <span className="min-w-0">
                                <span className="block truncate text-sm font-medium">{t.name}</span>
                                <span className="block truncate text-xs text-ink-muted">{t.description}</span>
                              </span>
                              {!used && <Plus className="size-4 shrink-0 text-accent-700" />}
                            </button>
                          )
                        })}
                        {tools.length === 0 && <div className="px-2 text-xs text-ink-muted">No tools. Refresh this upstream first.</div>}
                      </div>
                    )
                  })}
                  {upstreams.length === 0 && <div className="text-xs text-ink-muted">Register an upstream first.</div>}
                </div>
              </section>

              <section>
                <h3 className="mb-2 text-sm font-semibold">Exposed tools ({draft.tools.length})</h3>
                <div className="max-h-[420px] space-y-2 overflow-y-auto rounded-lg border border-dashed border-line p-3">
                  <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
                    <SortableContext items={draft.tools.map(key)} strategy={verticalListSortingStrategy}>
                      {draft.tools.map((t) => (
                        <div key={key(t)}>{servers.find((s) => s.slug === draft.slug)?.tools.find((x) => key(x) === key(t))?.orphaned && <p className="text-sm text-amber-700">Missing upstream tool: {t.upstream}/{t.tool}. Remove it or restore discovery before saving.</p>}<Row key={key(t)} id={key(t)} t={t} mode={draft.policy.tools[exposed(t)] ?? draft.policy.default} inherited={!(exposed(t) in draft.policy.tools)} readOnlyHint={catalog[t.upstream]?.find((x) => x.name === t.tool)?.annotations?.readOnlyHint === true} onMode={(m) => setMode(exposed(t), m)} dup={dupes.has(exposed(t))} upstreamDesc={catalog[t.upstream]?.find((x) => x.name === t.tool)?.description ?? ''} drift={servers.find((s) => s.slug === draft.slug)?.tools.find((x) => key(x) === key(t))?.descriptionDrift === true} onDescription={(d) => patchTool(key(t), { description: d })} onAlias={(a) => patchTool(key(t), { alias: a })} onRemove={() => removeTool(key(t))} /></div>
                      ))}
                    </SortableContext>
                  </DndContext>
                  {draft.tools.length === 0 && <div className="py-8 text-center text-xs text-ink-muted">Pick tools from the left.</div>}
                </div>
              </section>
            </div>

            <section className="mt-6 rounded-lg border border-line p-4">
              <h3 className="text-sm font-semibold">Rate limit</h3>
              <label className="mt-2 flex items-center gap-2 text-sm">
                Tool calls per minute for this server
                <input type="number" min={1} placeholder="unlimited" className="w-32 rounded-lg border border-line px-2 py-1 text-sm" value={draft.rateLimit} onChange={(e) => setDraft({ ...draft, rateLimit: e.target.value })} />
              </label>
              <p className="mt-1 text-xs text-ink-muted">Shared by all agents. Each agent key can have its own limit too. Excess calls get HTTP 429.</p>
            </section>

            <section className="mt-4 rounded-lg border border-line p-4">
              <h3 className="text-sm font-semibold">Tool policy</h3>
              <label className="mt-2 flex items-center gap-2 text-sm">
                Default for tools without a rule
                <select className="rounded-lg border border-line px-2 py-1 text-sm" value={draft.policy.default} onChange={(e) => setDraft({ ...draft, policy: { ...draft.policy, default: e.target.value as PolicyMode } })}>
                  {MODES.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
                </select>
              </label>
              <p className="mt-1 text-xs text-ink-muted">Read-only lets through only tools whose upstream declares <code>readOnlyHint</code>. Set per-tool rules in the list above.</p>
            </section>

            <section className="mt-4 rounded-lg border border-line p-4">
              <h3 className="text-sm font-semibold">PII masking</h3>
              <label className="mt-2 flex items-center gap-2 text-sm">
                <input type="checkbox" checked={draft.pii.enabled} onChange={(e) => setPii({ enabled: e.target.checked })} />
                Mask sensitive data in tool results
              </label>
              {draft.pii.enabled && (
                <div className="mt-3 space-y-3">
                  <div className="grid grid-cols-3 gap-2">
                    {DETECTORS.map(([id, label]) => (
                      <label key={id} className="flex items-center gap-2 text-sm">
                        <input
                          type="checkbox"
                          checked={draft.pii.detectors.includes(id)}
                          onChange={(e) => setPii({ detectors: e.target.checked ? [...draft.pii.detectors, id] : draft.pii.detectors.filter((d) => d !== id) })}
                        />
                        {label}
                      </label>
                    ))}
                  </div>
                  <label className="flex items-center gap-2 text-sm">
                    <input type="checkbox" checked={draft.pii.maskArguments} onChange={(e) => setPii({ maskArguments: e.target.checked })} />
                    Also mask tool arguments sent upstream
                  </label>
                  <div className="space-y-2">
                    {draft.pii.customRules.map((r, i) => (
                      <div key={i} className="flex gap-2">
                        <input className={input} placeholder="Name" value={r.name} onChange={(e) => setPii({ customRules: draft.pii.customRules.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)) })} />
                        <input className={clsx(input, 'font-mono')} placeholder="Regex (RE2)" value={r.pattern} onChange={(e) => setPii({ customRules: draft.pii.customRules.map((x, j) => (j === i ? { ...x, pattern: e.target.value } : x)) })} />
                        <button aria-label="Remove rule" onClick={() => setPii({ customRules: draft.pii.customRules.filter((_, j) => j !== i) })}><X className="size-4 text-ink-muted" /></button>
                      </div>
                    ))}
                    <button className={btn} disabled={draft.pii.customRules.length >= 20} onClick={() => setPii({ customRules: [...draft.pii.customRules, { name: '', pattern: '' }] })}>
                      <Plus className="size-4" /> Custom rule
                    </button>
                  </div>
                </div>
              )}
            </section>

            {problems.length > 0 && (
              <ul className="mt-4 list-disc rounded-lg bg-red-50 py-2 pl-7 pr-3 text-sm text-red-700">
                {problems.map((p, i) => <li key={i}>{p}</li>)}
              </ul>
            )}
            <div className="mt-6 flex justify-end gap-2">
              <button className={btn} onClick={() => setDraft(null)}>Cancel</button>
              <button className={primary} disabled={busy || !draft.name || !draft.slug || dupes.size > 0} onClick={save}>Save</button>
            </div>
          </div>
        ) : (
          <div className="grid place-items-center rounded-card border border-dashed border-line p-16 text-sm text-ink-muted">
            Select or create a virtual server.
          </div>
        )}
      </div>
    </div>
  )
}

function Row({ id, t, dup, mode, inherited, readOnlyHint, upstreamDesc, drift, onDescription, onMode, onAlias, onRemove }: { id: string; t: ToolSelection; upstreamDesc: string; drift: boolean; onDescription: (d: string) => void; dup: boolean; mode: PolicyMode; inherited: boolean; readOnlyHint: boolean; onMode: (m: PolicyMode) => void; onAlias: (a: string) => void; onRemove: () => void }) {
  const { attributes, listeners, setNodeRef, transform, transition } = useSortable({ id })
  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={clsx('flex items-center gap-2 rounded-lg border bg-surface px-2 py-2', dup ? 'border-red-300' : 'border-line')}
    >
      <button className="cursor-grab touch-none text-ink-muted" aria-label="Drag" {...attributes} {...listeners}>
        <GripVertical className="size-4" />
      </button>
      <div className="min-w-0 flex-1">
        <div className="truncate text-xs text-ink-muted">{t.upstream} / {t.tool}</div>
        <input
          className="w-full bg-transparent text-sm font-medium outline-none placeholder:text-ink"
          placeholder={`${t.upstream}__${t.tool}`}
          value={t.alias ?? ''}
          onChange={(e) => onAlias(e.target.value)}
        />
        <input
          className="w-full bg-transparent text-xs text-ink-muted outline-none placeholder:text-ink-muted/70"
          placeholder={upstreamDesc ? `Description: ${upstreamDesc}` : 'Description (upstream text)'}
          title={t.description ? `Override. Upstream: ${upstreamDesc}` : 'Type to override the description agents see'}
          value={t.description ?? ''}
          onChange={(e) => onDescription(e.target.value)}
        />
        {drift && (
          <div className="text-xs text-amber-700" title={upstreamDesc}>
            Upstream description changed since this override was written — save to accept.
          </div>
        )}
        {dup && <div className="text-xs text-red-600">Duplicate exposed name</div>}
      </div>
      <select
        aria-label="Policy"
        className={clsx('rounded-lg border px-1.5 py-1 text-xs', mode === 'deny' ? 'border-red-300 text-red-700' : 'border-line', inherited && 'text-ink-muted')}
        value={mode}
        onChange={(e) => onMode(e.target.value as PolicyMode)}
      >
        {MODES.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
      </select>
      {mode === 'read_only' && !readOnlyHint && <span className="text-xs text-amber-700" title="Upstream does not declare readOnlyHint, so this tool is blocked">blocked</span>}
      <button aria-label="Remove" onClick={onRemove}><X className="size-4 text-ink-muted" /></button>
    </div>
  )
}
