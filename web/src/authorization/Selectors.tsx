import { useEffect,useState } from 'react'
import { ApiError,api } from '../api'
import Button from '../Button'
import Select from '../Select'
import { Checkbox } from '../SelectionControls'
import { AccessError,Field,ModeSelect,TextInput } from './common'
import { scopeKinds,uuid } from './state'
import type { AccessPrincipal,AccessScope,AccessStatus,ResourceEpoch,ScopeOption } from './client'

function useOptions(kind:string,onDenied:()=>void){
 const [snapshot,setSnapshot]=useState<{kind:string;items:ScopeOption[];error:string}>({kind:'',items:[],error:''})
 const fetchable=!!kind&&kind!=='tenant'&&kind!=='unmanaged'
 useEffect(()=>{if(!fetchable)return;const ctrl=new AbortController()
 void api.accessScopes(kind,ctrl.signal).then(r=>{if(!ctrl.signal.aborted)setSnapshot({kind,items:r.items,error:''})}).catch(e=>{if(ctrl.signal.aborted)return;if(e instanceof ApiError&&(e.status===401||e.status===403)){onDenied();return};setSnapshot({kind,items:[],error:String(e)})})
 return()=>ctrl.abort()
 },[kind,fetchable,onDenied])
 return {items:snapshot.kind===kind?snapshot.items:[],error:snapshot.kind===kind?snapshot.error:'',loading:fetchable&&snapshot.kind!==kind}
}
export function PrincipalFields({value,onChange,status,onDenied}:{value:AccessPrincipal;onChange:(value:AccessPrincipal)=>void;status:AccessStatus;onDenied:()=>void}){
 const tokens=useOptions(value.kind==='control_token'?'control_token':'',onDenied)
 return <fieldset className="space-y-4"><legend className="field-label">Principal</legend>
 <ModeSelect label="Principal type" value={value.kind} options={['oidc','control_token']} onChange={kind=>onChange(kind==='oidc'?{kind:'oidc',id:'',issuer:status.trustedIssuer}:{kind:'control_token',id:''})}/>
 {value.kind==='oidc'?<div className="grid gap-4 sm:grid-cols-2"><Field label="Principal issuer" hint="Enabled access and simulation require the configured issuer exactly."><TextInput value={value.issuer??''} onChange={issuer=>onChange({...value,issuer})}/></Field><Field label="OIDC subject" hint="The provider's stable subject, not an email or group name."><TextInput value={value.id} onChange={id=>onChange({...value,id})}/></Field></div>:<Field label="Control token" hint="Select an existing token identity. No token secret is required or displayed."><Select value={value.id} disabled={tokens.loading} onChange={event=>onChange({kind:'control_token',id:event.target.value})}><option value="">Choose an active token</option>{value.id&&!tokens.items.some(t=>t.id===value.id)&&<option value={value.id}>{value.id} · unavailable or inactive</option>}{tokens.items.map(t=><option key={t.id} value={t.id}>{t.slug} · {t.id}</option>)}</Select></Field>}
 <AccessError error={tokens.error}/>
 </fieldset>
}
export function ScopeFields({value,onChange,onDenied,liveOnly=false}:{value:AccessScope;onChange:(value:AccessScope)=>void;onDenied:()=>void;liveOnly?:boolean}){
 const options=useOptions(value.kind,onDenied),[manual,setManual]=useState(false)
 const [snapshot,setSnapshot]=useState<{key:string;items:ResourceEpoch[];error:string}>({key:'',items:[],error:''}),[refresh,setRefresh]=useState(0)
 const resource=value.kind==='upstream'||value.kind==='virtual_server'
 const key=!liveOnly&&resource&&uuid(value.id??'')?`${value.kind}:${value.id}:${refresh}`:''
 const epochs=snapshot.key===key?snapshot.items:[],epochError=snapshot.key===key?snapshot.error:''
 useEffect(()=>{if(!key)return;const ctrl=new AbortController()
 void api.accessEpochs(value.kind,value.id!,ctrl.signal).then(r=>{if(!ctrl.signal.aborted)setSnapshot({key,items:r.items,error:''})}).catch(e=>{if(ctrl.signal.aborted)return;if(e instanceof ApiError&&(e.status===401||e.status===403)){onDenied();return};setSnapshot({key,items:[],error:String(e)})});return()=>ctrl.abort()
 },[key,value.kind,value.id,onDenied])
 const idChange=(id:string)=>onChange({kind:value.kind,id})
 return <fieldset className="space-y-4"><legend className="field-label">{liveOnly?'Simulation resource':'Assignment scope'}</legend>
 <ModeSelect label="Scope kind" value={value.kind} options={liveOnly?scopeKinds.filter(k=>k!=='unmanaged'):scopeKinds} onChange={kind=>{setManual(false);onChange({kind:kind as AccessScope['kind']})}}/>
 {value.kind!=='tenant'&&value.kind!=='unmanaged'&&<>
 <Field label="Resource" hint="Stable resource IDs prevent a recreated slug from inheriting access."><Select value={value.id??''} disabled={options.loading||manual} onChange={event=>idChange(event.target.value)}><option value="">Choose a resource</option>{value.id&&!options.items.some(o=>o.id===value.id)&&<option value={value.id}>{value.id} · recorded or unavailable resource</option>}{options.items.map(o=><option key={o.id} value={o.id}>{o.name} ({o.slug}) · {o.id}</option>)}</Select></Field>
 {resource&&!liveOnly&&<><Checkbox label="Use a resource UUID, including a deleted resource" checked={manual} onChange={event=>setManual(event.target.checked)}/>{manual&&<Field label="Resource UUID"><TextInput value={value.id??''} onChange={idChange}/></Field>}</>}
 </>}
 <AccessError error={options.error}/>
 {resource&&!liveOnly&&<>
 <Field label="Ownership period" hint="Audit roles pin an ownership period. Automatic on an existing assignment preserves its saved pin. Choose a current period explicitly to grant access after a transfer."><Select value={value.epoch??''} onChange={event=>{const epoch=epochs.find(e=>e.id===event.target.value);onChange({...value,epoch:event.target.value||undefined,ownerTeam:epoch?.ownerTeam})}}><option value="">Automatic (preserve saved pin when editing)</option>{value.epoch&&!epochs.some(e=>e.id===value.epoch)&&<option value={value.epoch}>{value.epoch} · saved period</option>}{epochs.map(e=><option key={e.id} value={e.id}>{e.endRevision===null?'Current':'Historical'} · revisions {e.startRevision}–{e.endRevision??'now'} · {e.id}</option>)}</Select></Field>
 <Button disabled={!uuid(value.id??'')} onClick={()=>setRefresh(v=>v+1)}>Refresh ownership periods</Button>
 <AccessError error={epochError}/>
 </>}
 {value.kind==='project'&&!liveOnly&&<Field label="Recorded project owner (optional)" hint="An omitted owner is pinned on creation and preserves the saved owner on update. Supplying another owner is an intentional historical grant validated by the server."><TextInput value={value.ownerTeam??''} onChange={ownerTeam=>onChange({...value,ownerTeam:ownerTeam||undefined})}/></Field>}
 {value.ownerTeam&&resource&&!liveOnly&&<p className="supporting-copy break-all">Recorded owner: {value.ownerTeam}</p>}
 </fieldset>
}
