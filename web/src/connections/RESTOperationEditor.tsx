import { useEffect, useState } from 'react'
import ResponseFieldSelector from './ResponseFieldSelector'
import SchemaEditor from './SchemaEditor'
import { Wrench } from 'lucide-react'
import type { RESTOperation } from '../api'
import { FormField } from '../FormField'
import Select from '../Select'
import { Switch } from '../SelectionControls'
import { useUnsaved } from '../interaction'
import { btn, input } from '../styles'
import { ServerPanel } from './ServerPanels'
import { pathParameters, schemaObject, responseSelectionError } from './rest-tool-preview'

const json = (value: unknown) => value === undefined ? '' : JSON.stringify(value, null, 2)
const validName = /^[A-Za-z0-9_.-]{1,128}$/

export default function RESTOperationEditor({ operation, index, onChange, onInvalidChange, names }: {
  operation: RESTOperation
  index: number
  onChange?: (operation: RESTOperation) => void
  onInvalidChange: (invalid: boolean) => void
  names: string[]
}) {
  // Invalid JSON remains local until corrected. Navigation must explicitly discard it.
  const [body, setBody] = useState(json(operation.bodySchema))
  const [response, setResponse] = useState(json(operation.responseSchema))
  const [headers, setHeaders] = useState(json(operation.headers))
  const [statuses, setStatuses] = useState((operation.successStatuses ?? []).join(', '))
  const [parametersJSON, setParametersJSON] = useState(json(operation.parameters))
  const [errors, setErrors] = useState<Record<string, string>>({})
  const invalid = Object.values(errors).some(Boolean) || !!responseSelectionError(operation.responseSchema, operation.responseFields)
  // Valid edits belong to the parent's draft, which is cleaned on Save. Only
  // unparsed local text needs a separate navigation guard.
  useUnsaved(invalid ? { body, response, headers, statuses, parametersJSON } : null)
  useEffect(() => { onInvalidChange(invalid); return () => onInvalidChange(false) }, [invalid, onInvalidChange])
  const set = (patch: Partial<RESTOperation>) => {
    if (patch.parameters) setParametersJSON(json(patch.parameters))
    onChange?.({ ...operation, ...patch })
  }
  const field = (name: string) => `rest.operations.${index}.${name}`
  const parse = (key: 'bodySchema' | 'responseSchema' | 'headers', text: string) => {
    try {
      const value: unknown = text.trim() ? JSON.parse(text) : undefined
      if (key === 'headers') {
        if (value !== undefined && (value === null || Array.isArray(value) || typeof value !== 'object' || Object.values(value).some(item => typeof item !== 'string'))) throw Error('Use a JSON object with string header values.')
      } else if (value !== undefined && typeof value !== 'boolean' && (value === null || Array.isArray(value) || typeof value !== 'object')) {
        throw Error('Use a JSON Schema object or boolean.')
      }
      set({ [key]: value })
      setErrors(previous => ({ ...previous, [key]: '' }))
    } catch (error) {
      setErrors(previous => ({ ...previous, [key]: error instanceof SyntaxError ? 'Invalid JSON. Correct this field before saving or switching tools.' : String((error as Error).message) }))
    }
  }
  const parameter = (position: number, patch: Partial<RESTOperation['parameters'][number]>) =>
    set({ parameters: operation.parameters.map((value, i) => i === position ? { ...value, ...patch } : value) })
  return <ServerPanel title="Request & tool" description={onChange ? 'Changes update the upstream draft. Use Save when ready.' : 'Saved operation definition'} icon={Wrench}>
    <fieldset disabled={!onChange} className="panel-body layout-stack min-w-0">
      <div className="grid gap-3 sm:grid-cols-[7rem_minmax(0,1fr)]">
        <FormField name={field('method')} label="HTTP method"><Select className={input} value={operation.method} onChange={event => set({ method: event.target.value as RESTOperation['method'], readOnly: false })}>{['GET', 'POST', 'PUT', 'PATCH', 'DELETE'].map(method => <option key={method}>{method}</option>)}</Select></FormField>
        <FormField name={field('path')} label="Operation path" required hint="Use {id} to create a path parameter automatically.">
          <input className={input} disabled={!!errors.parameters} value={operation.path} placeholder="/customers/{id}" onChange={event => set({ path: event.target.value, parameters: pathParameters(event.target.value, operation.parameters) })}/>
        </FormField>
      </div>
      <FormField name={field('name')} label="Tool name" required error={!validName.test(operation.name) ? 'Use 1–128 letters, numbers, dots, underscores or hyphens.' : names.filter(name => name === operation.name).length > 1 ? 'Choose a unique tool name.' : undefined}>
        <input className={input} value={operation.name} placeholder="get_customer" onChange={event => set({ name: event.target.value })}/>
      </FormField>
      {!!operation.headerNames?.length && <p className="text-xs text-ink-muted">Stored operation headers follow the saved tool name. Renaming this tool requires entering its header values again.</p>}
      <FormField name={field('description')} label="Tool description" hint="Explain when to use this tool and what it returns.">
        <textarea className={input} rows={3} maxLength={20000} value={operation.description} placeholder="Retrieve a customer by their known customer ID." onChange={event => set({ description: event.target.value })}/>
      </FormField>
      <div className="flex flex-wrap gap-4">
        <Switch label="Include in tool catalog" checked={operation.enabled} onChange={event => set({ enabled: event.target.checked })}/>
        <Switch label="Read-only operation" disabled={operation.method !== 'GET'} checked={operation.readOnly} onChange={event => set({ readOnly: event.target.checked })}/>
      </div>
      <section aria-label="Request parameters" className="layout-stack border-t border-line pt-4">
        <div><h3 className="text-sm font-semibold">Parameters</h3><p className="mt-1 text-xs text-ink-muted">Path values come from the URL above. Add query parameters for filters and options.</p></div>
        {errors.parameters && <p className="text-xs text-amber-800">Correct the advanced parameter JSON to continue editing parameter rows.</p>}
        <fieldset disabled={!!errors.parameters} className="layout-stack min-w-0">
        {!operation.parameters.length && <p className="text-xs text-ink-muted">This request has no path or query parameters.</p>}
        {operation.parameters.map((value, position) => <div key={position} role="group" aria-label={`Parameter ${position + 1}`} className="surface-inset p-3 layout-stack">
          <div className="grid gap-3 sm:grid-cols-2">
            <FormField name={field(`parameters.${position}.name`)} label={`Parameter ${position + 1} name`} hint={value.in === 'path' ? 'Path · edit the name in the URL.' : 'Query parameter'}
              error={!validName.test(value.name) || value.name === 'body' ? 'Use a valid name other than “body”.' : operation.parameters.filter(other => other.name === value.name).length > 1 ? 'Parameter names must be unique.' : undefined}>
              <input className={input} readOnly={value.in === 'path'} value={value.name} onChange={event => parameter(position, { name: event.target.value })}/>
            </FormField>
            <FormField name={field(`parameters.${position}.schema.type`)} label={`Parameter ${position + 1} type`}>
              <Select className={input} value={String(value.schema.type ?? 'string')} onChange={event => parameter(position, { schema: { ...value.schema, type: event.target.value } })}>
                {['string', 'integer', 'number', 'boolean'].map(type => <option key={type}>{type}</option>)}
              </Select>
            </FormField>
          </div>
          <FormField name={field(`parameters.${position}.schema.description`)} label={`Parameter ${position + 1} description`}>
            <input className={input} value={String(value.schema.description ?? '')} onChange={event => parameter(position, { schema: { ...value.schema, description: event.target.value } })}/>
          </FormField>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <Switch label={`Parameter ${position + 1} required`} disabled={value.in === 'path'} checked={value.in === 'path' || value.required} onChange={event => parameter(position, { required: event.target.checked })}/>
            {value.in === 'query' && onChange && <button className={btn} onClick={() => set({ parameters: operation.parameters.filter((_, i) => i !== position) })}>Remove parameter {position + 1}</button>}
          </div>
          {Object.keys(value.schema).some(key => !['type', 'description'].includes(key)) && <details><summary className="cursor-pointer text-xs">Imported schema constraints (preserved)</summary><pre className="mt-2 whitespace-pre-wrap break-all text-xs">{json(value.schema)}</pre></details>}
        </div>)}
        {onChange && <button className={`${btn} self-start`} disabled={operation.parameters.length >= 100} onClick={() => set({ parameters: [...operation.parameters, { name: '', in: 'query', required: false, schema: { type: 'string' } }] })}>Add query parameter</button>}
        </fieldset>
      </section>
      {operation.method !== 'GET' && <SchemaEditor label="Request body" value={operation.bodySchema} disabled={!!errors.bodySchema} onChange={schema => { setBody(json(schema)); set({ bodySchema: schema }) }}/>}
      <SchemaEditor label="Response" value={operation.responseSchema} disabled={!!errors.responseSchema} onChange={schema => { setResponse(json(schema)); set({ responseSchema: schema }) }}/>
      <ResponseFieldSelector schema={operation.responseSchema} fields={operation.responseFields} onChange={responseFields => set({ responseFields })}/>
      <details className="border-t border-line pt-4">
        <summary className="cursor-pointer text-sm font-semibold">Advanced · body, response & headers</summary>
        <div className="mt-4 layout-stack">
          <FormField name={field('parameters')} label="Request parameters (JSON)" error={errors.parameters} hint="Optional advanced editing of enums, patterns and other schema constraints. Changes also update the visual rows.">
            <textarea className={`${input} font-mono text-xs`} rows={5} value={parametersJSON} onChange={event => {
              const text = event.target.value; setParametersJSON(text)
              try {
                const parsed: unknown = JSON.parse(text)
                if (!Array.isArray(parsed) || parsed.some(item => {
                  const value = schemaObject(item), schema = schemaObject(value.schema)
                  return typeof value.name !== 'string' || !['path', 'query'].includes(String(value.in)) || typeof value.required !== 'boolean' || !['string', 'integer', 'number', 'boolean'].includes(String(schema.type))
                })) throw Error('Use an array of parameters with name, in (path/query), required and a scalar schema type.')
                onChange?.({ ...operation, parameters: parsed })
                setErrors(previous => ({ ...previous, parameters: '' }))
              } catch (error) {
                setErrors(previous => ({ ...previous, parameters: error instanceof SyntaxError ? 'Invalid parameter JSON.' : (error as Error).message }))
              }
            }}/>
          </FormField>
          <FormField name={field('bodySchema')} label="Request body schema" error={errors.bodySchema} hint={operation.method === 'GET' ? 'GET requests cannot have a body. Clear any existing body before saving.' : 'An inline JSON Schema adds a required “body” argument.'}>
            <textarea className={`${input} font-mono text-xs`} rows={5} value={body} onChange={event => { setBody(event.target.value); parse('bodySchema', event.target.value) }}/>
          </FormField>
          <FormField name={field('responseSchema')} label="Response schema" error={errors.responseSchema} hint="Optional inline JSON Schema. Existing complex schemas are preserved.">
            <textarea className={`${input} font-mono text-xs`} rows={5} value={response} onChange={event => { setResponse(event.target.value); parse('responseSchema', event.target.value) }}/>
          </FormField>
          <FormField name={field('successStatuses')} label="Success statuses" error={errors.statuses} hint="Comma-separated 2xx codes. Empty accepts all 2xx responses.">
            <input className={input} value={statuses} onChange={event => {
              const text = event.target.value; setStatuses(text)
              const codes = text.split(/[\s,]+/).filter(Boolean).map(Number)
              const error = codes.some(code => !Number.isInteger(code) || code < 200 || code > 299) ? 'Use HTTP status codes from 200 to 299.' : ''
              setErrors(previous => ({ ...previous, statuses: error }))
              if (!error) set({ successStatuses: codes })
            }}/>
          </FormField>
          <FormField name={field('headers')} label="Operation headers (JSON)" error={errors.headers} hint={`Write-only. Blank keeps stored values; {} clears them. Stored names: ${(operation.headerNames ?? []).join(', ') || 'none'}.`}>
            <textarea className={`${input} font-mono text-xs`} rows={3} value={headers} onChange={event => { setHeaders(event.target.value); parse('headers', event.target.value) }}/>
          </FormField>
        </div>
      </details>
    </fieldset>
  </ServerPanel>
}
