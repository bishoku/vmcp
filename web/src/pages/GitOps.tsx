import { useNotifications } from '../notifications'
import { EmptyState, LoadingState } from '../Feedback'
import { useConfirm } from '../confirmation'
import { useOperation, useUnsaved } from '../interaction'
import { Braces, FileCode2, GitCompareArrows, Upload } from 'lucide-react'
import { useState } from 'react'
import { api, type ApplyResult, type ConfigDocument } from '../api'
import { btn, input, primary } from '../styles'
import { DetailRow, ServerPanel } from '../connections/ServerPanels'
import { CopyButton, ErrorBox } from '../ui'

export default function GitOps({ canManage }: { canManage: boolean }) {
  const {notify}=useNotifications()
  const confirm=useConfirm()
 const [text,setText]=useState('')
 const [prune,setPrune]=useState(false)
 const [preview,setPreview]=useState<ApplyResult|null>(null)
 const [revision,setRevision]=useState<number|undefined>()
 const [error,setError]=useState('')
 const {busy,run:operate}=useOperation()
 const unsaved=useUnsaved(text)
 const exportDocument=()=>unsaved.guard(()=>{void operate(async()=>{try{const exported=JSON.stringify(await api.exportConfig(),null,2);setText(exported);unsaved.clean(exported);setPreview(null);setError('');notify({title:'Configuration exported',description:'Saved configuration is ready to review.'})}catch(e){setError(String(e instanceof Error?e.message:e))}})})
 const run=(dryRun:boolean)=>operate(async()=>{
  if(!canManage)return
  setError('')
  try{
   const document=JSON.parse(text) as ConfigDocument
   if (!dryRun && prune && !await confirm({title:'Apply preview and delete absent configuration?',description:'Items missing from this document will be removed according to the reviewed prune preview.',confirmLabel:'Apply and delete',tone:'danger'})) return
   const expected=dryRun?(await api.info()).configRevision:revision
   const result=await api.applyConfig(document,dryRun,prune,expected)
   notify({title:dryRun?'Configuration preview ready':'Configuration applied',description:dryRun?'Review the proposed changes before applying.':'Your configuration changes were saved.'})
   setRevision(expected);setPreview(dryRun?result:null)
   if(!dryRun) { const exported=JSON.stringify(await api.exportConfig(),null,2);setText(exported);unsaved.clean(exported) }
  }catch(e){setError(String(e instanceof Error?e.message:e));setPreview(null)}
 })
 return <section aria-label="Configuration as code" className="space-y-5">
  <div className="flex flex-wrap items-start justify-between gap-4"><div><h2 className="text-lg font-semibold tracking-tight">Configuration as code</h2><p className="mt-1 text-sm text-ink-muted">Export saved configuration or review a proposed import before applying it.</p></div><button className={btn} disabled={busy} onClick={exportDocument}><Braces className="size-3.5"/>Export configuration</button></div>
  {busy&&<LoadingState label="Working with configuration…" detail="Wait for the current operation to finish." compact/>}
  {error && <ErrorBox>{error}</ErrorBox>}
  <div className="grid items-start gap-6 xl:grid-cols-[1.4fr_1fr]">
   <ServerPanel title="Configuration source" description="JSON configuration for upstreams, virtual servers and identity bindings" icon={FileCode2} footer="Export excludes stored secrets. Import preserves omitted credentials.">
    {canManage?<fieldset disabled={busy} className="min-w-0 space-y-4 border-0 p-5 m-0">
     <label className="block rounded-lg border border-dashed border-line bg-canvas/50 p-4 text-xs font-medium"><span className="mb-3 inline-flex items-center gap-2"><Upload className="size-3.5"/>Import JSON file</span><input aria-label="Import configuration" type="file" accept="application/json,.json" className="block w-full min-w-0 text-xs text-ink-muted" onChange={async(e)=>{const file=e.target.files?.[0];if(file){try{const content=await file.text();unsaved.guard(()=>{setText(content);setPreview(null)})}catch{setError('Could not read configuration file.')}}}}/></label>
     <textarea aria-label="Configuration document" className={`${input} min-h-64 font-mono text-xs leading-relaxed`} rows={12} value={text} onChange={e=>{setText(e.target.value);setPreview(null)}} placeholder='{"apiVersion":"vmcp/v1","upstreams":[],"virtualServers":[]}'/>
     <label className="flex items-start gap-2 text-xs leading-relaxed text-ink-muted"><input type="checkbox" className="mt-0.5" checked={prune} onChange={e=>{setPrune(e.target.checked);setPreview(null)}}/>Prune configuration absent from document</label>
     <button className={btn} disabled={busy || !text} onClick={()=>run(true)}><GitCompareArrows className="size-3.5"/>Preview changes</button>
    </fieldset>:<div className="space-y-4 p-5">{text?<><pre className="max-h-96 overflow-auto rounded-lg border border-line bg-canvas p-4 font-mono text-xs leading-relaxed">{text}</pre><CopyButton label="Copy exported configuration" value={text}/></>:<p className="text-xs leading-relaxed text-ink-muted">Export the saved configuration to inspect its JSON document. Import and apply require editor or admin access.</p>}</div>}
   </ServerPanel>
   <ServerPanel title="Change preview" description="Proposed configuration changes before apply" icon={GitCompareArrows} footer="Editing the document or prune option invalidates the preview. Apply uses the preview’s base revision.">
    {preview?<><dl className="divide-y divide-line px-5"><DetailRow label="Planned changes">{preview.changes.length}</DetailRow><DetailRow label="Base revision">{revision ?? '—'}</DetailRow><DetailRow label="Prune mode">{prune?'Enabled':'Disabled'}</DetailRow></dl><div className="max-h-80 overflow-auto border-t border-line"><table className="w-full min-w-[320px] text-left text-xs"><caption className="sr-only">Proposed configuration changes</caption><thead className="bg-canvas text-ink-muted"><tr>{['Kind','Name','Change'].map(label=><th key={label} scope="col" className="px-4 py-3 font-medium">{label}</th>)}</tr></thead><tbody>{preview.changes.map((change,i)=><tr key={i} className="border-t border-line"><td className="px-4 py-3">{change.kind}</td><td className="max-w-48 break-all px-4 py-3 font-mono">{change.slug}</td><td className="px-4 py-3"><span className={`rounded-md px-2 py-1 font-medium ${change.action==='delete'?'bg-red-50 text-red-700':'bg-canvas text-ink-muted'}`}>{change.action}</span></td></tr>)}</tbody></table></div>{preview.changes.length===0 && <p className="px-5 py-4 text-xs text-ink-muted">No configuration changes in this preview.</p>}<div className="space-y-3 p-5">{prune && <p className="text-xs leading-relaxed text-amber-700">Applying with prune deletes configuration absent from this document. Confirmation is required.</p>}<button className={primary} disabled={busy || !canManage} onClick={()=>run(false)}>Apply preview</button></div></>:<EmptyState icon={GitCompareArrows} title="No preview yet" description={canManage?'Load or paste a configuration document, then preview its proposed changes.':'Preview and apply are available to editors and admins.'}/>}

   </ServerPanel>
  </div>
 </section>
}
