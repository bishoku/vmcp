import { useState } from 'react'
import type { RequestHeaderMapping } from '../api'
import { Modal, ErrorBox } from '../ui'
import { FormGroup } from '../FormField'
import { Switch } from '../SelectionControls'
import { btn, input, primary } from '../styles'
import { ServerPanel } from './ServerPanels'
import { ArrowRightLeft } from 'lucide-react'

export default function RequestHeaderMappings({value,onChange}:{value:RequestHeaderMapping[];onChange?:(rules:RequestHeaderMapping[])=>void}) {
 const [draft,setDraft]=useState<RequestHeaderMapping[]|null>(null)
 const [error,setError]=useState('')
 const apply=()=>{
  if(!draft)return
  const names=/^[!#$%&'*+.^_`|~0-9A-Za-z-]{1,128}$/
  if(draft.some(r=>!names.test(r.source)||!names.test(r.target))){setError('Enter a valid source and target header for every mapping.');return}
  if(new Set(draft.map(r=>r.target.toLowerCase())).size!==draft.length){setError('Each target header must be unique.');return}
  onChange?.(draft);setDraft(null)
 }
 return <FormGroup name="requestHeaderMappings" label="Request header mapping configuration"><ServerPanel title="Request header mappings" description="Forward selected HTTP headers from each tool call to this upstream" icon={ArrowRightLeft} actions={onChange&&<button className={btn} onClick={()=>{setDraft(value.map(r=>({...r})));setError('')}}>Configure mappings</button>}>
  <div className="panel-body layout-stack">
   {!value.length?<p className="text-sm text-ink-muted">No incoming HTTP headers are forwarded.</p>:value.map((r,i)=><div key={i} className="flex flex-wrap items-center gap-2 text-xs"><code className="break-all">{r.source}</code><span aria-label="maps to">→</span><code className="break-all">{r.target}</code><span className="text-ink-muted">{r.required?'Required':'Optional'}</span></div>)}
   <p className="text-xs text-ink-muted">Values are copied unchanged for the current tool call only. Transport and gateway authentication headers cannot be mapped.</p>
  </div>
  {draft&&<Modal title="Configure request header mappings" size="wide" description="Changes are added to your upstream draft. Save the upstream to apply them." onClose={()=>setDraft(null)}>
   <div className="layout-stack">
    {error&&<ErrorBox>{error}</ErrorBox>}
    {draft.map((r,i)=><div key={i} className="grid gap-3 rounded-lg border border-line p-4 sm:grid-cols-2">
     <label className="text-xs">Incoming header<input autoFocus={i===0} aria-label={`Source header ${i+1}`} className={`${input} mt-2 w-full`} value={r.source} onChange={e=>setDraft(draft.map((v,j)=>j===i?{...v,source:e.target.value,target:v.target===v.source?e.target.value:v.target}:v))}/></label>
     <label className="text-xs">Upstream header<input aria-label={`Target header ${i+1}`} className={`${input} mt-2 w-full`} value={r.target} onChange={e=>setDraft(draft.map((v,j)=>j===i?{...v,target:e.target.value}:v))}/></label>
     <Switch label={`Require header ${i+1}`} checked={!!r.required} onChange={e=>setDraft(draft.map((v,j)=>j===i?{...v,required:e.target.checked}:v))}/>
     <button className={`${btn} justify-self-start`} onClick={()=>setDraft(draft.filter((_,j)=>j!==i))}>Remove mapping {i+1}</button>
    </div>)}
    <button className={`${btn} self-start`} disabled={draft.length>=16} onClick={()=>setDraft([...draft,{source:'',target:''}])}>Add header mapping</button>
    <p className="text-xs text-ink-muted">Missing optional headers are omitted. Missing required headers stop the tool call before it reaches the upstream. Targets must not conflict with static headers or caller context mappings.</p>
    <div className="flex justify-end gap-3"><button className={btn} onClick={()=>setDraft(null)}>Cancel</button><button className={primary} onClick={apply}>Use mappings</button></div>
   </div>
  </Modal>}
 </ServerPanel></FormGroup>
}
