import type { ReactNode } from 'react'

/** Native disclosure keeps draft controls mounted and supports keyboard navigation. */
export default function ConfigurationGroup({title,description,children,defaultOpen=false}:{title:string;description:string;children:ReactNode;defaultOpen?:boolean}) {
 return <details className="min-w-0 surface-card group" open={defaultOpen||undefined} onInvalidCapture={event=>{event.currentTarget.open=true}}>
  <summary className="cursor-pointer panel-header marker:text-accent-700"><span className="ml-2 font-semibold text-sm">{title}</span><span className="mt-1 block text-xs text-ink-muted">{description}</span></summary>
  <div className="layout-stack border-t border-line panel-body">{children}</div>
 </details>
}
