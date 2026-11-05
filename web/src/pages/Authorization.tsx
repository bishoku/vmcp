import { useCallback,useContext,useEffect,useRef,useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { ShieldCheck,RefreshCw,Plus,Users,Fingerprint,KeyRound } from 'lucide-react'
import { ApiError,api } from '../api'
import Button from '../Button'
import { RoleContext } from '../interaction'
import { LoadingState } from '../Feedback'
import { useConfirm } from '../confirmation'
import type { AccessRole,IdentityMapping,RoleAssignment } from '../authorization/client'
import { AccessError } from '../authorization/common'
import { useEditor,type AccessData } from '../authorization/state'
import RoleEditor from '../authorization/RoleEditor'
import MappingEditor from '../authorization/MappingEditor'
import AssignmentEditor from '../authorization/AssignmentEditor'
import Simulator from '../authorization/Simulator'
import MigrationPanel from '../authorization/MigrationPanel'
import Inventory,{ type AccessEditor } from '../authorization/Inventory'
import { SummaryTile } from '../connections/ServerPanels'
import { btn } from '../styles'
const sections=[['roles','Roles'],['assignments','Assignments'],['mappings','Claim mappings'],['simulate','Simulation'],['migration','Access status']] as const


export default function Authorization({active=true}:{active?:boolean}){
 const role=useContext(RoleContext),[lost,setLost]=useState(false)
 useEffect(()=>{const clear=()=>setLost(true);window.addEventListener('vmcp:session-expired',clear);return()=>window.removeEventListener('vmcp:session-expired',clear)},[])
 const denied=useCallback(()=>setLost(true),[])
 if(role!=='admin'||!active||lost)return <section className="page-layout space-y-4"><h1 className="page-title">Access management</h1><p role="alert" className="supporting-copy">{lost||!active?'Access is no longer available. Sign in again to verify your administrator permissions.':'Platform administrator access is required. No management data has been loaded.'}</p></section>
 return <Workspace onDenied={denied}/>
}
function Workspace({onDenied}:{onDenied:()=>void}){
 const [params,setParams]=useSearchParams(),section=sections.some(s=>s[0]===params.get('section'))?params.get('section')!:'roles'
 const [data,setData]=useState<AccessData>(),[loading,setLoading]=useState(true),[error,setError]=useState(''),[editor,setEditor]=useState<AccessEditor>(),[message,setMessage]=useState(''),[generation,setGeneration]=useState(0)
 const pending=useRef<AbortController|null>(null),confirm=useConfirm(),action=useEditor(null,onDenied)
 const {setStale:setActionStale,setError:setActionError}=action
 const load=useCallback(async()=>{
  pending.current?.abort();const ctrl=new AbortController();pending.current=ctrl;setLoading(true);setError('')
  try{
   const [permissions,roles,assignments,mappings,status]=await Promise.all([api.accessPermissions(ctrl.signal),api.accessRoles(ctrl.signal),api.accessAssignments(ctrl.signal),api.accessMappings(ctrl.signal),api.accessAccessStatus(ctrl.signal)])
   if(ctrl.signal.aborted)return
   if([roles.authorizationRevision,assignments.authorizationRevision,mappings.authorizationRevision].some(rev=>rev!==status.authorizationRevision))throw new Error('The access policy changed while loading. Refresh before editing.')
   setData({permissions:permissions.items,roles:roles.items,assignments:assignments.items,mappings:mappings.items,status});setActionStale(false);setActionError('');setGeneration(v=>v+1)
  }catch(e){if(ctrl.signal.aborted)return;ctrl.abort();if(e instanceof ApiError&&(e.status===401||e.status===403)){onDenied();return};setError(e instanceof Error?e.message:String(e))}finally{if(pending.current===ctrl)setLoading(false)}
 },[onDenied,setActionStale,setActionError])
 useEffect(()=>{const timer=setTimeout(()=>void load(),0);return()=>{clearTimeout(timer);pending.current?.abort()}},[load])
 const saved=()=>{setEditor(undefined);setMessage('Access configuration saved. New policy decisions use the updated revision.');void load()}
 const remove=async(kind:'role'|'assignment'|'mapping',item:AccessRole|RoleAssignment|IdentityMapping)=>{
  if(loading||error)return
  if(!await confirm({title:`Delete ${kind}?`,description:'This removes the saved access configuration. Remove assignment references before deleting roles or mappings.',confirmLabel:'Delete',tone:'danger'}))return
  void action.execute(async signal=>{if(kind==='role')await api.deleteAccessRole(item as AccessRole,signal);else if(kind==='mapping')await api.deleteAccessMapping(item as IdentityMapping,signal);else await api.deleteAccessAssignment(item as RoleAssignment,signal);if(!signal.aborted){setMessage('Access configuration deleted.');await load()}})
 }
 const busy=loading||action.busy||action.stale||!!error
 return <div className="page-layout space-y-6">
 <div className="flex flex-wrap items-start justify-between gap-4"><div><h1 className="page-title">Access management</h1><p className="page-description mt-2 max-w-2xl text-sm leading-relaxed text-ink-muted">Manage explicit roles, scoped assignments and verified OIDC claim mappings. Platform administrator access comes from deployment-configured admin groups or admin credentials; assigning a role does not grant this access.</p></div><div className="flex flex-wrap gap-2"><Button className={btn} loading={loading} disabled={action.busy||!!editor} onClick={()=>{setMessage('');void load()}}><RefreshCw aria-hidden="true" className="icon-inline"/>Refresh access</Button>{data&&['roles','assignments','mappings'].includes(section)&&<Button variant="primary" disabled={busy||!!editor} onClick={()=>setEditor(section==='roles'?{kind:'role'}:section==='assignments'?{kind:'assignment'}:{kind:'mapping'})}><Plus aria-hidden="true" className="icon-inline"/>{section==='roles'?'Create role':section==='assignments'?'Create assignment':'Create mapping'}</Button>}</div></div>
 {data&&<p className="supporting-copy">Policy revision {data.status.authorizationRevision}. Permission and scope checks always apply.</p>}
 <AccessError error={error||action.error}/>{message&&<p role="status" className="supporting-copy">{message}</p>}
 {data&&<div className="grid grid-cols-2 gap-3 xl:grid-cols-4"><SummaryTile label="Roles" value={data.roles.length} detail="Builtin and explicit custom permission sets" icon={ShieldCheck}/><SummaryTile label="Custom roles" value={data.roles.filter(r=>!r.builtin).length} detail="Independent lists with no implicit additions" icon={Users}/><SummaryTile label="Enabled assignments" value={data.assignments.filter(a=>a.enabled).length} detail={`${data.assignments.length} total scoped identity assignments`} icon={KeyRound}/><SummaryTile label="Enabled mappings" value={data.mappings.filter(m=>m.enabled).length} detail="Verified OIDC claim conditions" icon={Fingerprint}/></div>}
 <nav aria-label="Access management sections" className="flex flex-wrap gap-2 surface-card panel-inset py-3">{sections.map(([id,label])=><Button key={id} aria-current={section===id?'page':undefined} variant={section===id?'primary':'secondary'} disabled={action.busy} onClick={()=>{setMessage('');setParams({section:id})}}>{label}</Button>)}</nav>
 {loading&&!data&&<LoadingState label="Loading access policy…"/>}
 {data&&<>
 <fieldset disabled={busy} className="min-w-0 space-y-4">
 {(['roles','assignments','mappings'] as const).map(kind=>section===kind&&<Inventory key={kind} section={kind} data={data} loading={loading} error={error||action.error} onEdit={setEditor} onDelete={(kind,item)=>void remove(kind,item)}/>)}
 {section==='simulate'&&<Simulator key={generation} data={data} onDenied={onDenied}/>}
 {section==='migration'&&<MigrationPanel status={data.status}/>}
 </fieldset>
 {editor?.kind==='role'&&<RoleEditor role={editor.value} copy={editor.copy} data={data} onClose={()=>setEditor(undefined)} onSaved={saved} onDenied={onDenied}/>}
 {editor?.kind==='assignment'&&<AssignmentEditor assignment={editor.value} data={data} onClose={()=>setEditor(undefined)} onSaved={saved} onDenied={onDenied}/>}
 {editor?.kind==='mapping'&&<MappingEditor mapping={editor.value} data={data} onClose={()=>setEditor(undefined)} onSaved={saved} onDenied={onDenied}/>}
 </>}
 </div>
}
