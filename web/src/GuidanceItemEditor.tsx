import { useState } from 'react'
import type { Guidance, GuidancePrompt, GuidanceResource, ToolSelection } from './api'
import { FormField } from './FormField'
import { Switch, Checkbox } from './SelectionControls'
import Select from './Select'
import { Modal } from './ui'
import { btn, primary, input } from './styles'
import { descriptionLength } from './description'
import { discardConfirmation, useConfirm } from './confirmation'
import { renderGuidance } from './guidance'
import TextField from './GuidanceTextField'

type Item = { kind: 'resource' | 'prompt'; index: number; value: GuidanceResource | GuidancePrompt }
export default function GuidanceItem({ item, guidance, tools, readOnly, onClose, onApply }: {
  item: Item
  guidance: Guidance
  tools: ToolSelection[]
  readOnly: boolean
  onClose: () => void
  onApply: (value: GuidanceResource | GuidancePrompt) => void
}) {
  const [value, setValue] = useState(item.value)
  const [preview, setPreview] = useState(false)
  const [args, setArgs] = useState<Record<string, string>>({})
  const confirm = useConfirm()
  const close = async () => {
    if (!readOnly && JSON.stringify(value) !== JSON.stringify(item.value) && !await confirm(discardConfirmation)) return
    onClose()
  }
  const patch = (update: Partial<GuidanceResource & GuidancePrompt>) => setValue({ ...value, ...update })
  const prompt = 'messages' in value ? value : null
  const field = `guidance.${prompt ? 'prompts' : 'resources'}.${Math.max(0, item.index)}`
  const total = prompt ? prompt.messages.reduce((n, message) => n + descriptionLength(message.text), 0) : descriptionLength((value as GuidanceResource).text)
  const siblings = prompt ? guidance.prompts : guidance.resources
  const nameError = !/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/.test(value.name) || value.name.length > 63
    ? 'Use 1–63 lowercase letters, numbers or hyphens; start and end with a letter or number.'
    : siblings.some((other, index) => index !== item.index && other.name === value.name) ? 'This identifier is already used.' : undefined
  const argumentErrors = prompt?.arguments.map((argument, index) => {
    if (!/^[A-Za-z][A-Za-z0-9_.-]{0,63}$/.test(argument.name)) return 'Start with a letter; use up to 64 letters, numbers, dots, underscores or hyphens.'
    if (prompt.arguments.some((other, otherIndex) => otherIndex !== index && other.name === argument.name)) return 'Argument names must be unique.'
    if (descriptionLength(argument.description) > 1000) return 'Argument descriptions must be at most 1,000 characters.'
    return undefined
  }) ?? []
  const valid = !nameError && !argumentErrors.some(Boolean) && total <= 20000 && descriptionLength(value.description) <= 20000 && descriptionLength(value.title) <= 120 && (!prompt || prompt.messages.length > 0)
  return <Modal title={`${readOnly ? 'View' : item.index < 0 ? 'Add' : 'Edit'} ${item.kind}`} size="wide" onClose={() => void close()}
    description="Content is plain text or Markdown. Agent preview does not run tools or fetch resources; runtime policy and PII checks still apply."
    footer={<>
      <button type="button" className={btn} onClick={() => void close()}>{readOnly ? 'Close' : 'Cancel'}</button>
      {!readOnly && <button type="button" className={primary} disabled={!valid} onClick={() => onApply(value)}>Apply to draft</button>}
    </>}>
    <div className="space-y-5">
      <div className="flex gap-2">
        <button type="button" className={btn} aria-pressed={!preview} onClick={() => setPreview(false)}>Definition</button>
        <button type="button" className={btn} aria-pressed={preview} onClick={() => setPreview(true)}>Agent preview</button>
      </div>
      {preview ? <div className="space-y-4">
        {prompt?.arguments.map((argument, index) => <FormField key={index} name={`preview.${argument.name}`} label={argument.name} required={argument.required} hint={argument.description}>
          <input className={input} value={args[argument.name] ?? ''} onChange={e => setArgs({ ...args, [argument.name]: e.target.value })} />
        </FormField>)}
        {(prompt ? prompt.messages : [{ role: 'resource', text: (value as GuidanceResource).text }]).map((message, index) => <div key={index}>
          <p className="mb-2 text-xs font-semibold uppercase text-ink-muted">{message.role}</p>
          <pre className="surface-inset whitespace-pre-wrap break-words p-4 text-sm leading-relaxed">{renderGuidance(message.text, guidance, tools, { ...Object.fromEntries((prompt?.arguments ?? []).filter(argument => !argument.required).map(argument => [argument.name, ''])), ...args })}</pre>
        </div>)}
      </div> : <>
        <div className="grid gap-4 sm:grid-cols-2">
          <FormField name={`${field}.name`} label="Identifier" required hint="Lowercase letters, numbers and hyphens; up to 63 characters." error={value.name ? nameError : undefined}>
            <input className={input} value={value.name} readOnly={readOnly} onChange={e => patch({ name: e.target.value })} />
          </FormField>
          <FormField name={`${field}.title`} label="Title" error={descriptionLength(value.title) > 120 ? 'Use 120 characters or fewer.' : undefined}>
            <input className={input} value={value.title} readOnly={readOnly} onChange={e => patch({ title: e.target.value })} />
          </FormField>
        </div>
        <Switch label="Publish this item" checked={value.enabled} disabled={readOnly} onChange={e => patch({ enabled: e.target.checked })} />
        <TextField label="Description" name={`${field}.description`} value={value.description} onChange={readOnly ? undefined : description => patch({ description })} />
        {!prompt ? <>
          <FormField name={`${field}.mimeType`} label="Content type">
            <Select className={input} disabled={readOnly} value={(value as GuidanceResource).mimeType} onChange={e => patch({ mimeType: e.target.value as GuidanceResource['mimeType'] })}>
              <option value="text/markdown">Markdown</option><option value="text/plain">Plain text</option>
            </Select>
          </FormField>
          <TextField label="Resource content" name={`${field}.text`} value={(value as GuidanceResource).text} onChange={readOnly ? undefined : text => patch({ text })} preview={text => renderGuidance(text, guidance, tools)} />
        </> : <>
          <section className="space-y-3">
            <h3 className="section-title">Arguments</h3>
            <p className="text-xs text-ink-muted">Insert {'{{arg:name}}'} into a message. Optional arguments default to empty text.</p>
            {prompt.arguments.map((argument, index) => <div className="surface-inset space-y-3 p-3" key={index}>
              <FormField name={`${field}.arguments.${index}.name`} label="Argument name" required error={argument.name ? argumentErrors[index] : undefined}>
                <input className={input} readOnly={readOnly} value={argument.name} onChange={e => patch({ arguments: prompt.arguments.map((other, j) => j === index ? { ...other, name: e.target.value } : other) })} />
              </FormField>
              <FormField name={`${field}.arguments.${index}.description`} label="Argument description" hint="Up to 1,000 characters.">
                <input className={input} readOnly={readOnly} value={argument.description} onChange={e => patch({ arguments: prompt.arguments.map((other, j) => j === index ? { ...other, description: e.target.value } : other) })} />
              </FormField>
              <Checkbox label="Required argument" disabled={readOnly} checked={argument.required} onChange={e => patch({ arguments: prompt.arguments.map((other, j) => j === index ? { ...other, required: e.target.checked } : other) })} />
              {!readOnly && <button type="button" className={btn} onClick={() => patch({ arguments: prompt.arguments.filter((_, j) => j !== index) })}>Remove argument</button>}
            </div>)}
            {!readOnly && <button type="button" className={btn} disabled={prompt.arguments.length >= 20} onClick={() => patch({ arguments: [...prompt.arguments, { name: '', description: '', required: false }] })}>Add argument</button>}
          </section>
          <section className="space-y-4">
            <h3 className="section-title">Messages</h3>
            {prompt.messages.map((message, index) => <div className="surface-inset space-y-3 p-3" key={index}>
              <FormField name={`${field}.messages.${index}.role`} label="Role">
                <Select className={input} disabled={readOnly} value={message.role} onChange={e => patch({ messages: prompt.messages.map((other, j) => j === index ? { ...other, role: e.target.value as 'user' | 'assistant' } : other) })}>
                  <option value="user">User</option><option value="assistant">Assistant</option>
                </Select>
              </FormField>
              <TextField name={`${field}.messages.${index}.text`} label={`Message ${index + 1}`} value={message.text} onChange={readOnly ? undefined : text => patch({ messages: prompt.messages.map((other, j) => j === index ? { ...other, text } : other) })} />
              {!readOnly && <button type="button" className={btn} disabled={prompt.messages.length === 1} onClick={() => patch({ messages: prompt.messages.filter((_, j) => j !== index) })}>Remove message</button>}
            </div>)}
            {!readOnly && <button type="button" className={btn} disabled={prompt.messages.length >= 20} onClick={() => patch({ messages: [...prompt.messages, { role: 'user', text: '' }] })}>Add message</button>}
            <p role={total > 20000 ? 'alert' : undefined} className={`text-xs ${total > 20000 ? 'text-red-700' : 'text-ink-muted'}`}>{total.toLocaleString()} / 20,000 total message characters</p>
          </section>
        </>}
      </>}
    </div>
  </Modal>
}
