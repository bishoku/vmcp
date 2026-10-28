import type { ToolGrant } from '../api'

export function groupGrants(items: ToolGrant[]) {
 const groups = new Map<string, ToolGrant[]>()
 for (const item of items) {
  const key = `${item.virtualServer} → ${item.upstream}`
  const group = groups.get(key) ?? []
  group.push(item)
  groups.set(key, group)
 }
 return groups
}

export const approvalLabel = (status?: string) => ({required:'Request not submitted',pending:'Pending owner approval',approved:'Approved',same_team:'Approval not required — same team',not_required:'Approval not required',rejected:'Rejected',revoked:'Revoked'}[status ?? ''] ?? 'Save to check approval')
