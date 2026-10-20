import { useState } from 'react'
import { btn, input } from '../styles'
import { inferSchema, mergeExample, objectSchema, type Schema } from './schema-editor'

function Node({ value, onChange, path, depth = 0 }: { value: unknown; onChange: (s: Schema) => void; path: string; depth?: number }) {
  const s = objectSchema(value)
  const complex = typeof value === 'boolean' || Array.isArray(s.type) || ['$ref', 'allOf', 'anyOf', 'oneOf', 'not', 'if', 'prefixItems', 'dependencies', 'dependentSchemas', 'patternProperties'].some(k => k in s) || Array.isArray(s.items)
  const constrained = Object.keys(s).some(k => !['type', 'description', 'properties', 'required', 'items'].includes(k))
  const properties = objectSchema(s.properties)
  const required = Array.isArray(s.required) ? s.required as string[] : []
  if (complex || depth > 20) return <p className="text-xs text-ink-muted">Complex schema preserved. Use the advanced JSON editor to change this section.</p>
  return <div className="layout-stack min-w-0">
    <div className="grid gap-2 sm:grid-cols-2">
      <label className="text-xs">Type<select aria-label={`${path} type`} className={input} value={String(s.type ?? '')} disabled={constrained || !!Object.keys(properties).length || s.items !== undefined} onChange={e => onChange({ ...s, type: e.target.value })}><option value="" disabled>Choose type</option>{['object', 'array', 'string', 'integer', 'number', 'boolean', 'null'].map(t => <option key={t}>{t}</option>)}</select></label>
      <label className="text-xs">Description<input aria-label={`${path} description`} className={input} value={String(s.description ?? '')} onChange={e => onChange({ ...s, description: e.target.value })}/></label>
    </div>
    {!s.type && <p className="text-xs text-ink-muted">Choose a type: null values, empty lists and mixed lists need your input.</p>}
    {constrained && <p className="text-xs text-ink-muted">Imported rules preserved. Change constrained types in advanced JSON.</p>}
    {s.type === 'object' && <>
      {Object.entries(properties).map(([name, child]) => <details key={name} open className="border-l border-line pl-3 layout-stack min-w-0">
        <summary className="cursor-pointer break-all text-sm font-semibold">{name}</summary>
        <div className="flex flex-wrap items-center gap-3"><label className="text-xs"><input type="checkbox" aria-label={`${path}.${name} required`} checked={required.includes(name)} onChange={e => onChange({ ...s, required: e.target.checked ? [...required, name] : required.filter(k => k !== name) })}/> Required</label><button type="button" className={btn} onClick={() => onChange({ ...s, properties: Object.fromEntries(Object.entries(properties).filter(([k]) => k !== name)), ...(s.required !== undefined ? { required: required.filter(k => k !== name) } : {}) })}>Remove {name}</button></div>
        <Node value={child} path={`${path}.${name}`} depth={depth + 1} onChange={v => onChange({ ...s, properties: { ...properties, [name]: v } })}/>
      </details>)}
      <AddField names={Object.keys(properties)} onAdd={name => onChange({ ...s, properties: { ...properties, [name]: { type: 'string' } } })}/>
    </>}
    {s.type === 'array' && <div className="border-l border-line pl-3 layout-stack"><h4 className="text-xs font-semibold">List item</h4><Node value={s.items} path={`${path}[]`} depth={depth + 1} onChange={items => onChange({ ...s, items })}/></div>}
  </div>
}
function AddField({ names, onAdd }: { names: string[]; onAdd: (name: string) => void }) {
  const [name, setName] = useState('')
  return <div className="flex flex-wrap gap-2"><input className={input} aria-label="New field name" placeholder="New field name" value={name} onChange={e => setName(e.target.value)}/><button type="button" className={btn} disabled={!name.trim() || names.includes(name.trim())} onClick={() => { onAdd(name.trim()); setName('') }}>Add field</button></div>
}
export default function SchemaEditor({ label, value, onChange, disabled }: { label: string; value: unknown; onChange: (s: Schema) => void; disabled?: boolean }) {
  const [example, setExample] = useState(''), [error, setError] = useState('')
  return <section aria-label={`${label} visual editor`} className="border-t border-line pt-4 layout-stack">
    <h3 className="text-sm font-semibold">{label}</h3>
    <p className="text-xs text-ink-muted">Paste example JSON or build fields below. Existing rules are kept; inferred fields are optional until you mark them required. Example values are not saved.</p>
    <fieldset disabled={disabled} className="layout-stack min-w-0">
      <label className="text-xs">Example JSON<textarea aria-label={`${label} example JSON`} className={`${input} font-mono text-xs`} rows={4} value={example} onChange={e => { setExample(e.target.value); setError('') }}/></label>
      <button type="button" className={`${btn} self-start`} disabled={!example.trim()} onClick={() => { try { if (example.length > 200000) throw Error('Use an example smaller than 200 KB.'); onChange(mergeExample(value, inferSchema(JSON.parse(example)))); setError('') } catch (e) { setError(e instanceof SyntaxError ? 'Enter valid JSON.' : (e as Error).message) } }}>Generate {label.toLowerCase()} schema</button>
      {error && <p role="alert" className="text-xs text-red-700">{error}</p>}
      <Node value={value} onChange={onChange} path={label}/>
    </fieldset>
  </section>
}
