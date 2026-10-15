import type { ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'

export function SummaryTile({label,value,detail,icon:Icon}:{label:string;value:ReactNode;detail:string;icon:LucideIcon}) {
  return <div className="min-w-0 rounded-card border border-line bg-surface p-4 sm:p-5 shadow-card">
    <div className="flex items-center justify-between gap-3"><p className="text-xs font-medium text-ink-muted">{label}</p><Icon className="size-4 shrink-0 text-accent-700" aria-hidden="true"/></div>
    <div className="mt-4 break-words text-xl font-semibold tracking-tight tabular-nums">{value}</div>
    <p className="mt-2 text-xs text-ink-muted">{detail}</p>
  </div>
}

export function ServerPanel({title,description,icon:Icon,children,footer,actions}:{title:string;description:string;icon:LucideIcon;children:ReactNode;footer?:ReactNode;actions?:ReactNode}) {
  return <section aria-label={title} className="min-w-0 flex flex-col overflow-hidden rounded-card border border-line bg-surface shadow-card">
    <div className="flex items-center justify-between gap-3 border-b border-line px-5 py-4"><div><h3 className="text-sm font-semibold">{title}</h3><p className="mt-1 text-xs text-ink-muted">{description}</p></div><div className="flex shrink-0 items-center gap-3"><Icon className="size-4 text-ink-muted" aria-hidden="true"/>{actions}</div></div>
    <div className="flex-1">{children}</div>
    {footer && <div className="border-t border-line px-5 py-3 text-xs text-ink-muted">{footer}</div>}
  </section>
}

export function DetailRow({label,children}:{label:string;children:ReactNode}) {
  return <div className="flex flex-wrap items-start justify-between gap-x-5 gap-y-2 py-3.5"><dt className="text-xs text-ink-muted">{label}</dt><dd className="min-w-0 max-w-full break-words text-xs font-medium">{children}</dd></div>
}
