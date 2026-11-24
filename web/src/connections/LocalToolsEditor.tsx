import { useRef, useState } from 'react'
import { api, ApiError, type LocalTool, type LocalParameter } from '../api'
import { Braces, ListPlus, Play, Plus, Settings2, Trash2 } from 'lucide-react'
import { FormField } from '../FormField'
import { btn, input, primary } from '../styles'
import { Modal, ErrorBox } from '../ui'

export default function LocalToolsEditor({
  value,
  onChange,
  disabled,
}: {
  value: LocalTool[]
  onChange: (tools: LocalTool[]) => void
  disabled: boolean
}) {
  const [editing, setEditing] = useState<number | null>(null)
  return (
    <section className="surface-card panel-body layout-stack">
      <h2 className="section-title">Local tools</h2>
      {value.map((tool, i) => (
        <div key={i} className="surface-inset p-3 flex items-center justify-between">
          <div>
            <strong>{tool.name}</strong>
            <p>{tool.description}</p>
          </div>
          <button type="button" className={btn} onClick={() => setEditing(i)}>
            {disabled ? 'View' : 'Edit'}
          </button>
        </div>
      ))}
      {!disabled && (
        <button type="button" className={btn} onClick={() => setEditing(value.length)}>
          Add tool
        </button>
      )}
      {editing !== null && (
        <ToolEditor
          initial={value[editing]}
          disabled={disabled}
          onClose={() => setEditing(null)}
          onApply={(tool) => {
            if (value.some((item, i) => i !== editing && item.name === tool.name))
              throw new Error('Choose a unique tool name.')
            const next = [...value]
            next[editing] = tool
            onChange(next)
            setEditing(null)
          }}
          onRemove={() => {
            onChange(value.filter((_, i) => i !== editing))
            setEditing(null)
          }}
        />
      )}
    </section>
  )
}

