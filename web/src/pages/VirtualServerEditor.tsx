import { ProjectSelect } from '../OwnershipSelect'
import ConfigurationGroup from '../connections/ConfigurationGroup'
import GuidanceEditor from '../GuidanceEditor'
import { normalizeGuidance } from '../guidance'
import type { Guidance } from '../api'
import DescriptionOverride from '../DescriptionOverride'
import { filterItem } from '../search-filters'
import { SearchField, ActiveFilters, ResultCount } from '../SearchControls'
import { Checkbox, Switch } from '../SelectionControls'
import Select from '../Select'
import { LoadMore } from '../DataTable'
import { FormField, FormScope, FormErrors, FormGroup } from '../FormField'
import { useFormValidation, requiredText, slugError, rateError } from '../form-validation'
import { useNotifications } from '../notifications'
import { EmptyState, LoadingState } from '../Feedback'
import { Link } from 'react-router-dom'
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
import { Fingerprint, GripVertical, Layers, ListChecks, Plus, ShieldCheck, SlidersHorizontal, Wrench, X } from 'lucide-react'
import clsx from 'clsx'
import { api, type PiiConfig, type PolicyMode, type ToolPolicy, type Tool, type ToolSelection, type Upstream, type VirtualServer } from '../api'

import { btn, input } from '../styles'

import { ServerPanel, SummaryTile } from '../connections/ServerPanels'
import EditorSaveBar from '../connections/EditorSaveBar'

const key = (s: ToolSelection) => `${s.upstream}\u0000${s.tool}`
const slugify = (s: string) =>
  s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 63)

