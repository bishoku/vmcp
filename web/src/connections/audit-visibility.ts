import type { AuditEvent,AuditCapabilities } from '../api'

const metadataKeys: (keyof AuditEvent)[]=['id','eventId','tenantId','operationId','requestId','traceId','schemaVersion','actorType','actorId','issuer','action','target','configRevision','stage','time','virtualServer','agentKey','clientIp','subject','method','tool','upstream','status','reason','durationMs','redactions','conversationId','rootConversationId','turnId','conversationGroupId','correlationStatus','auditMode','policyRevision','scope','boundary','direction','parentOperationId','payloadState','payloadOriginalBytes','payloadStoredBytes','payloadRedactions']
const permissionKeys: (keyof Omit<AuditCapabilities,'authorizationRevision'>)[]=['canReadMetadata','canReadIdentities','canReadDetails','canReadPayload','canReadRequestPayload','canReadResponsePayload']

// Raw/copy views are restricted to known metadata fields as well. Payload bytes
// have a separate endpoint and must never enter an event list/detail via extras.
export function visibleAuditEvent(event:AuditEvent):AuditEvent {
 const visible=Object.fromEntries(metadataKeys.filter(k=>k in event).map(k=>[k,event[k]])) as unknown as AuditEvent
 if(event.capabilities){
  visible.capabilities=Object.fromEntries(permissionKeys.map(k=>[k,event.capabilities?.[k]===true])) as unknown as AuditCapabilities
  if(typeof event.capabilities.authorizationRevision==='number')visible.capabilities.authorizationRevision=event.capabilities.authorizationRevision
 }
 if(visible.capabilities?.canReadIdentities!==true){delete visible.actorId;delete visible.issuer;delete visible.subject;delete visible.clientIp;delete visible.agentKey}
 if(visible.capabilities?.canReadDetails!==true){delete visible.reason;delete visible.target}
 return visible
}
