import Button from './Button'
import { useNotifications } from './notifications'
import { useEffect, useId, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { AlertCircle, AlertTriangle, Check, Copy, RefreshCw, PanelRightOpen, ScanLine, Trash2, X } from 'lucide-react'
import clsx from 'clsx'

import { useConfirm } from './confirmation'
import { btn, primary } from './styles'
export { StatusBadge as Pill } from './Badge'

export function ErrorBox({ children, title='Unable to complete this request', onRetry, retryLabel='Try again', busy=false }: { children: ReactNode; title?:string; onRetry?:()=>void|Promise<void>; retryLabel?:string; busy?:boolean }) {
  return <div role="alert" className="flex min-w-0 flex-wrap items-start gap-3 rounded-xl border border-red-200 bg-red-50 p-4"><span aria-hidden="true" className="grid size-8 shrink-0 place-items-center rounded-lg border border-red-200 bg-surface text-red-700"><AlertCircle className="size-4"/></span><div className="min-w-0 flex-[1_1_180px]"><p className="text-xs font-semibold text-red-800">{title}</p><div className="mt-1.5 break-words text-xs leading-relaxed text-red-700">{children}</div></div>{onRetry&&<Button className="shrink-0 text-xs" loading={busy} onClick={()=>void onRetry()}><RefreshCw aria-hidden="true" className="size-3.5"/>{retryLabel}</Button>}</div>
}



export function Modal({ title, children, onClose, variant = 'modal', busy = false, readOnly = false, size = 'standard', description, tone, initialFocus, returnFocus }: { title: string; children: ReactNode; onClose?: () => void; variant?: 'modal' | 'drawer'; busy?: boolean; readOnly?: boolean; size?: 'standard' | 'wide'; description?:string; tone?:'danger'|'warning'; initialFocus?:string; returnFocus?:HTMLElement|null }) {
  const ref = useRef<HTMLDialogElement>(null)
  const titleId = useId()
  const descriptionId=useId()
  const Icon=tone==='danger'?Trash2:tone==='warning'?AlertTriangle:variant==='drawer'?PanelRightOpen:ScanLine
  useLayoutEffect(() => {
    const dialog = ref.current!
    const trigger = returnFocus ?? (document.activeElement instanceof HTMLElement ? document.activeElement : null)
    dialog.showModal()
    const first=initialFocus?dialog.querySelector<HTMLElement>(initialFocus):null
    ;(first??dialog.querySelector<HTMLElement>('button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled])'))?.focus()
    return () => {
      dialog.close()
      const restore=()=>{if(trigger?.isConnected&&!trigger.matches(':disabled'))trigger.focus()}
      restore()
      if(trigger?.matches(':disabled'))requestAnimationFrame(restore)
    }
  }, [initialFocus,returnFocus])
  return (
    <dialog ref={ref} aria-labelledby={titleId} aria-describedby={description?descriptionId:undefined} aria-modal="true"
      className="fixed inset-0 m-0 h-full max-h-none w-full max-w-none border-0 bg-transparent p-0 text-ink backdrop:bg-slate-950/40 backdrop:backdrop-blur-[2px]"
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
        <section className={clsx('flex min-h-0 flex-col overflow-hidden border border-line bg-canvas shadow-card',
          variant === 'drawer' ? `h-full w-full ${size === 'wide' ? 'max-w-2xl' : 'max-w-[420px]'}` : `max-h-[calc(100dvh-2rem)] w-full ${size === 'wide' ? 'max-w-4xl' : 'max-w-lg'} rounded-card`)}>
          <div className="flex shrink-0 items-start justify-between gap-3 border-b border-line bg-surface px-5 py-4 sm:px-6">
            <div className="flex min-w-0 items-start gap-3"><div aria-hidden="true" className={clsx('grid size-9 shrink-0 place-items-center rounded-xl border',tone==='danger'?'border-red-200 bg-red-50 text-red-700':tone==='warning'?'border-amber-200 bg-amber-50 text-amber-700':'border-accent-100 bg-accent-50 text-accent-700')}><Icon className="size-4"/></div><div className="min-w-0 pt-1"><h2 id={titleId} className="dialog-title">{title}</h2>{description&&<p id={descriptionId} className="supporting-copy mt-2 whitespace-pre-line text-ink-muted">{description}</p>}</div></div>
            {onClose && <button type="button" className={`${btn} shrink-0`} aria-label="Close" disabled={busy} onClick={onClose}><X className="size-4" /></button>}
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto overscroll-y-contain"><fieldset disabled={busy || readOnly} className="min-w-0 border-0 m-0 p-5 sm:p-6">{children}</fieldset></div>
        </section>
      </div>
    </dialog>
  )
}

/** Shows a freshly issued secret exactly once. */
export function SecretModal({ title, secret, onClose }: { title: string; secret: string; onClose: () => void }) {
  return (
    <Modal title={title}>
      <p className="rounded-xl border border-accent-100 bg-accent-50 p-4 text-xs leading-relaxed text-accent-700">Copy it now. It won't be shown again.</p>
      <div className="mt-4 flex items-center gap-3 rounded-xl border border-line bg-surface p-4">
        <code className="flex-1 break-all text-xs">{secret}</code>
        <CopyButton value={secret} />
      </div>
      <div className="mt-6 flex justify-end border-t border-line pt-4">
        <button className={primary} onClick={onClose}>
          I have saved it
        </button>
      </div>
    </Modal>
  )
}

export function CopyButton({ value, label = 'Copy' }: { value: string | null; label?: string }) {
  const {notify,available}=useNotifications()
  const timer=useRef<ReturnType<typeof setTimeout>|null>(null)
  useEffect(()=>()=>{if(timer.current)clearTimeout(timer.current)},[])
  const [copied, setCopied] = useState<string | null>(null)
  const [copying,setCopying] = useState(false)
  const [error, setError] = useState('')
  return <span><Button aria-label={label} loading={copying} loadingLabel="Copying…" disabled={!value} onClick={async () => {
    if(copying)return
    setCopying(true)
    try { await navigator.clipboard.writeText(value!); setCopied(value); setError(''); notify({title:'Copied to clipboard',description:label==='Copy'?'Ready to paste.':`${label.replace(/^Copy /,'')} is ready to paste.`,key:'clipboard'}); if(timer.current)clearTimeout(timer.current);timer.current=setTimeout(()=>setCopied(null),2500) }
    catch { setCopied(null); setError('Copy failed. Select the text and copy it manually.') }
    finally { setCopying(false) }
  }}>{copied === value && value ? <Check className="size-4" /> : <Copy className="size-4" />}{copied === value && value ? 'Copied' : 'Copy'}</Button>
    <span role={available?undefined:"status"} className="sr-only">{!available && copied === value && value ? `${label} succeeded` : ''}</span>
    {error && <span role="alert" className="mt-2 block rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs leading-relaxed text-red-700">{error}</span>}
  </span>
}

export function StaleRecovery({ message, reload }: { message: string; reload: () => Promise<void> }) {
  const confirm=useConfirm()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  if (!message.includes('another session')) return null
  return <div className="mt-3 space-y-3 rounded-xl border border-amber-200 bg-amber-50 p-4 text-xs leading-relaxed text-amber-900"><p className="inline-flex items-center gap-2 font-semibold"><AlertTriangle className="size-4"/>A newer version is available</p><p>Your draft is retained. Copy any changes you want to keep before loading the latest version.</p>
    <Button variant="danger-outline" loading={busy} onClick={async () => {
      if (!await confirm('Discard this draft and load the latest version?')) return
      setBusy(true)
      try { await reload(); setError('') } catch (e) { setError(e instanceof Error ? e.message : String(e)) }
      finally { setBusy(false) }
    }}>Reload latest and discard draft</Button>
    {error && <p role="alert">{error}</p>}
  </div>
}
