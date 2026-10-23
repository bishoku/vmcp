import { useEffect, useState } from 'react'
import { checkSchema, schemaLimitError } from './schema-validation'
import { responseSelectionError, restToolPreview } from './rest-tool-preview'
import { inferSchema, mergeExample, objectSchema } from './schema-editor'
import RESTImportDialog from './RESTImportDialog'
import RESTOperationEditor from './RESTOperationEditor'
import RESTToolPreview from './RESTToolPreview'
import { Cable, KeyRound } from 'lucide-react'
import { api, type RESTAuth, type RESTConfig, type RESTOperation, type Upstream } from '../api'
import { ServerPanel } from './ServerPanels'
import { FormField } from '../FormField'
import Select from '../Select'
import { Switch } from '../SelectionControls'
import { ErrorBox, Modal } from '../ui'
import { btn, input, primary } from '../styles'
import { useConfirm } from '../confirmation'
import { useCapabilities, useOperation, useUnsaved } from '../interaction'

const newOperation = (): RESTOperation => ({name:'',description:'',enabled:true,method:'GET',path:'/',parameters:[],readOnly:false})
const json = (value:unknown) => value === undefined ? '' : JSON.stringify(value,null,2)

export default function RESTUpstreamEditor({value,onChange,upstream,onInvalidChange}:{value:RESTConfig;onChange?:(v:RESTConfig)=>void;upstream:Upstream|null;onInvalidChange?:(invalid:boolean)=>void}) {
 const [selected,setSelected]=useState(0),[importing,setImporting]=useState(false),[query,setQuery]=useState(''),[limit,setLimit]=useState(50)
 const [draft,setDraft]=useState<RESTOperation|null>(null),[draftIndex,setDraftIndex]=useState<number|null>(null),[initial,setInitial]=useState('')
 const [invalid,setInvalid]=useState(false),[issues,setIssues]=useState<string[]>([]),[checking,setChecking]=useState(false)
 useUnsaved(draft&&(invalid||JSON.stringify(draft)!==initial)?{draft,invalid}:null)
 const confirm=useConfirm()
 useEffect(()=>{onInvalidChange?.(!!draft||importing);return()=>onInvalidChange?.(false)},[draft,importing,onInvalidChange])
 const openOperation=(index:number|null,operation:RESTOperation)=>{setDraftIndex(index);setDraft(structuredClone(operation));setInitial(JSON.stringify(operation));setInvalid(false);setIssues([])}
 const closeOperation=async()=>{if(checking)return;if(draft&&(invalid||JSON.stringify(draft)!==initial)&&!await confirm({title:'Discard operation changes?',description:'Changes made in this operation window will be discarded. Applied upstream draft changes are kept.',confirmLabel:'Discard changes',tone:'warning'}))return;setDraft(null);setInvalid(false);setIssues([])}
 const apply=async()=>{
  if(!draft||!onChange||invalid)return
  const problems:string[]=[]
  if(!/^[A-Za-z0-9_.-]{1,128}$/.test(draft.name))problems.push('Tool name must contain 1–128 letters, numbers, dots, underscores or hyphens.')
  if(value.operations.some((o,i)=>i!==draftIndex&&o.name===draft.name))problems.push('Tool name must be unique.')
  if(!draft.path.startsWith('/'))problems.push('Operation path must start with /.')
  if(draft.method==='GET'&&draft.bodySchema!==undefined)problems.push('GET requests cannot have a body schema. Remove it in Advanced settings or choose another method.')
  const parameterNames=new Set<string>()
  for(const parameter of draft.parameters){
   if(!/^[A-Za-z0-9_.-]{1,128}$/.test(parameter.name)||parameter.name==='body'||parameterNames.has(parameter.name))problems.push(`Parameter name must be valid, unique and different from body: ${parameter.name||'(empty)'}`)
   parameterNames.add(parameter.name)
  }
  const tool=restToolPreview(draft)
  for(const [label,schema] of [['Combined request',tool.inputSchema],['MCP output',tool.outputSchema]] as const){const issue=schemaLimitError(schema);if(issue)problems.push(`${label}: ${issue}`)}
  const selection=responseSelectionError(draft.responseSchema,draft.responseFields,draft.responseFieldPaths)
  if(selection)problems.push(selection)
  setChecking(true)
  try {
   for(const [label,schema] of [['Request body',draft.bodySchema],['Response',draft.responseSchema]] as const)problems.push(...(await checkSchema(schema)).map(issue=>`${label}: ${issue}`))
   if(problems.length){setIssues(problems);return}
   const operations=draftIndex===null?[...value.operations,draft]:value.operations.map((o,i)=>i===draftIndex?draft:o)
   onChange({...value,operations});setSelected(draftIndex??value.operations.length);setDraft(null);setInvalid(false);setIssues([])
  }finally{setChecking(false)}
 }
 const operation=value.operations[selected]
 const shown=value.operations.map((op,i)=>({op,i})).filter(({op})=>`${op.name} ${op.method} ${op.path}`.toLowerCase().includes(query.toLowerCase()))
 return <section aria-label="REST to MCP workspace" className="layout-stack min-w-0">
  <div className="flex flex-wrap items-start justify-between gap-4"><div><h2 className="section-title">REST → MCP workspace</h2><p className="section-description mt-1 text-sm text-ink-muted">Manage operations here. Open a workspace to edit requests and schemas without extending this page.</p></div>{onChange&&<div className="flex flex-wrap gap-2"><button className={primary} disabled={value.operations.length>=200} onClick={()=>setImporting(true)}>Import OpenAPI / Swagger</button><button className={btn} disabled={value.operations.length>=200} onClick={()=>openOperation(null,newOperation())}>Add operation</button></div>}</div>
  <p role="status" className="text-xs text-ink-muted">{value.operations.filter(op=>op.enabled).length} enabled tools · {value.operations.length} total · Apply operation changes to the draft, then Save the upstream.</p>
  <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
   <section aria-label="REST operations" className="surface-card panel-body layout-stack min-w-0">
    <FormField name="rest.search" label="Find an operation"><input className={input} placeholder="Name, method or path" value={query} onChange={e=>{setQuery(e.target.value);setLimit(50)}}/></FormField>
    <div className="max-h-[32rem] overflow-auto layout-stack">{shown.slice(0,limit).map(({op,i})=><article key={i} data-validation-field={`rest.operations.${i}`} tabIndex={-1} className="surface-inset p-3 layout-stack"><div className="flex min-w-0 flex-wrap items-start justify-between gap-2"><button aria-label={`${op.name||'Untitled tool'} ${op.method} ${op.path}`} className="min-w-0 flex-1 text-left" onClick={()=>setSelected(i)} aria-current={i===selected?'true':undefined}><span className="block break-all text-sm font-semibold">{op.name||'Untitled tool'}</span><span className="mt-1 block break-all font-mono text-xs text-ink-muted">{op.method} {op.path}</span></button><span className="text-xs text-ink-muted">{op.enabled?'In catalog':'Disabled by configuration'}</span></div><div className="flex flex-wrap gap-2"><button className={btn} onClick={()=>openOperation(i,op)}>{onChange?'Edit':'View'} operation {op.name}</button>{onChange&&<button className={`${btn} action-danger-outline`} onClick={async()=>{if(await confirm({title:'Remove operation?',description:'After Save, virtual servers selecting this tool will no longer be able to call it.',confirmLabel:'Remove operation',tone:'danger'})){onChange({...value,operations:value.operations.filter((_,j)=>j!==i)});setSelected(0)}}}>Remove {op.name}</button>}</div></article>)}{!shown.length&&<p className="text-sm text-ink-muted">{value.operations.length?'No operations match your search.':'Import OpenAPI / Swagger or add your first operation.'}</p>}{shown.length>limit&&<button className={btn} onClick={()=>setLimit(n=>n+50)}>Show more operations</button>}</div>
   </section>
   <RESTToolPreview operation={operation}/>
  </div>
  {importing&&onChange&&<RESTImportDialog existing={value.operations} onClose={()=>setImporting(false)} onApply={operations=>{onChange({...value,operations:[...value.operations,...operations]});setSelected(value.operations.length);setQuery('');setImporting(false)}}/>}
  {draft&&<Modal title={`${onChange?'Edit':'View'} REST operation`} size="audit" busy={checking} onClose={()=>void closeOperation()} description="Apply changes to the upstream draft. Save the upstream afterward to publish them." footer={<div className="flex flex-wrap justify-end gap-3"><button className={btn} onClick={()=>void closeOperation()}>{onChange?'Cancel operation changes':'Close operation'}</button>{onChange&&<button className={primary} disabled={invalid||checking} onClick={()=>void apply()}>{checking?'Checking…':'Apply operation'}</button>}</div>}>
   <div className="layout-stack">{!!issues.length&&<ErrorBox title="Operation needs attention"><ul className="space-y-1">{issues.map((issue,i)=><li key={i}>{issue}</li>)}</ul></ErrorBox>}<div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)]"><RESTOperationEditor operation={draft} index={draftIndex??value.operations.length} names={[...value.operations.filter((_,i)=>i!==draftIndex).map(op=>op.name),draft.name]} onInvalidChange={setInvalid} onChange={onChange?setDraft:undefined}/><RESTToolPreview operation={draft} invalid={invalid}/></div></div>
  </Modal>}
  {upstream&&<RESTTest key={upstream.version} upstream={upstream} onUseResponse={onChange?(name,result)=>{const position=value.operations.findIndex(op=>op.name===name);if(position>=0){const op=value.operations[position];openOperation(position,{...op,responseSchema:mergeExample(op.responseSchema,inferSchema(result))})}}:undefined} draftNames={value.operations.map(op=>op.name)}/>}
 </section>
}

