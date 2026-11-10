import type { ReactNode, ReactElement, InputHTMLAttributes, ComponentProps } from 'react'
import { ShieldCheck } from 'lucide-react'
import { FormField } from '../FormField'
import { ServerPanel } from '../connections/ServerPanels'
import { ErrorBox } from '../ui'
import Select from '../Select'
import { input } from '../styles'
export function Field({
  label,
  children,
  hint,
}: {
  label: string
  children: ReactElement<InputHTMLAttributes<HTMLInputElement>>
  hint?: ReactNode
}) {
  return (
    <FormField name={label} label={label} hint={hint} required={children.props.required}>
      {children}
    </FormField>
  )
}
export function AccessSection({
  title,
  description,
  children,
}: {
  title: string
  description: string
  children: ReactNode
}) {
  return (
    <ServerPanel title={title} description={description} icon={ShieldCheck} headingLevel={3}>
      <div className="panel-body space-y-5">{children}</div>
    </ServerPanel>
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
