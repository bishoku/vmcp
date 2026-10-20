import { useMemo, useState } from 'react'
import { api, ApiError, type RESTImportResult, type RESTOperation } from '../api'
import { FormField } from '../FormField'
import { Checkbox } from '../SelectionControls'
import { SearchField } from '../SearchControls'
import { ErrorBox, Modal } from '../ui'
import { useOperation, useUnsaved } from '../interaction'
import { discardConfirmation, useConfirm } from '../confirmation'
import { btn, input, primary } from '../styles'

const maxBytes=2*1024*1024
const size=(text:string)=>new TextEncoder().encode(text).length
/** Appends only selected operations; the enclosing upstream draft owns persistence. */
export default function RESTImportDialog({existing,onApply,onClose}:{existing:RESTOperation[];onApply:(operations:RESTOperation[])=>void;onClose:()=>void}) {
 const [document,setDocument]=useState(''),[result,setResult]=useState<RESTImportResult|null>(null)
 const [selected,setSelected]=useState<string[]>([]),[query,setQuery]=useState(''),[error,setError]=useState(''),[limit,setLimit]=useState(50)
 const {busy,run}=useOperation(),confirm=useConfirm()
 useUnsaved({document,selected})
 const close=async()=>{if(!busy&&(!document||await confirm(discardConfirmation)))onClose()}
 const editDocument=(value:string)=>{setDocument(value);setResult(null);setSelected([]);setError('');setLimit(50)}
 const analyze=()=>run(async()=>{setError('');setResult(null);setSelected([]);try{
 if(size(document)>maxBytes)throw Error('Use a document of 2 MiB or less.')
 setResult(await api.importREST(document))
 }catch(e){setError(e instanceof ApiError&&e.errors.length?e.errors.map(issue=>issue.message).join(' '):e instanceof Error?e.message:'Unable to analyze this document.')}})
 const candidates=useMemo(()=>{
 const names=new Set(existing.map(o=>o.name))
 return (result?.operations??[]).map(candidate=>{
 const operation=candidate.operation?{...candidate.operation}:undefined
 const already=existing.some(o=>o.method===candidate.method&&o.path===candidate.path)
 let renamed=false
 if(operation){const base=operation.name.slice(0,112);for(let suffix=2;names.has(operation.name);suffix++){operation.name=`${base}_${suffix}`;renamed=true}names.add(operation.name)}
 return {...candidate,operation,already,renamed}
 })
 },[existing,result])
 const shown=candidates.filter(o=>`${o.method} ${o.path} ${o.summary} ${o.tags.join(' ')} ${o.operation?.name??''}`.toLowerCase().includes(query.toLowerCase()))
 const available=shown.filter(o=>o.operation&&!o.errors.length&&!o.already)
 const capacity=200-existing.length
 const apply=()=>{
 const chosen=candidates.filter(o=>selected.includes(o.id)&&o.operation&&!o.errors.length&&!o.already).map(o=>o.operation!)
 if(chosen.length===0||chosen.length>capacity){setError(`Select between 1 and ${Math.max(0,capacity)} operations.`);return}
 // Selection is append-only: no imported name can replace stored secrets/overrides.
 onApply(chosen)
 }
 return <Modal title="Import OpenAPI / Swagger" size="wide" busy={busy} onClose={()=>void close()} description="Analyze a definition, select operations, then add them to your upstream draft." footer={<><button className={btn} disabled={busy} onClick={()=>void close()}>Cancel</button><button className={primary} disabled={busy||!selected.length||selected.length>capacity} onClick={apply}>Add {selected.length} to draft</button></>}>
 <div className="layout-stack">
 <p className="text-xs leading-relaxed text-ink-muted">JSON or YAML · OpenAPI 3.0/3.1 or Swagger 2.0 · up to 2 MiB. Local references are expanded. URLs and external references are never fetched.</p>
 <FormField name="import.file" label="Definition file" hint="Loading a file only fills the definition below."><input type="file" accept=".json,.yaml,.yml,application/json,application/yaml,text/yaml" disabled={busy} className="w-full min-w-0 text-xs" onChange={e=>{const file=e.target.files?.[0];e.target.value='';if(!file)return;void run(async()=>{setResult(null);setSelected([]);try{if(file.size>maxBytes)throw Error('Use a file of 2 MiB or less.');editDocument(new TextDecoder('utf-8',{fatal:true}).decode(await file.arrayBuffer()))}catch(err){setError(err instanceof Error?err.message:'Unable to read this file.')}})}}/></FormField>
 <FormField name="import.document" label="OpenAPI / Swagger definition" hint="Paste a definition or choose a local file. Credentials and examples are not imported as settings."><textarea disabled={busy} className={`${input} font-mono text-xs`} rows={8} value={document} onChange={e=>editDocument(e.target.value)}/></FormField>
 <button className={`${btn} self-start`} disabled={busy||!document.trim()} onClick={()=>void analyze()}>{busy?'Analyzing…':'Analyze definition'}</button>
 {error&&<ErrorBox>{error}</ErrorBox>}
 {result&&<>
 <div className="surface-inset p-4 layout-stack"><p className="text-sm font-medium break-words">{result.title||'API definition'} <span className="text-xs text-ink-muted">{result.version}</span></p>{result.warnings.map((warning,i)=><p key={i} className="text-xs leading-relaxed text-ink-muted">{warning}</p>)}{result.servers.length>0&&<div><p className="text-xs font-medium">Declared servers · verify your upstream base URL</p>{result.servers.map((server,i)=><p key={i} className="mt-1 break-all font-mono text-xs text-ink-muted">{server}</p>)}</div>}</div>
 <SearchField label="Search import operations" value={query} onChange={value=>{setQuery(value);setLimit(50)}} placeholder="Method, path, tag or tool name"/>
 <div className="flex flex-wrap items-center justify-between gap-3"><p role="status" className="text-xs text-ink-muted">{shown.length} of {candidates.length} operations · {selected.length} selected · {Math.max(0,capacity)} slots available</p><div className="flex flex-wrap gap-2"><button className={btn} disabled={!available.length} onClick={()=>{const ids=[...new Set([...selected,...available.map(o=>o.id)])];if(ids.length>capacity){setError(`Only ${Math.max(0,capacity)} slots remain. Narrow the search or select fewer operations.`);return}setSelected(ids);setError('')}}>Select available results</button><button className={btn} disabled={!selected.length} onClick={()=>setSelected([])}>Clear selection</button></div></div>
 <div className="layout-stack">{shown.slice(0,limit).map(candidate=>{
 const checked=selected.includes(candidate.id),disabled=!candidate.operation||candidate.errors.length>0||candidate.already||(!checked&&selected.length>=capacity)
 return <div key={candidate.id} className="surface-inset p-3 layout-stack"><Checkbox className="break-all" label={candidate.id} checked={checked} disabled={disabled} description={candidate.summary||candidate.tags.join(' · ')} onChange={e=>setSelected(ids=>e.target.checked?[...ids,candidate.id]:ids.filter(id=>id!==candidate.id))}/>
 {candidate.operation&&<p className="break-all font-mono text-xs text-ink-muted">Tool: {candidate.operation.name}{candidate.renamed?' · renamed to preserve an existing tool':''}</p>}
 {candidate.already&&<p className="text-xs text-ink-muted">Already configured. Edit the existing operation to change it.</p>}
 {candidate.errors.map((reason,i)=><p key={i} className="break-words text-xs text-red-700">Unavailable: {reason}</p>)}
 {candidate.warnings.map((warning,i)=><p key={i} className="break-words text-xs text-amber-800">{warning}</p>)}
 {candidate.operation&&<OperationPreview operation={candidate.operation}/>}
 </div>
 })}{shown.length===0&&<p className="text-sm text-ink-muted">No operations match this search.</p>}</div>
 <>{shown.length>limit&&<button className={`${btn} self-start`} onClick={()=>setLimit(n=>n+50)}>Show more operations</button>}</>
 <p className="text-xs leading-relaxed text-ink-muted">Adding operations only changes the draft. Review tool names, descriptions and request/response schemas in the operation editor, then use the main Save action to publish them. Existing operations and security settings are preserved.</p>
 </>}
 </div></Modal>
}

function OperationPreview({operation}:{operation:RESTOperation}) {
 const [open,setOpen]=useState(false)
 return <details onToggle={e=>setOpen(e.currentTarget.open)}><summary className="cursor-pointer text-xs font-medium">Preview tool definition</summary>{open&&<pre className="mt-2 whitespace-pre-wrap break-all font-mono text-xs text-ink-muted">{JSON.stringify(operation,null,2)}</pre>}</details>
}
