import { useEffect, useState } from 'react'
import { api, type Catalog, type CatalogSelection, type Upstream } from '../api'
import { input, btn } from '../styles'

export default function CatalogPicker({upstreams, value, onChange}: {upstreams:Upstream[];value:CatalogSelection[];onChange:(value:CatalogSelection[])=>void}) {
 const [search,setSearch]=useState('')
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
 return <section className="mt-6 rounded-lg border border-line p-4">
  <h3 className="font-semibold">Resources and prompts</h3>
  <p className="mt-1 text-xs text-ink-muted">Select objects explicitly. Selecting tools does not grant resource or prompt access. Refresh the upstream to discover changes.</p>
  {loading && <p role="status">Loading catalogs…</p>}
  {error && <div role="alert" className="text-red-700">{error} <button className={btn} onClick={()=>setAttempt(a=>a+1)}>Retry catalogs</button></div>}
  {!loading && !error && choices.length===0 && <p className="text-sm text-ink-muted">No resources or prompts discovered.</p>}
  <input aria-label="Search catalog" placeholder="Search by upstream, kind, name or description" className={input+' mt-3'} value={search} onChange={e=>setSearch(e.target.value)}/>
  {search && !choices.some(c=>Object.values(c).join(' ').toLowerCase().includes(search.toLowerCase())) && <p role="status">No catalog matches. Try another search or refresh upstream discovery.</p>}
  <label className="mt-3 block text-sm">Add a resource, template or prompt
   <select aria-label="Add catalog object" className={input} value="" onChange={e=>{const choice=choices.find(c=>identity(c)===e.target.value);if(choice)onChange([...value,{upstream:choice.upstream,kind:choice.kind,name:choice.name}])}}>
    <option value="">Choose a discovered object</option>
    {choices.filter(c=>Object.values(c).join(' ').toLowerCase().includes(search.toLowerCase())).filter(c=>!value.some(v=>identity(v)===identity(c))).map(c=><option key={identity(c)} value={identity(c)}>{c.upstream} · {c.kind} · {c.name}</option>)}
   </select>
  </label>
  <div className="mt-3 space-y-3">{value.map((selection,index)=>{
   const discovered=choices.find(c=>identity(c)===identity(selection))
   const patch=(p:Partial<CatalogSelection>)=>onChange(value.map((s,i)=>i===index?{...s,...p}:s))
   return <div key={identity(selection)} className="rounded border border-line p-3">
    <div className="break-all text-sm">{selection.upstream} · {selection.kind} · {selection.name}</div>
    {!discovered && catalogs[selection.upstream] && <p className="text-xs text-amber-700">Missing from upstream discovery; this selection is not exposed.</p>}
    {selection.description && discovered && <p className="text-xs text-ink-muted">Current upstream description: {discovered.description}</p>}
    {selection.kind!=='template' && <label className="mt-2 block text-xs">Alias<input aria-label={`Alias ${selection.name}`} className={input} value={selection.alias??''} onChange={e=>patch({alias:e.target.value})}/></label>}
    <label className="mt-2 block text-xs">Description override<input aria-label={`Description ${selection.name}`} className={input} value={selection.description??''} onChange={e=>patch({description:e.target.value})}/></label>
    <button className={btn+' mt-2'} onClick={()=>onChange(value.filter((_,i)=>i!==index))}>Remove selection</button>
   </div>
  })}</div>
 </section>
}
