import { useInRouterContext, Link } from 'react-router-dom'
import { serverPath } from '../connections/model'
import type { SelectionProps } from '../connections/ManagementRoute'
import { useInitialSelection } from '../connections/useInitialSelection'
import { useCapabilities } from '../interaction'
import { useSources, source, readSources } from '../requests'
import { StaleRecovery } from '../ui'
import { CopyButton, ErrorBox } from '../ui'
import { useDraft, useOperation } from '../interaction'
import CatalogPicker from './CatalogPicker'
import { connectionEndpoint } from '../endpoint'
import type {CatalogSelection,ClientFeatures} from '../api'
import { useEffect, useMemo, useState } from 'react'
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
import { GripVertical, Plus, Trash2, X } from 'lucide-react'
import clsx from 'clsx'
import { api, ApiError, type PiiConfig, type PolicyMode, type ToolPolicy, type Tool, type ToolSelection, type Upstream, type VirtualServer } from '../api'

import { btn, input, primary } from '../styles'

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
 clientFeatures:ClientFeatures
 catalog:CatalogSelection[]
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

const blank: Draft = { isNew: true, slug: '', name: '', description: '', enabled: true, authRequired: true, pii: noPii, policy: { default: 'allow', tools: {} }, rateLimit: '', tools: [],catalog:[],clientFeatures:{sampling:false,roots:false,elicitation:false} }
const fromServer = (v: VirtualServer): Draft => ({
  isNew: false,
  version:v.version,
 slug: v.slug,
  name: v.name,
  description: v.description,
  enabled: v.enabled,
  authRequired: v.authRequired,
  pii: { ...noPii, ...v.pii },
  // Draft overrides use stable upstream/tool identities, never editable aliases.
  policy: { default: v.policy?.default ?? 'allow', tools: Object.fromEntries(v.tools.flatMap(t => {
    const mode = v.policy?.tools?.[t.alias || `${t.upstream}__${t.tool}`]
    return mode ? [[key(t), mode]] : []
  })) },
  rateLimit: v.rateLimitPerMinute?.toString() ?? '',
 clientFeatures:v.clientFeatures??{sampling:false,roots:false,elicitation:false},
 catalog:(v.catalog??[]).map(({upstream,kind,name,alias,description})=>({upstream,kind,name,alias,description})),
  tools: v.tools.map(({ upstream, tool, alias, description }) => ({ upstream, tool, ...(alias && { alias }), ...(description && { description }) })),
})

const MODES: [PolicyMode, string][] = [
  ['allow', 'Allow'],
  ['read_only', 'Read-only'],
  ['deny', 'Deny'],
]

/** Convert stable draft identities to final exposed names only at save time. */
function cleanPolicy(d: Draft): ToolPolicy {
  return { default: d.policy.default, tools: Object.fromEntries(d.tools.flatMap(t => {
    const mode = d.policy.tools[key(t)]
    return mode ? [[t.alias || `${t.upstream}__${t.tool}`, mode]] : []
  })) }
}

