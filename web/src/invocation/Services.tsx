import { useState } from 'react'
import { KeyRound, Pencil, Plus, Trash2 } from 'lucide-react'
import type { InvocationServices } from '../api'
import Button from '../Button'
import { ServerPanel } from '../connections/ServerPanels'
import { useConfirm } from '../confirmation'
import { EmptyState } from '../Feedback'
import { FormErrors, FormField, FormScope } from '../FormField'
import { requiredText, useFormValidation } from '../form-validation'
import { useDraft } from '../interaction'
import { input } from '../styles'
import { Modal } from '../ui'

const serviceName = /^[A-Za-z0-9:._/-]{1,200}$/

function httpsError(value:string, origin=false) {
  try {
    const url=new URL(value)
    if (url.protocol==='https:'&&!url.username&&!url.password&&!url.hash&&(!origin||(!url.search&&url.pathname==='/'))) return
  } catch { /* Return the field-specific message below. */ }
  return origin?'Enter an HTTPS origin without a path or query.':'Enter an HTTPS URL without embedded credentials or a fragment.'
}

export default function Services({services,sharedNames=[],used,writable,busy,scope,onChange}:{services:InvocationServices;sharedNames?:string[];used:Set<string>;writable:boolean;busy:boolean;scope:'upstream'|'shared';onChange:(services:InvocationServices)=>void}) {
  const [target,setTarget]=useState<string|null>(null)
  const confirm=useConfirm()
  return <ServerPanel title={scope==='shared'?'Shared verification & approval services':'Upstream verification & approval services'} description={scope==='shared'?'These services can be selected by any upstream.':'Only policies on this upstream can select these service definitions.'} icon={KeyRound}
    actions={writable&&<Button disabled={busy} onClick={()=>setTarget('')}><Plus className="icon-inline" aria-hidden="true"/>Add service</Button>}>
    {Object.keys(services).length>0?<ul className="divide-y divide-line">{Object.entries(services).sort(([a],[b])=>a.localeCompare(b)).map(([name,service])=><li key={name} className="flex flex-wrap items-center justify-between gap-3 panel-body">
      <div className="min-w-0"><p className="break-all text-sm font-semibold">{name}</p><p className="break-all text-xs text-ink-muted">{service.url}</p><p className="text-xs text-ink-muted">{service.token?'Token pending save':service.hasToken?'Token stored':'Token required'}</p></div>
      {writable&&<div className="flex flex-wrap items-center gap-2">
        <Button disabled={busy} aria-label={`Edit service ${name}`} onClick={()=>setTarget(name)}><Pencil className="icon-inline" aria-hidden="true"/>Edit</Button>
        <Button disabled={busy||used.has(name)} aria-label={`Remove service ${name}`} variant="danger-outline" onClick={async()=>{
          if(await confirm(`Remove ${name}? Save the configuration to apply this change.`)){
            const next={...services}
            delete next[name]
            onChange(next)
          }
        }}><Trash2 className="icon-inline" aria-hidden="true"/>Remove</Button>
        {used.has(name)&&<span className="text-xs text-ink-muted">Used by a policy</span>}
      </div>}
    </li>)}</ul>:<EmptyState icon={KeyRound} title="No services configured" description="Add a service for caller-context verification or human approval."/>}
    {target!==null&&<ServiceEditor name={target} services={services} sharedNames={sharedNames} onClose={()=>setTarget(null)} onStage={next=>{onChange(next);setTarget(null)}}/>}
  </ServerPanel>
}

function ServiceEditor({name,services,sharedNames,onClose,onStage}:{name:string;services:InvocationServices;sharedNames:string[];onClose:()=>void;onStage:(services:InvocationServices)=>void}) {
  const current=services[name]
  const [draft,setDraft,,,dirty]=useDraft({name,url:current?.url??'',origin:current?.approvalOrigin??'',token:'',ca:current?.caPem??''},()=>name)
  const validation=useFormValidation()
  const confirm=useConfirm()
  const close=async()=>{if(!dirty||await confirm('Discard changes to this service?'))onClose()}
  const tokenRequired=!(current?.hasToken||current?.token)||current?.url!==draft.url
  const stage=()=>{
    if(!validation.check({
      name:!serviceName.test(draft.name)?'Use 1 to 200 letters, numbers or : . _ / - characters.':!name&&(Object.hasOwn(services,draft.name)||sharedNames.includes(draft.name))?'This name is already in use.':undefined,
      url:httpsError(draft.url),origin:httpsError(draft.origin,true),
      token:tokenRequired?requiredText(draft.token,'Service token'):undefined,
    }))return
    onStage({...services,[draft.name]:{url:draft.url,approvalOrigin:draft.origin,token:draft.token||current?.token,hasToken:current?.hasToken,caPem:draft.ca||undefined}})
  }
  return <Modal title={name?'Edit verification service':'Add verification service'} size="wide" onClose={()=>void close()} footer={<><Button onClick={()=>void close()}>Cancel</Button><Button variant="primary" onClick={stage}>Stage service</Button></>}>
    <FormScope validation={validation}><FormErrors validation={validation}/><div className="layout-stack">
      <div className="grid gap-4 md:grid-cols-2">
        <FormField name="name" label="Service name" required><input className={input} disabled={!!name} value={draft.name} onChange={e=>setDraft({...draft,name:e.target.value})}/></FormField>
        <FormField name="token" label="Service token" required={tokenRequired} hint={tokenRequired?'Enter the write-only credential for this endpoint.':'Leave blank to retain the existing token.'}><input className={input} type="password" autoComplete="new-password" value={draft.token} onChange={e=>setDraft({...draft,token:e.target.value})}/></FormField>
        <FormField name="url" label="HTTPS decision endpoint" required><input className={input} value={draft.url} onChange={e=>setDraft({...draft,url:e.target.value})}/></FormField>
        <FormField name="origin" label="Trusted approval screen origin" required hint="Review links must use this HTTPS origin."><input className={input} value={draft.origin} onChange={e=>setDraft({...draft,origin:e.target.value})}/></FormField>
      </div>
      <details className="surface-inset p-4"><summary className="cursor-pointer text-sm font-medium">Private certificate authority</summary><FormField className="mt-4" name="ca" label="Custom CA certificate" hint="Optional PEM certificate."><textarea className={`${input} min-h-32 font-mono`} value={draft.ca} onChange={e=>setDraft({...draft,ca:e.target.value})}/></FormField></details>
    </div></FormScope>
  </Modal>
}
