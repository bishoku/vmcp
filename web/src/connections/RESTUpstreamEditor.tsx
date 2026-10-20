import { useState } from 'react'
import RESTImportDialog from './RESTImportDialog'
import { Cable, KeyRound, Wrench } from 'lucide-react'
import { api, type RESTAuth, type RESTConfig, type RESTOperation, type Upstream } from '../api'
import { ServerPanel } from './ServerPanels'
import { FormField } from '../FormField'
import GuidanceTextField from '../GuidanceTextField'
import Select from '../Select'
import { Switch } from '../SelectionControls'
import { ErrorBox, Modal } from '../ui'
import { btn, input, primary } from '../styles'
import { discardConfirmation, useConfirm } from '../confirmation'
import { useCapabilities, useOperation, useUnsaved } from '../interaction'

const newOperation = (): RESTOperation => ({name:'',description:'',enabled:true,method:'GET',path:'/',parameters:[],readOnly:false})
const json = (value:unknown) => value === undefined ? '' : JSON.stringify(value,null,2)

export default function RESTUpstreamEditor({value,onChange,upstream}:{value:RESTConfig;onChange?:(v:RESTConfig)=>void;upstream:Upstream|null}) {
 const [selected,setSelected]=useState<number|null>(null)
 const [importing,setImporting]=useState(false)
 const set=(patch:Partial<RESTConfig>)=>onChange?.({...value,...patch})
 const auth=(patch:Partial<RESTAuth>)=>set({auth:{...value.auth,...patch}})
 const confirm=useConfirm()
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
 <ServerPanel title="REST operations" description="Each enabled operation becomes a selectable MCP tool after Save" icon={Wrench}>
 <div className="panel-body layout-stack">
 {value.operations.length===0&&<p className="text-sm text-ink-muted">No operations configured. Add a request and its response schema to publish a tool.</p>}
 {value.operations.map((op,i)=><div key={i} className="surface-inset flex flex-wrap items-center gap-3 p-3"><div className="min-w-0 flex-1"><p className="break-all text-sm font-medium">{op.name} <span className="text-xs text-ink-muted">{op.enabled?'Enabled':'Disabled'}</span></p><p className="break-all font-mono text-xs text-ink-muted">{op.method} {op.path}</p><p className="line-clamp-2 text-xs text-ink-muted">{op.description}</p></div><button className={btn} onClick={()=>setSelected(i)}>{onChange?'Edit operation':'View operation'}</button>{onChange&&<button className={`${btn} action-danger-outline`} onClick={async()=>{if(await confirm({title:'Remove operation?',description:'After Save, virtual servers selecting this tool will no longer be able to call it.',confirmLabel:'Remove operation',tone:'danger'}))set({operations:value.operations.filter((_,n)=>n!==i)})}}>Remove</button>}</div>)}
 {onChange&&<button className={`${btn} self-start`} disabled={value.operations.length>=200} onClick={()=>setImporting(true)}>Import OpenAPI / Swagger</button>}
 {onChange&&<button className={`${btn} self-start`} disabled={value.operations.length>=200} onClick={()=>setSelected(value.operations.length)}>Add operation</button>}
 <p className="text-xs text-ink-muted">Saving prepares the tool catalog without contacting the REST service. Select these tools in a virtual server as usual.</p>
 </div></ServerPanel>
 {importing&&onChange&&<RESTImportDialog existing={value.operations} onClose={()=>setImporting(false)} onApply={operations=>{set({operations:[...value.operations,...operations]});setImporting(false)}}/>}
 {selected!==null&&<OperationEditor key={selected} operation={value.operations[selected]??newOperation()} editable={!!onChange} onClose={()=>setSelected(null)} onApply={op=>{const ops=[...value.operations];ops[selected]=op;set({operations:ops});setSelected(null)}}/>}
 {upstream&&!onChange&&<RESTTest upstream={upstream}/>}
 </>
}