export default function VirtualServers({ initialSelection }: SelectionProps) {
  const inRouter = useInRouterContext()
  const { edit: canEdit } = useCapabilities()
  const [servers, setServers] = useState<VirtualServer[]>([])
  const [upstreams, setUpstreams] = useState<Upstream[]>([])
  const [catalog, setCatalog] = useState<Record<string, Tool[]>>({})
  const [draft, setDraft, resetDraft, openDraft, dirty] = useDraft<Draft | null>(null, d => d ? d.isNew ? 'new' : d.slug : null)
  const [problems, setProblems] = useState<string[]>([])
  const [error, setError] = useState('')
  const [filter, setFilter] = useState('')
  const { busy, run } = useOperation()

  const { load, loading, readError } = useSources(() => [source('Virtual servers', api.listVirtualServers, setServers),
    {name:'Upstreams',load:async signal=>{
      const us=await api.listUpstreams(signal)
      if(signal.aborted)return
      setUpstreams(us)
      const failures=await readSources(us.map(u=>source(u.slug,s=>api.listTools(u.slug,s),tools=>setCatalog(previous=>({...previous,[u.slug]:tools})))),signal)
      if(failures)throw new Error(failures)
    }}])
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
    draft && setDraft({ ...draft, tools: draft.tools.filter((t) => key(t) !== k), policy: {
      ...draft.policy, tools: Object.fromEntries(Object.entries(draft.policy.tools).filter(([id]) => id !== k)),
    } })

  const [saved,setSaved] = useState('')
  const save = () => run(async () => {
    if (!draft) return
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
 catalog:draft.catalog,clientFeatures:draft.clientFeatures,
      },draft.version)
      setSaved(`Saved ${draft.name}. Refreshing the saved configuration.`)
      resetDraft(null)
      await load()
    } catch (e) {
      if (e instanceof ApiError) {
        setProblems(e.errors.length ? e.errors.map((x) => `${x.field ?? ''} ${x.message}`.trim()) : [e.message])
      } else setProblems([String(e)])
    }
  })

  const remove = (v: VirtualServer) => run(async () => {
    if (!confirm(`Delete virtual server "${v.slug}"?`)) return
    try {
      await api.deleteVirtualServer(v.slug,v.version)
      if (draft?.slug === v.slug) resetDraft(null)
      await load()
    } catch (e) { setError(e instanceof Error ? e.message : String(e)) }
  })

  const endpointPath = draft && !draft.isNew ? servers.find(v => v.slug === draft.slug)?.endpoint ?? '' : ''
  const endpoint = connectionEndpoint(endpointPath)

  useInitialSelection(initialSelection, servers, item => openDraft(fromServer(item)))

  return (
    <div className="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-10 lg:py-10">
      <div className="flex flex-wrap gap-3 items-start justify-between">
        <div>
          {draft && <StaleRecovery message={problems.join('; ')} reload={async () => {
        const latest = (await api.listVirtualServers()).find(item => item.slug === draft.slug)
        if (!latest) throw new Error('This item was deleted. Cancel the draft and refresh the list.')
        resetDraft(fromServer(latest)); setProblems([])
      }} />}
      {readError && <ErrorBox>{readError}</ErrorBox>}
      {loading && <p role="status">Loading…</p>}
      <button className={btn} disabled={loading} onClick={()=>{setError('');void load()}}>Refresh list</button>
      <h1 className="text-2xl font-semibold tracking-tight">Virtual Servers</h1>
          <p className="mt-1 text-sm text-ink-muted">Compose tools into agent-specific endpoints.</p>
        </div>
        <button disabled={!canEdit || busy} className={clsx(primary, 'inline-flex items-center gap-1.5')} onClick={() => { openDraft(blank); setProblems([]) }}>
          <Plus className="size-4" /> New virtual server
        </button>
      </div>
      {error && <div className="mt-6 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>}

      {saved && <p role="status" className="mt-3 text-green-700">{saved}</p>}
      <div className="mt-6 grid grid-cols-1 lg:grid-cols-[260px_minmax(0,1fr)] gap-6">
        <div className="rounded-card border border-line bg-surface shadow-card">
          {servers.map((v) => (
            <div
              key={v.slug}
              onClick={() => { openDraft(fromServer(v)); setProblems([]) }}
              className={clsx(
                'flex cursor-pointer items-center justify-between border-b border-line px-4 py-3 last:border-0 hover:bg-canvas',
                draft?.slug === v.slug && !draft.isNew && 'bg-accent-50',
              )}
            >
              <div className="min-w-0">
                <button type="button" className="truncate text-sm font-medium hover:underline" onClick={e => { e.stopPropagation(); openDraft(fromServer(v)); setProblems([]) }}>{v.name}</button>
                <div className="text-xs text-ink-muted">{v.tools.length} tools{v.enabled ? '' : ' · disabled'}</div>
              </div>
              {inRouter && <Link className="text-xs text-accent-700 underline" to={serverPath(v.slug)} onClick={e=>e.stopPropagation()}>Details</Link>}
              <button disabled={!canEdit || busy} className="p-1" aria-label="Delete" onClick={(e) => { e.stopPropagation(); void remove(v) }}>
                <Trash2 className="size-4 text-red-600" />
              </button>
            </div>
          ))}
          {!loading && !readError && servers.length === 0 && <div className="p-6 text-center text-sm text-ink-muted">No virtual servers yet. Register and refresh an upstream, then create a server to select its capabilities.</div>}
        </div>

        {draft ? (
          <div className="rounded-card border border-line bg-surface p-6 shadow-card">
            <p role="status" className="mb-4 text-sm text-ink-muted">{draft.isNew ? 'New unsaved draft' : `Editing draft of saved version ${draft.version}`}. {dirty ? 'Unsaved changes' : 'No unsaved changes'}. Changes apply only after Save.</p>
            {inRouter && !draft.isNew && <Link className="mb-4 inline-block text-sm text-accent-700 underline" to={serverPath(draft.slug)}>View saved configuration</Link>}
            <fieldset disabled={!canEdit || busy} className="min-w-0 border-0 p-0 m-0">
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
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
              <label className="sm:col-span-3 flex items-center gap-2 text-sm">
                <input type="checkbox" checked={draft.authRequired} onChange={(e) => setDraft({ ...draft, authRequired: e.target.checked })} />
                Require an agent key
                {!draft.authRequired && <span className="text-xs text-amber-700">Anyone who can reach the data plane can call these tools.</span>}
              </label>
            </div>
            <label className="mt-4 block text-sm font-medium">
              Description
              <input className={clsx(input, 'mt-1')} value={draft.description} onChange={(e) => setDraft({ ...draft, description: e.target.value })} />
            </label>
            {endpointPath && (
              <div className="mt-4 flex items-center gap-2 rounded-lg bg-canvas px-3 py-2 text-xs">
                <code className="flex-1 truncate">{endpoint ?? endpointPath}</code>
                <CopyButton label="Copy endpoint" value={endpoint} />
              </div>
            )}

            {endpointPath && !endpoint && <p role="status" className="mt-2 text-sm text-amber-700">Public gateway URL is not configured. Ask your administrator to set VITE_PUBLIC_GATEWAY_URL; the path above is not a complete connection URL.</p>}
            <section className="mt-6 rounded border border-line p-4"><h3 className="font-semibold">Client feature permissions</h3><p className="text-xs text-ink-muted">Also requires client negotiation. The MCP client controls approval; sampling uses the client’s model access. Roots never expose gateway files.</p>{(['sampling','roots','elicitation'] as const).map(feature=><label key={feature} className="mr-4 inline-flex items-center gap-2 text-sm"><input type="checkbox" checked={draft.clientFeatures[feature]} onChange={e=>setDraft({...draft,clientFeatures:{...draft.clientFeatures,[feature]:e.target.checked}})}/>{feature}</label>)}</section>
 <CatalogPicker upstreams={upstreams} value={draft.catalog} onChange={catalog=>setDraft({...draft,catalog})}/>
 <div className="mt-6 grid grid-cols-1 sm:grid-cols-2 gap-6">
              <section>
                <div className="mb-2 flex flex-wrap gap-2 items-center justify-between">
                  <h3 className="text-sm font-semibold">Available tools</h3>
                  <input className="w-40 rounded-lg border border-line px-2 py-1 text-xs" aria-label="Filter tools" placeholder="Filter" value={filter} onChange={(e) => setFilter(e.target.value)} />
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
                        <div key={key(t)}>{servers.find((s) => s.slug === draft.slug)?.tools.find((x) => key(x) === key(t))?.orphaned && <p className="text-sm text-amber-700">Missing upstream tool: {t.upstream}/{t.tool}. Remove it or restore discovery before saving.</p>}<Row key={key(t)} id={key(t)} t={t} mode={draft.policy.tools[key(t)] ?? draft.policy.default} inherited={!(key(t) in draft.policy.tools)} readOnlyHint={catalog[t.upstream]?.find((x) => x.name === t.tool)?.annotations?.readOnlyHint === true} onMode={(m) => setMode(key(t), m)} dup={dupes.has(exposed(t))} upstreamDesc={catalog[t.upstream]?.find((x) => x.name === t.tool)?.description ?? ''} drift={servers.find((s) => s.slug === draft.slug)?.tools.find((x) => key(x) === key(t))?.descriptionDrift === true} onDescription={(d) => patchTool(key(t), { description: d })} onAlias={(a) => patchTool(key(t), { alias: a })} onRemove={() => removeTool(key(t))} /></div>
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
                  <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
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
            </fieldset><div className="mt-6 flex justify-end gap-2">
              <button disabled={busy} className={btn} onClick={() => setDraft(null)}>Cancel</button>
              <button className={primary} disabled={!canEdit || busy || !draft.name || !draft.slug || dupes.size > 0} onClick={save}>Save</button>
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
      <button className="cursor-grab touch-none text-ink-muted" aria-label={`Reorder ${t.upstream}/${t.tool}`} {...attributes} {...listeners}>
        <GripVertical className="size-4" />
      </button>
      <div className="min-w-0 flex-1">
        <div className="truncate text-xs text-ink-muted">{t.upstream} / {t.tool}</div>
        <input
          className="w-full bg-transparent text-sm font-medium outline-none placeholder:text-ink"
          aria-label={`Alias for ${t.upstream}/${t.tool}`}
          placeholder={`${t.upstream}__${t.tool}`}
          value={t.alias ?? ''}
          onChange={(e) => onAlias(e.target.value)}
        />
        <input
          className="w-full bg-transparent text-xs text-ink-muted outline-none placeholder:text-ink-muted/70"
          aria-label={`Description for ${t.upstream}/${t.tool}`}
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
