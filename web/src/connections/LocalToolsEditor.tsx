import { useRef, useState } from 'react'
import { api, ApiError, type LocalTool, type LocalParameter } from '../api'
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
      onClose={() => {
        if (!busy) onClose()
      }}
      footer={
        <div className="flex gap-2 justify-end">
          {initial && !disabled && (
            <button disabled={busy} className={btn} onClick={onRemove}>
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
      <fieldset disabled={busy || disabled} className="layout-stack">
        <label>
          Tool name
          <input
            className={input}
            value={tool.name}
            onChange={(e) => change({ ...tool, name: e.target.value })}
          />
        </label>
        <label>
          Description
          <input
            className={input}
            value={tool.description}
            onChange={(e) => change({ ...tool, description: e.target.value })}
          />
        </label>
        <h3>Input parameters</h3>
        {tool.parameters.map((p, i) => (
          <div className="surface-inset p-3 grid gap-2 sm:grid-cols-2" key={i}>
            <label>
              Parameter name
              <input
                className={input}
                value={p.name}
                onChange={(e) => parameter(i, { ...p, name: e.target.value })}
              />
            </label>
            <label>
              Type
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
            </label>
            {p.type === 'string' && (
              <label>
                Allowed values (one per line)
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
              </label>
            )}
            {p.type === 'boolean' && !p.required && (
              <label>
                Default
                <select
                  className={input}
                  value={String(p.default ?? '')}
                  onChange={(e) =>
                    parameter(i, {
                      ...p,
                      default: e.target.value === '' ? undefined : e.target.value === 'true',
                    })
                  }
                >
                  <option value="">Unset</option>
                  <option>true</option>
                  <option>false</option>
                </select>
              </label>
            )}
            <label>
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
              Remove parameter
            </button>
          </div>
        ))}
        <button
          className={btn}
          type="button"
          onClick={() =>
            change({ ...tool, parameters: [...tool.parameters, { name: '', type: 'string' }] })
          }
        >
          Add parameter
        </button>
        <label>
          Response (text or Markdown)
          <textarea
            ref={response}
            className={input + ' font-mono'}
            rows={10}
            value={tool.response}
            onChange={(e) => change({ ...tool, response: e.target.value })}
          />
        </label>
        {!!tool.parameters.length && (
          <div className="flex flex-wrap gap-2">
            <select
              aria-label="Template parameter"
              className={input}
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
      </fieldset>
      <fieldset disabled={busy} className="layout-stack mt-4">
        <legend>Try draft</legend>
        {tool.parameters
          .filter((p) => p.name)
          .map((p, i) => {
            const update = (v: string) => {
              setArgs({ ...args, [p.name]: v })
              setResult(null)
            }
            return (
              <label key={i}>
                {p.name}
                {p.required ? ' (required)' : ''}
                {p.type === 'boolean' || p.choices?.length ? (
                  <select
                    className={input}
                    value={args[p.name] ?? ''}
                    onChange={(e) => update(e.target.value)}
                  >
                    <option value="">Omit</option>
                    {(p.type === 'boolean' ? ['true', 'false'] : (p.choices ?? [])).map((v) => (
                      <option key={v}>{v}</option>
                    ))}
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
        <button className={btn} onClick={() => void run(false)}>
          Run preview
        </button>
      </fieldset>
      {error && <ErrorBox>{error}</ErrorBox>}
      {result !== null && (
        <pre aria-label="Tool response" className="surface-inset p-3 whitespace-pre-wrap">
          {result}
        </pre>
      )}
    </Modal>
  )
}
