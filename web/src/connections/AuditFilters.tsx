import { useState } from 'react'
import { CopyButton } from '../ui'
import { input, btn } from '../styles'
import { ChevronDown, ChevronUp, Filter, SlidersHorizontal, X } from 'lucide-react'
import type { AuditQuery } from '../api'
import { ServerPanel } from './ServerPanels'

const labels:Partial<Record<keyof AuditQuery,string>>={virtualServer:'Virtual server',agentKey:'Agent key / oauth:binding',subject:'Subject',tool:'Tool',status:'Status',from:'From',to:'To',operationId:'Operation ID',actorId:'Actor ID',requestId:'Request ID',stage:'Stage',incomplete:'Incomplete operations only'}
export default function AuditFilters({filters,set,when,reset,link,zone}:{filters:AuditQuery;set:(key:keyof AuditQuery,value:string)=>void;when:(key:'from'|'to',value:string)=>void;reset:()=>void;link:string;zone:string}) {
  const [expanded,setExpanded]=useState(true)
  const [advanced,setAdvanced]=useState(!!(filters.operationId || filters.actorId || filters.requestId || filters.stage))
  const active=Object.entries(filters).filter(([key,value])=>key!=='cursor' && !!value)
  const localTime=(value?:string)=>{
    if(!value)return ''
    const date=new Date(value)
    return Number.isFinite(date.getTime())?new Date(date.getTime()-date.getTimezoneOffset()*60000).toISOString().slice(0,16):''
  }
  const textField=(key:keyof AuditQuery)=><label key={key} className="block min-w-0 text-xs font-medium">{labels[key]}<input className={`${input} mt-2 font-normal`} aria-label={labels[key]} placeholder={labels[key]} value={filters[key] ?? ''} onChange={e=>set(key,e.target.value.trim())}/></label>
  return <ServerPanel title="Filter events" description="Narrow stored activity by server, identity, result or time" icon={Filter} actions={<button className="inline-flex items-center gap-1 text-xs font-medium text-accent-700" aria-expanded={expanded} aria-controls="audit-filter-controls" onClick={()=>setExpanded(v=>!v)}>{expanded?<>Hide filters<ChevronUp className="size-3.5"/></>:<>Show filters<ChevronDown className="size-3.5"/></>}</button>} footer={<div className="flex flex-wrap items-center justify-between gap-3"><span>Times: {zone} · Filters apply after a short delay.</span><CopyButton label="Copy audit filter link" value={link}/></div>}>
    <div className="space-y-5 p-5">{expanded && <div id="audit-filter-controls" className="space-y-5"><div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">{textField('virtualServer')}{textField('agentKey')}{textField('tool')}<label className="block text-xs font-medium">Status<select aria-label="Status" className={`${input} mt-2 font-normal`} value={filters.status ?? ''} onChange={e=>set('status',e.target.value)}><option value="">Any status</option>{['started','ok','tool_error','error','denied'].map(status=><option key={status} value={status}>{status.replace('_',' ')}</option>)}</select></label></div>
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">{textField('subject')}{(['from','to'] as const).map(key=><label key={key} className="block min-w-0 text-xs font-medium">{labels[key]}<input type="datetime-local" aria-label={labels[key]} className={`${input} mt-2 font-normal`} value={localTime(filters[key])} onChange={e=>when(key,e.target.value)}/></label>)}</div>
      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-line pt-4"><label className="inline-flex items-center gap-2 text-xs text-ink-muted"><input type="checkbox" checked={filters.incomplete==='true'} onChange={e=>set('incomplete',e.target.checked?'true':'')}/>Incomplete operations only</label><button className={`${btn} ${advanced?'text-accent-700':''}`} aria-expanded={advanced} aria-controls="audit-correlation-filters" onClick={()=>setAdvanced(v=>!v)}><SlidersHorizontal className="size-3.5"/>Correlation filters</button></div>
      {advanced && <div id="audit-correlation-filters" className="grid gap-4 rounded-lg border border-line bg-canvas/50 p-4 sm:grid-cols-2">{textField('operationId')}{textField('actorId')}{textField('requestId')}<label className="block text-xs font-medium">Stage<select aria-label="Stage" className={`${input} mt-2 font-normal`} value={filters.stage ?? ''} onChange={e=>set('stage',e.target.value)}><option value="">Any stage</option>{['event','admission','outcome','mutation'].map(stage=><option key={stage} value={stage}>{stage}</option>)}</select></label></div>}
      </div>}
      {active.length>0 && <div className="flex flex-wrap items-center gap-2 border-t border-line pt-4">{active.map(([key,value])=><button key={key} className="inline-flex max-w-full items-center gap-2 rounded-md border border-accent-500/15 bg-accent-50 px-2.5 py-1.5 text-[11px] text-accent-700" aria-label={`Remove ${labels[key as keyof AuditQuery] ?? key} filter`} onClick={()=>set(key as keyof AuditQuery,'')}><span className="min-w-0 break-all">{labels[key as keyof AuditQuery] ?? key}: {key==='incomplete'?'yes':value}</span><X className="size-3 shrink-0"/></button>)}<button className="px-2 text-xs font-medium text-ink-muted hover:text-ink" onClick={reset}>Clear all filters</button></div>}
    </div>
  </ServerPanel>
}
