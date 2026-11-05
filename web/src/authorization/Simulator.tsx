import { useEffect,useRef,useState } from 'react'
import { ApiError,api } from '../api'
import Button from '../Button'
import { Fingerprint,ShieldCheck,ScanLine } from 'lucide-react'
import { ServerPanel } from '../connections/ServerPanels'
import Select from '../Select'
import { input } from '../styles'
import type { AccessPrincipal,AccessScope,Evaluation,EvaluationInput } from './client'
import { AccessError,Field,ModeSelect,TextInput } from './common'
import { uuid,type AccessData } from './state'
import { PrincipalFields,ScopeFields } from './Selectors'
export default function Simulator({data,onDenied}:{data:AccessData;onDenied:()=>void}){
 const [principal,setPrincipal]=useState<AccessPrincipal>({kind:'oidc',id:'',issuer:data.status.trustedIssuer})
 const [permission,setPermission]=useState(''),[target,setTarget]=useState('resource'),[scope,setScope]=useState<AccessScope>({kind:'tenant'}),[eventId,setEventId]=useState(''),[claims,setClaims]=useState('{}')
 const [result,setResult]=useState<Evaluation>(),[error,setError]=useState(''),[busy,setBusy]=useState(false),pending=useRef<AbortController|null>(null)
 // Clear a result synchronously with the edit that invalidates it.
 const invalidate=()=>{pending.current?.abort();setResult(undefined);setError('');setBusy(false)}
 useEffect(()=>()=>pending.current?.abort(),[])
 const evaluate=async()=>{
  if(busy)return
  let sample:Record<string,unknown>|undefined
  try{
   if(!principal.id||!permission)throw new Error('Select a principal and a permission.')
   if(principal.kind==='oidc'){
    if(!data.status.trustedIssuer||principal.issuer!==data.status.trustedIssuer)throw new Error('Simulation requires the configured OIDC issuer exactly.')
    const parsed:unknown=JSON.parse(claims);if(!parsed||Array.isArray(parsed)||typeof parsed!=='object')throw new Error('Sample claims must be a JSON object.')
    sample=parsed as Record<string,unknown>
   }
   if(target==='event'&&!uuid(eventId))throw new Error('Enter an audit event UUID.')
   if(target==='resource'&&scope.kind!=='tenant'&&!uuid(scope.id??''))throw new Error('Select a live resource.')
  }catch(e){setError(e instanceof Error?e.message:'Enter valid sample claims JSON.');return}
  pending.current?.abort();const ctrl=new AbortController();pending.current=ctrl;setBusy(true);setError('');setResult(undefined)
  const body:EvaluationInput={principal:principal.kind==='oidc'?{kind:'oidc',id:principal.id,issuer:principal.issuer}:{kind:'control_token',id:principal.id},permission,...(sample?{claims:sample}:{}),...(target==='event'?{auditEventId:eventId}:{resource:{kind:scope.kind as Exclude<AccessScope['kind'],'unmanaged'>,...(scope.id?{id:scope.id}:{})}})}
  try{const out=await api.evaluateAccess(body,ctrl.signal);if(!ctrl.signal.aborted)setResult(out)}catch(e){if(ctrl.signal.aborted)return;if(e instanceof ApiError&&(e.status===401||e.status===403)){onDenied();return};setError(e instanceof ApiError&&e.errors.length?e.errors.map(v=>v.message).join(' '):String(e))}finally{if(!ctrl.signal.aborted)setBusy(false)}
 }
 return <section aria-label="Permission simulation" className="space-y-5">
 <p className="supporting-copy">Policy simulation only. Identity, token audience, signatures and claim freshness are not verified. A positive result does not open an authenticated session or grant live configuration access. Sample claims stay in this form and are cleared when you leave it.</p>
 <form className="space-y-5" onSubmit={event=>{event.preventDefault();void evaluate()}}>
 <AccessError error={error}/>
 <div className="grid items-start gap-6 xl:grid-cols-2"><ServerPanel title="Sample identity" description="Typed claim samples, never an authenticated session" icon={Fingerprint}><div className="space-y-5 panel-body">
 <PrincipalFields value={principal} status={data.status} onChange={value=>{invalidate();setPrincipal(value)}} onDenied={onDenied}/>
 {principal.kind==='oidc'&&<Field label="Sample OIDC claims" hint="Use a JSON object with typed sample values. Do not paste ID tokens, access tokens or real sensitive data."><textarea autoComplete="off" spellCheck={false} className={`${input} min-h-32 font-mono`} rows={6} value={claims} onChange={event=>{invalidate();setClaims(event.target.value)}}/></Field>}
 </div></ServerPanel><ServerPanel title="Permission target" description="Current resources or immutable persisted audit context" icon={ScanLine}><div className="space-y-5 panel-body">
 <Field label="Permission to evaluate"><Select required value={permission} onChange={event=>{invalidate();setPermission(event.target.value)}}><option value="">Choose a permission</option>{data.permissions.map(p=><option key={p.id} value={p.id}>{p.id}</option>)}</Select></Field>
 <ModeSelect label="Evaluation target" value={target} options={['resource','event']} onChange={value=>{invalidate();setTarget(value)}}/>
 {target==='event'?<Field label="Audit event UUID" hint="Only persisted metadata and ownership context are read. Simulation never downloads or decrypts request/response messages."><TextInput value={eventId} onChange={value=>{invalidate();setEventId(value)}}/></Field>:<ScopeFields value={scope} onChange={value=>{invalidate();setScope(value)}} liveOnly onDenied={onDenied}/>}
 </div></ServerPanel></div>
 <Button type="submit" variant="primary" loading={busy}>Evaluate permission</Button>
 </form>
 {result&&<ServerPanel title="Simulation result" description="A stored-policy decision, not proof of live identity or claim freshness" icon={ShieldCheck}><div className="space-y-3 panel-body">
 <h3 className="panel-title">{result.decision.allowed?'Allowed by stored policy':'Denied by stored policy'}</h3><p className="supporting-copy">{result.decision.reason} · policy revision {result.authorizationRevision}</p>
 <p className="supporting-copy">Stored assignments only; platform administrator identity authority is not inferred. Authentication verified: {result.authenticationValidated?'yes':'no'} · Simulation only: {result.simulationOnly?'yes':'no'}</p>
 <p className="supporting-copy">Claim sessions: {result.claimMappings}</p>
 <div className="grid gap-4 sm:grid-cols-2"><div><h4 className="field-label">Matched mappings</h4>{result.matchedMappingIds.length?result.matchedMappingIds.map(id=><p className="mt-2 break-all text-xs" key={id}>{id}</p>):<p className="supporting-copy">None</p>}</div><div><h4 className="field-label">Candidate assignments</h4><p className="supporting-copy">Candidates still need matching scope and permission.</p>{result.candidateAssignmentIds.map(id=><p className="mt-2 break-all text-xs" key={id}>{id}</p>)}</div></div>
 </div></ServerPanel>}
 </section>
}
