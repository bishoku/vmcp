import { Boxes, FileText, KeyRound, Layers3, MessageSquare, Tag, Wrench, type LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { statusMeta, type BadgeTone } from './status-meta'

export function Badge({ children, tone = 'neutral', icon: Icon = Tag, className = '' }: { children: ReactNode; tone?: BadgeTone; icon?: LucideIcon; className?: string }) {
 return <span className={`badge ${className}`} data-tone={tone}><Icon aria-hidden="true" className="badge-icon"/><span className="badge-label">{children}</span></span>
}
export function StatusBadge({ status, label, variant = 'badge', className = '' }: { status?: string | null; label?: string; variant?: 'badge' | 'inline'; className?: string }) {
 const { tone, icon: Icon } = statusMeta(status)
 return <span className={`badge ${variant === 'inline' ? 'badge-inline' : ''} ${className}`} data-tone={tone} data-state={status ?? 'unknown'}><Icon aria-hidden="true" className="badge-icon"/><span className="badge-label">{label ?? (status?.trim() || 'unknown').replaceAll('_', ' ')}</span></span>
}
export function TagBadge({ label, className = '' }: { label: string; className?: string }) {
 const icons: Record<string, LucideIcon> = { tool: Wrench, resource: FileText, template: Boxes, prompt: MessageSquare, 'agent-key': KeyRound, binding: KeyRound, upstream: Layers3 }
 return <Badge icon={icons[label] ?? Tag} className={className}>{label}</Badge>
}
