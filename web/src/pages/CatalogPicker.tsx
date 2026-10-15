import { filterItem } from '../search-filters'
import { SearchField, ActiveFilters, ResultCount } from '../SearchControls'
import Select from '../Select'
import { LoadMore } from '../DataTable'
import { FormContext } from '../form-validation'
import { FormField } from '../FormField'
import { ErrorBox } from '../ui'
import { EmptyState, LoadingState } from '../Feedback'
import { useContext, useEffect, useState } from 'react'
import { api, type Catalog, type CatalogSelection, type Upstream } from '../api'
import { BookOpen } from 'lucide-react'
import { ServerPanel } from '../connections/ServerPanels'
import { input, btn } from '../styles'

export default function CatalogPicker({upstreams, value, savedCatalog, onChange}: {upstreams:Upstream[];value:CatalogSelection[];savedCatalog?:(CatalogSelection & {descriptionDrift?:boolean;upstreamDescription?:string})[];onChange:(value:CatalogSelection[])=>void}) {
 const validation=useContext(FormContext)
 const [search,setSearch]=useState('')
 const [selectionLimit,setSelectionLimit]=useState(50)
 const [kind,setKind]=useState('')
 const [catalogs,setCatalogs]=useState<Record<string,Catalog>>({})
 const [error,setError]=useState('')
 const [loading,setLoading]=useState(false)
 const [attempt,setAttempt]=useState(0)
 useEffect(()=>{
  const controller=new AbortController()
  const timer=setTimeout(()=>{setLoading(true);setError('');setCatalogs({})},0)
  void Promise.allSettled(upstreams.map(async u=>[u.slug,await api.listCatalog(u.slug,controller.signal)] as const)).then(results=>{
   clearTimeout(timer)
   if(controller.signal.aborted)return
   const entries=results.flatMap(result=>result.status==='fulfilled'?[result.value]:[])
   setCatalogs(Object.fromEntries(entries))
   const failed=results.flatMap((result,i)=>result.status==='rejected'?[upstreams[i].slug]:[])
   setError(failed.length?`Catalogs unavailable for upstreams: ${failed.join(', ')}. Other catalogs remain available.`:'')
   setLoading(false)
  })
  return()=>{clearTimeout(timer);controller.abort()}
 },[upstreams,attempt])
 const identity=(s:CatalogSelection)=>`${s.upstream}\0${s.kind}\0${s.name}`
 const choices=upstreams.flatMap(u=>{
  const catalog=catalogs[u.slug];if(!catalog)return []
  return ([['resource',catalog.resources,'uri'],['template',catalog.resourceTemplates,'uriTemplate'],['prompt',catalog.prompts,'name']] as const).flatMap(([kind,objects,key])=>objects.map(o=>({upstream:u.slug,kind,name:String(o[key]),description:String(o.description??'')})))
 })
 const available=choices.filter(c=>!value.some(v=>identity(v)===identity(c)))
 const matching=choices.filter(c=>(!kind||c.kind===kind)&&Object.values(c).join(' ').toLowerCase().includes(search.toLowerCase())).filter(c=>!value.some(v=>identity(v)===identity(c)))
 return <ServerPanel headingLevel={3} title="Resources and prompts" description="Select objects explicitly. Tool selections do not grant resource or prompt access." icon={BookOpen} footer="Choices use stored discovery. Refresh the upstream to discover changes."><div className="space-y-5 p-5">
  {loading && <LoadingState label="Loading catalogs…" compact/>}
  {error && <ErrorBox title="Some catalogs could not be loaded" onRetry={()=>setAttempt(a=>a+1)} retryLabel="Retry catalogs" busy={loading}>{error}</ErrorBox>}
  {!loading && !error && choices.length===0 && <EmptyState title="No resources or prompts discovered." description="Refresh discovery from the upstream configuration to find available catalog objects." compact/>}
  <div className="grid gap-4 sm:grid-cols-[2fr_1fr]"><SearchField label="Search catalog" value={search} onChange={value=>setSearch(value)} placeholder="Upstream, name or description" className="block text-xs font-medium" visibleLabel/><label className="block text-xs font-medium">Catalog kind<Select className={input+' mt-2'} value={kind} onChange={e=>setKind(e.target.value)}><option value="">All kinds</option><option value="resource">Resources</option><option value="template">Templates</option><option value="prompt">Prompts</option></Select></label></div>
  <ActiveFilters label="Active catalog filters" items={[filterItem('Search',search,()=>setSearch('')),filterItem('Kind',kind,()=>setKind(''))]} onReset={()=>{setSearch('');setKind('')}}><ResultCount matched={matching.length} total={available.length} noun="available objects" busy={loading}/></ActiveFilters>
  {(search||kind) && matching.length===0 && <p role="status">No catalog matches. Try another search or refresh upstream discovery.</p>}
  <label className="mt-3 block text-sm">Add a resource, template or prompt
   <Select aria-label="Add catalog object" className={input} value="" onChange={e=>{const choice=choices.find(c=>identity(c)===e.target.value);if(choice)onChange([...value,{upstream:choice.upstream,kind:choice.kind,name:choice.name}])}}>
    <option value="">Choose a discovered object</option>
    {matching.map(c=><option key={identity(c)} value={identity(c)}>{c.upstream} · {c.kind} · {c.name}</option>)}
   </Select>
  </label>
  <div><p className="mb-3 text-xs font-semibold">Selected catalog objects ({value.length})</p><div className="space-y-3">{value.slice(0,validation?.issues.some(issue=>issue.field?.startsWith('catalog.'))?value.length:selectionLimit).map((selection,index)=>{
   const discovered=choices.find(c=>identity(c)===identity(selection))
   const saved=savedCatalog?.find(c=>identity(c)===identity(selection))
   const patch=(p:Partial<CatalogSelection>)=>onChange(value.map((s,i)=>i===index?{...s,...p}:s))
   return <div key={identity(selection)} className="rounded-xl border border-line bg-canvas p-4">
    <div className="break-all text-xs font-medium">{selection.upstream} · {selection.kind} · {selection.name}</div>
    {!discovered && catalogs[selection.upstream] && <p className="text-xs text-amber-700">Missing from upstream discovery; this selection is not exposed.</p>}
    {selection.description && discovered && <p className="text-xs text-ink-muted">Current upstream description: {discovered.description}</p>}
    {saved?.descriptionDrift && <p tabIndex={0} className="text-xs text-amber-700" data-tooltip={saved.upstreamDescription}>Upstream description changed since this override was written — save to accept.</p>}
    {selection.kind!=='template' && <FormField name={`catalog.${index}.alias`} label="Alias" className="mt-2" hint="Optional. Leave empty to keep the upstream name."><input aria-label={`Alias ${selection.name}`} className={input} value={selection.alias??''} onChange={e=>patch({alias:e.target.value})}/></FormField>}
    <FormField name={`catalog.${index}.description`} label="Description override" className="mt-2" hint="Optional. Leave empty to keep the upstream description."><input aria-label={`Description ${selection.name}`} className={input} value={selection.description??''} onChange={e=>patch({description:e.target.value})}/></FormField>
    <button className={btn+' action-danger-outline mt-2'} onClick={()=>onChange(value.filter((_,i)=>i!==index))}>Remove selection</button>
   </div>
  })}{value.length>selectionLimit&&<LoadMore shown={Math.min(selectionLimit,value.length)} total={value.length} noun="objects" label="Show 50 more selected objects" onLoad={()=>setSelectionLimit(n=>n+50)}/>}{!value.length&&<p className="rounded-lg border border-dashed border-line p-5 text-center text-xs text-ink-muted">Choose a discovered object to expose it to clients.</p>}</div></div></div>
 </ServerPanel>
}