interface Draft {
 project:string
 guidance:Guidance
 version?: number
  isNew: boolean
  slug: string
  name: string
  description: string
  enabled: boolean
  omniEnabled: boolean
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

const blank: Draft = { project:"default", guidance:normalizeGuidance(), isNew: true, slug: '', name: '', description: '', enabled: true, omniEnabled: false, authRequired: true, pii: noPii, policy: { default: 'allow', tools: {} }, rateLimit: '', tools: [],catalog:[],clientFeatures:{sampling:false,roots:false,elicitation:false} }
const fromServer = (v: VirtualServer): Draft => ({
 project:v.project??"default", guidance:normalizeGuidance(v.guidance),
  isNew: false,
  version:v.version,
 slug: v.slug,
  name: v.name,
  description: v.description,
  enabled: v.enabled,
  omniEnabled: v.omniEnabled ?? false,
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

export default function VirtualServerEditor({ initialSelection, onSaved, onCancel }: SelectionProps & {onSaved?: (slug: string) => void; onCancel?: () => void}) {
  const {notify,available}=useNotifications()
  const { edit: canEdit } = useCapabilities()
  const [servers, setServers] = useState<VirtualServer[]>([])
  const [upstreams, setUpstreams] = useState<Upstream[]>([])
  const [sharedTools,setSharedTools]=useState<import("../api").ToolCatalogEntry[]>([])
 const [catalog, setCatalog] = useState<Record<string, Tool[]>>({})
  const [draft, setDraft, resetDraft, openDraft, dirty] = useDraft<Draft | null>(initialSelection ? null : blank, d => d ? d.isNew ? 'new' : d.slug : null)
  const validation=useFormValidation()
  const problems=validation.issues.map(issue=>issue.message)
  const [filter, setFilter] = useState('')
  const [toolUpstream,setToolUpstream] = useState('')
  const [availableLimit,setAvailableLimit]=useState(50)
  const [exposedLimit,setExposedLimit]=useState(50)
  const { busy, run } = useOperation()

  const { load, loading, readError } = useSources(() => [source('Virtual servers', async signal => { const items=await api.listVirtualServers(signal); const selected=items.find(v=>v.slug===initialSelection); if(selected?.hasGuidance){const full=await api.getVirtualServer(selected.slug,signal); return items.map(v=>v.slug===full.slug?full:v)} return items }, setServers),
    source('Shared tools',signal=>api.toolCatalog(signal),setSharedTools),
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
  const save = async () => {
    let savedSlug: string | undefined
    await run(async () => {
      if (!draft || !canEdit) return
      if(!validation.check({name:requiredText(draft.name,'Name',120),slug:slugError(draft.slug),description:draft.description.length>1000?'Use 1,000 characters or fewer.':undefined,rateLimitPerMinute:rateError(draft.rateLimit),tools:dupes.size?'Resolve duplicate exposed tool names before saving.':undefined}))return
      try {
        await api.putVirtualServer(draft.slug, {
          project:draft.project, name: draft.name,
          description: draft.description,
          enabled: draft.enabled,
          authRequired: draft.authRequired,
          omniEnabled: draft.omniEnabled,
          pii: draft.pii,
          policy: cleanPolicy(draft),
          rateLimitPerMinute: draft.rateLimit ? Number(draft.rateLimit) : null,
          tools: draft.tools,
   catalog:draft.catalog,clientFeatures:draft.clientFeatures,guidance:draft.guidance,
        },draft.version)
        notify({title:'Virtual server saved',description:draft.name})
        setSaved(`Saved ${draft.name}. Refreshing the saved configuration.`)
        resetDraft(null)
        savedSlug = draft.slug
        if (!onSaved) await load()
      } catch (e) {
        validation.fail(e)
      }
    })
    if (savedSlug && onSaved) onSaved(savedSlug)
  }

  const endpointPath = draft && !draft.isNew ? servers.find(v => v.slug === draft.slug)?.endpoint ?? '' : ''
  const endpoint = connectionEndpoint(endpointPath)

  useInitialSelection(initialSelection, servers, item => openDraft(fromServer(item)))

  return (
    <FormScope validation={validation}><div className="page-layout">
      <header className="mb-6 flex flex-wrap items-start justify-between gap-4">
        <div><h1 className="page-title">{initialSelection ? 'Edit virtual server' : 'New virtual server'}</h1><p className="page-description mt-1 text-sm text-ink-muted">Compose capabilities and configure access protection.</p></div>
        <Link className={btn} to={initialSelection?serverPath(initialSelection):'/virtual-servers'}>{initialSelection?'View saved configuration':'Back to virtual servers'}</Link>
      </header>
      {draft && <EditorSaveBar status={`${draft.isNew?'New unsaved draft':`Editing draft of saved version ${draft.version}`}. ${dirty?'Unsaved changes':'No unsaved changes'}. Changes apply only after Save.`} detail={dupes.size?'Resolve duplicate exposed tool names before saving.':!draft.name||!draft.slug?'Enter a name and identifier before saving.':'Review capability selections and access protection before saving.'} busy={busy} disabled={!canEdit||dupes.size>0} onSave={()=>void save()} onCancel={()=>{if(onCancel)onCancel();else setDraft(null)}}/>}
      <FormErrors validation={validation}/>
      {draft && <StaleRecovery message={problems.join('; ')} reload={async () => {
        const latest = await api.getVirtualServer(draft.slug)
        if (!latest) throw new Error('This item was deleted. Return to the list and refresh.')
        resetDraft(fromServer(latest)); validation.reset()
      }} />}
      {readError && <ErrorBox title="Saved configuration could not be loaded" onRetry={load} busy={loading}>{readError}</ErrorBox>}
      {loading && <LoadingState label="Loading capability catalogs…" compact/>}
      {saved && !available && <p role="status" className="mt-3 text-green-700">{saved}</p>}
      <div className="mt-6">
        {draft ? (
          <div className="layout-stack">
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4"><SummaryTile label="Selected tools" value={draft.tools.length} detail="Explicit tool selections in this draft" icon={Wrench}/><SummaryTile label="Resources & prompts" value={draft.catalog.length} detail="Resources, templates and prompt selections" icon={ListChecks}/><SummaryTile label="Upstream servers" value={new Set([...draft.tools,...draft.catalog].map(item=>item.upstream)).size} detail="Sources composed into this draft" icon={Layers}/><SummaryTile label="Authentication" value={draft.authRequired?'Required':'Anonymous'} detail="Draft client access requirement" icon={Fingerprint}/></div>
            <fieldset disabled={!canEdit || busy} className="min-w-0 layout-stack border-0 p-0 m-0">
            <ServerPanel title="Server identity" description="Name, stable identifier and client access settings" icon={Layers}><div className="space-y-4 panel-body">
            <ProjectSelect value={draft.project} onChange={project=>setDraft({...draft,project})} existing={!draft.isNew}/>
 <p className="text-xs text-ink-muted">Tools from another owner team require approval. After saving, request access in Teams &amp; projects.</p>
 <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
              <FormField name="name" label="Name" required hint="Display name shown to users; up to 120 characters."><input
                  className={clsx(input, 'mt-1')}
                  value={draft.name}
                  onChange={(e) =>
                    {if(draft.isNew)validation.clear('slug');setDraft({ ...draft, name: e.target.value, ...(draft.isNew && { slug: slugify(e.target.value) }) })}
                  }
                /></FormField>
              <FormField name="slug" label="Slug" required hint="Stable identifier used in the endpoint. It cannot be changed after creation."><input className={clsx(input, 'mt-1')} disabled={!draft.isNew} value={draft.slug} onChange={(e) => setDraft({ ...draft, slug: e.target.value })} /></FormField>
              <Switch className="self-start" label="Enabled" field="enabled" checked={draft.enabled} onChange={(e) => setDraft({ ...draft, enabled: e.target.checked })}/>
              <Switch className="sm:col-span-3" label="Publish through Omni MCP" field="omniEnabled" checked={draft.omniEnabled} onChange={(e) => setDraft({ ...draft, omniEnabled: e.target.checked })} description="Allow authorized agents to discover and invoke this server through Omni. Existing tool approvals and runtime access still apply. The server description is used for semantic search."/>
              <Switch className="sm:col-span-3" label="Require an agent key" field="authRequired" checked={draft.authRequired} onChange={(e) => setDraft({ ...draft, authRequired: e.target.checked })} description={!draft.authRequired ? <span className="text-amber-700">Anyone who can reach the data plane can call these tools.</span> : 'Clients must authenticate before using this server.'}/>
            </div>
            <FormField name="description" label="Description" hint="Optional. Explain the purpose in up to 1,000 characters."><input className={clsx(input, 'mt-1')} value={draft.description} onChange={(e) => setDraft({ ...draft, description: e.target.value })} /></FormField>
            {endpointPath && (
              <div className="mt-4 flex items-center gap-2 surface-inset px-3 py-2 text-xs">
                <code className="min-w-0 flex-1 break-all">{endpoint ?? endpointPath}</code>
                <CopyButton label="Copy endpoint" value={endpoint} />
              </div>
            )}

            {endpointPath && !endpoint && <p role="status" className="mt-2 text-sm text-amber-700">Public gateway URL is not configured. Ask your administrator to configure the public gateway URL; the path above is not a complete connection URL.</p>}
            </div></ServerPanel>

<div><h2 className="section-title">Capability composition</h2><p className="section-description mt-1 text-sm text-ink-muted">Select tools, resources, templates and prompts from stored upstream discovery.</p></div>
 <div className="grid grid-cols-1 xl:grid-cols-2 layout-grid">
              <ServerPanel title="Shared tools" description="Tool metadata from other teams; connections remain private" icon={Wrench}><div className="space-y-2 panel-body">{sharedTools.filter(t=>!upstreams.some(u=>u.slug===t.upstream)&&`${t.name} ${t.description}`.toLowerCase().includes(filter.toLowerCase())).slice(0,availableLimit).map(t=><div key={`${t.upstream}/${t.name}`} className="flex items-center justify-between gap-2"><span className="text-sm">{t.upstream}/{t.name} · {t.ownerTeam||'Default'}</span><button type="button" className={btn} disabled={selected.has(`${t.upstream}\u0000${t.name}`)} onClick={()=>setDraft({...draft,tools:[...draft.tools,{upstream:t.upstream,tool:t.name}]})}>Select</button></div>)}</div></ServerPanel>
 <ServerPanel title="Available tools" description="Add tools from stored upstream discovery; selection is explicit" icon={Wrench}>
                <div className="space-y-3 border-b border-line panel-body"><SearchField label="Filter tools" value={filter} onChange={value=>{setFilter(value);setAvailableLimit(50)}} placeholder="Search tools or descriptions" className="block text-xs font-medium" visibleLabel/><label className="block text-xs font-medium">Tool upstream<Select className={`${input} mt-2`} value={toolUpstream} onChange={e=>setToolUpstream(e.target.value)}><option value="">All upstreams</option>{upstreams.map(u=><option key={u.slug} value={u.slug}>{u.name}</option>)}</Select></label></div>
                <ActiveFilters className="border-b border-line panel-inset py-3" label="Active tool picker filters" items={[filterItem('Search',filter,()=>{setFilter('');setAvailableLimit(50)}),filterItem('Upstream',upstreams.find(u=>u.slug===toolUpstream)?.name??toolUpstream,()=>{setToolUpstream('');setAvailableLimit(50)})]} onReset={()=>{setFilter('');setToolUpstream('');setAvailableLimit(50)}}><ResultCount matched={upstreams.filter(u=>!toolUpstream||u.slug===toolUpstream).reduce((sum,u)=>sum+(catalog[u.slug]??[]).filter(t=>`${t.name} ${t.description??''}`.toLowerCase().includes(filter.trim().toLowerCase())).length,0)} total={upstreams.reduce((sum,u)=>sum+(catalog[u.slug]??[]).length,0)} noun="tools" busy={loading}/></ActiveFilters>
                <div className="space-y-4 panel-body">
                  {upstreams.filter(u=>!toolUpstream||u.slug===toolUpstream).map((u) => {
                    const tools = (catalog[u.slug] ?? []).filter((t) => `${t.name} ${t.description??''}`.toLowerCase().includes(filter.trim().toLowerCase()))
                    return (
                      <div key={u.slug}>
                        <div className="mb-1 text-xs font-semibold uppercase tracking-wide text-ink-muted">{u.name}</div>
                        {tools.slice(0,availableLimit).map((t) => {
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
                              {!used && <Plus aria-hidden="true" className="icon-control shrink-0 text-accent-700" />}
                            </button>
                          )
                        })}
                        {tools.length>availableLimit&&<LoadMore shown={Math.min(availableLimit,tools.length)} total={tools.length} noun="tools" label="Show 50 more available tools" onLoad={()=>setAvailableLimit(n=>n+50)}/>}
                        {!loading && tools.length === 0 && <div className="px-2 text-xs text-ink-muted">{filter?'No matching tools. Try another search.':'No tools. Refresh this upstream first.'}</div>}
                      </div>
                    )
                  })}
                  {!loading&&upstreams.length===0&&<EmptyState title="Register an upstream first." description="Add a server and refresh its discovery to select tools." compact/>}
                </div>
              </ServerPanel>

              <ServerPanel title={`Exposed tools (${draft.tools.length})`} description="Set client-facing aliases, descriptions and policy overrides. Drag or use the keyboard to reorder." icon={ListChecks}>
                <div className="space-y-3 panel-body">
                  <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
                    <SortableContext items={draft.tools.map(key)} strategy={verticalListSortingStrategy}>
                      {draft.tools.slice(0,validation.issues.some(issue=>issue.field?.startsWith('tools.'))?draft.tools.length:exposedLimit).map((t,index) => (
                        <div key={key(t)}>{["required","pending","rejected","revoked"].includes(servers.find(s=>s.slug===draft.slug)?.tools.find(x=>key(x)===key(t))?.approvalStatus??"")&&<p className="text-sm text-amber-700">Upstream owner approval: {servers.find(s=>s.slug===draft.slug)?.tools.find(x=>key(x)===key(t))?.approvalStatus}. This tool is not available to agents. <Link to="/ownership" className="underline">Manage access requests</Link></p>}{servers.find((s) => s.slug === draft.slug)?.tools.find((x) => key(x) === key(t))?.orphaned && <p className="text-sm text-amber-700">Missing upstream tool: {t.upstream}/{t.tool}. Remove it or restore discovery before saving.</p>}<Row index={index} key={key(t)} id={key(t)} t={t} mode={draft.policy.tools[key(t)] ?? draft.policy.default} inherited={!(key(t) in draft.policy.tools)} readOnlyHint={catalog[t.upstream]?.find((x) => x.name === t.tool)?.annotations?.readOnlyHint === true} onMode={(m) => setMode(key(t), m)} dup={dupes.has(exposed(t))} upstreamDesc={catalog[t.upstream]?.find((x) => x.name === t.tool)?.description ?? servers.find(s => s.slug === draft.slug)?.tools.find(x => key(x) === key(t))?.upstreamDescription ?? ''} drift={servers.find((s) => s.slug === draft.slug)?.tools.find((x) => key(x) === key(t))?.descriptionDrift === true} onDescription={(d) => patchTool(key(t), { description: d })} onAlias={(a) => patchTool(key(t), { alias: a })} onRemove={() => removeTool(key(t))} /></div>
                      ))}
                    </SortableContext>
                  </DndContext>
                  {draft.tools.length>exposedLimit&&<LoadMore shown={Math.min(exposedLimit,draft.tools.length)} total={draft.tools.length} noun="tools" label="Show 50 more exposed tools" onLoad={()=>setExposedLimit(n=>n+50)}/>}
                  {draft.tools.length === 0 && <EmptyState title="Pick tools from the left." description="Only selected tools are composed into this virtual server." compact/>}
                </div>
              </ServerPanel>
            </div>

 <CatalogPicker upstreams={upstreams} value={draft.catalog} savedCatalog={servers.find((s) => s.slug === draft.slug)?.catalog} onChange={catalog=>setDraft({...draft,catalog})}/>
 <ConfigurationGroup title="Agent guidance" description="Instructions and descriptions presented to agents"><GuidanceEditor value={draft.guidance} tools={draft.tools} upstreams={upstreams} onChange={guidance=>setDraft({...draft,guidance})}/></ConfigurationGroup>

            <ConfigurationGroup title="Access & protection" description="Client permissions, rate limits, tool policies and PII masking">            <ServerPanel title="Client feature permissions" description="Requires client negotiation and client approval. Roots never expose gateway files." icon={Fingerprint}><div className="grid gap-3 panel-body sm:grid-cols-3">{(['sampling','roots','elicitation'] as const).map(feature=><Checkbox key={feature} label={feature} field={`clientFeatures.${feature}`} checked={draft.clientFeatures[feature]} onChange={e=>setDraft({...draft,clientFeatures:{...draft.clientFeatures,[feature]:e.target.checked}})}/>)}</div></ServerPanel>
            <div className="grid items-start layout-grid xl:grid-cols-2"><ServerPanel title="Rate limit" description="Bound tool calls across client identities" icon={SlidersHorizontal}><div className="panel-body">
              <FormField name="rateLimitPerMinute" label="Rate limit" help="Server and credential limits can both apply. Empty means this configuration does not add a rate limit." hint="Optional. 1–1,000,000 calls per minute; empty means unlimited."><input type="number" min={1} placeholder="unlimited" className={`${input} max-w-40`} value={draft.rateLimit} onChange={(e) => setDraft({ ...draft, rateLimit: e.target.value })} /></FormField>
              <p className="mt-1 text-xs text-ink-muted">Shared by all agents. Each agent key can have its own limit too. Excess calls get HTTP 429.</p>
            </div></ServerPanel>

            <ServerPanel title="Tool policy" description="Default behavior for tools without an override" icon={ShieldCheck}><div className="panel-body">
              <label className="mt-2 flex flex-wrap items-center gap-3 text-sm">
                Default for tools without a rule
                <Select className={`${input} max-w-48`} value={draft.policy.default} onChange={(e) => setDraft({ ...draft, policy: { ...draft.policy, default: e.target.value as PolicyMode } })}>
                  {MODES.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
                </Select>
              </label>
              <p className="mt-1 text-xs text-ink-muted">Read-only lets through only tools whose upstream declares <code>readOnlyHint</code>. Set per-tool rules in the list above.</p>
            </div></ServerPanel></div>

            <ServerPanel title="PII masking" description="Mask sensitive data in tool results and optionally in upstream arguments" icon={ShieldCheck}><div className="panel-body">
              <Switch label="Mask sensitive data in tool results" field="pii.enabled" checked={draft.pii.enabled} onChange={(e) => setPii({ enabled: e.target.checked })}/>
              {draft.pii.enabled && (
                <div className="mt-3 space-y-3">
                  <p className="text-xs text-ink-muted">Leaving detectors unselected uses all built-in detectors.</p>
                  <FormGroup name="pii.detectors" label="Built-in detectors"><div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
                    {DETECTORS.map(([id, label]) => (
                      <Checkbox key={id} label={label} checked={draft.pii.detectors.includes(id)} onChange={(e) => setPii({ detectors: e.target.checked ? [...draft.pii.detectors, id] : draft.pii.detectors.filter((d) => d !== id) })}/>
                    ))}
                  </div></FormGroup>
                  <Switch label="Also mask tool arguments sent upstream" field="pii.maskArguments" checked={draft.pii.maskArguments} onChange={(e) => setPii({ maskArguments: e.target.checked })}/>
                  <div className="space-y-2">
                    {draft.pii.customRules.map((r, i) => (
                      <div key={i} className="flex flex-wrap gap-2">
                        <FormField name={`pii.customRules.${i}.name`} label={`Custom rule ${i+1} name`} className="w-full" required hint="Short identifier for this masking rule."><input aria-label={`Custom rule ${i+1} name`} className={input} placeholder="Name" value={r.name} onChange={(e) => setPii({ customRules: draft.pii.customRules.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)) })} /></FormField>
                        <FormField name={`pii.customRules.${i}.pattern`} label={`Custom rule ${i+1} pattern`} className="w-full" required hint="Use an RE2 regular expression. Pattern validity is checked on Save."><input aria-label={`Custom rule ${i+1} pattern`} className={clsx(input, 'font-mono')} placeholder="Regex (RE2)" value={r.pattern} onChange={(e) => setPii({ customRules: draft.pii.customRules.map((x, j) => (j === i ? { ...x, pattern: e.target.value } : x)) })} /></FormField>
                        <button className={`${btn} action-danger-outline action-icon`} aria-label="Remove rule" onClick={() => {validation.clear('pii.customRules');setPii({ customRules: draft.pii.customRules.filter((_, j) => j !== i) })}}><X aria-hidden="true" className="icon-control text-ink-muted" /></button>
                      </div>
                    ))}
                    <button className={btn} disabled={draft.pii.customRules.length >= 20} onClick={() => setPii({ customRules: [...draft.pii.customRules, { name: '', pattern: '' }] })}>
                      <Plus aria-hidden="true" className="icon-control" /> Custom rule
                    </button>
                  </div>
                </div>
              )}
            </div></ServerPanel>
            </ConfigurationGroup>

            </fieldset>
          </div>
        ) : (
          <div className="surface-card">{initialSelection&&loading?<LoadingState label="Loading saved configuration…"/>:<EmptyState title={initialSelection?'Virtual server configuration is unavailable. Return to the list or refresh this page.':'Saved. Select a virtual server from the list.'} description={initialSelection?'Your current draft is not replaced by a failed read.':'Open the saved configuration to review your changes.'} actions={initialSelection?<button className={btn} onClick={()=>void load()}>Retry</button>:undefined}/>}</div>
        )}
      </div>
    </div></FormScope>
  )
}

function Row({ index, id, t, dup, mode, inherited, readOnlyHint, upstreamDesc, drift, onDescription, onMode, onAlias, onRemove }: { index:number; id: string; t: ToolSelection; upstreamDesc: string; drift: boolean; onDescription: (d: string) => void; dup: boolean; mode: PolicyMode; inherited: boolean; readOnlyHint: boolean; onMode: (m: PolicyMode) => void; onAlias: (a: string) => void; onRemove: () => void }) {
  const { attributes, listeners, setNodeRef, transform, transition } = useSortable({ id })
  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={clsx('surface-inset p-4', dup ? 'border-red-300' : 'border-line')}
    >
      <div className="flex items-start gap-2"><button className={`${btn} action-icon cursor-grab touch-none`} aria-label={`Reorder ${t.upstream}/${t.tool}`} {...attributes} {...listeners}><GripVertical aria-hidden="true" className="icon-control" /></button><p className="min-w-0 flex-1 break-all pt-1 text-xs font-medium text-ink-muted">{t.upstream} / {t.tool}</p><button className={`${btn} action-danger-outline action-icon`} aria-label="Remove" onClick={onRemove}><X aria-hidden="true" className="icon-inline text-ink-muted"/></button></div>
      <div className="mt-3 space-y-3">
        <FormField name={`tools.${index}.alias`} label="Exposed alias" error={dup?'Choose a unique exposed name. Another tool uses this name.':undefined} hint="Optional. Leave empty to use the default upstream tool name."><input className={`${input} mt-2`} aria-label={`Alias for ${t.upstream}/${t.tool}`} placeholder={`${t.upstream}__${t.tool}`} value={t.alias??''} onChange={e=>onAlias(e.target.value)}/></FormField>
        <DescriptionOverride name={`tools.${index}.description`} identity={`${t.upstream}/${t.tool}`} exposedName={t.alias || `${t.upstream}__${t.tool}`} value={t.description ?? ''} upstream={upstreamDesc} drift={drift} onChange={onDescription}/>
        <div className="flex flex-wrap items-end gap-3"><label className="min-w-0 flex-1 text-xs font-medium">Tool policy<Select aria-label="Policy" className={clsx(input,'mt-2',mode==='deny'&&'border-red-300 text-red-700')} value={mode} onChange={e=>onMode(e.target.value as PolicyMode)}>{MODES.map(([v,l])=><option key={v} value={v}>{l}</option>)}</Select></label><span className="pb-2 text-xs text-ink-muted">{inherited?'Uses default':'Override'}</span></div>
        {mode==='read_only'&&!readOnlyHint&&<p className="text-xs text-amber-700">Blocked: upstream does not declare readOnlyHint.</p>}
        {dup&&<p className="sr-only">Duplicate exposed name</p>}
      </div>
    </div>
  )
}
