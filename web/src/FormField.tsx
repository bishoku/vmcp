import { TagBadge } from './Badge'
import HelpTip from './HelpTip'
import { cloneElement, useContext, useId, type InputHTMLAttributes, type ReactElement, type ReactNode } from 'react'
import { AlertCircle, ArrowUpRight } from 'lucide-react'
import { ErrorBox } from './ui'
import { FormContext, focusField, fieldLabel, type Validation } from './form-validation'

export function FormScope({ validation, children }: { validation: Validation; children: ReactNode }) {
  const {root,...context}=validation
  return <FormContext.Provider value={context}><div ref={root} className="contents">{children}</div></FormContext.Provider>
}

/** Preserves explicit control labels while linking hints and errors to the input. */
export function FormField({ name, label, hint, required = false, children, className = '', renderControl, error, help }: { name: string; label: string; hint?: ReactNode; required?: boolean; children: ReactElement<InputHTMLAttributes<HTMLInputElement>>; className?: string; error?: string; help?: string; renderControl?: (control: ReactElement) => ReactNode }) {
  const validation = useContext(FormContext)
  const id = useId()
  const controlId = children.props.id ?? `${id}-control`
  const errors = [...(validation?.issues.filter(issue => issue.field === name || issue.field?.startsWith(`${name}.`)) ?? []),...(error?[{message:error}]:[])]
  const describedBy = [children.props['aria-describedby'], hint ? `${id}-hint` : '', errors.length ? `${id}-error` : ''].filter(Boolean).join(' ') || undefined
  const control = cloneElement(children, { id: controlId, 'aria-required': required || undefined, 'aria-invalid': errors.length ? true : undefined, 'aria-describedby': describedBy, ...{ 'data-validation-field': name }, onChange: event => { validation?.clear(name); children.props.onChange?.(event) } })
  return <div className={`min-w-0 space-y-2 ${className}`}>
    <div className="flex flex-wrap items-center gap-2"><label htmlFor={controlId} className="field-label">{label}</label>{help&&<HelpTip label={`About ${label.toLowerCase()}`} content={help}/>} {required && <span aria-hidden="true"><TagBadge label="Required" className="badge-small"/></span>}</div>
    {renderControl ? renderControl(control) : control}
    {hint && <p id={`${id}-hint`} className="supporting-copy text-ink-muted">{hint}</p>}
    {errors.length > 0 && <div id={`${id}-error`} className="flex items-start gap-1.5 text-xs leading-relaxed text-red-700"><AlertCircle aria-hidden="true" className="mt-0.5 size-3.5 shrink-0"/><div>{errors.map((error, i) => <p key={i}>{error.message}</p>)}</div></div>}
  </div>
}
export function FormErrors({ validation, title = 'The changes could not be saved' }: { validation: Validation; title?: string }) {
  if (!validation.issues.length) return null
  return <div data-validation-summary tabIndex={-1} className="scroll-mt-28 rounded-xl focus-visible:outline-2 focus-visible:outline-red-700"><ErrorBox title={title}><p className="mb-2">Review the details below. Your entered values are kept.</p><ul className="space-y-1.5">{validation.issues.map((issue, i) => <li key={i}>{issue.field ? <button type="button" className="inline-flex items-start gap-1 text-left underline decoration-red-300 underline-offset-2 hover:decoration-red-700" onClick={() => focusField(validation.root.current, issue.field)}>{fieldLabel(issue.field)}: {issue.message}<ArrowUpRight aria-hidden="true" className="mt-0.5 size-3 shrink-0"/></button> : issue.message}</li>)}</ul></ErrorBox></div>
}

export function FormGroup({name,label,children}:{name:string;label:string;children:ReactNode}) {
  const validation=useContext(FormContext)
  const id=useId()
  const errors=validation?.issues.filter(issue=>issue.field===name||issue.field?.startsWith(`${name}.`))??[]
  return <div role="group" aria-label={label} aria-invalid={errors.length?true:undefined} aria-describedby={errors.length?id:undefined} data-validation-field={name} tabIndex={-1} className="min-w-0 rounded-xl" onChangeCapture={()=>validation?.clear(name)}>{children}{errors.length>0&&<div id={id} className="mx-5 mb-4 flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 p-3 text-xs text-red-700"><AlertCircle aria-hidden="true" className="size-3.5 shrink-0"/><div>{errors.map((issue,i)=><p key={i}>{issue.message}</p>)}</div></div>}</div>
}
