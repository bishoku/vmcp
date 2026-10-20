import TextField from './GuidanceTextField';
import GuidanceItem from './GuidanceItemEditor';
import { useEffect, useState } from 'react';
import { BookOpen, FileText, MessageSquare, Plus, Trash2 } from 'lucide-react';
import { api, type Guidance, type GuidancePrompt, type GuidanceResource, type ToolSelection, type Upstream } from './api';
import { ServerPanel } from './connections/ServerPanels';
import { FormField, FormGroup } from './FormField';
import Select from './Select';
import { ErrorBox } from './ui';
import { EmptyState, LoadingState } from './Feedback';
import { TagBadge } from './Badge';
import { btn, input } from './styles';
import { useConfirm } from './confirmation';
import { normalizeGuidance, renderGuidance } from './guidance';
export default function GuidanceEditor({ value, onChange, tools, upstreams }: {
    value?: Guidance;
    onChange?: (g: Guidance) => void;
    tools: ToolSelection[];
    upstreams: Upstream[];
}) {
    const g = normalizeGuidance(value);
    const [item, setItem] = useState<{
        kind: 'resource';
        index: number;
        value: GuidanceResource;
    } | {
        kind: 'prompt';
        index: number;
        value: GuidancePrompt;
    } | null>(null);
    const [upstream, setUpstream] = useState('');
    const confirm = useConfirm();
    const change = (p: Partial<Guidance>) => onChange?.({ ...g, ...p });
    const nextReference = () => { let n = 1; while (g.toolReferences.some(r => r.key === `tool${n}`))
        n++; return `tool${n}`; };
    const remove = async (kind: 'resources' | 'prompts', index: number) => { if (await confirm({ title: 'Remove guidance?', description: 'This removes the item from your draft. Save the virtual server to publish the change.', confirmLabel: 'Remove', tone: 'danger' }))
        change({ [kind]: g[kind].filter((_, i) => i !== index) }); };
    return <FormGroup name="guidance" label="Agent guidance">
    <ServerPanel title="Agent guidance" description="Publish instructions, resources and reusable prompts alongside upstream capabilities. Client support determines how agents discover and use this guidance." icon={BookOpen}>
    <div className="panel-body space-y-6">
    <TextField label="Server instructions" name="guidance.instructions" value={g.instructions} limit={4000} onChange={onChange ? v => change({ instructions: v }) : undefined} preview={v => renderGuidance(v, g, tools)}/>
    <section className="space-y-3">
    <div>
    <h3 className="section-title">Tool references</h3>
    <p className="mt-1 text-xs text-ink-muted">Use {'{{tool:key}}'} in instructions and content. References follow the selected tool’s published name when its alias changes. Content depending on unavailable or policy-hidden tools is not published.</p>
    </div>{g.toolReferences.map((r, i) => <div key={i} className="grid items-end gap-3 sm:grid-cols-[1fr_2fr_auto]">
        <FormField name={`guidance.toolReferences.${i}.key`} label="Reference key">
        <input className={input} value={r.key} readOnly={!onChange} onChange={e => change({ toolReferences: g.toolReferences.map((a, j) => j === i ? { ...a, key: e.target.value } : a) })}/>
        </FormField>
        <FormField name={`guidance.toolReferences.${i}.tool`} label="Selected tool">
        <Select className={input} disabled={!onChange} value={`${r.upstream}\0${r.tool}`} onChange={e => { const [upstream, tool] = e.target.value.split('\0'); change({ toolReferences: g.toolReferences.map((a, j) => j === i ? { ...a, upstream, tool } : a) }); }}>{!tools.some(t => t.upstream === r.upstream && t.tool === r.tool) && <option value={`${r.upstream}\0${r.tool}`}>Unavailable: {r.upstream} / {r.tool}</option>}{tools.map(t => <option key={`${t.upstream}\0${t.tool}`} value={`${t.upstream}\0${t.tool}`}>{t.alias || `${t.upstream}__${t.tool}`}</option>)}</Select>
        </FormField>{onChange && <button type="button" className={btn} aria-label={`Remove reference ${r.key}`} onClick={() => change({ toolReferences: g.toolReferences.filter((_, j) => i !== j) })}>
            <Trash2 className="icon-control" aria-hidden="true"/>
            </button>}</div>)}{onChange && <button type="button" className={btn} disabled={!tools.length || g.toolReferences.length >= 100} onClick={() => change({ toolReferences: [...g.toolReferences, { key: nextReference(), upstream: tools[0].upstream, tool: tools[0].tool }] })}>
        <Plus className="icon-inline" aria-hidden="true"/>Add tool reference</button>}</section>
    <section className="space-y-3">
    <div className="flex flex-wrap items-center justify-between gap-3">
    <h3 className="section-title">Local resources</h3>{onChange && <button type="button" className={btn} disabled={g.resources.length >= 100} onClick={() => setItem({ kind: 'resource', index: -1, value: { name: '', title: '', description: '', enabled: true, mimeType: 'text/markdown', text: '' } })}>
        <Plus className="icon-inline" aria-hidden="true"/>Add resource</button>}</div>{!g.resources.length ? <EmptyState title="No local resources" description="Add workflow guides or reference material without requiring an upstream server."/> : g.resources.map((r, i) => <div key={i} className="surface-inset flex flex-wrap items-center gap-3 p-4">
        <FileText aria-hidden="true" className="icon-control text-accent-700"/>
        <div className="min-w-0 flex-1">
        <p className="text-sm font-medium break-words">{r.title || r.name}</p>
        <p className="line-clamp-2 text-xs text-ink-muted">{r.description || r.text}</p>
        <p className="mt-1 break-all font-mono text-xs text-ink-muted">vmcp://guidance/resource/{r.name}</p>
        </div>
        <TagBadge label={r.enabled ? 'Local · Enabled' : 'Local · Disabled'}/>
        <button type="button" className={btn} onClick={() => setItem({ kind: 'resource', index: i, value: structuredClone(r) })}>{onChange ? 'Edit' : 'View'} resource</button>{onChange && <button type="button" className={btn} aria-label={`Remove resource ${r.name}`} onClick={() => void remove('resources', i)}>
            <Trash2 aria-hidden="true" className="icon-control"/>
            </button>}</div>)}</section>
    <section className="space-y-3">
    <div className="flex flex-wrap items-center justify-between gap-3">
    <h3 className="section-title">Local prompts</h3>{onChange && <button type="button" className={btn} disabled={g.prompts.length >= 100} onClick={() => setItem({ kind: 'prompt', index: -1, value: { name: '', title: '', description: '', enabled: true, arguments: [], messages: [{ role: 'user', text: '' }] } })}>
        <Plus className="icon-inline" aria-hidden="true"/>Add prompt</button>}</div>{!g.prompts.length ? <EmptyState title="No local prompts" description="Describe a reusable workflow with optional arguments. Prompts provide guidance; they do not execute tools."/> : g.prompts.map((p, i) => <div key={i} className="surface-inset flex flex-wrap items-center gap-3 p-4">
        <MessageSquare aria-hidden="true" className="icon-control text-accent-700"/>
        <div className="min-w-0 flex-1">
        <p className="text-sm font-medium break-words">{p.title || p.name}</p>
        <p className="line-clamp-2 text-xs text-ink-muted">{p.description}</p>
        <p className="mt-1 break-all font-mono text-xs text-ink-muted">vmcp_local__{p.name} · {p.arguments.length} arguments</p>
        </div>
        <TagBadge label={p.enabled ? 'Local · Enabled' : 'Local · Disabled'}/>
        <button type="button" className={btn} onClick={() => setItem({ kind: 'prompt', index: i, value: structuredClone(p) })}>{onChange ? 'Edit' : 'View'} prompt</button>{onChange && <button type="button" className={btn} aria-label={`Remove prompt ${p.name}`} onClick={() => void remove('prompts', i)}>
            <Trash2 aria-hidden="true" className="icon-control"/>
            </button>}</div>)}</section>
    <section className="space-y-3">
    <h3 className="section-title">Upstream instructions</h3>
    <p className="text-xs leading-relaxed text-ink-muted">Selected upstream instructions are published as separate resources. Existing upstream resource, template and prompt selections remain in Capabilities. Instructions are not automatically merged into server instructions.</p>{g.upstreamInstructions.map(slug => <UpstreamGuide key={slug} slug={slug} onRemove={onChange ? () => change({ upstreamInstructions: g.upstreamInstructions.filter(s => s !== slug) }) : undefined}/>)}{onChange && <div className="flex flex-wrap items-end gap-3">
        <FormField name="guidance.upstreamInstructions" label="Upstream source">
        <Select className={input} value={upstream} onChange={e => setUpstream(e.target.value)}>
        <option value="">Choose an upstream</option>{upstreams.filter(u => !g.upstreamInstructions.includes(u.slug)).map(u => <option key={u.slug} value={u.slug}>{u.name}</option>)}</Select>
        </FormField>
        <button type="button" className={btn} disabled={!upstream || g.upstreamInstructions.length >= 100} onClick={() => { change({ upstreamInstructions: [...g.upstreamInstructions, upstream] }); setUpstream(''); }}>Include instructions</button>
        </div>}</section>
    {item && <GuidanceItem key={`${item.kind}:${item.index}`} item={item} guidance={g} tools={tools} readOnly={!onChange} onClose={() => setItem(null)} onApply={v => { if (item.kind === 'resource')
        change({ resources: item.index < 0 ? [...g.resources, v as GuidanceResource] : g.resources.map((r, i) => i === item.index ? v as GuidanceResource : r) });
    else
        change({ prompts: item.index < 0 ? [...g.prompts, v as GuidancePrompt] : g.prompts.map((p, i) => i === item.index ? v as GuidancePrompt : p) }); setItem(null); }}/>}
  </div>
    </ServerPanel>
    </FormGroup>;
}
function UpstreamGuide({ slug, onRemove }: {
    slug: string;
    onRemove?: () => void;
}) {
    const [text, setText] = useState<string | null>(null), [error, setError] = useState(''), [retry, setRetry] = useState(0);
    useEffect(() => { const controller = new AbortController(); api.listCatalog(slug, controller.signal).then(c => { if (!controller.signal.aborted) {
        setText(c.instructions ?? '');
        setError('');
    } }).catch(e => { if (!controller.signal.aborted)
        setError(String(e)); }); return () => controller.abort(); }, [slug, retry]);
    return <div className="surface-inset space-y-3 p-4">
    <div className="flex items-center justify-between gap-3">
    <span className="min-w-0 break-all text-sm font-medium">{slug}</span>
    <TagBadge label="Upstream"/>{onRemove && <button type="button" className={btn} onClick={onRemove}>Remove</button>}</div>
    <p className="break-all font-mono text-xs text-ink-muted">vmcp://guidance/upstream/{slug}</p>{error ? <ErrorBox onRetry={() => setRetry(r => r + 1)}>{error}</ErrorBox> : text === null ? <LoadingState compact label="Loading upstream instructions…"/> : text ? <TextField name={`guidance.upstreamInstructions.${slug}`} label="Upstream instructions" value={text}/> : <p className="text-xs text-ink-muted">No instructions discovered. This resource will appear when the enabled upstream exposes instructions.</p>}</div>;
}
