import { useEffect, useId, useState } from 'react'
import { Wrench } from 'lucide-react'
import type { Tool, ToolCatalogEntry, ToolSelection, Upstream } from '../api'
import { ServerPanel } from '../connections/ServerPanels'
import { SearchField } from '../SearchControls'
import Select from '../Select'
import Button from '../Button'
import { LoadMore } from '../DataTable'
import { input, btn } from '../styles'

const identity = (tool: ToolSelection) => JSON.stringify([tool.upstream,tool.tool])
export default function ToolPicker({ upstreams, catalog, shared, selected, disabled, loading, onAdd }: {
 upstreams: Upstream[]; catalog: Record<string,Tool[]>; shared: ToolCatalogEntry[];
 selected: ToolSelection[]; disabled: boolean; loading: boolean; onAdd: (tools:ToolSelection[])=>void
}) {
 const uid=useId()
 const [tab,setTab]=useState<'available'|'shared'>('available')
 const [scope,setScope]=useState(''), [query,setQuery]=useState('')
 const [opened,setOpened]=useState<Record<string,boolean>>({}), [limits,setLimits]=useState<Record<string,number>>({})
 const [groupLimit,setGroupLimit]=useState(10)
 const [expanded,setExpanded]=useState(()=>window.matchMedia('(min-width: 1280px)').matches)
 useEffect(()=>{
  const media=window.matchMedia('(min-width: 1280px)')
  const change=()=>setExpanded(media.matches)
  media.addEventListener('change',change)
  return ()=>media.removeEventListener('change',change)
 },[])
 const reset=()=>{setOpened({});setLimits({});setGroupLimit(10)}
 const owned=new Set(upstreams.map(up=>up.slug)), used=new Set(selected.map(identity))
 const groups=new Map<string,{name:string;owner?:string;tools:{name:string;description?:string}[]}>()
 if(tab==='available') for(const up of upstreams) groups.set(up.slug,{name:up.name,tools:catalog[up.slug]??[]})
 else for(const tool of shared) {
  if(owned.has(tool.upstream))continue
  const group=groups.get(tool.upstream)??{name:tool.upstream,owner:tool.ownerTeam,tools:[]}
  group.tools.push(tool);groups.set(tool.upstream,group)
 }
 const search=query.trim().toLowerCase()
 const shown=[...groups].filter(([slug])=>scope==='all'||scope===slug||(!scope&&!!search)).map(([slug,group])=>({slug,...group,tools:search?group.tools.filter(tool=>`${slug} ${group.name} ${tool.name} ${tool.description??''}`.toLowerCase().includes(search)):group.tools})).filter(group=>!search||group.tools.length>0)
 const switchTab=(next:'available'|'shared')=>{setTab(next);setScope('');reset()}
 return <details open={expanded} onToggle={event=>setExpanded(event.currentTarget.open)} className="min-w-0 self-start">
  <summary className="surface-card cursor-pointer p-4 font-medium xl:hidden">Add tools · {selected.length} selected</summary>
  <ServerPanel title="Tool picker" description="Browse stored discovery and add tools to the draft. Saving and requesting approval are separate actions." icon={Wrench}>
   <div className="panel-body space-y-3 border-b border-line">
    <div role="tablist" aria-label="Tool sources" className="flex gap-2">{(['available','shared'] as const).map((value,index)=><button key={value} type="button" role="tab" id={`${uid}-${value}`} aria-controls={`${uid}-panel`} aria-selected={tab===value} tabIndex={tab===value?0:-1} className={`${btn} aria-selected:border-accent-100 aria-selected:bg-accent-50 aria-selected:text-accent-700`} onClick={()=>switchTab(value)} onKeyDown={event=>{
     if(!['ArrowLeft','ArrowRight','Home','End'].includes(event.key))return
     event.preventDefault();const next=event.key==='Home'?'available':event.key==='End'?'shared':index===0?'shared':'available'
     switchTab(next);document.getElementById(`${uid}-${next}`)?.focus()
    }}>{value==='available'?'Available':'Shared'}</button>)}</div>
    <p className="supporting-copy text-ink-muted">{tab==='available'?'Tools from upstreams you can manage.':'Tool metadata from other teams; upstream connections remain private.'}</p>
    <SearchField visibleLabel label="Filter tools" value={query} onChange={value=>{setQuery(value);reset()}} placeholder="Search upstreams, tools or descriptions"/>
    <label className="block text-xs font-medium">Tool upstream<Select className={`${input} mt-2`} value={scope} onChange={event=>{setScope(event.target.value);reset()}}><option value="">Select an upstream</option><option value="all">All upstreams</option>{[...groups].map(([slug,group])=><option key={slug} value={slug}>{group.name} · {group.tools.length} tools</option>)}</Select></label>
    {(scope||query)&&<Button onClick={()=>{setScope('');setQuery('');reset()}}>Clear picker filters</Button>}
   </div>
   <div id={`${uid}-panel`} role="tabpanel" aria-labelledby={`${uid}-${tab}`} tabIndex={0} className="max-h-[28rem] overflow-y-auto overscroll-contain panel-body space-y-3">
    {loading&&<p role="status" className="text-sm">Loading stored tools…</p>}
    {!scope&&!search&&<p className="supporting-copy text-ink-muted">Select an upstream to browse tools, or search across this tab’s catalog. All upstreams shows collapsed groups.</p>}
    {shown.slice(0,groupLimit).map(group=>{
     const remaining=group.tools.filter(tool=>!used.has(identity({upstream:group.slug,tool:tool.name})))
     const limit=limits[group.slug]??25
     return <details key={`${tab}:${scope}:${search}:${group.slug}`} open={opened[group.slug]??(scope===group.slug||!!search)} onToggle={event=>{const open=event.currentTarget.open;setOpened(previous=>previous[group.slug]===open?previous:{...previous,[group.slug]:open})}} className="surface-inset rounded-xl">
      <summary className="cursor-pointer p-3 text-sm font-medium break-words">{group.name} · {group.tools.length} tools · {group.tools.length-remaining.length} selected</summary>
      <div className="p-3 pt-0 space-y-2">
       {group.owner&&<p className="text-xs text-ink-muted break-all">Owner: {group.owner}</p>}
       <Button disabled={disabled||!remaining.length} onClick={()=>onAdd(remaining.map(tool=>({upstream:group.slug,tool:tool.name})))}>{search?`Add ${remaining.length} matching tools`:`Add all ${remaining.length} remaining tools`}</Button>
       {group.tools.slice(0,limit).map(tool=>{
        const added=used.has(identity({upstream:group.slug,tool:tool.name}))
        return <button key={tool.name} type="button" disabled={disabled||added} onClick={()=>onAdd([{upstream:group.slug,tool:tool.name}])} className="block w-full rounded-lg p-2 text-left hover:bg-canvas disabled:opacity-50"><span className="block text-sm font-medium break-all">{tool.name}{added?' · Selected':''}</span>{' '}<span className="block text-xs text-ink-muted break-words">{tool.description}</span></button>
       })}
       {group.tools.length>limit&&<LoadMore shown={Math.min(limit,group.tools.length)} total={group.tools.length} noun="tools" label="Show 25 more tools" onLoad={()=>setLimits(previous=>({...previous,[group.slug]:limit+25}))}/>}
       {!group.tools.length&&!loading&&<p className="text-sm text-ink-muted">No stored tools. Refresh this upstream’s discovery first.</p>}
      </div>
     </details>
    })}
    {shown.length>groupLimit&&<LoadMore shown={groupLimit} total={shown.length} noun="upstreams" label="Show 10 more upstreams" onLoad={()=>setGroupLimit(n=>n+10)}/>}
    {!loading&&(scope||search)&&!shown.length&&<p className="text-sm text-ink-muted">No matching tools. Try another upstream, tab or search.</p>}
   </div>
  </ServerPanel>
 </details>
}
