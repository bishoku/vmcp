import { cloneElement, isValidElement, useId } from 'react'
import type { ReactNode, ComponentProps } from 'react'
import { ErrorBox } from '../ui'
import Select from '../Select'
import { input } from '../styles'
export function Field({
  label,
  children,
  hint,
}: {
  label: string
  children: ReactNode
  hint?: ReactNode
}) {
  const id = useId(),
    controlId = isValidElement<{ id?: string }>(children) ? (children.props.id ?? id) : id,
    hintId = `${id}-hint`
  const control = isValidElement<{ id?: string; 'aria-describedby'?: string }>(children)
    ? cloneElement(children, {
        id: children.props.id ?? id,
        'aria-describedby':
          [children.props['aria-describedby'], hint ? hintId : ''].filter(Boolean).join(' ') ||
          undefined,
      })
    : children
  return (
    <div className="min-w-0 space-y-2 text-xs">
      <label className="field-label" htmlFor={controlId}>
        {label}
      </label>
      {control}
      {hint && (
        <p id={hintId} className="supporting-copy text-ink-muted">
          {hint}
        </p>
      )}
    </div>
  )
}
export function ModeSelect({
  label,
  value,
  options,
  onChange,
}: {
  label: string
  value: string
  options: readonly string[]
  onChange: (value: string) => void
}) {
  return (
    <Field label={label}>
      <Select value={value} onChange={(e) => onChange(e.target.value)}>
        {options.map((v) => (
          <option key={v} value={v}>
            {v.replaceAll('_', ' ')}
          </option>
        ))}
      </Select>
    </Field>
  )
}
export function AccessError({ error }: { error: string }) {
  return error ? <ErrorBox>{error}</ErrorBox> : null
}

export function TextInput({
  value,
  onChange,
  ...rest
}: { value: string; onChange: (value: string) => void } & Omit<
  ComponentProps<'input'>,
  'value' | 'onChange'
>) {
  return (
    <input {...rest} className={input} value={value} onChange={(e) => onChange(e.target.value)} />
  )
}
