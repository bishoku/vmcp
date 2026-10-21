import type { CallerContextConfig } from '../api'
import { Switch } from '../SelectionControls'
import { btn, input } from '../styles'

export default function CallerContextEditor({value,kind,onChange}:{value:CallerContextConfig;kind:'mcp'|'rest';onChange?:(value:CallerContextConfig)=>void}) {
 const update=(patch:Partial<CallerContextConfig>)=>onChange?.({...value,...patch})
 return <section aria-label="Caller context" className="surface-card panel-body space-y-4">
  <div><h2 className="panel-title">Caller context</h2><p className="mt-1 text-xs text-ink-muted">Forward selected fields from each tool call’s _meta. The upstream validates tokens; vMCP does not verify these values.</p></div>
  <fieldset disabled={!onChange} className="min-w-0 space-y-4">
   <Switch label="Forward caller context" checked={value.enabled} onChange={e=>update({enabled:e.target.checked})}/>
   {value.enabled&&<>
    <label className="block text-xs">Metadata namespace<input aria-label="Metadata namespace" className={`${input} mt-2 w-full`} value={value.metaKey} placeholder="com.bank/caller-context" onChange={e=>update({metaKey:e.target.value})}/></label>
    {value.fields.map((field,index)=><div key={index} className="grid gap-3 rounded-lg border border-line p-3 sm:grid-cols-2">
     <label className="text-xs">Source field<input aria-label={`Context field ${index+1}`} className={`${input} mt-2 w-full`} value={field.name} onChange={e=>update({fields:value.fields.map((f,i)=>i===index?{...f,name:e.target.value}:f)})}/></label>
     {kind==='rest'&&<label className="text-xs">Target HTTP header<input aria-label={`Context header ${index+1}`} className={`${input} mt-2 w-full`} value={field.header??''} onChange={e=>update({fields:value.fields.map((f,i)=>i===index?{...f,header:e.target.value}:f)})}/></label>}
     <Switch label={`Require context field ${index+1}`} checked={!!field.required} onChange={e=>update({fields:value.fields.map((f,i)=>i===index?{...f,required:e.target.checked}:f)})}/>
     {kind==='rest'&&<Switch label={`Bearer prefix for field ${index+1}`} checked={!!field.bearer} onChange={e=>update({fields:value.fields.map((f,i)=>i===index?{...f,bearer:e.target.checked}:f)})}/>}
     <button type="button" className={`${btn} justify-self-start`} onClick={()=>update({fields:value.fields.filter((_,i)=>i!==index)})}>Remove context field {index+1}</button>
    </div>)}
    <button type="button" className={btn} disabled={value.fields.length>=16} onClick={()=>update({fields:[...value.fields,{name:'',...(kind==='rest'?{header:''}:{})}]})}>Add context field</button>
    <p className="text-xs text-ink-muted">{kind==='mcp'?'Selected fields retain this namespace in the upstream tool call.':'Header names must not conflict with static headers or managed authentication. Bearer prefix is only supported for Authorization.'} Missing optional fields are omitted. Use string values up to 8 KiB per field; total metadata is limited to 32 KiB.</p>
   </>}
  </fieldset>
 </section>
}