function OperationEditor({operation,editable,onClose,onApply}:{operation:RESTOperation;editable:boolean;onClose:()=>void;onApply:(op:RESTOperation)=>void}) {
 const [draft,setDraft]=useState(operation)
 const [parameters,setParameters]=useState(json(operation.parameters))
 const [body,setBody]=useState(json(operation.bodySchema))
 const [response,setResponse]=useState(json(operation.responseSchema))
 const [headers,setHeaders]=useState(json(operation.headers))
 const [fields,setFields]=useState((operation.responseFields??[]).join(', '))
 const [statuses,setStatuses]=useState((operation.successStatuses??[]).join(', '))
 const [error,setError]=useState('')
 useUnsaved({draft,parameters,body,response,headers,fields,statuses})
 const confirm=useConfirm()
 const dirty=json(draft)!==json(operation)||parameters!==json(operation.parameters)||body!==json(operation.bodySchema)||response!==json(operation.responseSchema)||headers!==json(operation.headers)||fields!==(operation.responseFields??[]).join(', ')||statuses!==(operation.successStatuses??[]).join(', ')
 const close=async()=>{if(!editable||!dirty||await confirm(discardConfirmation))onClose()}
 const set=(patch:Partial<RESTOperation>)=>setDraft({...draft,...patch})
 const apply=()=>{try{
 if(!/^[A-Za-z0-9_.-]{1,128}$/.test(draft.name))throw Error('Use a tool name of 1–128 letters, numbers, dots, underscores or hyphens.')
 if(!draft.path.startsWith('/')||draft.path.startsWith('//'))throw Error('Use a relative operation path beginning with one slash.')
 const parsed=JSON.parse(parameters);if(!Array.isArray(parsed))throw Error('Parameters must be a JSON array.')
 const parsedHeaders=headers.trim()?JSON.parse(headers):undefined;if(parsedHeaders&&(typeof parsedHeaders!=='object'||Array.isArray(parsedHeaders)||Object.values(parsedHeaders).some(v=>typeof v!=='string')))throw Error('Headers must be a JSON object with string values.')
 const codes=statuses.split(/[\s,]+/).filter(Boolean).map(Number);if(codes.some(n=>!Number.isInteger(n)||n<200||n>299))throw Error('Success statuses must be HTTP codes from 200 to 299.')
 onApply({...draft,parameters:parsed,bodySchema:body.trim()?JSON.parse(body):undefined,responseSchema:response.trim()?JSON.parse(response):undefined,headers:parsedHeaders,responseFields:fields.split(/[\s,]+/).filter(Boolean),successStatuses:codes})
 }catch(e){setError(e instanceof Error?e.message:'Invalid JSON')}}
 return <Modal title={editable?'Edit REST operation':'REST operation'} description="Changes stay in the upstream draft until Save." onClose={()=>void close()} size="wide" footer={<><button className={btn} onClick={()=>void close()}>{editable?'Cancel':'Close'}</button>{editable&&<button className={primary} onClick={apply}>Use operation</button>}</>}>
 <div className="layout-stack">{error&&<ErrorBox>{error}</ErrorBox>}
 <fieldset disabled={!editable} className="layout-stack min-w-0">
 <FormField name="operation.name" label="Tool name" required><input className={input} value={draft.name} onChange={e=>set({name:e.target.value})}/></FormField>
 <div className="grid gap-4 sm:grid-cols-2"><FormField name="operation.method" label="HTTP method"><Select className={input} value={draft.method} onChange={e=>set({method:e.target.value as RESTOperation['method'],readOnly:false})}>{['GET','POST','PUT','PATCH','DELETE'].map(m=><option key={m}>{m}</option>)}</Select></FormField><FormField name="operation.path" label="Operation path" required hint="Appended to the base URL; e.g. /orders/{id}"><input className={input} value={draft.path} onChange={e=>set({path:e.target.value})}/></FormField></div>
 <Switch label="Publish operation" checked={draft.enabled} onChange={e=>set({enabled:e.target.checked})}/><Switch label="Read-only operation" disabled={draft.method!=='GET'} checked={draft.readOnly} onChange={e=>set({readOnly:e.target.checked})}/>
 </fieldset>
 <GuidanceTextField name="operation.description" label="Tool description" value={draft.description} onChange={editable?v=>set({description:v}):undefined}/>
 <p className="text-xs text-ink-muted">Parameters are an array of name, in (path or query), required and schema. Example: {`[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}]`}. Use inline JSON schemas; references are not supported.</p>
 <GuidanceTextField name="operation.parameters" label="Request parameters" limit={65536} value={parameters} onChange={editable?setParameters:undefined}/>
 <GuidanceTextField name="operation.body" label="Request body schema" limit={65536} value={body} onChange={editable?setBody:undefined}/>
 <p className="text-xs text-ink-muted">A body schema adds a required “body” tool argument. GET operations cannot have a body.</p>
 <GuidanceTextField name="operation.response" label="Response schema" limit={65536} value={response} onChange={editable?setResponse:undefined}/>
 <fieldset disabled={!editable} className="layout-stack min-w-0"><FormField name="operation.fields" label="Response fields" hint="Optional comma-separated top-level fields from an object response schema."><input className={input} value={fields} onChange={e=>setFields(e.target.value)}/></FormField><FormField name="operation.statuses" label="Success statuses" hint="Optional comma-separated 2xx codes. Empty accepts all 2xx responses."><input className={input} value={statuses} onChange={e=>setStatuses(e.target.value)}/></FormField>
 <FormField name="operation.headers" label="Operation headers (JSON)" hint={`Write-only. Leave blank to preserve stored headers; {} clears them. Stored names: ${(operation.headerNames??[]).join(', ')||'none'}.`}><textarea className={`${input} font-mono`} rows={4} value={headers} onChange={e=>setHeaders(e.target.value)}/></FormField></fieldset>
 </div></Modal>
}

