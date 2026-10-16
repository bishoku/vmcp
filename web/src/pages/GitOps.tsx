import Button from '../Button'
import { StatusBadge } from '../Badge'
import { Checkbox } from '../SelectionControls'
import { DataTable } from '../DataTable'
import { useTableSort } from '../table-sorting'
import { FormField, FormScope, FormErrors } from '../FormField'
import { useFormValidation, requiredText } from '../form-validation'
import { useNotifications } from '../notifications'
import { EmptyState, LoadingState } from '../Feedback'
import { useConfirm } from '../confirmation'
import { useOperation, useUnsaved } from '../interaction'
import { Braces, FileCode2, GitCompareArrows, Upload } from 'lucide-react'
import { useState } from 'react'
import { api, ApiError, type ApplyResult, type ConfigDocument } from '../api'
import { btn, input, primary } from '../styles'
import { DetailRow, ServerPanel } from '../connections/ServerPanels'
import { CopyButton, ErrorBox } from '../ui'

export default function GitOps({ canManage }: { canManage: boolean }) {
  const {notify}=useNotifications()
  const validation=useFormValidation()
  const confirm=useConfirm()
 const [text,setText]=useState('')
 const [prune,setPrune]=useState(false)
 const [preview,setPreview]=useState<ApplyResult|null>(null)
 const [revision,setRevision]=useState<number|undefined>()
 const [error,setError]=useState('')
 const {busy,run:operate}=useOperation()
 const unsaved=useUnsaved(text)
 const sorting=useTableSort(preview?.changes??[],{Kind:c=>c.kind,Name:c=>c.slug,Change:c=>c.action})
 const exportDocument=()=>unsaved.guard(()=>{void operate(async()=>{try{const exported=JSON.stringify(await api.exportConfig(),null,2);setText(exported);unsaved.clean(exported);setPreview(null);setError('');validation.reset();notify({title:'Configuration exported',description:'Saved configuration is ready to review.'})}catch(e){setError(String(e instanceof Error?e.message:e))}})})
 const run=(dryRun:boolean)=>operate(async()=>{
  if(!canManage)return
  setError('')
  if(!validation.check({document:requiredText(text,'Configuration document')}))return
  try{
   const document=JSON.parse(text) as ConfigDocument
   if(!document || typeof document!=='object' || Array.isArray(document))throw new SyntaxError('Use a JSON object for the configuration document.')
   if (!dryRun && prune && !await confirm({title:'Apply preview and delete absent configuration?',description:'Items missing from this document will be removed according to the reviewed prune preview.',confirmLabel:'Apply and delete',tone:'danger'})) return
   const expected=dryRun?(await api.info()).configRevision:revision
   const result=await api.applyConfig(document,dryRun,prune,expected)
   notify({title:dryRun?'Configuration preview ready':'Configuration applied',description:dryRun?'Review the proposed changes before applying.':'Your configuration changes were saved.'})
   setRevision(expected);setPreview(dryRun?result:null)
   if(!dryRun) { const exported=JSON.stringify(await api.exportConfig(),null,2);setText(exported);unsaved.clean(exported) }
  }catch(e){validation.fail(new ApiError(0,'Configuration could not be processed',undefined,e instanceof ApiError && e.errors.length?e.errors.map(issue=>({field:'/document',message:`${issue.field??''} ${issue.message}`.trim()})):[{field:e instanceof SyntaxError?'/document':undefined,message:e instanceof SyntaxError?'Configuration is not valid JSON. Check the document and try again.':e instanceof Error?e.message:String(e)}]));setPreview(null)}
 })
 return <FormScope validation={validation}><section aria-label="Configuration as code" className="layout-stack">
  <div className="flex flex-wrap items-start justify-between gap-4"><div><h2 className="section-title">Configuration as code</h2><p className="section-description mt-1 text-sm text-ink-muted">Export saved configuration or review a proposed import before applying it.</p></div><button className={btn} disabled={busy} onClick={exportDocument}><Braces aria-hidden="true" className="icon-inline"/>Export configuration</button></div>
  {busy&&<LoadingState label="Working with configuration…" detail="Wait for the current operation to finish." compact/>}
  {error && <ErrorBox>{error}</ErrorBox>}
  <FormErrors validation={validation} title="Configuration could not be processed"/>
  <div className="grid items-start layout-grid xl:grid-cols-[1.4fr_1fr]">
   <ServerPanel headingLevel={3} title="Configuration source" description="JSON configuration for upstreams, virtual servers and identity bindings" icon={FileCode2} footer="Export excludes stored secrets. Import preserves omitted credentials.">
    {canManage?<fieldset disabled={busy} className="min-w-0 space-y-4 border-0 panel-body m-0">
     <label className="block rounded-lg border border-dashed border-line bg-canvas/50 p-4 text-xs font-medium"><span className="mb-3 inline-flex items-center gap-2"><Upload aria-hidden="true" className="icon-inline"/>Import JSON file</span><input aria-label="Import configuration" type="file" accept="application/json,.json" className="block w-full min-w-0 text-xs text-ink-muted" onChange={async(e)=>{const file=e.target.files?.[0];if(file){try{const content=await file.text();unsaved.guard(()=>{setText(content);setPreview(null)})}catch{setError('Could not read configuration file.')}}}}/></label>
     <FormField name="document" label="Configuration document" required hint="Paste a JSON object or import a file. Preview changes before applying."><textarea aria-label="Configuration document" className={`${input} min-h-64 font-mono text-xs leading-relaxed`} rows={12} value={text} onChange={e=>{setText(e.target.value);setPreview(null)}} placeholder='{"apiVersion":"vmcp/v1","upstreams":[],"virtualServers":[]}'/></FormField>
     <Checkbox label="Prune configuration absent from document" description="Review the preview and confirm before removing configuration." checked={prune} onChange={e=>{setPrune(e.target.checked);setPreview(null)}}/>
     <Button loading={busy} className={btn} disabled={busy} onClick={()=>run(true)}><GitCompareArrows aria-hidden="true" className="icon-inline"/>Preview changes</Button>
    </fieldset>:<div className="space-y-4 panel-body">{text?<><pre className="overflow-x-auto whitespace-pre-wrap break-all surface-inset p-4 font-mono text-xs leading-relaxed">{text}</pre><CopyButton label="Copy exported configuration" value={text}/></>:<p className="text-xs leading-relaxed text-ink-muted">Export the saved configuration to inspect its JSON document. Import and apply require editor or admin access.</p>}</div>}
   </ServerPanel>
   <ServerPanel headingLevel={3} title="Change preview" description="Proposed configuration changes before apply" icon={GitCompareArrows} footer="Editing the document or prune option invalidates the preview. Apply uses the preview’s base revision.">
    {preview?<><dl className="divide-y divide-line panel-inset"><DetailRow label="Planned changes">{preview.changes.length}</DetailRow><DetailRow label="Base revision">{revision ?? '—'}</DetailRow><DetailRow label="Prune mode">{prune?'Enabled':'Disabled'}</DetailRow></dl><DataTable caption="Proposed configuration changes" columns={['Kind','Name','Change']} sorting={sorting} primary={1} minWidth={320}>{sorting.rows.map((change,i)=><tr key={i} className="border-t border-line"><td className="px-4 py-3">{change.kind}</td><td className="max-w-48 break-all px-4 py-3 font-mono">{change.slug}</td><td className="px-4 py-3"><StatusBadge status={change.action}/></td></tr>)}</DataTable>{preview.changes.length===0 && <p className="panel-inset py-4 text-xs text-ink-muted">No configuration changes in this preview.</p>}<div className="space-y-3 panel-body">{prune && <p className="text-xs leading-relaxed text-amber-700">Applying with prune deletes configuration absent from this document. Confirmation is required.</p>}<Button loading={busy} className={primary} disabled={busy || !canManage} onClick={()=>run(false)}>Apply preview</Button></div></>:<EmptyState icon={GitCompareArrows} title="No preview yet" description={canManage?'Load or paste a configuration document, then preview its proposed changes.':'Preview and apply are available to editors and admins.'}/>}

   </ServerPanel>
  </div>
 </section></FormScope>
}
