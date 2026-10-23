import { fmt } from '../format'
import { ActiveFilters } from '../SearchControls'
import DateTimeField from '../DateTimeField'
import { localDateTime, auditDateErrors } from '../date-time'
import { Checkbox } from '../SelectionControls'
import Select from '../Select'
import { useState } from 'react'
import { CopyButton } from '../ui'
import { input, btn } from '../styles'
import { ChevronDown, ChevronUp, Filter, SlidersHorizontal } from 'lucide-react'
import type { AuditQuery } from '../api'
import { ServerPanel } from './ServerPanels'

const labels:Partial<Record<keyof AuditQuery,string>>={activity:'Activity',virtualServer:'Virtual server',agentKey:'Agent key / oauth:binding',subject:'Subject',tool:'Tool',status:'Status',from:'From',to:'To',operationId:'Operation ID',actorId:'Actor ID',requestId:'Request ID',stage:'Stage',incomplete:'Incomplete operations only'}
const activities:Record<string,string>={virtual:'Virtual MCP calls',control:'Control plane API',system:'System activity',upstream:'Upstream calls only'}
export default function AuditFilters({filters,set,when,reset,link,zone}:{filters:AuditQuery;set:(key:keyof AuditQuery,value:string)=>void;when:(key:'from'|'to',value:string)=>void;reset:()=>void;link:string;zone:string}) {
  const [expanded,setExpanded]=useState(true)
  const [advanced,setAdvanced]=useState(!!(filters.operationId || filters.actorId || filters.requestId || filters.stage))
  const active=Object.entries(filters).filter(([key,value])=>key!=='cursor' && !!value)
  const textField=(key:keyof AuditQuery)=><label key={key} className="block min-w-0 text-xs font-medium">{labels[key]}<input className={`${input} mt-2 font-normal`} aria-label={labels[key]} placeholder={labels[key]} value={filters[key] ?? ''} onChange={e=>set(key,e.target.value.trim())}/></label>
  return <ServerPanel title="Filter events" description="Narrow stored activity by server, identity, result or time" icon={Filter} actions={<button className="inline-flex items-center gap-1 text-xs font-medium text-accent-700" aria-expanded={expanded} aria-controls="audit-filter-controls" onClick={()=>setExpanded(v=>!v)}>{expanded?<>Hide filters<ChevronUp aria-hidden="true" className="icon-inline"/></>:<>Show filters<ChevronDown aria-hidden="true" className="icon-inline"/></>}</button>} footer={<div className="flex flex-wrap items-center justify-between gap-3"><span>Times: {zone} · Filters apply after a short delay.</span><CopyButton label="Copy audit filter link" value={link}/></div>}>
    <div className="layout-stack panel-body">{expanded && <div id="audit-filter-controls" className="layout-stack"><label className="block min-w-0 text-xs font-medium">Activity<Select aria-label="Activity" className={`${input} mt-2 font-normal`} value={filters.activity??''} onChange={e=>set('activity',e.target.value)}><option value="">All activity</option>{Object.entries(activities).map(([value,label])=><option key={value} value={value}>{label}</option>)}</Select></label><p className="text-xs text-ink-muted">Virtual MCP calls include their linked upstream attempts. Control plane API and system activity can be viewed separately.</p><div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">{textField('virtualServer')}{textField('agentKey')}{textField('tool')}<label className="block text-xs font-medium">Status<Select aria-label="Status" className={`${input} mt-2 font-normal`} value={filters.status ?? ''} onChange={e=>set('status',e.target.value)}><option value="">Any status</option>{['started','ok','tool_error','error','denied'].map(status=><option key={status} value={status}>{status.replace('_',' ')}</option>)}</Select></label></div>
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">{textField('subject')}{(['from','to'] as const).map(key=><DateTimeField key={key} name={key} label={labels[key]!} value={localDateTime(filters[key])} onChange={value=>when(key,value)} emptyLabel="Clear date" hint={`Local time: ${zone}. Empty means no ${key==='from'?'start':'end'} limit.`} error={auditDateErrors(filters)[key]}/>)}</div>
      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-line pt-4"><Checkbox variant="plain" label="Incomplete operations only" checked={filters.incomplete==='true'} onChange={e=>set('incomplete',e.target.checked?'true':'')}/><button className={`${btn} ${advanced?'text-accent-700':''}`} aria-expanded={advanced} aria-controls="audit-correlation-filters" onClick={()=>setAdvanced(v=>!v)}><SlidersHorizontal aria-hidden="true" className="icon-inline"/>Correlation filters</button></div>
      {advanced && <div id="audit-correlation-filters" className="grid gap-4 surface-inset p-4 sm:grid-cols-2">{textField('operationId')}{textField('actorId')}{textField('requestId')}<label className="block text-xs font-medium">Stage<Select aria-label="Stage" className={`${input} mt-2 font-normal`} value={filters.stage ?? ''} onChange={e=>set('stage',e.target.value)}><option value="">Any stage</option>{['event','admission','outcome','mutation'].map(stage=><option key={stage} value={stage}>{stage}</option>)}</Select></label></div>}
      </div>}
      <ActiveFilters className="border-t border-line pt-4" label="Active audit filters" resetLabel="Clear all filters" items={active.map(([key,value])=>({id:key,label:labels[key as keyof AuditQuery]??key,value:key==='activity'?(activities[String(value)]??String(value)):key==='incomplete'?'yes':(key==='from'||key==='to')&&Number.isFinite(Date.parse(String(value)))?fmt(String(value)):String(value),onRemove:()=>set(key as keyof AuditQuery,'')}))} onReset={reset}/>

    </div>
  </ServerPanel>
}
