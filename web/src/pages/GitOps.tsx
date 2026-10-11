import { useOperation, useUnsaved } from '../interaction'
import { useState } from 'react'
import { api, type ApplyResult, type ConfigDocument } from '../api'
import { btn, input, primary } from '../styles'
import { ErrorBox } from '../ui'

export default function GitOps({ canManage }: { canManage: boolean }) {
 const [text,setText]=useState('')
 const [prune,setPrune]=useState(false)
 const [preview,setPreview]=useState<ApplyResult|null>(null)
 const [revision,setRevision]=useState<number|undefined>()
 const [error,setError]=useState('')
 const {busy,run:operate}=useOperation()
 const unsaved=useUnsaved(text)
 const exportDocument=()=>unsaved.guard(()=>{void operate(async()=>{try{const exported=JSON.stringify(await api.exportConfig(),null,2);setText(exported);unsaved.clean(exported);setPreview(null);setError('')}catch(e){setError(String(e instanceof Error?e.message:e))}})})
 const run=(dryRun:boolean)=>operate(async()=>{
  setError('')
  try{
   const document=JSON.parse(text) as ConfigDocument
   if (!dryRun && prune && !confirm('Apply this preview and delete configuration absent from the document?')) return
   const expected=dryRun?(await api.info()).configRevision:revision
   const result=await api.applyConfig(document,dryRun,prune,expected)
   setRevision(expected);setPreview(dryRun?result:null)
   if(!dryRun) { const exported=JSON.stringify(await api.exportConfig(),null,2);setText(exported);unsaved.clean(exported) }
  }catch(e){setError(String(e instanceof Error?e.message:e));setPreview(null)}
 })
 return <section className="mt-10 space-y-3">
  <h2 className="text-lg font-semibold">Configuration as code</h2>
  <p className="text-sm text-ink-muted">Export excludes stored secrets. Import preserves omitted credentials. Preview changes before applying.</p>
  <button className={btn} disabled={busy} onClick={exportDocument}>Export configuration</button>
  {canManage && <>
   <fieldset disabled={busy} className="space-y-3">
   <label className="block text-sm">Import JSON file<input aria-label="Import configuration" type="file" accept="application/json,.json" onChange={async(e)=>{const file=e.target.files?.[0];if(file){try{const content=await file.text();unsaved.guard(()=>{setText(content);setPreview(null)})}catch{setError('Could not read configuration file.')}}}}/></label>
   <textarea aria-label="Configuration document" className={`${input} font-mono text-xs`} rows={10} value={text} onChange={(e)=>{setText(e.target.value);setPreview(null)}}/>
   <label className="flex gap-2 text-sm"><input type="checkbox" checked={prune} onChange={(e)=>{setPrune(e.target.checked);setPreview(null)}}/>Prune configuration absent from document</label>
   <button className={btn} disabled={busy || !text} onClick={()=>run(true)}>Preview changes</button>
   {preview && <><table className="w-full text-left text-sm"><thead><tr><th>Kind</th><th>Name</th><th>Change</th></tr></thead><tbody>{preview.changes.map((c,i)=><tr key={i}><td>{c.kind}</td><td>{c.slug}</td><td>{c.action}</td></tr>)}</tbody></table><button className={primary} disabled={busy} onClick={()=>run(false)}>Apply preview</button></>}
   </fieldset>
  </>}
  {error && <ErrorBox>{error}</ErrorBox>}
 </section>
}