export function RESTConnectionSettings({value,onChange}:{value:RESTConfig;onChange?:(v:RESTConfig)=>void}) {
 const set=(patch:Partial<RESTConfig>)=>onChange?.({...value,...patch})
 const auth=(patch:Partial<RESTAuth>)=>set({auth:{...value.auth,...patch}})
 return <>
 <ServerPanel title="REST authentication" description="Service credentials are encrypted and never returned by the API" icon={KeyRound}>
 <fieldset disabled={!onChange} className="panel-body layout-stack min-w-0">
 <FormField name="rest.auth.type" label="Authentication"><Select className={input} value={value.auth.type} onChange={e=>set({auth:{type:e.target.value as RESTAuth['type'],tokenAuthMethod:'client_secret_basic'}})}><option value="none">None / manual headers</option><option value="basic">Basic Auth</option><option value="bearer">Bearer token</option><option value="oauth2">OAuth 2.0 Client Credentials</option></Select></FormField>
 {value.auth.type==='basic'&&<><FormField name="rest.auth.username" label="Username" required><input className={input} value={value.auth.username??''} onChange={e=>auth({username:e.target.value})}/></FormField><FormField name="rest.auth.password" label="Password" hint="Leave empty to keep the stored password."><input className={input} type="password" autoComplete="new-password" placeholder={value.auth.hasPassword?'Stored password':''} value={value.auth.password??''} onChange={e=>auth({password:e.target.value})}/></FormField></>}
 {value.auth.type==='bearer'&&<FormField name="rest.auth.token" label="Bearer token" hint="Leave empty to keep the stored token."><input className={input} type="password" autoComplete="new-password" placeholder={value.auth.hasToken?'Stored token':''} value={value.auth.token??''} onChange={e=>auth({token:e.target.value})}/></FormField>}
 {value.auth.type==='oauth2'&&<>
 <FormField name="rest.auth.tokenUrl" label="Token endpoint" required><input className={input} value={value.auth.tokenUrl??''} onChange={e=>auth({tokenUrl:e.target.value})}/></FormField>
 <FormField name="rest.auth.clientId" label="Client ID" required><input className={input} value={value.auth.clientId??''} onChange={e=>auth({clientId:e.target.value})}/></FormField>
 <FormField name="rest.auth.clientSecret" label="Client secret" hint="Leave empty to keep the stored secret."><input className={input} type="password" autoComplete="new-password" placeholder={value.auth.hasClientSecret?'Stored secret':''} value={value.auth.clientSecret??''} onChange={e=>auth({clientSecret:e.target.value})}/></FormField>
 <FormField name="rest.auth.scopes" label="Scopes" hint="Space-separated scopes"><input className={input} value={(value.auth.scopes??[]).join(' ')} onChange={e=>auth({scopes:e.target.value.split(' ')})}/></FormField>
 <FormField name="rest.auth.audience" label="Audience"><input className={input} value={value.auth.audience??''} onChange={e=>auth({audience:e.target.value})}/></FormField>
 <FormField name="rest.auth.tokenAuthMethod" label="Token authentication"><Select className={input} value={value.auth.tokenAuthMethod??'client_secret_basic'} onChange={e=>auth({tokenAuthMethod:e.target.value as RESTAuth['tokenAuthMethod']})}><option value="client_secret_basic">Authorization header</option><option value="client_secret_post">Request body</option></Select></FormField>
 <p className="text-xs text-ink-muted">Tokens are cached. REST operations are never automatically replayed, including after a 401 response.</p>
 </>}
 </fieldset></ServerPanel>
 <ServerPanel title="REST request controls" description="Explicit identity forwarding and bounded execution" icon={Cable}>
 <fieldset disabled={!onChange} className="panel-body layout-stack min-w-0">
 <FormField name="rest.timeoutSeconds" label="Request timeout (seconds)" hint="1–120 seconds, including token acquisition"><input type="number" min={1} max={120} className={input} value={value.timeoutSeconds} onChange={e=>set({timeoutSeconds:Number(e.target.value)})}/></FormField>
 <Switch label="Forward verified agent identity" checked={value.forwardIdentity} onChange={e=>set({forwardIdentity:e.target.checked,requireIdentity:e.target.checked&&value.requireIdentity})}/>
 <p className="text-xs leading-relaxed text-ink-muted">Adds X-VMCP-Agent-Id, X-VMCP-Auth-Type and X-VMCP-Virtual-Server from the authenticated session. Anonymous callers send no identity headers. Caller-supplied headers cannot override this identity.</p>
 <Switch label="Require verified identity" disabled={!value.forwardIdentity} checked={value.requireIdentity} onChange={e=>set({requireIdentity:e.target.checked})}/>
 <Switch label="Allow unencrypted HTTP" checked={value.allowInsecureHttp} onChange={e=>set({allowInsecureHttp:e.target.checked})}/>
 <p className="text-xs text-ink-muted">HTTPS is the default. The HTTP exception also applies to the token endpoint; use it only in trusted development environments. Network access rules still apply.</p>
 </fieldset></ServerPanel>
 </>
}

