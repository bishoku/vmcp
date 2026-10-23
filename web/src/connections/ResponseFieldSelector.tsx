import {useState} from 'react'
import {btn,input} from '../styles'
import {Checkbox} from '../SelectionControls'
import {containerType,displayPath,isPrefix,objectProperties,pathKey,projectionObject,responseProjection,schemaAtPath,togglePath,type FieldPath} from './response-projection'

export default function ResponseFieldSelector({schema,fields=[],onChange}:{schema:unknown;fields?:FieldPath[];onChange:(fields:FieldPath[])=>void}) {
 const [query,setQuery]=useState('')
 const available=objectProperties(schema)??{}
 const filtering=fields.length>0
 const error=responseProjection(schema,[],fields).error
 const roots=Object.keys(available).map(name=>[name])
 const missing=fields.filter(path=>schemaAtPath(schema,path)===undefined)
 return <section aria-label="Fields returned to the agent" className="layout-stack border-t border-line pt-4">
  <div><h3 className="text-sm font-semibold">Fields returned to the agent</h3><p className="mt-1 text-xs text-ink-muted">Select whole branches or individual nested fields. Array selections apply to every item and preserve its position. Narrowed branches omit unselected and undeclared fields.</p></div>
  <div className="flex flex-wrap gap-2" role="group" aria-label="Response field mode"><button type="button" className={btn} aria-pressed={!filtering} onClick={()=>onChange([])}>Full response</button><button type="button" className={btn} aria-pressed={filtering} disabled={!roots.length} onClick={()=>{if(!filtering)onChange(roots.length>256?roots.slice(0,1):roots)}}>Selected fields only</button></div>
  {roots.length>256&&!filtering&&<p className="text-xs text-ink-muted">This schema has more than 256 root fields. Selected fields mode starts with the first field; add up to 256 paths.</p>}
  {!filtering&&<p className="text-xs text-ink-muted">The full response is returned, including fields not declared in the schema.</p>}
  {!roots.length&&<p className="text-xs text-ink-muted">Define an object response schema or an array with object items to select fields. Complex branches can be selected whole from a parent object.</p>}
  {filtering&&<><p className="text-xs text-ink-muted">{fields.length} selected paths / 256 · Keep at least one path or explicitly choose Full response. Selecting a parent includes its entire contents; unchecking a child narrows that branch to declared fields.</p>
   <label className="text-xs">Find a response field<input aria-label="Find response fields" className={`${input} mt-2`} value={query} placeholder="Name or nested path" onChange={e=>setQuery(e.target.value.toLowerCase())}/></label>
   {missing.map(path=><div key={pathKey(path)} className="surface-inset p-3 text-xs"><p>Missing from schema: {displayPath(schema,path)}</p><button className={btn} disabled={fields.length===1} onClick={()=>onChange(fields.filter(p=>pathKey(p)!==pathKey(path)))}>Remove missing selection {displayPath(schema,path)}</button></div>)}
   <SelectionRows schema={schema} value={schema} prefix={[]} fields={fields} onChange={onChange} query={query}/>
  </>}
  {error&&<p role="alert" className="text-xs text-red-700">{error}</p>}
 </section>
}
function SelectionRows({schema,value,prefix,fields,onChange,query,depth=0}:{schema:unknown;value:unknown;prefix:FieldPath;fields:FieldPath[];onChange:(paths:FieldPath[])=>void;query:string;depth?:number}) {
 const properties=objectProperties(value)
 if(!properties||depth>32)return null
 return <div className="layout-stack">{Object.entries(properties).filter(([name,child])=>!query||`${displayPath(schema,[...prefix,name])} ${JSON.stringify(child)}`.toLowerCase().includes(query)).map(([name,child])=>{
  const path=[...prefix,name],label=displayPath(schema,path)
  const checked=fields.some(p=>isPrefix(p,path))
  const partial=!checked&&fields.some(p=>isPrefix(path,p))
  const next=togglePath(schema,fields,path,!checked)
  const limit=next.length>256||next.some(p=>p.length>32)
  const blocked=(checked&&!next.length)||limit
  const children=objectProperties(child)
  return <div key={name} className="surface-inset p-3 layout-stack min-w-0">
   <Checkbox variant="row" label={name} aria-label={`Return field ${label}`} checked={checked} indeterminate={partial} disabled={blocked} description={`${label} · ${containerType(child)||'schema'}${partial?' · partial selection':''}`} onChange={e=>onChange(togglePath(schema,fields,path,e.target.checked))}/>
   {limit&&<p className="text-xs text-ink-muted">This change exceeds the limit of 256 paths or 32 property levels. Select a whole branch or reduce other selections first.</p>}
   {typeof projectionObject(child).description==='string'&&<p className="text-xs text-ink-muted">{String(projectionObject(child).description)}</p>}
   {children&&Object.keys(children).length>0&&<SelectionBranch schema={schema} value={child} prefix={path} fields={fields} onChange={onChange} query={query} depth={depth+1}/>}
  </div>
 })}</div>
}
function SelectionBranch(props:Parameters<typeof SelectionRows>[0]) {
 const [open,setOpen]=useState(false)
 return <details open={!!props.query||open} onToggle={e=>setOpen(e.currentTarget.open)} className="min-w-0 border-l border-line pl-3"><summary className="cursor-pointer py-1 text-xs font-medium">Choose fields inside {displayPath(props.schema,props.prefix)}{containerType(props.value)==='array'?' · every array item':''}</summary>{(open||props.query)&&<SelectionRows {...props}/>}</details>
}
