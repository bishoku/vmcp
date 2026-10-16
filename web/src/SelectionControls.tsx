import { useContext, useId, useLayoutEffect, useRef, type ComponentProps, type ReactNode } from 'react'
import { AlertCircle } from 'lucide-react'
import { FormContext } from './form-validation'
import { btn } from './styles'

type ChoiceProps = Omit<ComponentProps<'input'>, 'type' | 'children' | 'ref'> & {
  label: string
  description?: ReactNode
  end?: ReactNode
  field?: string
  error?: string
  variant?: 'card' | 'row' | 'plain'
  indeterminate?: boolean
}
function Choice({ label, description, end, field, error, variant = 'card', indeterminate = false, kind, className = '', id: suppliedId, onChange, ...props }: ChoiceProps & { kind: 'checkbox' | 'switch' }) {
  const validation = useContext(FormContext)
  const generatedId = useId()
  const id = suppliedId ?? `${generatedId}-choice`
  const native = useRef<HTMLInputElement>(null)
  useLayoutEffect(() => { if (native.current) native.current.indeterminate = indeterminate }, [indeterminate])
  const errors = [...(field ? validation?.issues.filter(issue => issue.field === field || issue.field?.startsWith(`${field}.`)) ?? [] : []), ...(error ? [{ message: error }] : [])]
  const invalid = errors.length > 0 || props['aria-invalid'] === true || props['aria-invalid'] === 'true'
  const describedBy = [props['aria-describedby'], description ? `${id}-description` : '', errors.length ? `${id}-error` : ''].filter(Boolean).join(' ') || undefined
  return <div className={`choice-field choice-${variant} ${className}`}>
    <label htmlFor={id} className="choice-label">
      <input {...props} ref={native} id={id} type="checkbox" role={kind === 'switch' ? 'switch' : undefined} className={`choice-input choice-${kind}`} aria-labelledby={props['aria-label'] ? props['aria-labelledby'] : props['aria-labelledby'] ?? `${id}-label`} aria-describedby={describedBy} aria-invalid={invalid || undefined} data-validation-field={field} onChange={event => { if (field) validation?.clear(field); onChange?.(event) }}/>
      <span className="min-w-0 flex-1"><span id={`${id}-label`} className="block text-xs font-medium text-ink">{label}</span>{description && <span id={`${id}-description`} className="mt-1.5 block text-xs font-normal leading-relaxed text-ink-muted">{description}</span>}</span>
      {end && <span className="shrink-0">{end}</span>}
    </label>
    {errors.length > 0 && <div id={`${id}-error`} className="flex items-start gap-1.5 px-3 pb-3 text-xs leading-relaxed text-red-700"><AlertCircle aria-hidden="true" className="mt-0.5 icon-inline shrink-0"/><div>{errors.map((issue, i) => <p key={i}>{issue.message}</p>)}</div></div>}
  </div>
}
export function Checkbox(props: ChoiceProps) { return <Choice {...props} kind="checkbox"/> }
export function Switch(props: Omit<ChoiceProps, 'indeterminate'>) { return <Choice {...props} kind="switch"/> }

export function RadioGroup({ label, value, onChange, options, disabled = false, error, description, className = '' }: { label: string; value: string; onChange: (value: string) => void; options: { value: string; label: string; disabled?: boolean }[]; disabled?: boolean; error?: string; description?: string; className?: string }) {
  const id = useId()
  return <fieldset role="radiogroup" aria-label={label} aria-invalid={error ? true : undefined} aria-describedby={[description ? `${id}-description` : '', error ? `${id}-error` : ''].filter(Boolean).join(' ') || undefined} disabled={disabled} className={`m-0 min-w-0 border-0 p-0 ${className}`}>
    {description && <p id={`${id}-description`} className="mb-2 text-xs leading-relaxed text-ink-muted">{description}</p>}
    <div className="choice-radio-group">{options.map(option => <label key={option.value} className="choice-radio-label"><input className="choice-input choice-radio" type="radio" name={id} value={option.value} checked={option.value === value} disabled={option.disabled} aria-invalid={error ? true : undefined} aria-describedby={error ? `${id}-error` : undefined} onChange={event => { if (event.target.checked) onChange(option.value) }}/><span className="min-w-0 break-words text-xs font-medium">{option.label}</span></label>)}</div>
    {error && <p id={`${id}-error`} className="mt-2 flex items-start gap-1.5 text-xs leading-relaxed text-red-700"><AlertCircle aria-hidden="true" className="mt-0.5 icon-inline shrink-0"/>{error}</p>}
  </fieldset>
}

export function ToggleButton({ className = '', variant = 'outline', ...props }: ComponentProps<'button'> & { variant?: 'outline' | 'ghost' }) {
  return <button type="button" {...props} className={`toggle-button ${variant === 'outline' ? btn : 'action-button action-ghost'} ${className}`}/>
}
