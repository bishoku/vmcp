import { useEffect,useRef,useState } from 'react';
import { ApiError } from '../api';
import { useOperation,useUnsaved } from '../interaction';
import type { AccessScope,IdentityMapping,AccessStatus,Permission,RoleAssignment,AccessRole } from './client';
export interface AccessData {permissions:Permission[];roles:AccessRole[];mappings:IdentityMapping[];assignments:RoleAssignment[];status:AccessStatus}
export const captureScopes=['virtual','upstream','control','reads','system','omni','omni_diagnostics','omni_embedding','omni_rerank']
export const scopeKinds:AccessScope['kind'][]=['tenant','unmanaged','team','project','upstream','virtual_server']
export const uuid=(value:string)=>/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(value)

export function useEditor<T>(initial:T,denied:()=>void){
 const [draft,setDraft]=useState(initial),[error,setError]=useState(''),[stale,setStale]=useState(false)
 const {busy,run}=useOperation(),unsaved=useUnsaved(draft),pending=useRef<AbortController|null>(null),mounted=useRef(true)
 useEffect(()=>{mounted.current=true;return()=>{mounted.current=false;pending.current?.abort()}},[])
 const execute=(fn:(signal:AbortSignal)=>Promise<void>)=>{if(!mounted.current)return Promise.resolve();return run(async()=>{
  pending.current?.abort();const controller=new AbortController();pending.current=controller
  setError('')
  try{await fn(controller.signal)}catch(e){if(controller.signal.aborted)return;if(e instanceof ApiError&&(e.status===401||e.status===403)){denied();return};setStale(e instanceof ApiError&&e.status===412);setError(e instanceof ApiError&&e.errors.length?e.errors.map(v=>v.message).join(' '):e instanceof Error?e.message:String(e))}
 })}
 return {draft,setDraft,error,setError,stale,setStale,busy,execute,unsaved}
}