function RESTTest({upstream}:{upstream:Upstream}) {
 const [operation,setOperation]=useState(upstream.rest?.operations.find(o=>o.enabled)?.name??'')
 const [args,setArgs]=useState('{}'),[result,setResult]=useState(''),[error,setError]=useState('')
 const {manageTokens:admin,edit}=useCapabilities(),{busy,run}=useOperation(),confirm=useConfirm()
 const perform=(action:'preview'|'test')=>run(async()=>{setError('');setResult('');try{
 const parsed=JSON.parse(args)
 if(action==='test'&&!await confirm({title:'Execute REST operation?',description:'This sends a real request using saved credentials and may change data in the REST service. It will not be retried or impersonate an agent.',confirmLabel:'Execute request',tone:'warning'}))return
 setResult(json(await api.restOperation(upstream.slug,action,operation,parsed,upstream.version)))
 }catch(e){setError(e instanceof Error?e.message:'Request failed')}})
 return <ServerPanel title="Operation preview & test" description="Uses the saved configuration; preview never sends a request" icon={Cable}><div className="panel-body layout-stack">
 <FormField name="rest.test.operation" label="Operation"><Select className={input} value={operation} onChange={e=>{setOperation(e.target.value);setResult('')}}>{upstream.rest?.operations.filter(o=>o.enabled).map(o=><option key={o.name}>{o.name}</option>)}</Select></FormField>
 <FormField name="rest.test.arguments" label="Tool arguments (JSON)"><textarea className={`${input} font-mono`} rows={5} value={args} onChange={e=>setArgs(e.target.value)}/></FormField>
 <div className="flex flex-wrap gap-2"><button className={btn} disabled={!edit||busy||!operation} onClick={()=>void perform('preview')}>Preview request</button><button className={btn} disabled={!admin||busy||!operation||!upstream.enabled||upstream.rest?.requireIdentity} onClick={()=>void perform('test')}>Execute test request</button></div>
 <p className="text-xs text-ink-muted">Execution requires admin access and an enabled upstream. Identity-required operations must be tested through an authenticated MCP client.</p>
 {error&&<ErrorBox>{error}</ErrorBox>}{result&&<pre className="surface-inset whitespace-pre-wrap break-all p-3 text-xs" aria-label="REST operation result">{result}</pre>}
 </div></ServerPanel>
}
