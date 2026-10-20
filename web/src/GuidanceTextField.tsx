import { useState } from 'react'
import { FormField } from './FormField'
import { Modal } from './ui'
import { btn, primary, input } from './styles'
import { descriptionLength } from './description'
import { discardConfirmation, useConfirm } from './confirmation'

/** Shared long-text editor: applying changes only updates the enclosing draft. */
export default function GuidanceTextField({ label, value, onChange, limit = 20000, name, preview }: {
  label: string
  value: string
  onChange?: (value: string) => void
  limit?: number
  name: string
  preview?: (value: string) => string
}) {
  const [draft, setDraft] = useState<string | null>(null)
  const [view, setView] = useState(false)
  const confirm = useConfirm()
  const close = async () => {
    if (draft !== value && onChange && !await confirm(discardConfirmation)) return
    setDraft(null)
  }
  const count = descriptionLength(draft ?? value)
  return <>
    <FormField name={name} label={label} hint={`${descriptionLength(value).toLocaleString()} / ${limit.toLocaleString()} characters`}
      renderControl={control => <div className="surface-inset mt-2 p-3">
        <p className="line-clamp-3 whitespace-pre-wrap break-words text-xs leading-relaxed text-ink-muted">{value || 'No content configured.'}</p>
        {control}
      </div>}>
      <button type="button" aria-label={`${onChange ? 'Edit' : 'View'} ${label.toLowerCase()}`} className={`${btn} mt-3`} onClick={() => { setDraft(value); setView(!onChange) }}>
        {onChange ? 'Edit' : 'View'} {label.toLowerCase()}
      </button>
    </FormField>
    {draft !== null && <Modal title={label} size="wide"
      description={onChange ? 'Apply changes to the draft, then save the virtual server to publish.' : 'Saved configuration. Preview does not execute tools.'}
      onClose={() => void close()}
      footer={<>
        <button type="button" className={btn} onClick={() => void close()}>{onChange ? 'Cancel' : 'Close'}</button>
        {onChange && <button type="button" className={primary} disabled={count > limit} onClick={() => { onChange(draft); setDraft(null) }}>Apply to draft</button>}
      </>}>
      <div className="space-y-4">
        {preview && <div className="flex gap-2">
          <button type="button" className={btn} aria-pressed={!view} onClick={() => setView(false)}>Source</button>
          <button type="button" className={btn} aria-pressed={view} onClick={() => setView(true)}>Agent preview</button>
        </div>}
        {view
          ? <pre className="surface-inset min-h-64 whitespace-pre-wrap break-words p-4 text-sm leading-relaxed">{preview ? preview(draft) : draft}</pre>
          : <textarea aria-label={label} aria-invalid={count > limit || undefined} className={`${input} min-h-80 font-mono leading-relaxed`} value={draft} readOnly={!onChange} onChange={e => setDraft(e.target.value)} />}
        <p role={count > limit ? 'alert' : undefined} className={`text-xs ${count > limit ? 'text-red-700' : 'text-ink-muted'}`}>{count.toLocaleString()} / {limit.toLocaleString()} characters</p>
      </div>
    </Modal>}
  </>
}
