import { useEffect, useState } from 'react'
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
import { ErrorBox } from '../ui'
import { btn, input, primary } from '../styles'
import { useConfirm } from '../confirmation'
import { useCapabilities, useOperation } from '../interaction'

const newOperation = (): RESTOperation => ({name:'',description:'',enabled:true,method:'GET',path:'/',parameters:[],readOnly:false})
const json = (value:unknown) => value === undefined ? '' : JSON.stringify(value,null,2)

export default function RESTUpstreamEditor({value,onChange,upstream,onInvalidChange}:{value:RESTConfig;onChange?:(v:RESTConfig)=>void;upstream:Upstream|null;onInvalidChange?:(invalid:boolean)=>void}) {
 const [selected,setSelected]=useState(0)
 const [importing,setImporting]=useState(false)
 const [query,setQuery]=useState('')
 const [invalid,setInvalid]=useState(false)
 const [editorRevision,setEditorRevision]=useState(0)
 const [mobilePanel,setMobilePanel]=useState<'editor'|'preview'>('editor')
 const [importPreview,setImportPreview]=useState<RESTOperation>()
 const confirm=useConfirm()
 useEffect(()=>{onInvalidChange?.(invalid||importing);return()=>onInvalidChange?.(false)},[invalid,importing,onInvalidChange])
 const navigate=async(action:()=>void)=>{if(!invalid||await confirm({title:'Discard invalid field changes?',description:'Valid changes stay in the upstream draft. Unfinished JSON or status edits for this operation will be discarded.',confirmLabel:'Discard invalid changes',tone:'warning'})){if(invalid)setEditorRevision(revision=>revision+1);setInvalid(false);setMobilePanel('editor');action()}}
 const operation=value.operations[selected]
 return <section aria-label="REST to MCP workspace" className="layout-stack min-w-0">
   <div className="flex flex-wrap items-start justify-between gap-4">
     <div><h2 className="section-title">REST → MCP workspace</h2><p className="section-description mt-1 text-sm text-ink-muted">Import an API or build a request. See its MCP tool definition as you edit.</p></div>
     {onChange&&<div className="flex flex-wrap gap-2">
       <button className={primary} disabled={value.operations.length>=200||importing} onClick={()=>void navigate(()=>{setImportPreview(undefined);setImporting(true)})}>Import OpenAPI / Swagger</button>
       <button className={btn} disabled={value.operations.length>=200||importing} onClick={()=>void navigate(()=>{onChange({...value,operations:[...value.operations,newOperation()]});setSelected(value.operations.length);setQuery('')})}>Add operation</button>
     </div>}
   </div>
   <p role="status" className="text-xs text-ink-muted">{upstream?'Saved catalog':'Upstream draft'} · {value.operations.filter(op=>op.enabled).length} enabled tools · {onChange?'Save validates and updates the catalog.':'Select tools in a virtual server to expose them to agents.'}</p>
   <div role="group" aria-label="Workspace view" className="flex gap-2 xl:hidden">
     <button className={btn} aria-pressed={mobilePanel==='editor'} onClick={()=>setMobilePanel('editor')}>Editor</button>
     <button className={btn} aria-pressed={mobilePanel==='preview'} onClick={()=>setMobilePanel('preview')}>MCP preview</button>
   </div>
   <div className="grid items-start gap-4 xl:grid-cols-[minmax(11rem,0.65fr)_minmax(0,1.6fr)_minmax(0,1fr)]">
     <nav aria-label="REST operations" className="surface-card min-w-0 p-3 layout-stack">
       <FormField name="rest.search" label="Find an operation"><input className={input} placeholder="Name, method or path" value={query} onChange={event=>setQuery(event.target.value)}/></FormField>
       <div className="max-h-72 overflow-auto xl:max-h-[42rem] layout-stack">
         {value.operations.map((op,i)=>({op,i})).filter(({op})=>`${op.name} ${op.method} ${op.path}`.toLowerCase().includes(query.toLowerCase())).map(({op,i})=><button key={i} aria-label={`${op.name||'Untitled tool'} ${op.method} ${op.path}`} className={`rounded-lg border p-3 text-left ${i===selected&&!importing?'border-accent-500 bg-accent-50':'border-line hover:bg-canvas'}`} aria-current={i===selected&&!importing?'true':undefined} disabled={importing} onClick={()=>void navigate(()=>setSelected(i))}>
           <span className="block break-all text-sm font-medium">{op.name||'Untitled tool'}</span><span className="mt-1 block break-all font-mono text-xs text-ink-muted">{op.method} {op.path}</span><span className="mt-1 block text-xs text-ink-muted">{op.enabled?'Included in catalog':'Disabled'}</span>
         </button>)}
         {value.operations.length===0&&<p className="text-xs text-ink-muted">Start with an OpenAPI / Swagger file, or add a request manually.</p>}
         {!!value.operations.length&&!value.operations.some(op=>`${op.name} ${op.method} ${op.path}`.toLowerCase().includes(query.toLowerCase()))&&<p className="text-xs text-ink-muted">No operations match your search.</p>}
       </div>
     </nav>
     <div className={`layout-stack min-w-0 ${mobilePanel==='editor'?'':'hidden xl:flex'}`}>
       {importing&&onChange?<RESTImportDialog inline existing={value.operations} onPreview={setImportPreview} onClose={()=>setImporting(false)} onApply={operations=>{onChange({...value,operations:[...value.operations,...operations]});setSelected(value.operations.length);setQuery('');setImporting(false)}}/>:operation?<>
         <RESTOperationEditor key={`${selected}-${editorRevision}`} operation={operation} index={selected} names={value.operations.map(op=>op.name)} onInvalidChange={setInvalid} onChange={onChange?op=>onChange({...value,operations:value.operations.map((old,i)=>i===selected?op:old)}):undefined}/>
         {onChange&&<button className={`${btn} action-danger-outline self-start`} onClick={async()=>{if(await confirm({title:'Remove operation?',description:'After Save, virtual servers selecting this tool will no longer be able to call it.',confirmLabel:'Remove operation',tone:'danger'})){onChange({...value,operations:value.operations.filter((_,i)=>i!==selected)});setSelected(0);setEditorRevision(revision=>revision+1);setInvalid(false)}}}>Remove operation</button>}
       </>:<div className="surface-card p-6 layout-stack"><h3 className="text-base font-semibold">Turn your first request into a tool</h3><p className="text-sm text-ink-muted">Import OpenAPI to select existing endpoints, or add an operation and enter its method and path. Path parameters are created automatically.</p></div>}
     </div>
     <div className={`min-w-0 xl:sticky xl:top-24 ${mobilePanel==='preview'?'':'hidden xl:block'}`}><RESTToolPreview operation={importing?importPreview:operation} invalid={invalid}/></div>
   </div>
   {upstream&&<RESTTest key={upstream.version} upstream={upstream} onUseResponse={onChange&&!invalid&&!importing?(name,result)=>{
     const position=value.operations.findIndex(op=>op.name===name)
     if(position<0)return
     onChange({...value,operations:value.operations.map((op,i)=>i===position?{...op,responseSchema:mergeExample(op.responseSchema,inferSchema(result))}:op)})
     setSelected(position);setEditorRevision(v=>v+1);setMobilePanel('editor')
   }:undefined} draftNames={value.operations.map(op=>op.name)}/>}
   {!upstream&&<p className="text-xs text-ink-muted">Save the upstream to execute a test request and generate a response schema from its result.</p>}
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