function RESTTest({upstream,onUseResponse,draftNames}:{upstream:Upstream;onUseResponse?:(name:string,value:unknown)=>void;draftNames:string[]}) {
 const [operation,setOperation]=useState(upstream.rest?.operations.find(o=>o.enabled)?.name??'')
 const [sample,setSample]=useState<{operation:string;value:unknown}>()
 const [args,setArgs]=useState('{}'),[result,setResult]=useState(''),[error,setError]=useState('')
 const {manageTokens:admin,edit}=useCapabilities(),{busy,run}=useOperation(),confirm=useConfirm()
 const perform=(action:'preview'|'test')=>run(async()=>{setError('');setResult('');setSample(undefined);try{
 const parsed=JSON.parse(args)
 if(action==='test'&&!await confirm({title:'Execute REST operation?',description:'This sends a real request using saved credentials and may change data in the REST service. It will not be retried or impersonate an agent.',confirmLabel:'Execute request',tone:'warning'}))return
 const response=await api.restOperation(upstream.slug,action,operation,parsed,upstream.version)
 setResult(json(response))
 const envelope=objectSchema(response),structured=objectSchema(envelope.structuredContent)
 if(envelope.isError){const content=Array.isArray(envelope.content)?envelope.content:[];const message=content.map(item=>objectSchema(item).text).filter(text=>typeof text==='string').join(' ');setError(message||'The saved REST operation returned an error.')}
 if(action==='test'&&!envelope.isError&&Object.hasOwn(structured,'result'))setSample({operation,value:structured.result})
 }catch(e){setError(e instanceof Error?e.message:'Request failed')}})
 return <ServerPanel title="Operation preview & test" description="Uses the saved configuration; preview never sends a request" icon={Cable}><div className="panel-body layout-stack">
 <FormField name="rest.test.operation" label="Operation"><Select className={input} value={operation} onChange={e=>{setOperation(e.target.value);setResult('');setSample(undefined)}}>{upstream.rest?.operations.filter(o=>o.enabled).map(o=><option key={o.name}>{o.name}</option>)}</Select></FormField>
 <FormField name="rest.test.arguments" label="Tool arguments (JSON)"><textarea className={`${input} font-mono`} rows={5} value={args} onChange={e=>setArgs(e.target.value)}/></FormField>
 <div className="flex flex-wrap gap-2"><button className={btn} disabled={!edit||busy||!operation} onClick={()=>void perform('preview')}>Preview request</button><button className={btn} disabled={!admin||busy||!operation||!upstream.enabled||upstream.rest?.requireIdentity} onClick={()=>void perform('test')}>Execute test request</button></div>
 <p className="text-xs text-ink-muted">Execution requires admin access and an enabled upstream. Identity-required operations must be tested through an authenticated MCP client.</p>
 {sample&&<><p className="text-xs text-ink-muted">Generate from the saved operation’s returned result (including any saved field projection). Existing schema rules are preserved. Edit the upstream to apply it to the draft.</p><button className={btn} disabled={!onUseResponse||busy||!draftNames.includes(sample.operation)} onClick={()=>{try{onUseResponse?.(sample.operation,sample.value);setSample(undefined)}catch(e){setError((e as Error).message)}}}>Use test response as schema</button></>}
 {error&&<ErrorBox>{error}</ErrorBox>}{result&&<pre className="surface-inset whitespace-pre-wrap break-all p-3 text-xs" aria-label="REST operation result">{result}</pre>}
 </div></ServerPanel>
}
