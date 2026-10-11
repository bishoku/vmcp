import { useId, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { Check, Copy, X } from 'lucide-react'
import clsx from 'clsx'

import { btn, primary } from './styles'
const tones: Record<string, string> = {
  active: 'bg-green-50 text-green-700',
  ok: 'bg-green-50 text-green-700',
  expired: 'bg-amber-50 text-amber-700',
  tool_error: 'bg-amber-50 text-amber-700',
  revoked: 'bg-red-50 text-red-700',
  error: 'bg-red-50 text-red-700',
  denied: 'bg-red-50 text-red-700',
}

export function Pill({ status }: { status: string }) {
  return (
    <span className={clsx('rounded-full px-2 py-0.5 text-xs font-medium', tones[status] ?? 'bg-canvas text-ink-muted')}>
      {status.replace('_', ' ')}
    </span>
  )
}

export function ErrorBox({ children }: { children: ReactNode }) {
  return <div role="alert" className="mt-6 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{children}</div>
}



export function Modal({ title, children, onClose, variant = 'modal', busy = false, readOnly = false }: { title: string; children: ReactNode; onClose?: () => void; variant?: 'modal' | 'drawer'; busy?: boolean; readOnly?: boolean }) {
  const ref = useRef<HTMLDialogElement>(null)
  const titleId = useId()
  useLayoutEffect(() => {
    const dialog = ref.current!
    const trigger = document.activeElement instanceof HTMLElement ? document.activeElement : null
    dialog.showModal()
    dialog.querySelector<HTMLElement>('button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled])')?.focus()
    return () => {
      dialog.close()
      if (trigger?.isConnected) trigger.focus()
    }
  }, [])
  return (
    <dialog ref={ref} aria-labelledby={titleId} aria-modal="true"
      className="fixed inset-0 m-0 h-full max-h-none w-full max-w-none border-0 bg-transparent p-0 text-ink backdrop:bg-black/30"
      onCancel={e => { e.preventDefault(); if (!busy) onClose?.() }}
      onKeyDown={e => {
        if (e.key !== 'Tab') return
        const items = Array.from(e.currentTarget.querySelectorAll<HTMLElement>(
          'button, input, select, textarea, a[href], summary, [tabindex]:not([tabindex="-1"])',
        )).filter(item => !item.matches(':disabled') && item.tabIndex >= 0 && item.getClientRects().length > 0)
        const first = items[0], last = items[items.length - 1]
        if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last?.focus() }
        else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first?.focus() }
      }}>
      <div className={clsx('flex h-full w-full', variant === 'drawer' ? 'justify-end' : 'items-center justify-center p-4')}
        onClick={e => { if (e.target === e.currentTarget) if (!busy) onClose?.() }}>
        <section className={clsx('overflow-y-auto border border-line bg-surface p-6 shadow-card',
          variant === 'drawer' ? 'h-full w-full max-w-[420px]' : 'max-h-full w-full max-w-lg rounded-card')}>
          <div className="mb-4 flex items-center justify-between gap-3">
            <h2 id={titleId} className="text-lg font-semibold">{title}</h2>
            {onClose && <button type="button" className={btn} aria-label="Close" disabled={busy} onClick={onClose}><X className="size-4" /></button>}
          </div>
          <fieldset disabled={busy || readOnly} className="min-w-0 border-0 m-0 p-0">{children}</fieldset>
        </section>
      </div>
    </dialog>
  )
}

/** Shows a freshly issued secret exactly once. */
export function SecretModal({ title, secret, onClose }: { title: string; secret: string; onClose: () => void }) {
  return (
    <Modal title={title}>
      <p className="mt-1 text-sm text-ink-muted">Copy it now. It won't be shown again.</p>
      <div className="mt-4 flex items-center gap-2 rounded-lg bg-canvas px-3 py-2">
        <code className="flex-1 break-all text-xs">{secret}</code>
        <CopyButton value={secret} />
      </div>
      <div className="mt-6 flex justify-end">
        <button className={primary} onClick={onClose}>
          I have saved it
        </button>
      </div>
    </Modal>
  )
}

export function CopyButton({ value, label = 'Copy' }: { value: string | null; label?: string }) {
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState('')
  return <span><button type="button" className={btn} aria-label={label} disabled={!value} onClick={async () => {
    try { await navigator.clipboard.writeText(value!); setCopied(true); setError('') }
    catch { setCopied(false); setError('Copy failed. Select the text and copy it manually.') }
  }}>{copied ? <Check className="size-4" /> : <Copy className="size-4" />}{copied ? 'Copied' : 'Copy'}</button>
    {error && <span role="alert" className="block text-sm text-red-700">{error}</span>}
  </span>
}

export function StaleRecovery({ message, reload }: { message: string; reload: () => Promise<void> }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  if (!message.includes('another session')) return null
  return <div className="mt-3 text-sm"><p>Your draft is retained. Copy any changes you want to keep before loading the latest version.</p>
    <button className={btn} disabled={busy} onClick={async () => {
      if (!confirm('Discard this draft and load the latest version?')) return
      setBusy(true)
      try { await reload(); setError('') } catch (e) { setError(e instanceof Error ? e.message : String(e)) }
      finally { setBusy(false) }
    }}>Reload latest and discard draft</button>
    {error && <p role="alert">{error}</p>}
  </div>
}
