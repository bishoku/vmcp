import { filterItem } from '../search-filters'
import { SearchField, ActiveFilters, ResultCount, type ActiveFilter } from '../SearchControls'
import Select from '../Select'
import { Pagination, SortControl } from '../DataTable'
import type { TableSorting } from '../table-sorting'
import type { ReactNode } from 'react'
import { SlidersHorizontal } from 'lucide-react'
import { btn, input } from '../styles'
import { EmptyState, LoadingState } from '../Feedback'
import { ServerPanel } from './ServerPanels'

export const serverPageSize = 24

export function ServerListFilters({title,query,onQuery,searchLabel,children,activeFilters,onReset}:{title:string;query:string;onQuery:(value:string)=>void;searchLabel:string;children:ReactNode;activeFilters:ActiveFilter[];onReset:()=>void}) {
  return <ServerPanel title={title} description="Search saved configuration and narrow the server inventory" icon={SlidersHorizontal}>
    <div className="flex flex-wrap items-end gap-4 p-5"><SearchField label={searchLabel} value={query} onChange={value=>onQuery(value)} placeholder="Name, identifier or configuration…" className="min-w-0 flex-[2_1_240px]" visibleLabel/>{children}</div><ActiveFilters className="border-t border-line px-5 py-3" label={`${title} active filters`} items={[filterItem('Search',query,()=>onQuery('')),...activeFilters]} onReset={onReset}/>
  </ServerPanel>
}

export function ServerListResults({title,total,matched,page,onPage,loading,readError,empty,children,onClear,emptyAction,sorting}:{title:string;sorting?:TableSorting;total:number;matched:number;page:number;onPage:(page:number)=>void;loading:boolean;readError:string;empty:string;children:ReactNode;onClear?:()=>void;emptyAction?:ReactNode}) {
  const pages=Math.max(1,Math.ceil(matched/serverPageSize))
  return <section aria-label={title} className="space-y-4">
    <div className="flex flex-wrap items-center justify-between gap-2"><h2 className="panel-title">{title}</h2><ResultCount matched={matched} total={total} noun="servers" busy={loading} note={readError?'last loaded configuration':undefined}/>{sorting&&<SortControl sorting={{...sorting,select:value=>{sorting.select(value);onPage(0)}}} label={`Sort ${title}`}/>}</div>
    {matched>0?<div className="grid items-stretch gap-4 md:grid-cols-2 2xl:grid-cols-3">{children}</div>:<div className="rounded-card border border-line bg-surface shadow-card">{loading?<LoadingState label="Loading server inventory…" detail="Reading saved server configuration."/>:<EmptyState title={readError&&!total?'Server inventory unavailable':empty} description={readError&&!total?'Use Try again above to reload saved configuration.':total?'Try another search or clear your filters.':'Saved servers appear here once configured.'} actions={total&&onClear?<button className={btn} onClick={onClear}>Reset search</button>:!readError?emptyAction:undefined}/>}</div>}

    {pages>1&&<Pagination page={page} pageSize={serverPageSize} total={matched} onPage={onPage} label={`${title} pagination`}/>}
  </section>
}

export function ServerListSelect({label,value,onChange,children}:{label:string;value:string;onChange:(value:string)=>void;children:ReactNode}) {
  return <label className="min-w-0 flex-[1_1_150px] text-xs font-medium">{label}<Select className={`${input} mt-2`} value={value} onChange={e=>onChange(e.target.value)}>{children}</Select></label>
}
