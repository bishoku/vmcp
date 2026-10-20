import DescriptionComparison from './DescriptionComparison'
import { useContext, useId, useState, useSyncExternalStore } from 'react'
import { FileText, Pencil } from 'lucide-react'
import { FormField } from './FormField'
import { FormContext } from './form-validation'
import { discardConfirmation, useConfirm } from './confirmation'
import { TagBadge } from './Badge'
import { Modal } from './ui'
import { btn, input, primary } from './styles'

import { DESCRIPTION_LIMIT, descriptionLength } from './description'

const desktopQuery = '(min-width: 1024px)'
const isDesktop = () => window.matchMedia?.(desktopQuery).matches ?? false
const subscribeDesktop = (notify: () => void) => {
  const media = window.matchMedia?.(desktopQuery)
  media?.addEventListener('change', notify)
  return () => media?.removeEventListener('change', notify)
}

type Props = {
  name: string
  identity: string
  exposedName: string
  value: string
  upstream: string
  drift?: boolean
  onChange?: (value: string) => void
}

/** Keeps long descriptions out of lists; edits remain local until explicitly applied. */
export default function DescriptionOverride(props: Props) {
  const [open, setOpen] = useState(false)
  const validation = useContext(FormContext)
  const effective = props.value || props.upstream
  return <div className="min-w-0 space-y-2">
    <div className="flex flex-wrap gap-2"><TagBadge label={props.value ? 'Description override' : 'Upstream description'}/>{props.drift && <TagBadge label="Upstream changed"/>}</div>
    <p className="line-clamp-3 whitespace-pre-wrap break-words text-xs leading-relaxed text-ink-muted [overflow-wrap:anywhere]">{effective || 'No description provided.'}</p>
    <FormField name={props.name} label="Description" hint={props.onChange ? 'Changes apply to this draft. Save the virtual server to publish them.' : undefined}>
      <button type="button" className={btn+' text-xs'} aria-label={`${props.onChange ? 'Edit' : 'Inspect'} description for ${props.identity}`} onClick={() => setOpen(true)}>{props.onChange ? <Pencil aria-hidden="true" className="icon-inline"/> : <FileText aria-hidden="true" className="icon-inline"/>}{props.onChange ? 'Edit description' : 'Inspect description'}</button>
    </FormField>
    {open && <DescriptionEditor {...props} onClose={() => setOpen(false)} onApply={value => { props.onChange?.(value); validation?.clear(props.name); setOpen(false) }}/>}
  </div>
}

function DescriptionEditor({identity, exposedName, value, upstream, drift, onChange, onClose, onApply}: Props & {onClose: () => void; onApply: (value: string) => void}) {
  const [draft, setDraft] = useState(value)
  const [view, setView] = useState<'edit' | 'upstream' | 'agent' | 'compare'>(onChange ? 'edit' : 'agent')
  const desktop = useSyncExternalStore(subscribeDesktop, isDesktop)
  const activeView = view === 'compare' && !desktop ? 'agent' : view
  const [confirming, setConfirming] = useState(false)
  const confirm = useConfirm()
  const id = useId()
  const count = descriptionLength(draft)
  const invalid = count > DESCRIPTION_LIMIT
  const close = async () => {
    if (confirming) return
    if (draft !== value) {
      setConfirming(true)
      try { if (!await confirm(discardConfirmation)) return } finally { setConfirming(false) }
    }
    onClose()
  }
  return <Modal title="Capability description" description={identity} size="wide" initialFocus={onChange ? 'textarea' : undefined} onClose={() => void close()} busy={confirming} footer={<div className="flex flex-wrap justify-end gap-3"><button type="button" className={btn} onClick={() => void close()}>{onChange ? 'Cancel' : 'Close'}</button>{onChange && <button type="button" className={primary} disabled={invalid} onClick={() => onApply(draft)}>Apply to draft</button>}</div>}>
    <div className="space-y-5">
      {drift && <p className="rounded-xl border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">The upstream description changed since this override was saved. Review the current upstream text before saving the virtual server.</p>}
      <div role="group" aria-label="Description views" className="flex flex-wrap gap-2">
        {(['edit', 'upstream', 'agent', 'compare'] as const).filter(tab => (tab !== 'edit' || onChange) && (tab !== 'compare' || desktop)).map(tab => <button key={tab} type="button" aria-pressed={activeView === tab} className={activeView === tab ? primary : btn} onClick={() => setView(tab)}>{tab === 'edit' ? 'Edit override' : tab === 'upstream' ? 'Upstream' : tab === 'compare' ? 'Compare' : 'Agent view'}</button>)}
      </div>
      {activeView === 'compare' ? <DescriptionComparison upstream={upstream} override={draft}/> : activeView === 'edit' ? <div className="space-y-3">
        <label htmlFor={id} className="field-label">Description override</label>
        <p id={id+'-hint'} className="text-xs leading-relaxed text-ink-muted">Plain text. Line breaks and spacing are preserved. Leave empty to use the upstream description.</p>
        <textarea id={id} className={input+' min-h-80 resize-y text-sm leading-7'} value={draft} onChange={event => setDraft(event.target.value)} aria-invalid={invalid || undefined} aria-describedby={`${id}-hint ${id}-count${invalid ? ` ${id}-error` : ''}`} spellCheck={false}/>
        <div className="flex flex-wrap items-center justify-between gap-3"><div className="flex flex-wrap gap-2"><button type="button" className={btn} disabled={!upstream || draft === upstream} onClick={() => setDraft(upstream)}>Start from upstream</button><button type="button" className={btn} disabled={!draft} onClick={() => setDraft('')}>Use upstream description</button></div><p id={id+'-count'} className="text-xs text-ink-muted">{count.toLocaleString('en-US')} / 20,000 characters</p></div>
      </div> : <section aria-label={activeView === 'upstream' ? 'Upstream description' : 'Agent description'} className="space-y-3">
        <div className="flex items-start gap-2"><FileText aria-hidden="true" className="icon-control shrink-0 text-accent-700"/><div className="min-w-0"><h3 className="break-words text-sm font-semibold [overflow-wrap:anywhere]">{activeView === 'upstream' ? 'Current upstream description' : exposedName}</h3><p className="mt-1 text-xs text-ink-muted">{activeView === 'upstream' ? 'Read-only source from stored upstream discovery.' : `${draft ? 'Override' : 'Upstream description'} · configuration preview before any server masking rules.`}</p></div></div>
        <div className="surface-inset min-h-64 whitespace-pre-wrap break-words p-5 text-sm leading-7 [overflow-wrap:anywhere]">{(activeView === 'upstream' ? upstream : draft || upstream) || 'No description provided.'}</div>
      </section>}
      {invalid && <p id={id+'-error'} role="alert" className="text-sm text-red-700">Use 20,000 characters or fewer. Your text has been kept.</p>}

    </div>
  </Modal>
}
