import { ErrorBox } from '../ui'
import { EmptyState, LoadingState } from '../Feedback'
import { useEffect, useState } from 'react'
import { api, type Catalog, type CatalogSelection, type Upstream } from '../api'
import { BookOpen, Search } from 'lucide-react'
import { ServerPanel } from '../connections/ServerPanels'
import { input, btn } from '../styles'

export default function CatalogPicker({upstreams, value, savedCatalog, onChange}: {upstreams:Upstream[];value:CatalogSelection[];savedCatalog?:(CatalogSelection & {descriptionDrift?:boolean;upstreamDescription?:string})[];onChange:(value:CatalogSelection[])=>void}) {
 const [search,setSearch]=useState('')
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
 return <ServerPanel title="Resources and prompts" description="Select objects explicitly. Tool selections do not grant resource or prompt access." icon={BookOpen} footer="Choices use stored discovery. Refresh the upstream to discover changes."><div className="space-y-5 p-5">
  {loading && <LoadingState label="Loading catalogs…" compact/>}
  {error && <ErrorBox title="Some catalogs could not be loaded" onRetry={()=>setAttempt(a=>a+1)} retryLabel="Retry catalogs" busy={loading}>{error}</ErrorBox>}
  {!loading && !error && choices.length===0 && <EmptyState title="No resources or prompts discovered." description="Refresh discovery from the upstream configuration to find available catalog objects." compact/>}
  <div className="grid gap-4 sm:grid-cols-[2fr_1fr]"><label className="block text-xs font-medium">Search catalog<div className="relative mt-2"><Search className="absolute left-3 top-2.5 size-4 text-ink-muted"/><input aria-label="Search catalog" placeholder="Upstream, name or description" className={input+' pl-9'} value={search} onChange={e=>setSearch(e.target.value)}/></div></label><label className="block text-xs font-medium">Catalog kind<select className={input+' mt-2'} value={kind} onChange={e=>setKind(e.target.value)}><option value="">All kinds</option><option value="resource">Resources</option><option value="template">Templates</option><option value="prompt">Prompts</option></select></label></div>
  {search && !choices.some(c=>Object.values(c).join(' ').toLowerCase().includes(search.toLowerCase())) && <p role="status">No catalog matches. Try another search or refresh upstream discovery.</p>}
  <label className="mt-3 block text-sm">Add a resource, template or prompt
   <select aria-label="Add catalog object" className={input} value="" onChange={e=>{const choice=choices.find(c=>identity(c)===e.target.value);if(choice)onChange([...value,{upstream:choice.upstream,kind:choice.kind,name:choice.name}])}}>
    <option value="">Choose a discovered object</option>
    {choices.filter(c=>(!kind||c.kind===kind)&&Object.values(c).join(' ').toLowerCase().includes(search.toLowerCase())).filter(c=>!value.some(v=>identity(v)===identity(c))).map(c=><option key={identity(c)} value={identity(c)}>{c.upstream} · {c.kind} · {c.name}</option>)}
   </select>
  </label>
  <div><p className="mb-3 text-xs font-semibold">Selected catalog objects ({value.length})</p><div className="max-h-[560px] space-y-3 overflow-y-auto">{value.map((selection,index)=>{
   const discovered=choices.find(c=>identity(c)===identity(selection))
   const saved=savedCatalog?.find(c=>identity(c)===identity(selection))
   const patch=(p:Partial<CatalogSelection>)=>onChange(value.map((s,i)=>i===index?{...s,...p}:s))
   return <div key={identity(selection)} className="rounded-xl border border-line bg-canvas p-4">
    <div className="break-all text-xs font-medium">{selection.upstream} · {selection.kind} · {selection.name}</div>
    {!discovered && catalogs[selection.upstream] && <p className="text-xs text-amber-700">Missing from upstream discovery; this selection is not exposed.</p>}
    {selection.description && discovered && <p className="text-xs text-ink-muted">Current upstream description: {discovered.description}</p>}
    {saved?.descriptionDrift && <p className="text-xs text-amber-700" title={saved.upstreamDescription}>Upstream description changed since this override was written — save to accept.</p>}
    {selection.kind!=='template' && <label className="mt-2 block text-xs">Alias<input aria-label={`Alias ${selection.name}`} className={input} value={selection.alias??''} onChange={e=>patch({alias:e.target.value})}/></label>}
    <label className="mt-2 block text-xs">Description override<input aria-label={`Description ${selection.name}`} className={input} value={selection.description??''} onChange={e=>patch({description:e.target.value})}/></label>
    <button className={btn+' mt-2'} onClick={()=>onChange(value.filter((_,i)=>i!==index))}>Remove selection</button>
   </div>
  })}{!value.length&&<p className="rounded-lg border border-dashed border-line p-5 text-center text-xs text-ink-muted">Choose a discovered object to expose it to clients.</p>}</div></div></div>
 </ServerPanel>
}
