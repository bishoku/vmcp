import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowLeft, Save } from 'lucide-react'
import { api, type SharedInvocationServices } from '../api'
import Button from '../Button'
import { useConfigurationChecks } from '../configuration-access'
import { LoadingState } from '../Feedback'
import { useDraft, useOperation } from '../interaction'
import Services from '../invocation/Services'
import { ErrorBox } from '../ui'

export default function InvocationSecurity() {
  const {ready,allowed}=useConfigurationChecks([{method:'GET',path:'/invocation-security/shared-services'},{method:'PUT',path:'/invocation-security/shared-services'}])
  const [draft,setDraft,reset,open,dirty]=useDraft<SharedInvocationServices|null>(null,()=> 'shared-invocation')
  const [error,setError]=useState('')
  const {busy,run}=useOperation()
  const readable=allowed[0]===true
  const writable=allowed[1]===true
  useEffect(()=>{
    if(!readable)return
    const controller=new AbortController()
    void api.sharedInvocationServices(controller.signal).then(value=>{if(!controller.signal.aborted)reset(value)}).catch(reason=>{if(!controller.signal.aborted)setError(String(reason))})
    return ()=>controller.abort()
    // Load the initial snapshot when read permission is granted.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  },[readable])

  return <div className="page-layout">
    <Link to="/settings" className="inline-flex w-fit items-center gap-2 text-sm text-ink-muted"><ArrowLeft className="icon-inline" aria-hidden="true"/>Settings</Link>
    <header><h1 className="page-title">Shared invocation services</h1><p className="page-description mt-1">Manage trusted services available to upstreams. Configure policies and local services from <Link className="underline" to="/upstreams">Upstreams</Link>.</p></header>
    {error&&<ErrorBox>{error}<Button disabled={busy} onClick={()=>void api.sharedInvocationServices().then(value=>{open(value);setError('')}).catch(reason=>setError(String(reason)))}>Reload shared services</Button></ErrorBox>}
    {(!ready||(readable&&!draft&&!error))&&<LoadingState label="Loading shared services…"/>}
    {ready&&!readable&&<p className="surface-inset panel-body text-sm text-ink-muted">Reading shared services requires tenant invocation security permission.</p>}
    {readable&&draft&&<>
      <Services scope="shared" services={draft.services} writable={writable} busy={busy} used={new Set(draft.inUse)} onChange={services=>setDraft({...draft,services})}/>
      <div className="surface-card panel-body flex flex-wrap items-center justify-between gap-3">
        <p role="status" className="text-sm text-ink-muted">{dirty?'Unsaved changes.':'All changes saved.'}</p>
        {writable&&<Button variant="primary" disabled={!dirty} loading={busy} onClick={()=>void run(async()=>{
          setError('')
          try{reset(await api.saveSharedInvocationServices({revision:draft.revision,services:draft.services}))}catch(reason){setError(String(reason))}
        })}><Save className="icon-inline" aria-hidden="true"/>Save shared services</Button>}
      </div>
    </>}
  </div>
}
