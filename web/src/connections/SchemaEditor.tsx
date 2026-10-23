import { useEffect, useRef, useState, type ReactNode } from 'react'
import { btn, input, primary } from '../styles'
import { Modal, ErrorBox } from '../ui'
import { useUnsaved } from '../interaction'
import { useConfirm, discardConfirmation } from '../confirmation'
import { inferSchema, mergeExample, objectSchema, type Schema } from './schema-editor'
import { checkSchema, schemaBytes, schemaLimitError } from './schema-validation'
import ResponseFieldSelector from './ResponseFieldSelector'
import { responseSelectionError } from './rest-tool-preview'

function SchemaBranch({name,query,expanded,children}:{name:string;query:string;expanded:boolean;children:ReactNode}) {
 const [open,setOpen]=useState(expanded)
 return <details open={!!query||open} onToggle={e=>setOpen(e.currentTarget.open)} className="border-l border-line pl-3 min-w-0">
  <summary className="cursor-pointer break-all py-2 text-sm font-semibold">{name}</summary>
  {(open||query)&&<div className="layout-stack pb-3">{children}</div>}
 </details>
}
function Node({ value, onChange, path, depth = 0, query = '', expanded = false }: { value: unknown; onChange: (s: Schema) => void; path: string; depth?: number; query?: string; expanded?: boolean }) {
  const s = objectSchema(value)
  const types = Array.isArray(s.type) ? s.type as string[] : []
  const nullable = types.includes('null') && types.length === 2
  const type = nullable ? types.find(t=>t!=='null') : s.type
  const complex = typeof value === 'boolean' || (Array.isArray(s.type) && !nullable) || ['$ref', 'allOf', 'anyOf', 'oneOf', 'not', 'if', 'prefixItems', 'dependencies', 'dependentSchemas', 'patternProperties'].some(k => k in s) || Array.isArray(s.items)
  const constrained = Object.keys(s).some(k => !['type', 'description', 'properties', 'required', 'items'].includes(k))
  const properties = objectSchema(s.properties)
  const required = Array.isArray(s.required) ? s.required as string[] : []
  if (complex || depth > 20) return <p className="text-xs text-ink-muted">Complex schema preserved. Use the advanced JSON editor to change this section.</p>
  return <div className="layout-stack min-w-0">
    <div className="grid gap-2 sm:grid-cols-2">
      <label className="text-xs">Type<select aria-label={`${path} type`} className={input} value={String(type ?? '')} disabled={constrained || !!Object.keys(properties).length || s.items !== undefined} onChange={e => onChange({ ...s, type: nullable ? [e.target.value, 'null'] : e.target.value })}><option value="" disabled>Choose type</option>{['object', 'array', 'string', 'integer', 'number', 'boolean', 'null'].map(t => <option key={t}>{t}</option>)}</select></label>
      <label className="text-xs">Description<input aria-label={`${path} description`} className={input} value={String(s.description ?? '')} onChange={e => onChange({ ...s, description: e.target.value })}/></label>
    </div>
    {!type && <p className="text-xs text-ink-muted">Choose a type: null values, empty lists and mixed lists need your input.</p>}
    {!!type && type !== 'null' && <label className="text-xs"><input type="checkbox" aria-label={`${path} nullable`} disabled={constrained} checked={nullable} onChange={e=>onChange({...s,type:e.target.checked?[type,'null']:type})}/> Allow null</label>}
    {constrained && <p className="text-xs text-ink-muted">Imported rules preserved. Change constrained types in advanced JSON.</p>}
    {type === 'object' && <>
      {Object.entries(properties).filter(([name,child])=>!query||`${path}.${name} ${JSON.stringify(child)}`.toLowerCase().includes(query)).map(([name, child]) => <SchemaBranch key={`${name}:${expanded}`} name={name} query={query} expanded={expanded}>
        <div className="flex flex-wrap items-center gap-3"><label className="text-xs"><input type="checkbox" aria-label={`${path}.${name} required`} checked={required.includes(name)} onChange={e => onChange({ ...s, required: e.target.checked ? [...required, name] : required.filter(k => k !== name) })}/> Required</label><button type="button" className={btn} onClick={() => onChange({ ...s, properties: Object.fromEntries(Object.entries(properties).filter(([k]) => k !== name)), ...(s.required !== undefined ? { required: required.filter(k => k !== name) } : {}) })}>Remove {name}</button></div>
        <Node value={child} path={`${path}.${name}`} depth={depth + 1} query={query} expanded={expanded} onChange={v => onChange({ ...s, properties: { ...properties, [name]: v } })}/>
      </SchemaBranch>)}
      {query&&depth===0&&<p className="text-xs text-ink-muted">Clear the field search to add new fields.</p>}
      {!query&&<AddField names={Object.keys(properties)} onAdd={name => onChange({ ...s, properties: { ...properties, [name]: { type: 'string' } } })}/>}
    </>}
    {type === 'array' && <div className="border-l border-line pl-3 layout-stack"><h4 className="text-xs font-semibold">List item</h4><Node value={s.items} path={`${path}[]`} depth={depth + 1} query={query} expanded={expanded} onChange={items => onChange({ ...s, items })}/></div>}
  </div>
}
function AddField({ names, onAdd }: { names: string[]; onAdd: (name: string) => void }) {
  const [name, setName] = useState('')
  return <div className="flex flex-wrap gap-2"><input className={input} aria-label="New field name" placeholder="New field name" value={name} onChange={e => setName(e.target.value)}/><button type="button" className={btn} disabled={!name.trim() || names.includes(name.trim())} onClick={() => { onAdd(name.trim()); setName('') }}>Add field</button></div>
}
export default function SchemaEditor({label,value,onChange,disabled,responseFields,onFieldsChange}:{label:string;value:unknown;onChange:(s:Schema|undefined)=>void;disabled?:boolean;responseFields?:string[];onFieldsChange?:(fields:string[],schema:Schema|undefined)=>void}) {
 const [open,setOpen]=useState(false)
 const root=objectSchema(value)
 return <section aria-label={`${label} visual editor`} className="surface-inset p-4 layout-stack">
  <div className="flex flex-wrap items-center justify-between gap-3"><div><h3 className="text-sm font-semibold">{label}</h3><p className="mt-1 text-xs text-ink-muted">{value===undefined?'No schema configured':`${Array.isArray(root.type)?root.type.join(' / '):root.type||'JSON Schema'} · ${Object.keys(objectSchema(root.properties)).length} top-level fields · ${(schemaBytes(value)/1024).toFixed(1)} / 64 KiB`}</p></div><button type="button" className={btn} disabled={disabled} onClick={()=>setOpen(true)}>Edit {label.toLowerCase()} schema</button></div>
  {schemaLimitError(value)&&<p role="alert" className="text-xs text-red-700">{schemaLimitError(value)}</p>}
  {open&&<SchemaWorkspace label={label} initial={value} initialFields={responseFields} onClose={()=>setOpen(false)} onApply={(schema,fields)=>{if(fields&&onFieldsChange)onFieldsChange(fields,schema);else onChange(schema);setOpen(false)}}/>}
 </section>
}

