import { expect, test } from 'vitest'
import type { AuditEvent } from '../api'
import { auditOperationSummary, groupAuditOperations } from './audit-operations'

const event = (id: number, operationId: string, stage: AuditEvent['stage'], parentOperationId?: string): AuditEvent =>
  ({ id, operationId, stage, parentOperationId, requestId: 'shared', traceId: 'shared' }) as AuditEvent

test('parent, upstream retries and nested children form one group, unrelated operations stay separate', () => {
  const events = [event(6, 'retry-2', 'outcome', 'tool'), event(5, 'retry-1', 'outcome', 'tool'), event(4, 'nested', 'outcome', 'retry-1'), event(3, 'tool', 'outcome'), event(2, 'other', 'outcome'), event(1, 'tool', 'admission')]
  const groups = groupAuditOperations(events)
  expect(groups.map(group => group.map(e => e.id))).toEqual([[6, 5, 4, 3, 1], [2]])
  expect(auditOperationSummary(groups[0])).toMatchObject({operationId: 'tool', event: events[3], missing: false, incomplete: false})
})

test('child outcome cannot complete the parent admission', () => {
  const parent = event(1, 'tool', 'admission')
  expect(auditOperationSummary([event(2, 'child', 'outcome', 'tool'), parent])).toMatchObject({operationId: 'tool', event: parent, missing: true, incomplete: true})
})

test('children group under the known parent ID even when parent records are outside the page', () => {
  const children = [event(2, 'child-2', 'outcome', 'tool'), event(1, 'child-1', 'outcome', 'tool')]
  expect(groupAuditOperations(children)).toEqual([children])
  expect(auditOperationSummary(children)).toMatchObject({operationId: 'tool', missing: true})
  const loaded = groupAuditOperations([...children, event(3, 'tool', 'outcome')])
  expect(loaded).toHaveLength(1)
  expect(auditOperationSummary(loaded[0])).toMatchObject({event: {id: 3}, missing: false})
})

test('a missing child outcome remains visible without overriding a completed parent status', () => {
  expect(auditOperationSummary([event(2, 'tool', 'outcome'), event(1, 'child', 'admission', 'tool')])).toMatchObject({missing: false, incomplete: true})
})

test('uncorrelated legacy events are not merged by shared request or trace IDs', () => {
  const events = [event(1, '', 'event'), event(2, '', 'event')]
  expect(groupAuditOperations(events)).toEqual([[events[0]], [events[1]]])
})
