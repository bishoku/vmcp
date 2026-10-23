import {useCallback,useEffect,useId,useState} from 'react'
import type {MockRule} from '../api'
import {btn,input} from '../styles'
import {useUnsaved} from '../interaction'
import {useConfirm} from '../confirmation'
import {parseMockRules} from './mock-rules'
import {bodyPaths,conditions,isFallback,moveRule,orderedRules,putCondition,removeCondition,type Condition,type ConditionEntry} from './mock-visual'

const pretty=(value:unknown)=>JSON.stringify(value,null,2)
export default function MockRulesEditor({rules=[],bodySchema,onChange,onInvalidChange}:{rules?:MockRule[];bodySchema?:unknown;onChange:(rules:MockRule[])=>void;onInvalidChange:(invalid:boolean)=>void}){
 const [mode,setMode]=useState<'visual'|'json'>('visual'),[text,setText]=useState(pretty(rules)),[error,setError]=useState(''),[responseError,setResponseError]=useState(''),[conditionError,setConditionError]=useState(''),[selected,setSelected]=useState(0),[revision,setRevision]=useState(0)
 const confirm=useConfirm(),invalid=!!(error||responseError||conditionError)
 const reportCondition=useCallback((message:string)=>setConditionError(message),[])
 const reportResponse=useCallback((message:string)=>setResponseError(message),[])
 useUnsaved(invalid?{text,error,responseError,conditionError}:null)
 useEffect(()=>{onInvalidChange(invalid);return()=>onInvalidChange(false)},[invalid,onInvalidChange])
 const publish=(next:MockRule[])=>{const value=pretty(next);setText(value);onChange(next);try{parseMockRules(value);setError('')}catch(e){setError((e as Error).message)}}
 const updateJSON=(value:string)=>{setText(value);try{onChange(parseMockRules(value));setError('');setSelected(0);setRevision(n=>n+1)}catch(e){setError(e instanceof SyntaxError?'Enter valid JSON. Your draft is kept until corrected.':(e as Error).message)}}
 const replace=(rule:MockRule)=>publish(rules.map((r,i)=>i===selected?rule:r))
 const unique=(prefix:string)=>{let n=1;while(rules.some(r=>r.name===`${prefix} ${n}`))n++;return `${prefix} ${n}`}
 const add=(kind:'static'|'dynamic'|'error')=>{
  const priority=Math.min(1000,Math.max(0,...rules.map(r=>r.priority))+1)
  publish([...rules,{name:unique('Response'),priority,response:kind==='dynamic'?{status:200,jsonBody:{message:'Hello {{request.query.name}}'},transformers:['response-template']}:kind==='error'?{status:500,jsonBody:{error:'Simulated upstream error'}}:{status:200,jsonBody:{ok:true}}}]);setSelected(rules.length)
 }
 const rule=rules[selected],ordered=orderedRules(rules),position=ordered.findIndex(r=>r.index===selected)
 return <section className="layout-stack min-w-0" aria-label="Mock response rules">
  <div><h3 className="text-sm font-semibold">Mock responses</h3><p className="mt-1 text-xs text-ink-muted">Rules are checked in priority order. All conditions in a rule must match. A fallback has no conditions; place it last. Method and path come from this operation.</p></div>
  <div className="flex flex-wrap gap-2" role="group" aria-label="Rule editor view"><button type="button" className={btn} aria-pressed={mode==='visual'} disabled={invalid} onClick={()=>setMode('visual')}>Visual editor</button><button type="button" className={btn} aria-pressed={mode==='json'} disabled={invalid&&mode!=='json'} onClick={()=>{if(mode!=='json'){setText(pretty(rules));setMode('json')}}}>WireMock JSON</button></div>
  <div className="flex flex-wrap gap-2">{(['static','dynamic','error'] as const).map(kind=><button type="button" className={btn} key={kind} disabled={invalid||rules.length>=20} onClick={()=>add(kind)}>Add {kind} response</button>)}</div>
  {mode==='json'?<label className="text-xs">Response rules JSON<textarea aria-label="Mock response rules JSON" className={`${input} mt-2 font-mono text-xs`} rows={20} value={text} onChange={e=>updateJSON(e.target.value)}/></label>:<>
   <div className="grid gap-2 sm:grid-cols-2" aria-label="Response cards">{ordered.map(({rule:r,index},i)=><button type="button" key={index} className={`surface-inset min-w-0 p-3 text-left ${index===selected?'ring-2 ring-accent-500':''}`} aria-pressed={index===selected} disabled={invalid} onClick={()=>setSelected(index)}><span className="block break-all text-sm font-semibold">{r.name}</span><span className="mt-1 block text-xs text-ink-muted">#{i+1} · Priority {r.priority} · HTTP {r.response.status} · {r.response.fixedDelayMilliseconds??0} ms</span><span className="block text-xs text-ink-muted">{isFallback(r)?'Fallback':`${conditions(r.request).length} conditions`}{r.response.transformers?.length?' · Dynamic':''}</span></button>)}</div>
   {!rule&&<p className="text-sm text-ink-muted">Add a response to start defining this mock operation.</p>}
   {rule&&<>
    <div className="flex flex-wrap gap-2"><button type="button" className={btn} disabled={invalid||position<=0} onClick={()=>{publish(moveRule(rules,selected,-1));setSelected(position-1)}}>Move earlier</button><button type="button" className={btn} disabled={invalid||position>=rules.length-1} onClick={()=>{publish(moveRule(rules,selected,1));setSelected(position+1)}}>Move later</button><button type="button" className={btn} disabled={invalid||rules.length>=20} onClick={()=>{publish([...rules,{...structuredClone(rule),name:unique('Copy'),priority:Math.min(1000,Math.max(...rules.map(r=>r.priority))+1)}]);setSelected(rules.length)}}>Duplicate response</button><button type="button" className={`${btn} action-danger-outline`} onClick={async()=>{if(await confirm({title:'Remove response?',description:'This removes the response and its conditions from the operation draft.',confirmLabel:'Remove response',tone:'danger'})){publish(rules.filter((_,i)=>i!==selected));setSelected(0);setResponseError('');setRevision(n=>n+1)}}}>Remove response</button></div>
    {ordered.some(({rule:r},i)=>isFallback(r)&&i<ordered.length-1)&&<p role="status" className="text-xs text-amber-800">A fallback appears before other responses and may prevent them from matching. Select that fallback and move it last.</p>}
    <button type="button" className={btn} disabled={invalid} onClick={async()=>{if(isFallback(rule)||await confirm({title:'Make this the fallback?',description:'All request conditions in this response will be removed, including advanced matchers. The response will move last.',confirmLabel:'Make fallback',tone:'warning'})){publish([...ordered.filter(item=>item.index!==selected).map(item=>item.rule),{...rule,request:undefined}].map((r,i)=>({...r,priority:i+1})));setSelected(rules.length-1)}}}>{isFallback(rule)?'Move fallback last':'Make fallback and move last'}</button>
    <p className="text-xs text-ink-muted">Move actions assign consecutive priorities in the displayed order. Equal priorities in imported JSON retain WireMock’s tie-breaking behavior.</p>
    <label className="text-xs">Response name<input aria-label="Response name" className={`${input} mt-1`} value={rule.name} onChange={e=>replace({...rule,name:e.target.value})}/></label>
    <label className="text-xs">Priority<input aria-label="Response priority" className={`${input} mt-1`} type="number" min={1} max={1000} value={rule.priority} onChange={e=>replace({...rule,priority:Number(e.target.value)})}/></label>
    <ConditionEditor key={`condition-${selected}-${revision}`} rule={rule} schema={bodySchema} onPending={reportCondition} onChange={replace} disabled={!!responseError}/>
    <ResponseEditor key={`response-${selected}-${revision}`} response={rule.response} onChange={response=>replace({...rule,response})} onError={reportResponse}/>
   </>}
  </>}
  {(error||responseError||conditionError)&&<p role="alert" className="text-xs text-red-700">{error||responseError||conditionError}</p>}
  <p className="text-xs text-ink-muted">Both views edit the same configuration. Complex matchers remain intact and can be edited in WireMock JSON. Replies must match the operation’s response schema. Dynamic replies require Save and an active WireMock publication to test.</p>
 </section>
}

function ConditionEditor({rule,schema,onChange,disabled,onPending}:{rule:MockRule;schema:unknown;onChange:(rule:MockRule)=>void;disabled:boolean;onPending:(message:string)=>void}){
 const initial:Condition={source:'queryParameters',field:'',operator:'equalTo',value:''}
 const [draft,setDraft]=useState<Condition>(initial),[editing,setEditing]=useState<ConditionEntry>(),[error,setError]=useState('')
 const pending=!!(draft.field||draft.value||editing)
 useEffect(()=>{onPending(pending?'Apply or cancel the pending request condition before leaving this response.':'');return()=>onPending('')},[pending,onPending])
 const paths=bodyPaths(schema),id=useId(),entries=conditions(rule.request)
 const set=(patch:Partial<Condition>)=>{setDraft({...draft,...patch});setError('')}
 return <fieldset disabled={disabled} className="surface-inset p-3 layout-stack min-w-0"><legend className="text-sm font-semibold">Request conditions · AND</legend>
  {!entries.length&&<p className="text-xs text-ink-muted">Fallback: matches every request for this operation. Add conditions to make this response specific.</p>}
  {entries.map((entry,i)=><div key={i} className="border-b border-line pb-2 text-xs"><div className="flex flex-wrap justify-between gap-2"><span className="min-w-0 break-all">{entry.condition?`${entry.source==='bodyPatterns'?'Body JSONPath':entry.source==='headers'?'Header':'Query'} · ${entry.condition.field} · ${entry.condition.operator} ${entry.condition.value}`:'Advanced condition · preserved in WireMock JSON'}</span><div className="flex gap-2">{entry.condition&&<button type="button" className={btn} disabled={pending} onClick={()=>{setDraft(entry.condition!);setEditing(entry);setError('')}}>Edit condition {i+1}</button>}<button type="button" className={btn} disabled={pending} onClick={()=>{onChange({...rule,request:removeCondition(rule.request,entry)});setEditing(undefined);setDraft(initial)}}>Remove condition {i+1}</button></div></div>{!entry.condition&&<pre className="mt-2 max-h-40 overflow-auto whitespace-pre-wrap break-all">{pretty(entry.raw)}</pre>}</div>)}
  <div className="grid gap-2 sm:grid-cols-2">
   <label className="text-xs">Source<select aria-label="Condition source" className={`${input} mt-1`} value={draft.source} onChange={e=>set({source:e.target.value as Condition['source'],field:'',operator:'equalTo'})}><option value="queryParameters">Query parameter</option><option value="headers">Header</option><option value="bodyPatterns">Body JSONPath</option></select></label>
   <label className="text-xs">Field<input aria-label="Condition field" list={draft.source==='bodyPatterns'?id:undefined} className={`${input} mt-1 font-mono`} placeholder={draft.source==='bodyPatterns'?'$.order.id':'name'} value={draft.field} onChange={e=>set({field:e.target.value})}/></label>
   <datalist id={id}>{paths.map(path=><option key={path} value={path}/>)}</datalist>
   <label className="text-xs">Match<select aria-label="Condition operator" className={`${input} mt-1`} value={draft.operator} onChange={e=>set({operator:e.target.value as Condition['operator']})}><option value="equalTo">Equals (text)</option>{draft.source==='bodyPatterns'&&<option value="anyEqualTo">Any array item equals</option>}<option value="contains">Contains</option><option value="matches">Matches regex</option><option value="present">Is present</option>{draft.source!=='bodyPatterns'&&<option value="absent">Is absent</option>}</select></label>
   {!['present','absent'].includes(draft.operator)&&<label className="text-xs">Value<input aria-label="Condition value" className={`${input} mt-1`} value={draft.value} onChange={e=>set({value:e.target.value})}/></label>}
  </div>
  {draft.source==='bodyPatterns'&&paths.length>0&&<label className="text-xs">Choose a request body field<select aria-label="Body schema field" className={`${input} mt-1 font-mono`} value="" onChange={e=>{if(e.target.value)set({field:e.target.value})}}><option value="">Select nested field or array item…</option>{paths.map(path=><option key={path} value={path}>{path}</option>)}</select></label>}
  {draft.source==='bodyPatterns'&&draft.operator==='anyEqualTo'&&<p className="text-xs text-ink-muted">Enter text, or a JSON number / boolean / null. Quote numeric text to compare it as a string. The saved condition becomes an Is present JSONPath filter.</p>}
  {draft.source==='bodyPatterns'&&<p className="text-xs text-ink-muted">Paths refer to the REST body, without MCP’s body wrapper. Select a schema field or enter JSONPath. For any array item, use a filter such as $.order.items[?(@.productId == "SKU")] with Is present. Presence requires a non-null scalar or nonempty selection.</p>}
  <div className="flex flex-wrap gap-2"><button type="button" className={btn} onClick={()=>{try{onChange({...rule,request:putCondition(rule.request,draft,editing)});setDraft(initial);setEditing(undefined);setError('')}catch(e){setError((e as Error).message)}}}>{editing?'Apply condition':'Add condition'}</button>{pending&&<button type="button" className={btn} onClick={()=>{setDraft(initial);setEditing(undefined);setError('')}}>Cancel condition edit</button>}</div>
  {error&&<p role="alert" className="text-xs text-red-700">{error}</p>}
  <p className="text-xs text-ink-muted">Conditions do not add tool arguments. Define query/body inputs in Parameters / Schemas and header forwarding in upstream settings.</p>
  <p className="text-xs text-ink-muted">Click {editing?'Apply condition':'Add condition'} to include the fields above in this rule.</p>
 </fieldset>
}

function ResponseEditor({response,onChange,onError}:{response:MockRule['response'];onChange:(response:MockRule['response'])=>void;onError:(error:string)=>void}){
 const [format,setFormat]=useState<'json'|'text'>(response.body===undefined?'json':'text'),[body,setBody]=useState(response.body??(response.jsonBody===undefined?'':pretty(response.jsonBody))),[headers,setHeaders]=useState<[string,string][]>(Object.entries(response.headers??{})),[error,setError]=useState('')
 useEffect(()=>{onError(error);return()=>onError('')},[error,onError])
 const commit=(nextBody:string,nextFormat:typeof format,nextHeaders:typeof headers,base=response)=>{
  setBody(nextBody);setFormat(nextFormat);setHeaders(nextHeaders)
  try{
   const names=new Set<string>()
   for(const [key,value] of nextHeaders){if(!/^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/.test(key)||names.has(key.toLowerCase())||/[\r\n]/.test(value))throw Error('Response headers need unique valid names and single-line values.');names.add(key.toLowerCase())}
   const next={...base,headers:Object.fromEntries(nextHeaders)};delete next.body;delete next.jsonBody
   if(nextFormat==='json'){if(nextBody.trim())next.jsonBody=JSON.parse(nextBody)}else next.body=nextBody
   onChange(next);setError('')
  }catch(e){setError(e instanceof SyntaxError?'Enter a valid JSON response body. Your draft is kept until corrected.':(e as Error).message)}
 }
 const preset=(status:number,value:unknown)=>commit(pretty(value),'json',headers,{...response,status,transformers:undefined})
 return <section className="surface-inset p-3 layout-stack min-w-0" aria-label="Response editor"><h4 className="text-sm font-semibold">Response</h4>
  <div className="flex flex-wrap gap-2" role="group" aria-label="Response presets">{[[200,'Success',{ok:true}],[200,'Empty list',[]],[404,'Not found',{error:'Not found'}],[401,'Unauthorized',{error:'Unauthorized'}],[500,'Server error',{error:'Simulated upstream error'}]].map(([status,label,value])=><button type="button" className={btn} key={label as string} onClick={()=>preset(status as number,value)}>{label as string}</button>)}</div>
  <div className="grid gap-2 sm:grid-cols-2"><label className="text-xs">Status<input aria-label="Response status" className={`${input} mt-1`} type="number" min={200} max={599} value={response.status} onChange={e=>onChange({...response,status:Number(e.target.value)})}/></label><label className="text-xs">Delay (ms)<input aria-label="Response delay" className={`${input} mt-1`} type="number" min={0} max={120000} value={response.fixedDelayMilliseconds??0} onChange={e=>onChange({...response,fixedDelayMilliseconds:Number(e.target.value)})}/></label></div>
  <div className="layout-stack"><h5 className="text-xs font-semibold">Response headers</h5>{headers.map(([key,value],i)=><div key={i} className="grid gap-2 sm:grid-cols-[1fr_1fr_auto]"><input aria-label={`Response header ${i+1} name`} className={input} value={key} onChange={e=>commit(body,format,headers.map((h,j)=>j===i?[e.target.value,h[1]]:h))}/><input aria-label={`Response header ${i+1} value`} className={input} value={value} onChange={e=>commit(body,format,headers.map((h,j)=>j===i?[h[0],e.target.value]:h))}/><button type="button" className={btn} onClick={()=>commit(body,format,headers.filter((_,j)=>i!==j))}>Remove header {i+1}</button></div>)}<button type="button" className={btn} onClick={()=>commit(body,format,[...headers,['','']])}>Add response header</button></div>
  <label className="text-xs">Body format<select aria-label="Response body format" className={`${input} mt-1`} value={format} onChange={e=>commit(body,e.target.value as typeof format,headers)}><option value="json">JSON</option><option value="text">Text</option></select></label>
  <label className="text-xs">Response body<textarea aria-label="Response body" className={`${input} mt-1 font-mono text-xs`} rows={10} value={body??''} onChange={e=>commit(e.target.value,format,headers)}/></label>
  <div className="flex flex-wrap gap-2"><button type="button" className={btn} disabled={format!=='json'} onClick={()=>{try{commit(pretty(JSON.parse(body)),'json',headers)}catch{setError('Enter a valid JSON response body before formatting.')}}}>Format JSON body</button><label className="flex items-center gap-2 text-xs"><input aria-label="Enable dynamic response templates" type="checkbox" checked={!!response.transformers?.length} onChange={e=>onChange({...response,transformers:e.target.checked?['response-template']:undefined})}/>Dynamic response templates</label></div>
  {response.transformers?.length?<p className="text-xs text-ink-muted">WireMock evaluates templates after publication, for example {'{{request.query.name}}'}. This editor displays the template source; use Execute test request on the saved upstream for its rendered result.</p>:<details><summary className="cursor-pointer text-xs font-semibold">Static body preview</summary><pre aria-label="Static response preview" className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-all text-xs">{body}</pre></details>}
 </section>
}
