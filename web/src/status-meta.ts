import { Ban, CheckCheck, CircleCheck, CircleHelp, CircleMinus, CircleX, Clock3, Database, FileClock, Globe, Info, KeyRound, LockKeyhole, ShieldAlert, TriangleAlert, Unplug, type LucideIcon } from 'lucide-react'
export type BadgeTone = 'success' | 'neutral' | 'warning' | 'danger' | 'info'
type StatusMeta = { tone: BadgeTone; icon: LucideIcon }
const success: StatusMeta = { tone: 'success', icon: CircleCheck }
const neutral: StatusMeta = { tone: 'neutral', icon: CircleMinus }
const warning: StatusMeta = { tone: 'warning', icon: TriangleAlert }
const danger: StatusMeta = { tone: 'danger', icon: CircleX }
const info: StatusMeta = { tone: 'info', icon: Info }
const statuses: Record<string, StatusMeta> = {
 active: success, enabled: success, healthy: success, ok: success, connected: success,
 disabled: neutral, inactive: neutral, 'server disabled': neutral, 'upstream disabled': neutral, 'disabled jwt': neutral,
 unknown: { tone: 'neutral', icon: CircleHelp },
 warning, degraded: warning, expired: { tone: 'warning', icon: Clock3 }, tool_error: warning,
 pending: { tone: 'info', icon: Clock3 }, started: { tone: 'info', icon: Clock3 },
 'outcome not loaded': { tone: 'warning', icon: Clock3 },
 failure: danger, error: danger, unhealthy: danger, failed: danger, 'refresh failed': danger,
 revoked: { tone: 'danger', icon: Ban }, denied: { tone: 'danger', icon: ShieldAlert },
 missing: { tone: 'warning', icon: Unplug }, unavailable: { tone: 'warning', icon: CircleHelp },
 'missing object': warning, 'missing upstream': warning, 'missing virtual server': warning, 'missing configuration': warning,
 'not loaded': { tone: 'neutral', icon: CircleHelp }, 'not refreshed': { tone: 'neutral', icon: Clock3 },
 ready: { tone: 'info', icon: Database }, discovered: { tone: 'info', icon: Database }, 'snapshot available': { tone: 'info', icon: Database },
 'public access': { tone: 'warning', icon: Globe }, anonymous: { tone: 'warning', icon: Globe },
 'anonymous access bypasses credentials': { tone: 'warning', icon: Globe },
 'conditional jwt': { tone: 'info', icon: KeyRound }, required: { tone: 'info', icon: LockKeyhole },
 admission: { tone: 'info', icon: FileClock }, outcome: { tone: 'neutral', icon: FileClock }, event: { tone: 'neutral', icon: FileClock }, mutation: { tone: 'info', icon: FileClock },
 allow: info, read_only: { tone: 'info', icon: LockKeyhole }, deny: { tone: 'danger', icon: Ban },
 'breaker closed': { tone: 'neutral', icon: CircleCheck }, 'breaker open': danger, 'breaker half_open': warning,
 create: { tone: 'info', icon: CircleCheck }, update: { tone: 'info', icon: CheckCheck }, delete: { tone: 'danger', icon: Ban }, unchanged: neutral,
}
const severity: Record<BadgeTone, number> = { success: 0, info: 1, neutral: 2, warning: 3, danger: 4 }
export function statusMeta(status?: string | null): StatusMeta {
 const normalized = status?.trim().toLowerCase() ?? ''
 if (statuses[normalized]) return statuses[normalized]
 // Relationship labels combine credential state with server or access qualifications.
 const known = normalized.split(' · ').flatMap(part => statuses[part] ? [statuses[part]] : [])
 return known.sort((a, b) => severity[b.tone] - severity[a.tone])[0] ?? { tone: 'neutral', icon: CircleHelp }
}