function SchemaWorkspace({label,initial,initialFields,onClose,onApply}:{label:string;initial:unknown;initialFields?:string[];onClose:()=>void;onApply:(schema:Schema|undefined,fields?:string[])=>void}) {
 const [draft,setDraft]=useState(initial as Schema|undefined),[fields,setFields]=useState(initialFields??[])
 const [raw,setRaw]=useState(initial===undefined?'':JSON.stringify(initial,null,2)),[rawError,setRawError]=useState('')
 const [example,setExample]=useState(''),[mode,setMode]=useState<'merge'|'replace'>('merge'),[tab,setTab]=useState<'example'|'fields'|'json'>('example')
 const [query,setQuery]=useState(''),[expanded,setExpanded]=useState(false),[errors,setErrors]=useState<string[]>([]),[status,setStatus]=useState(''),[busy,setBusy]=useState(false)
 useUnsaved({draft,fields,raw,example})
 const resultRef=useRef<HTMLDivElement>(null)
 useEffect(()=>{if(errors.length)resultRef.current?.scrollIntoView?.({block:'nearest'})},[errors])
 const confirm=useConfirm()
 const update=(schema:Schema|undefined)=>{setDraft(schema);setRaw(schema===undefined?'':JSON.stringify(schema,null,2));setRawError('');setErrors([]);setStatus('')}
 const close=async()=>{if(busy)return;const dirty=example||rawError||JSON.stringify(draft)!==JSON.stringify(initial)||JSON.stringify(fields)!==JSON.stringify(initialFields??[]);if(!dirty||await confirm(discardConfirmation))onClose()}
 const generate=async()=>{
  setErrors([]);setStatus('')
  try {
   if(new TextEncoder().encode(example).length>200000)throw Error('Use an example smaller than 200 KB.')
   const sample=JSON.parse(example)
   const inferred=inferSchema(sample)
   const schema=mode==='replace'?inferred:mergeExample(draft,inferred)
   const limit=schemaLimitError(schema);if(limit)throw Error(limit)
   if(mode==='replace'&&draft!==undefined&&!await confirm({title:'Replace existing schema?',description:'This replaces existing types, required fields and imported constraints in this schema draft. Review response field selections afterward.',confirmLabel:'Replace schema',tone:'warning'}))return
   update(schema)
   setBusy(true)
   const issues=await checkSchema(schema,{value:sample})
   setErrors(issues)
   setStatus(issues.length?'Example conflicts with preserved rules. Review the reported fields or replace the schema.':'Schema generated. Review types and nullability; fields are optional until marked required.')
   setTab('fields')
  }catch(error){setErrors([error instanceof SyntaxError?'Enter valid JSON.':(error as Error).message])}finally{setBusy(false)}
 }
 const validate=async()=>{
  if(rawError)return
  setBusy(true);setErrors([]);setStatus('')
  try {const sample=example.trim()?{value:JSON.parse(example)}:undefined;const issues=await checkSchema(draft,sample);setErrors(issues);setStatus(issues.length?'Review the schema and example mismatch.':'Schema checks passed'+(sample?' and the example matches.':'.'))}catch{setErrors(['Enter valid JSON in the example before checking it.'])}finally{setBusy(false)}
 }
 const apply=async()=>{
  if(rawError)return
  setBusy(true);setErrors([])
  try {const sample=example.trim()?{value:JSON.parse(example)}:undefined;const issues=await checkSchema(draft,sample);const selection=initialFields!==undefined?responseSelectionError(draft,fields):'';if(selection)issues.push(selection);if(issues.length){setErrors(issues);return}onApply(draft,initialFields===undefined?undefined:fields)}catch(error){setErrors([error instanceof SyntaxError?'Enter valid JSON in the example or clear it before applying.':(error as Error).message])}finally{setBusy(false)}
 }
 return <Modal title={`${label} schema`} size="audit" busy={busy} onClose={()=>void close()} description="Edit a schema in this workspace, apply it to the operation draft, then save the upstream." footer={<div className="flex flex-wrap justify-between gap-3"><div className="min-w-0 space-y-2"><button className={btn} disabled={busy||!!rawError} onClick={()=>void validate()}>Check schema & example</button>{status&&<p role="status" className="max-w-xl text-xs text-ink-muted">{status}</p>}</div><div className="flex flex-wrap gap-3"><button className={btn} onClick={()=>void close()}>Cancel schema changes</button><button className={primary} disabled={busy||!!rawError||!!schemaLimitError(draft)} onClick={()=>void apply()}>{busy?'Checking…':'Apply schema'}</button></div></div>}>
  <div className="layout-stack">
   <div className="sticky top-0 z-10 space-y-3 border-b border-line bg-surface py-3"><div className="flex flex-wrap items-center justify-between gap-3"><nav aria-label={`${label} schema views`} className="flex flex-wrap gap-2">{(['example','fields','json'] as const).map(view=><button key={view} className={btn} aria-pressed={tab===view} onClick={()=>setTab(view)}>{view==='example'?'Example JSON':view==='fields'?'Fields':'Schema JSON'}</button>)}</nav><p className="text-xs text-ink-muted">{(schemaBytes(draft)/1024).toFixed(1)} / 64 KiB</p></div>{tab==='fields'&&<div className="flex flex-wrap gap-3"><label className="min-w-0 flex-1 text-xs">Find a field<input aria-label="Find schema fields" className={`${input} mt-2`} placeholder="Name, path or description" value={query} onChange={e=>setQuery(e.target.value.toLowerCase())}/></label><button className={btn} onClick={()=>setExpanded(!expanded)}>{expanded?'Collapse all fields':'Expand all fields'}</button></div>}</div>
   <div ref={resultRef}>{!!errors.length&&<ErrorBox title="Schema needs attention"><ul className="space-y-1">{errors.map((error,i)=><li key={i} className="break-words">{error}</li>)}</ul></ErrorBox>}
   </div>
   {tab==='example'&&<div className="layout-stack"><p className="text-sm text-ink-muted">Paste the raw REST response or request body. MCP output wraps the raw REST response in result; do not paste that wrapper. One example cannot describe every valid response.</p>
    <label className="text-xs">Example JSON<textarea aria-label={`${label} example JSON`} className={`${input} mt-2 font-mono text-xs`} rows={12} value={example} onChange={e=>{setExample(e.target.value);setErrors([]);setStatus('')}}/></label>
    <label className="text-xs">Generation mode<select aria-label="Schema generation mode" className={`${input} mt-2`} value={mode} onChange={e=>setMode(e.target.value as 'merge'|'replace')}><option value="merge">Merge · preserve existing rules</option><option value="replace">Replace · build a fresh schema</option></select></label>
    <button className={`${primary} self-start`} disabled={!example.trim()||busy||!!rawError} onClick={()=>void generate()}>Generate {label.toLowerCase()} schema</button>
   </div>}
   {tab==='fields'&&<div className="layout-stack"><p className="text-xs text-ink-muted">Expand a field to edit its type, nullability, required flag or description. Empty lists and null-only examples need a type choice. Integer fields reject fractional values; choose number when decimals are possible.</p><fieldset disabled={!!rawError} className="min-w-0"><Node value={draft} path={label} query={query} expanded={expanded} onChange={update}/></fieldset>
    {initialFields!==undefined&&<ResponseFieldSelector schema={draft} fields={fields} onChange={setFields}/>}
   </div>}
   {tab==='json'&&<div className="layout-stack"><p className="text-xs text-ink-muted">Inline JSON Schema only. Blank removes the schema. Existing constraints are preserved by visual edits; complex schemas can be edited here.</p><label className="text-xs">{label} schema<textarea aria-label={`${label} schema`} className={`${input} mt-2 font-mono text-xs`} rows={18} value={raw} onChange={e=>{const text=e.target.value;setRaw(text);setErrors([]);setStatus('');try{const schema=text.trim()?JSON.parse(text):undefined;const issue=schemaLimitError(schema);setRawError(issue);if(!issue)setDraft(schema)}catch{setRawError('Invalid JSON. Correct it before applying the schema.')}}}/></label>{rawError&&<p role="alert" className="text-xs text-red-700">{rawError}</p>}<button className={`${btn} self-start`} onClick={()=>void (async()=>{if(await confirm({title:'Remove schema?',description:'The schema and its response field selection will be removed from this draft.',confirmLabel:'Remove schema',tone:'warning'})){update(undefined);setFields([])}})()}>Remove schema</button></div>}
  </div>
 </Modal>
}
