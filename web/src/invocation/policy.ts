import type { InvocationRule } from '../api'

export const splitFields=(value:string)=>[...new Set(value.split(',').map(field=>field.trim()).filter(Boolean))]
export const identityLabels={oauth_subject:'OAuth user',caller_context:'Customer session'}

// Mirrors the gateway's policy composition for the unsaved preview.
export function effectivePolicy(parent:InvocationRule,child:InvocationRule={}):{rule:InvocationRule;error?:{field:string;message:string}} {
  const rule:InvocationRule={
    identity:parent.identity??child.identity,
    stepUp:parent.stepUp??child.stepUp,
    approvalServices:[...new Set([...(parent.approvalServices??[]),...(child.approvalServices??[])])],
    shareFields:parent.shareFields&&child.shareFields?parent.shareFields.filter(field=>child.shareFields!.includes(field)):parent.shareFields??child.shareFields,
  }
  if(parent.identity&&child.identity&&(
    parent.identity.source!==child.identity.source||parent.identity.service!==child.identity.service||
    JSON.stringify(parent.identity.fields??[])!==JSON.stringify(child.identity.fields??[])
  ))return {rule,error:{field:'identity',message:'A tool cannot replace its inherited identity source, service or fields.'}}
  if(parent.stepUp&&child.stepUp){
    const acceptedAcrValues=parent.stepUp.acceptedAcrValues.filter(value=>child.stepUp!.acceptedAcrValues.includes(value))
    if(!acceptedAcrValues.length)return {rule,error:{field:'acr',message:'Choose at least one ACR accepted by the upstream policy.'}}
    rule.stepUp={acceptedAcrValues,maxAgeSeconds:Math.min(parent.stepUp.maxAgeSeconds,child.stepUp.maxAgeSeconds)}
  }
  if(!rule.identity&&(rule.stepUp||rule.approvalServices?.length))return {rule,error:{field:'identity',message:'Step-up and approval require a user identity source.'}}
  return {rule}
}

export function ruleSummary(rule:InvocationRule) {
  const requirements:string[]=[]
  if(rule.identity)requirements.push(identityLabels[rule.identity.source]+(rule.identity.service?` via ${rule.identity.service}`:''))
  if(rule.identity?.fields?.length)requirements.push(`Caller context: ${rule.identity.fields.join(', ')}`)
  if(rule.stepUp)requirements.push(`Step-up: ${rule.stepUp.acceptedAcrValues.join(', ')} · max ${rule.stepUp.maxAgeSeconds}s`)
  if(rule.approvalServices?.length)requirements.push(`Approval: ${rule.approvalServices.join(', ')}`)
  if(rule.shareFields?.length||rule.approvalServices?.length||rule.identity?.service)requirements.push(rule.shareFields?.length?`Shared arguments: ${rule.shareFields.join(', ')}`:'No arguments shared')
  return requirements.join(' · ')||'No invocation requirements'
}

export function referencedServices(rules: Iterable<{identity?:{service?:string};approvalServices?:string[]}>) {
  const names=new Set<string>()
  for(const rule of rules){
    if(rule.identity?.service)names.add(rule.identity.service)
    for(const name of rule.approvalServices??[])names.add(name)
  }
  return names
}
