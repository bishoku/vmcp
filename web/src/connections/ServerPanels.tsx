import type { ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'

export function SummaryTile({label,value,detail,icon:Icon}:{label:string;value:ReactNode;detail:string;icon:LucideIcon}) {
  return <div className="min-w-0 rounded-card border border-line bg-surface p-4 sm:p-5 shadow-card">
    <div className="flex items-center justify-between gap-3"><p className="meta-copy font-medium">{label}</p><Icon className="size-4 shrink-0 text-accent-700" aria-hidden="true"/></div>
    <div className="summary-value mt-3">{value}</div>
    <p className="supporting-copy mt-2 text-ink-muted">{detail}</p>
  </div>
}

export function ServerPanel({title,description,icon:Icon,children,footer,actions,headingLevel=2}:{title:string;description:string;icon:LucideIcon;children:ReactNode;footer?:ReactNode;actions?:ReactNode;headingLevel?:2|3}) {
  const Heading=headingLevel===2?'h2':'h3'
  return <section aria-label={title} className="min-w-0 flex flex-col overflow-hidden rounded-card border border-line bg-surface shadow-card">
    <div className="flex items-start justify-between gap-3 border-b border-line px-5 py-4"><div className="min-w-0 flex-1"><Heading className="panel-title">{title}</Heading><p className="panel-description">{description}</p></div><div className="flex shrink-0 items-center gap-3"><Icon className="size-4 text-ink-muted" aria-hidden="true"/>{actions}</div></div>
    <div className="flex-1">{children}</div>
    {footer && <div className="border-t border-line px-5 py-3 text-xs text-ink-muted">{footer}</div>}
  </section>
}

export function DetailRow({label,children}:{label:string;children:ReactNode}) {
  return <div className="flex flex-wrap items-start justify-between gap-x-5 gap-y-2 py-3.5"><dt className="meta-copy">{label}</dt><dd className="detail-value min-w-0 max-w-full break-words">{children}</dd></div>
}
