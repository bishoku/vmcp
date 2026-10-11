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
 const [busy,setBusy]=useState(false)
 const exportDocument=async()=>{try{setText(JSON.stringify(await api.exportConfig(),null,2));setPreview(null);setError('')}catch(e){setError(String(e instanceof Error?e.message:e))}}
 const run=async(dryRun:boolean)=>{
  setBusy(true);setError('')
  try{
   const document=JSON.parse(text) as ConfigDocument
   if (!dryRun && prune && !confirm('Apply this preview and delete configuration absent from the document?')) return
   const expected=dryRun?(await api.info()).configRevision:revision
   const result=await api.applyConfig(document,dryRun,prune,expected)
   setRevision(expected);setPreview(dryRun?result:null)
   if(!dryRun) setText(JSON.stringify(await api.exportConfig(),null,2))
  }catch(e){setError(String(e instanceof Error?e.message:e));setPreview(null)}finally{setBusy(false)}
 }
 return <section className="mt-10 space-y-3">
  <h2 className="text-lg font-semibold">Configuration as code</h2>
  <p className="text-sm text-ink-muted">Export excludes stored secrets. Import preserves omitted credentials. Preview changes before applying.</p>
  <button className={btn} onClick={exportDocument}>Export configuration</button>
  {canManage && <>
   <label className="block text-sm">Import JSON file<input aria-label="Import configuration" type="file" accept="application/json,.json" onChange={async(e)=>{const file=e.target.files?.[0];if(file){setText(await file.text());setPreview(null)}}}/></label>
   <textarea aria-label="Configuration document" className={`${input} font-mono text-xs`} rows={10} value={text} onChange={(e)=>{setText(e.target.value);setPreview(null)}}/>
   <label className="flex gap-2 text-sm"><input type="checkbox" checked={prune} onChange={(e)=>{setPrune(e.target.checked);setPreview(null)}}/>Prune configuration absent from document</label>
   <button className={btn} disabled={busy || !text} onClick={()=>run(true)}>Preview changes</button>
   {preview && <><table className="w-full text-left text-sm"><thead><tr><th>Kind</th><th>Name</th><th>Change</th></tr></thead><tbody>{preview.changes.map((c,i)=><tr key={i}><td>{c.kind}</td><td>{c.slug}</td><td>{c.action}</td></tr>)}</tbody></table><button className={primary} disabled={busy} onClick={()=>run(false)}>Apply preview</button></>}
  </>}
  {error && <ErrorBox>{error}</ErrorBox>}
 </section>
}