function ToolEditor({
  initial,
  disabled,
  onClose,
  onApply,
  onRemove,
}: {
  initial?: LocalTool
  disabled: boolean
  onClose: () => void
  onApply: (tool: LocalTool) => void
  onRemove: () => void
}) {
  const [tool, setTool] = useState<LocalTool>(
    initial
      ? { ...initial, parameters: initial.parameters ?? [] }
      : { name: '', description: '', parameters: [], response: '' },
  )
  const [preview, setPreview] = useState(false)
  const [args, setArgs] = useState<Record<string, string>>({})
  const [selected, setSelected] = useState(0)
  const [result, setResult] = useState<string | null>(null),
    [error, setError] = useState(''),
    [busy, setBusy] = useState(false)
  const response = useRef<HTMLTextAreaElement>(null)
  const change = (next: LocalTool) => {
    setTool(next)
    setError('')
    setResult(null)
  }
  const parameter = (index: number, next: LocalParameter) =>
    change({ ...tool, parameters: tool.parameters.map((p, i) => (i === index ? next : p)) })
  function insert(text: string) {
    const el = response.current
    if (!el) return
    change({
      ...tool,
      response:
        tool.response.slice(0, el.selectionStart) + text + tool.response.slice(el.selectionEnd),
    })
    el.focus()
  }
  async function run(apply: boolean) {
    setBusy(true)
    setError('')
    setResult(null)
    try {
      const values: Record<string, unknown> = {}
      if (!apply)
        for (const p of tool.parameters) {
          const raw = args[p.name]
          if (raw === undefined || raw === '') continue
          values[p.name] =
            p.type === 'boolean' ? raw === 'true' : p.type === 'string[]' ? raw.split('\n') : raw
        }
      const res = await api.previewLocalTool(tool, apply ? undefined : values)
      if (apply) onApply(tool)
      else setResult(res.text)
    } catch (e) {
      setError(
        e instanceof ApiError && e.errors.length
          ? e.errors.map((issue) => issue.message).join('\n')
          : (e as Error).message,
      )
    } finally {
      setBusy(false)
    }
  }
  const key = tool.parameters[selected]?.name
  return (
    <Modal
      title={initial?.name || 'New local tool'}
      size="wide"
      busy={busy}
      initialFocus="#local-tool-name"
      onClose={() => {
        if (!busy) onClose()
      }}
      footer={
        <div className="flex flex-wrap items-center justify-end gap-2">
          {initial && !disabled && (
            <button disabled={busy} className={btn + ' mr-auto text-red-700'} onClick={onRemove}>
              Remove from draft
            </button>
          )}
          <button disabled={busy} className={btn} onClick={onClose}>
            Close
          </button>
          {!disabled && (
            <button disabled={busy} className={primary} onClick={() => void run(true)}>
              Apply tool to draft
            </button>
          )}
        </div>
      }
    >
      <div className="grid min-w-0 gap-4">
        <div
          role="group"
          aria-label="Tool editor view"
          className="inline-flex w-fit rounded-xl border border-line bg-canvas p-1"
        >
          {[
            { label: 'Definition', active: !preview, icon: Settings2 },
            { label: 'Try draft', active: preview, icon: Play },
          ].map(({ label, active, icon: Icon }, i) => (
            <button
              key={label}
              type="button"
              aria-pressed={active}
              className={
                'inline-flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium ' +
                (active ? 'bg-surface text-accent-700 shadow-sm' : 'text-ink-muted')
              }
              onClick={() => setPreview(i === 1)}
            >
              <Icon className="icon-inline" aria-hidden="true" />
              {label}
            </button>
          ))}
        </div>
        {!preview ? (
          <fieldset disabled={disabled} className="m-0 min-w-0 border-0 p-0">
            <div className="grid min-w-0 gap-4">
              <section className="surface-card panel-body grid gap-4 sm:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
                <FormField name="local.tool.name" label="Tool name" required>
                  <input
                    id="local-tool-name"
                    className={input}
                    value={tool.name}
                    onChange={(e) => change({ ...tool, name: e.target.value })}
                  />
                </FormField>
                <FormField name="local.tool.description" label="Description">
                  <input
                    className={input}
                    value={tool.description}
                    onChange={(e) => change({ ...tool, description: e.target.value })}
                  />
                </FormField>
              </section>
              <section className="surface-card min-w-0 overflow-hidden">
                <div className="panel-header flex flex-wrap items-center justify-between gap-3">
                  <h3 className="panel-title flex items-center gap-2">
                    <ListPlus className="icon-control text-accent-700" aria-hidden="true" />
                    Input parameters
                  </h3>{' '}
                  <button
                    className={btn}
                    type="button"
                    onClick={() =>
                      change({
                        ...tool,
                        parameters: [...tool.parameters, { name: '', type: 'string' }],
                      })
                    }
                  >
                    <Plus className="icon-inline" aria-hidden="true" /> Add parameter
                  </button>
                </div>
                {tool.parameters.map((p, i) => (
                  <div
                    className="panel-body border-t border-line grid min-w-0 items-start gap-4 sm:grid-cols-2"
                    key={i}
                  >
                    <FormField name={`local.parameters.${i}.name`} label="Parameter name">
                      <input
                        className={input}
                        value={p.name}
                        onChange={(e) => parameter(i, { ...p, name: e.target.value })}
                      />
                    </FormField>
                    <FormField name={`local.parameters.${i}.type`} label="Type">
                      <select
                        className={input}
                        value={p.type}
                        onChange={(e) =>
                          parameter(i, {
                            name: p.name,
                            required: p.required,
                            type: e.target.value as LocalParameter['type'],
                          })
                        }
                      >
                        <option value="string">Text</option>
                        <option value="boolean">Boolean</option>
                        <option value="string[]">Text array</option>
                      </select>
                    </FormField>
                    {p.type === 'string' && (
                      <FormField
                        name={`local.parameters.${i}.choices`}
                        label="Allowed values (one per line)"
                      >
                        <textarea
                          className={input}
                          value={p.choices?.join('\n') ?? ''}
                          onChange={(e) =>
                            parameter(i, {
                              ...p,
                              choices: e.target.value ? e.target.value.split('\n') : [],
                            })
                          }
                        />
                      </FormField>
                    )}
                    {p.type === 'boolean' && !p.required && (
                      <FormField name={`local.parameters.${i}.default`} label="Default">
                        <select
                          className={input}
                          value={String(p.default ?? '')}
                          onChange={(e) =>
                            parameter(i, {
                              ...p,
                              default:
                                e.target.value === '' ? undefined : e.target.value === 'true',
                            })
                          }
                        >
                          <option value="">Unset</option>
                          <option>true</option>
                          <option>false</option>
                        </select>
                      </FormField>
                    )}
                    <div className="flex flex-wrap items-center justify-between gap-3 sm:col-span-2">
                      <label className="field-label inline-flex items-center gap-2">
                        <input
                          type="checkbox"
                          checked={!!p.required}
                          onChange={(e) =>
                            parameter(i, { ...p, required: e.target.checked, default: undefined })
                          }
                        />{' '}
                        Required
                      </label>
                      <button
                        type="button"
                        className={btn}
                        onClick={() =>
                          change({ ...tool, parameters: tool.parameters.filter((_, j) => i !== j) })
                        }
                      >
                        <Trash2 className="icon-inline" aria-hidden="true" />
                        Remove parameter
                      </button>
                    </div>
                  </div>
                ))}
              </section>
              <section className="surface-card min-w-0 overflow-hidden">
                <div className="panel-header border-b border-line flex items-center gap-2">
                  <Braces aria-hidden="true" className="icon-control text-accent-700" />
                  <label htmlFor="local-tool-response" className="panel-title">
                    Response (text or Markdown)
                  </label>
                </div>
                <div className="panel-body grid gap-3">
                  <textarea
                    ref={response}
                    id="local-tool-response"
                    spellCheck={false}
                    className={input + ' min-h-36 resize-y bg-canvas/50 font-mono leading-6'}
                    rows={6}
                    value={tool.response}
                    onChange={(e) => change({ ...tool, response: e.target.value })}
                  />
                  {!!tool.parameters.length && (
                    <div className="flex flex-wrap items-center gap-2">
                      <select
                        aria-label="Template parameter"
                        className={input + ' sm:max-w-48'}
                        value={selected}
                        onChange={(e) => setSelected(Number(e.target.value))}
                      >
                        {tool.parameters.map((p, i) => (
                          <option key={i} value={i}>
                            {p.name}
                          </option>
                        ))}
                      </select>
                      <button
                        className={btn}
                        disabled={!key}
                        onClick={() => insert('{{ .Args.' + key + ' }}')}
                      >
                        Insert parameter
                      </button>
                      <button
                        className={btn}
                        disabled={!key}
                        onClick={() => insert('{{ if .Args.' + key + ' }}\n\n{{ end }}')}
                      >
                        Insert condition
                      </button>
                      <button
                        className={btn}
                        disabled={tool.parameters[selected]?.type !== 'string[]'}
                        onClick={() => insert('{{ range .Args.' + key + ' }}\n{{ . }}\n{{ end }}')}
                      >
                        Insert loop
                      </button>
                    </div>
                  )}
                </div>
              </section>
            </div>
          </fieldset>
        ) : (
          <section className="surface-card overflow-hidden">
            <div className="panel-header flex flex-wrap items-center justify-between gap-3 border-b border-line">
              <h3 className="panel-title">Try draft</h3>
              <button className={btn} onClick={() => void run(false)}>
                <Play className="icon-inline" aria-hidden="true" />
                Run preview
              </button>
            </div>
            <div className="panel-body grid gap-4">
              {tool.parameters
                .filter((p) => p.name)
                .map((p, i) => {
                  const update = (v: string) => {
                    setArgs({ ...args, [p.name]: v })
                    setResult(null)
                  }
                  return (
                    <label className="grid min-w-0 gap-2 field-label" key={i}>
                      {p.name}
                      {p.required ? ' (required)' : ''}
                      {p.type === 'boolean' || p.choices?.length ? (
                        <select
                          className={input}
                          value={args[p.name] ?? ''}
                          onChange={(e) => update(e.target.value)}
                        >
                          <option value="">Omit</option>
                          {(p.type === 'boolean' ? ['true', 'false'] : (p.choices ?? [])).map(
                            (v) => (
                              <option key={v}>{v}</option>
                            ),
                          )}
                        </select>
                      ) : p.type === 'string[]' ? (
                        <textarea
                          className={input}
                          placeholder="One value per line"
                          value={args[p.name] ?? ''}
                          onChange={(e) => update(e.target.value)}
                        />
                      ) : (
                        <input
                          className={input}
                          value={args[p.name] ?? ''}
                          onChange={(e) => update(e.target.value)}
                        />
                      )}
                    </label>
                  )
                })}
              {result === null ? (
                <p className="supporting-copy text-ink-muted">
                  Run the preview to see the rendered response.
                </p>
              ) : (
                <pre
                  aria-label="Tool response"
                  role="status"
                  className="surface-inset min-w-0 overflow-x-auto whitespace-pre-wrap break-words p-4 text-sm leading-6"
                >
                  {result}
                </pre>
              )}
            </div>
          </section>
        )}
        {error && <ErrorBox>{error}</ErrorBox>}
      </div>
    </Modal>
  )
}
