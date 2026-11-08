import { useState } from 'react'
import type { InvocationRule, UpstreamInvocationSecurity } from '../api'
import Button from '../Button'
import { useConfirm } from '../confirmation'
import { FormErrors, FormField, FormGroup, FormScope } from '../FormField'
import { useFormValidation } from '../form-validation'
import { useDraft } from '../interaction'
import { Checkbox } from '../SelectionControls'
import Select from '../Select'
import { input } from '../styles'
import { Modal } from '../ui'
import { effectivePolicy, ruleSummary, splitFields, retrySummary } from './policy'

const validAcr=/^[A-Za-z0-9:._/-]{1,200}$/
const toggle=(values:string[],value:string,checked:boolean)=>checked?[...values,value]:values.filter(item=>item!==value)

export default function RuleEditor({doc,tool,onClose,onStage}:{doc:UpstreamInvocationSecurity;tool:string|null;onClose:()=>void;onStage:(rule:InvocationRule)=>void}) {
  const current=tool?doc.policy.tools?.[tool]:doc.policy
  const [draft,setDraft,,,dirty]=useDraft({
    retry:current?.retry??('' as ''|'never'|'once'),readOnly:current?.readOnly===undefined?'':String(current.readOnly),
    identity:current?.identity?.source??('' as ''|NonNullable<InvocationRule['identity']>['source']),service:current?.identity?.service??'',context:current?.identity?.fields??[],
    acr:current?.stepUp?.acceptedAcrValues.join(', ')??'',age:String(current?.stepUp?.maxAgeSeconds??300),
    approvals:current?.approvalServices??[],share:current?.shareFields?.join(', ')??'',
  },()=>tool)
  const [showPreview,setShowPreview]=useState(false)
  const validation=useFormValidation()
  const confirm=useConfirm()
  const close=async()=>{
    if(dirty&&!await confirm('Discard changes to this policy?'))return false
    onClose()
    return true
  }
  const openContext=async()=>{
    if(!await close())return
    const group=document.getElementById('upstream-call-context')?.closest('details')
    if(group){
      group.open=true
      requestAnimationFrame(()=>{group.scrollIntoView({block:'center'});group.querySelector('summary')?.focus({preventScroll:true})})
    }
  }
  const names=[...Object.keys(doc.policy.services??{}).sort(),...doc.sharedServices]
  const inherited=tool?doc.policy.identity:undefined
  const identityLocked=!!inherited&&!draft.identity
  const identity:InvocationRule['identity']=draft.identity==='caller_context'?{source:draft.identity,service:draft.service,fields:draft.context}:draft.identity?{source:draft.identity}:undefined
  const acr=splitFields(draft.acr)
  const shareFields=splitFields(draft.share)
  const rule:InvocationRule={
    retry:draft.retry||undefined,readOnly:draft.readOnly===''?undefined:draft.readOnly==='true',
    identity,stepUp:acr.length?{acceptedAcrValues:acr,maxAgeSeconds:Number(draft.age)}:undefined,
    approvalServices:draft.approvals.length?draft.approvals:undefined,
    shareFields:shareFields.length?shareFields:undefined,
  }
  const preview=tool?effectivePolicy(doc.policy,rule):effectivePolicy(rule)
  const blockedTools=tool?[]:Object.entries(doc.policy.tools??{}).flatMap(([name,child])=>{
    const {error}=effectivePolicy(rule,child)
    return error?[{name,...error}]:[]
  })
  const stage=()=>{
    const unknownContext=draft.context.some(field=>!doc.callerContextFields.includes(field))
    const [conflict]=blockedTools
    const conflictMessage=conflict?`${conflict.name}: ${conflict.message}`:undefined
    const mergeError=(field:string)=>preview.error?.field===field?preview.error.message:conflict?.field===field?conflictMessage:undefined
    if(!validation.check({
      identity:mergeError('identity'),
      service:draft.identity==='caller_context'&&!names.includes(draft.service)?'Choose a configured session verification service.':undefined,
      context:draft.identity==='caller_context'&&(!draft.context.length||unknownContext||draft.context.length>16)?'Select 1 to 16 configured caller context fields.':undefined,
      acr:acr.length>16||acr.some(value=>!validAcr.test(value))?'Enter up to 16 valid ACR values.':mergeError('acr'),
      age:acr.length&&(!Number.isInteger(Number(draft.age))||Number(draft.age)<1||Number(draft.age)>86400)?'Use a whole number from 1 to 86,400.':undefined,
      approvals:draft.approvals.length>8||draft.approvals.some(name=>!names.includes(name))?'Select up to eight configured approval services.':undefined,
      share:shareFields.length>64||shareFields.some(field=>field.length>200)?'Use up to 64 argument names, each at most 200 characters.':undefined,
    }))return
    onStage(rule)
  }
  return <Modal title={tool?`Invocation policy for ${tool}`:'Upstream default invocation policy'} description={tool?'Review inherited requirements and tool-specific execution settings.':'These requirements apply to operations routed to this upstream.'} size="wide" onClose={()=>void close()} footer={<><Button onClick={()=>void close()}>Cancel</Button><Button variant="primary" onClick={stage}>Stage policy</Button></>}>
    <FormScope validation={validation}><FormErrors validation={validation}/><div className="layout-stack">
      {tool&&<p className="surface-inset p-3 text-xs text-ink-muted">Upstream default · {ruleSummary(doc.policy)}</p>}
      <FormField name="retry" label="Automatic retry" hint="A lost response may hide a completed operation. Enable only when repeating the operation is safe. REST and approval-bound calls are never automatically retried.">
        <Select className={input} value={draft.retry} onChange={event=>setDraft({...draft,retry:event.target.value as typeof draft.retry})}>
          <option value="">{tool?'Use upstream default':'Off (default)'}</option>
          <option value="never">Off</option><option value="once">Allow one retry after a transport failure</option>
        </Select>
      </FormField>
      <FormField name="readOnly" label="Read-only classification" hint="Independent of retry. This operator decision controls read-only tool filters; upstream hints alone do not grant access. Restrict upstream credentials accordingly.">
        <Select className={input} value={draft.readOnly} onChange={event=>setDraft({...draft,readOnly:event.target.value})}>
          <option value="">{tool?'Use upstream default':'Not classified (default)'}</option>
          <option value="false">Not read-only</option><option value="true">Classify as read-only</option>
        </Select>
      </FormField>
      <FormField name="identity" label="User identity source" hint={inherited?`Inherits the upstream’s ${inherited.source==='caller_context'?'customer session':'OAuth user'} identity.`:undefined}>
        <Select className={input} value={draft.identity} disabled={identityLocked} onChange={event=>setDraft({...draft,identity:event.target.value as typeof draft.identity})}>
          <option value="">{tool?'Use upstream identity':'Select an identity source'}</option>
          <option value="oauth_subject" disabled={inherited?.source==='caller_context'}>OAuth subject is the user</option>
          <option value="caller_context" disabled={inherited?.source==='oauth_subject'}>Application identity + caller context user</option>
        </Select>
      </FormField>
      {draft.identity==='caller_context'&&<>
        <FormField name="service" label="Session verification service" required>
          <Select className={input} value={draft.service} onChange={event=>setDraft({...draft,service:event.target.value})}>
            <option value="">Select service</option>{names.map(name=><option key={name} value={name}>{name} · {doc.sharedServices.includes(name)?'Shared':'Upstream'}</option>)}
          </Select>
        </FormField>
        <FormGroup name="context" label="Caller context fields">
          <fieldset className="space-y-2"><legend className="field-label">Caller context fields sent for verification</legend>
            {doc.callerContextFields.length===0&&<p className="text-sm text-ink-muted">Enable caller metadata and select fields in <button type="button" className="underline" onClick={()=>void openContext()}>Per-call context</button>, then reload this configuration.</p>}
            {[...new Set([...doc.callerContextFields,...draft.context])].map(field=><Checkbox key={field} variant="plain" label={field} checked={draft.context.includes(field)} onChange={event=>setDraft({...draft,context:toggle(draft.context,field,event.target.checked)})}/>)}
          </fieldset>
        </FormGroup>
      </>}
      <div className="grid gap-4 md:grid-cols-2">
        <FormField name="acr" label="Accepted ACR values" hint={tool?'Comma-separated. Leave empty to keep the upstream requirement.':'Comma-separated. Leave empty to skip step-up.'}><input className={input} value={draft.acr} onChange={event=>setDraft({...draft,acr:event.target.value})}/></FormField>
        <FormField name="age" label="Maximum authentication age (seconds)"><input className={input} type="number" min={1} max={86400} disabled={!acr.length} value={draft.age} onChange={event=>setDraft({...draft,age:event.target.value})}/></FormField>
      </div>
      <FormGroup name="approvals" label="Approval services">
        <fieldset className="space-y-2"><legend className="field-label">{tool?'Additional approval services':'Required approval services'}</legend>
          {names.length?names.map(name=>{
            const required=tool?doc.policy.approvalServices?.includes(name):false
            return <Checkbox key={name} variant="plain" label={name} description={required?'Required by upstream':doc.sharedServices.includes(name)?'Shared service':'Upstream service'} disabled={required&&!draft.approvals.includes(name)} checked={required||draft.approvals.includes(name)} onChange={event=>setDraft({...draft,approvals:toggle(draft.approvals,name,event.target.checked)})}/>
          }):<p className="text-sm text-ink-muted">Add or stage a verification service first.</p>}
        </fieldset>
      </FormGroup>
      <FormField name="share" label="Argument fields shared with verification and approval services" hint={tool?'Comma-separated top-level names. Empty inherits the upstream list. When both lists exist, only fields in both lists are shared.':'Comma-separated top-level names. Empty shares no arguments by default.'}>
        <input className={input} value={draft.share} onChange={event=>setDraft({...draft,share:event.target.value})}/>
      </FormField>
      <div className="surface-inset space-y-2 p-4">
        <p className="text-sm font-semibold">{tool?'Effective tool policy':'Effective upstream default'}</p>
        <p className="text-xs text-ink-muted">Automatic retry: {retrySummary(preview.rule)}. Read-only classification: {preview.rule.readOnly?'yes':'no'}.</p>
        <p className="break-words text-xs text-ink-muted">{preview.error?.message??ruleSummary(preview.rule)}</p>
        {!!blockedTools.length&&<p className="text-xs text-red-700">{blockedTools.length} tool {blockedTools.length===1?'rule conflicts':'rules conflict'} with this default.</p>}
        {!!blockedTools.length&&<Button onClick={()=>setShowPreview(!showPreview)} aria-expanded={showPreview}>{showPreview?'Hide conflicts':'Show conflicts'}</Button>}
        {showPreview&&<ul className="space-y-1 text-xs">{blockedTools.map(conflict=><li key={conflict.name}>{conflict.name} · {conflict.message}</li>)}</ul>}
      </div>
    </div></FormScope>
  </Modal>
}
