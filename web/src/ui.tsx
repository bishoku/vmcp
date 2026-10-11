import { useState, type ReactNode } from 'react'
import { Check, Copy } from 'lucide-react'
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
  return <div className="mt-6 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{children}</div>
}



export function Modal({ title, children, onClose }: { title: string; children: ReactNode; onClose?: () => void }) {
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-black/30 p-4" onClick={onClose}>
      <div className="w-full max-w-lg rounded-card border border-line bg-surface p-6 shadow-card" onClick={(e) => e.stopPropagation()}>
        <h2 className="text-lg font-semibold">{title}</h2>
        {children}
      </div>
    </div>
  )
}

/** Shows a freshly issued secret exactly once. */
export function SecretModal({ title, secret, onClose }: { title: string; secret: string; onClose: () => void }) {
  const [copied, setCopied] = useState(false)
  return (
    <Modal title={title}>
      <p className="mt-1 text-sm text-ink-muted">Copy it now. It won't be shown again.</p>
      <div className="mt-4 flex items-center gap-2 rounded-lg bg-canvas px-3 py-2">
        <code className="flex-1 break-all text-xs">{secret}</code>
        <button
          className={btn}
          onClick={async () => {
            await navigator.clipboard.writeText(secret)
            setCopied(true)
          }}
        >
          {copied ? <Check className="size-4" /> : <Copy className="size-4" />} {copied ? 'Copied' : 'Copy'}
        </button>
      </div>
      <div className="mt-6 flex justify-end">
        <button className={primary} onClick={onClose}>
          I have saved it
        </button>
      </div>
    </Modal>
  )
}
