import {useState} from 'react'
import type {RESTOperation} from '../api'
import {Modal,ErrorBox} from '../ui'
import {btn,input,primary} from '../styles'
import {useUnsaved} from '../interaction'
import {useConfirm} from '../confirmation'
import {inferSchema} from './schema-editor'
import {pathParameters} from './rest-tool-preview'

export default function QuickMockOperation({onReview,onClose}:{onReview:(operation:RESTOperation)=>void;onClose:()=>void}){
 const [name,setName]=useState(''),[method,setMethod]=useState('GET'),[path,setPath]=useState('/'),[request,setRequest]=useState(''),[response,setResponse]=useState('{\n  "ok": true\n}'),[status,setStatus]=useState(200),[inferResponse,setInferResponse]=useState(false),[error,setError]=useState('')
 const confirm=useConfirm(),dirty=!!name||method!=='GET'||path!=='/'||!!request||response!=='{\n  "ok": true\n}'||status!==200||inferResponse
 useUnsaved(dirty?{name,method,path,request,response,status,inferResponse}:null)
 const close=async()=>{if(!dirty||await confirm({title:'Discard quick mock?',description:'The examples and operation fields in this window will be discarded.',confirmLabel:'Discard changes',tone:'warning'}))onClose()}
 const review=()=>{try{
  if(new TextEncoder().encode(request+response).length>512*1024)throw Error('Use request and response examples totaling at most 512 KiB.')
  if(!/^[A-Za-z0-9_.-]{1,128}$/.test(name))throw Error('Enter a tool name using letters, numbers, dots, underscores or hyphens.')
  if(!path.startsWith('/')||/[?#]/.test(path))throw Error('Use a path starting with / without query or fragment. Add query parameters in the review window.')
  if(method==='GET'&&request.trim())throw Error('GET operations cannot send a request body. Select another method or clear the example.')
  if(!Number.isInteger(status)||status<200||status>599)throw Error('Response status must be 200–599.')
  const reply=JSON.parse(response)
  const operation:RESTOperation={name,description:'',enabled:true,method:method as RESTOperation['method'],path,parameters:pathParameters(path,[]),readOnly:method==='GET',mockRules:[{name:'Default response',priority:1,response:{status,jsonBody:reply}}]}
  if(request.trim())operation.bodySchema=inferSchema(JSON.parse(request))
  if(inferResponse)operation.responseSchema=inferSchema(reply)
  onReview(operation)
 }catch(e){setError(e instanceof SyntaxError?'Enter valid JSON examples.':(e as Error).message)}}
 return <Modal title="Quick mock operation" size="wide" onClose={()=>void close()} description="Create a reply from examples, then review the operation before applying it to the upstream draft." footer={<div className="flex flex-wrap justify-end gap-2"><button type="button" className={btn} onClick={()=>void close()}>Cancel quick mock</button><button type="button" className={primary} onClick={review}>Review operation</button></div>}><div className="layout-stack">
  {error&&<ErrorBox>{error}</ErrorBox>}
  <div className="grid gap-3 sm:grid-cols-2"><label className="text-xs">Tool name<input aria-label="Quick mock tool name" className={`${input} mt-1`} value={name} onChange={e=>setName(e.target.value)}/></label><label className="text-xs">Method<select aria-label="Quick mock method" className={`${input} mt-1`} value={method} onChange={e=>setMethod(e.target.value)}>{['GET','POST','PUT','PATCH','DELETE'].map(m=><option key={m}>{m}</option>)}</select></label></div>
  <label className="text-xs">Path<input aria-label="Quick mock path" className={`${input} mt-1 font-mono`} value={path} onChange={e=>setPath(e.target.value)}/></label>
  <label className="text-xs">Request example (optional JSON)<textarea aria-label="Quick mock request example" className={`${input} mt-1 font-mono text-xs`} rows={6} value={request} onChange={e=>setRequest(e.target.value)}/></label>
  <p className="text-xs text-ink-muted">A request example generates the MCP body schema. Nested objects and arrays are supported. Examples do not create request conditions; add conditions in Mock responses when needed.</p>
  <label className="text-xs">Response status<input aria-label="Quick mock response status" className={`${input} mt-1`} type="number" min={200} max={599} value={status} onChange={e=>setStatus(Number(e.target.value))}/></label>
  <label className="text-xs">Response example (JSON)<textarea aria-label="Quick mock response example" className={`${input} mt-1 font-mono text-xs`} rows={8} value={response} onChange={e=>setResponse(e.target.value)}/></label>
  <label className="flex items-center gap-2 text-xs"><input type="checkbox" checked={inferResponse} onChange={e=>setInferResponse(e.target.checked)}/>Infer response schema from this example</label>
  <p className="text-xs text-ink-muted">Inferred schemas need review: examples cannot establish required fields or all possible types. A strict response schema also applies to error replies.</p>
 </div></Modal>
}
