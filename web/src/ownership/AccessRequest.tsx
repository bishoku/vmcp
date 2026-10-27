import { useEffect, useState } from 'react'
import { api, type ToolCatalogEntry, type ToolGrant, type VirtualServer } from '../api'
import Button from '../Button'
import Select from '../Select'
import { FormErrors, FormField, FormScope } from '../FormField'
import { useFormValidation, requiredText } from '../form-validation'
import { LoadingState } from '../Feedback'
import { useOperation, useUnsaved } from '../interaction'
import { source, useSources } from '../requests'
import { SearchField } from '../SearchControls'
import { btn, input, primary } from '../styles'
import { ErrorBox, Modal } from '../ui'

export default function AccessRequest({ servers, grants, onClose, onSaved }: { servers: VirtualServer[]; grants: ToolGrant[]; onClose: () => void; onSaved: (title: string) => Promise<void> }) {
 const [tools, setTools] = useState<ToolCatalogEntry[]>([])
 const [draft, setDraft] = useState({ server: '', tool: '', reason: '' }), [query, setQuery] = useState('')
 const validation = useFormValidation(), { busy, run } = useOperation(), unsaved = useUnsaved(draft)
 const { load, loading, readError } = useSources(() => [source('Tool catalog', api.toolCatalog, setTools)])
 useEffect(() => { const timer = setTimeout(() => { void load() }, 0); return () => clearTimeout(timer) }, [load])
 const shown = tools.filter(tool => `${tool.upstream} ${tool.name} ${tool.description}`.toLowerCase().includes(query.trim().toLowerCase()))
 const selected = tools.find(tool => JSON.stringify([tool.upstream, tool.name]) === draft.tool)
 const options = shown.slice(0, 100)
 if (selected && !options.includes(selected)) options.unshift(selected)
 const existing = grants.find(grant => grant.virtualServer === draft.server && grant.upstream === selected?.upstream && grant.tool === selected?.name)
 const save = () => run(async () => {
  if (!validation.check({ virtualServer: requiredText(draft.server, 'Virtual server'), tool: selected ? undefined : 'Select an upstream tool.', reason: draft.reason.length > 2000 ? 'Use 2,000 characters or fewer.' : undefined })) return
  if (!selected || loading || readError) return
  try {
   await api.saveToolGrant({ virtualServer: draft.server, upstream: selected.upstream, tool: selected.name, status: 'pending', reason: draft.reason, version: existing?.version ?? 0 })
   unsaved.clean(draft); await onSaved('Access request submitted')
  } catch (error) { validation.fail(error) }
 })
 const close = () => unsaved.guard(onClose)
 return <Modal title="Request tool access" variant="drawer" size="wide" busy={busy} onClose={close} description="Request permission to expose one upstream tool through a virtual server."
  footer={<div className="flex flex-wrap justify-end gap-2"><button className={btn} onClick={close}>Cancel</button><Button className={primary} type="submit" form="access-request" loading={busy} disabled={loading || !!readError || !servers.length || existing?.status === 'approved' || existing?.status === 'pending'}>Submit request</Button></div>}>
  <FormScope validation={validation}><form id="access-request" noValidate onSubmit={event => { event.preventDefault(); void save() }} className="space-y-6">
   <FormErrors validation={validation}/>
   <FormField name="virtualServer" label="Virtual server" required hint="The approval applies only to this server, not every server in its project."><Select value={draft.server} onChange={event => setDraft({ ...draft, server: event.target.value })}><option value="">Select a virtual server</option>{servers.map(server => <option key={server.slug} value={server.slug}>{server.name} · {server.slug}</option>)}</Select></FormField>
   {!servers.length && <p className="supporting-copy text-ink-muted">Create a virtual server in your project before requesting tool access.</p>}
   <div className="space-y-3"><SearchField visibleLabel label="Search available tools" value={query} onChange={setQuery} placeholder="Tool, upstream or description"/>
    {loading && <LoadingState compact label="Loading tool catalog…"/>}{readError && <ErrorBox onRetry={load} busy={loading}>{readError}</ErrorBox>}
    <FormField name="tool" label="Tool" required hint={`${shown.length} matching tools. ${shown.length > 100 ? 'Showing the first 100; refine your search to find more.' : 'Select the upstream tool to request.'}`}><Select disabled={loading || !!readError} value={draft.tool} onChange={event => setDraft({ ...draft, tool: event.target.value })}><option value="">Select a tool</option>{options.map(tool => <option key={JSON.stringify([tool.upstream, tool.name])} value={JSON.stringify([tool.upstream, tool.name])}>{tool.upstream} / {tool.name}</option>)}</Select></FormField>
   </div>
   {selected && <div className="surface-inset rounded-xl p-4 space-y-2"><p className="text-sm font-medium break-words">{selected.name}</p><p className="supporting-copy break-words text-ink-muted">{selected.description || 'No tool description provided.'}</p><p className="supporting-copy text-ink-muted">Owner: {selected.ownerTeam || 'Default — unmanaged'}</p></div>}
   {existing && <p role="status" className="supporting-copy text-ink-muted">Existing request: {existing.status}. {existing.status === 'approved' ? 'This server already has an approved grant.' : existing.status === 'pending' ? 'Wait for the upstream owner to review it.' : 'You can submit a new request.'}</p>}
   <FormField name="reason" label="Reason" hint="Help the upstream owner understand why this server needs access."><textarea className={input} rows={3} maxLength={2000} value={draft.reason} onChange={event => setDraft({ ...draft, reason: event.target.value })}/></FormField>
   <p className="supporting-copy text-ink-muted">Unmanaged upstreams need no approval. Same-team requests are approved automatically.</p>
  </form></FormScope>
 </Modal>
}
