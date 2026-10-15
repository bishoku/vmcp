import type { ReactNode } from 'react'
import { Search, SlidersHorizontal, X } from 'lucide-react'
import { btn, input } from '../styles'
import { ServerPanel } from './ServerPanels'

export const serverPageSize = 24

export function ServerListFilters({title,query,onQuery,searchLabel,children,filtered,onReset}:{title:string;query:string;onQuery:(value:string)=>void;searchLabel:string;children:ReactNode;filtered:boolean;onReset:()=>void}) {
  return <ServerPanel title={title} description="Search saved configuration and narrow the server inventory" icon={SlidersHorizontal}>
    <div className="flex flex-wrap items-end gap-4 p-5"><label className="min-w-0 flex-[2_1_240px] text-xs font-medium">{searchLabel}<div className="relative mt-2"><Search aria-hidden="true" className="absolute left-3 top-2.5 size-4 text-ink-muted"/><input className={`${input} pl-9`} aria-label={searchLabel} placeholder="Name, identifier or configuration…" value={query} onChange={e=>onQuery(e.target.value)}/></div></label>{children}{filtered&&<button className={btn} onClick={onReset}><X className="size-3.5"/>Clear filters</button>}</div>
  </ServerPanel>
}

export function ServerListResults({title,total,matched,page,onPage,loading,readError,empty,children}:{title:string;total:number;matched:number;page:number;onPage:(page:number)=>void;loading:boolean;readError:string;empty:string;children:ReactNode}) {
  const pages=Math.max(1,Math.ceil(matched/serverPageSize))
  return <section aria-label={title} className="space-y-4">
    <div className="flex flex-wrap items-center justify-between gap-2"><h2 className="text-sm font-semibold">{title}</h2><p className="text-xs text-ink-muted">{matched} of {total} servers{readError?' · last loaded configuration':''}</p></div>
    {matched>0?<div className="grid items-stretch gap-4 md:grid-cols-2 2xl:grid-cols-3">{children}</div>:<div className="rounded-card border border-dashed border-line bg-surface px-5 py-12 text-center"><p className="text-sm font-medium">{loading?'Loading server inventory…':readError?'Server inventory unavailable':empty}</p><p className="mt-2 text-xs text-ink-muted">{readError?'Refresh the list to try again.':total?'Try another search or clear your filters.':'Saved servers appear here once configured.'}</p></div>}
    {pages>1&&<div className="flex flex-wrap items-center justify-between gap-3 rounded-card border border-line bg-surface px-5 py-3"><p className="text-xs text-ink-muted">{page*serverPageSize+1}–{Math.min((page+1)*serverPageSize,matched)} of {matched} · Page {page+1} of {pages}</p><div className="flex gap-2"><button className={btn} disabled={page===0} onClick={()=>onPage(page-1)}>Previous</button><button className={btn} disabled={page+1>=pages} onClick={()=>onPage(page+1)}>Next</button></div></div>}
  </section>
}

export function ServerListSelect({label,value,onChange,children}:{label:string;value:string;onChange:(value:string)=>void;children:ReactNode}) {
  return <label className="min-w-0 flex-[1_1_150px] text-xs font-medium">{label}<select className={`${input} mt-2`} value={value} onChange={e=>onChange(e.target.value)}>{children}</select></label>
}
