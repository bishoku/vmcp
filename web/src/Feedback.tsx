import type { ReactNode } from 'react'
import { Inbox, LoaderCircle, type LucideIcon } from 'lucide-react'
import clsx from 'clsx'

export function LoadingState({label='Loading…',detail,compact=false}:{label?:string;detail?:string;compact?:boolean}) {
  return <div role="status" data-testid="loading-state" className={clsx('flex min-w-0 items-start gap-3',compact?'panel-inset py-4':'surface-card panel-body')}>
    <span className="grid size-9 shrink-0 place-items-center rounded-xl border border-accent-100 bg-accent-50 text-accent-700"><LoaderCircle aria-hidden="true" className="icon-control motion-spin"/></span><div className="min-w-0 flex-1"><p className="text-xs font-semibold">{label}</p>{detail&&<p className="mt-1.5 text-xs leading-relaxed text-ink-muted">{detail}</p>}{!compact&&<div aria-hidden="true" className="mt-4 space-y-2 motion-pulse"><div className="h-1.5 w-2/3 rounded bg-line"/><div className="h-1.5 w-1/3 rounded bg-line"/></div>}</div>
  </div>
}

export function EmptyState({title,description,icon:Icon=Inbox,actions,compact=false}:{title:string;description?:string;icon?:LucideIcon;actions?:ReactNode;compact?:boolean}) {
  return <div data-testid="empty-state" className={clsx('flex min-w-0 flex-col items-center text-center',compact?'px-4 py-6':'panel-inset py-10 sm:py-12')}>
    <span aria-hidden="true" className="grid size-11 shrink-0 place-items-center rounded-xl border border-accent-100 bg-accent-50 text-accent-700"><Icon aria-hidden="true" className="icon-feature"/></span><h3 className="panel-title mt-4 max-w-lg break-words">{title}</h3>{description&&<p className="mt-2 max-w-md break-words text-xs leading-relaxed text-ink-muted">{description}</p>}{actions&&<div className="mt-5 flex flex-wrap items-center justify-center gap-3">{actions}</div>}
  </div>
}
