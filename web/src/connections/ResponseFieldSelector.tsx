import { btn } from '../styles'
import { schemaObject, responseSelectionError } from './rest-tool-preview'

export default function ResponseFieldSelector({ schema, fields = [], onChange }: { schema: unknown; fields?: string[]; onChange: (fields: string[]) => void }) {
  const root = schemaObject(schema), properties = schemaObject(root.properties)
  const available = root.type === 'object' ? Object.keys(properties) : []
  const filtering = fields.length > 0
  const names = [...new Set([...available, ...fields])]
  const error = responseSelectionError(schema, fields)
  return <section aria-label="Fields returned to the agent" className="layout-stack border-t border-line pt-4">
    <div><h3 className="text-sm font-semibold">Fields returned to the agent</h3><p className="mt-1 text-xs text-ink-muted">Choose top-level response fields. Objects and lists include their entire contents. Save applies this selection to real tool responses and the MCP output schema.</p></div>
    <div className="flex flex-wrap gap-2" role="group" aria-label="Response field mode">
      <button type="button" className={btn} aria-pressed={!filtering} onClick={() => onChange([])}>Full response</button>
      <button type="button" className={btn} aria-pressed={filtering} disabled={!available.length} onClick={() => { if (!filtering) onChange(available) }}>Selected fields only</button>
    </div>
    {!filtering && <p className="text-xs text-ink-muted">The full response is returned, including fields not declared in the schema.</p>}
    {!available.length && <p className="text-xs text-ink-muted">Generate or define an object response schema to select its fields.</p>}
    {filtering && <>
      <p className="text-xs text-ink-muted">{fields.length} selected. Keep at least one field, or explicitly choose Full response. Unselected and undeclared fields are omitted.</p>
      {names.map(name => {
        const declared = available.includes(name), checked = fields.includes(name)
        const child = schemaObject(properties[name])
        return <label key={name} className="surface-inset p-3 flex items-start gap-3 min-w-0">
          <input type="checkbox" aria-label={`Return field ${name}`} checked={checked} disabled={checked && fields.length === 1} onChange={e => onChange(e.target.checked ? [...fields, name] : fields.filter(field => field !== name))}/>
          <span className="min-w-0 text-sm"><span className="break-all font-semibold">{name}</span><span className="ml-2 text-xs text-ink-muted">{declared ? String(child.type ?? 'schema') : 'Missing from schema'}</span>{typeof child.description === 'string' && <span className="block break-words text-xs text-ink-muted">{child.description}</span>}</span>
        </label>
      })}
    </>}
    {error && <p role="alert" className="text-xs text-red-700">{error}</p>}
  </section>
}
