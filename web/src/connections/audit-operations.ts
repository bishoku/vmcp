import type { AuditEvent } from '../api'

// Only explicit operation/parent links establish a group. Request and trace IDs
// can be shared by unrelated operations and must not be used as grouping keys.
function roots(events: AuditEvent[]) {
  const parents = new Map<string, string>()
  for (const event of events) {
    if (event.operationId && event.parentOperationId) parents.set(event.operationId, event.parentOperationId)
  }
  return (event: AuditEvent): string | undefined => {
    let id = event.operationId || event.parentOperationId || undefined
    const seen = new Set<string>()
    while (id && parents.has(id)) {
      if (seen.has(id)) return [...seen].sort()[0]
      seen.add(id)
      id = parents.get(id)
    }
    return id
  }
}

export function groupAuditOperations(events: AuditEvent[]): AuditEvent[][] {
  const root = roots(events)
  const groups = new Map<string, AuditEvent[]>()
  for (const event of events) {
    const id = root(event)
    const key = id ? `operation:${id}` : `event:${event.id}`
    const group = groups.get(key) ?? []
    group.push(event)
    groups.set(key, group)
  }
  return [...groups.values()]
}

export function auditOperationSummary(events: AuditEvent[]) {
  const operationId = roots(events)(events[0])
  const parent = operationId ? events.filter(event => event.operationId === operationId) : events
  const outcome = parent.find(event => event.stage === 'outcome')
  // A child's outcome must never stand in for an unloaded parent outcome.
  const missing = !outcome && (parent.length === 0 || parent.some(event => event.stage === 'admission'))
  const outcomes = new Set(events.filter(event => event.stage === 'outcome' && event.operationId).map(event => event.operationId))
  const incomplete = missing || events.some(event => event.stage === 'admission' && (!event.operationId || !outcomes.has(event.operationId)))
  return {
    operationId,
    event: outcome ?? parent[0] ?? events.find(event => event.stage === 'outcome') ?? events[0],
    missing,
    incomplete,
  }
}
