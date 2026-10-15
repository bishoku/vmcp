import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Check, Info, AlertTriangle, X } from 'lucide-react'
import { NotificationContext, type Notification } from './notifications'

type Entry = Notification & { id: number }
function Toast({ notice, dismiss }: { notice: Entry; dismiss: (id: number) => void }) {
  const [hovered, setHovered] = useState(false)
  const [focused, setFocused] = useState(false)
  const [hidden, setHidden] = useState(document.hidden)
  const remaining = useRef(notice.duration ?? 6000)
  useEffect(() => {
    const change = () => setHidden(document.hidden)
    document.addEventListener('visibilitychange', change)
    return () => document.removeEventListener('visibilitychange', change)
  }, [])
  useEffect(() => {
    if (hovered || focused || hidden || notice.duration === 0) return
    const start = Date.now()
    const timer = setTimeout(() => dismiss(notice.id), remaining.current)
    return () => { clearTimeout(timer); remaining.current = Math.max(0, remaining.current - (Date.now() - start)) }
  }, [hovered, focused, hidden, notice.id, notice.duration, dismiss])
  const tone = notice.tone ?? 'success'
  const Icon = tone === 'warning' ? AlertTriangle : tone === 'info' ? Info : Check
  const color = tone === 'warning' ? 'border-amber-200 bg-amber-50 text-amber-700' : tone === 'info' ? 'border-accent-100 bg-accent-50 text-accent-700' : 'border-emerald-200 bg-emerald-50 text-emerald-700'
  return <div data-testid="toast" className="pointer-events-auto flex items-start gap-3 rounded-2xl border border-line bg-surface p-4 text-ink shadow-lg" onMouseEnter={() => setHovered(true)} onMouseLeave={() => setHovered(false)} onFocusCapture={() => setFocused(true)} onBlurCapture={e => { if (!e.currentTarget.contains(e.relatedTarget)) setFocused(false) }}>
    <span aria-hidden="true" className={`grid size-9 shrink-0 place-items-center rounded-xl border ${color}`}><Icon className="size-4"/></span>
    <div role="status" aria-atomic="true" className="min-w-0 flex-1 pt-0.5"><p className="break-words text-sm font-semibold tracking-tight">{notice.title}</p>{notice.description && <p className="mt-1 break-words text-xs leading-relaxed text-ink-muted">{notice.description}</p>}</div>
    <button type="button" aria-label={`Dismiss notification: ${notice.title}`} className="shrink-0 rounded-lg p-1.5 text-ink-muted transition-colors hover:bg-canvas focus-visible:outline-2 focus-visible:outline-accent-700" onClick={() => dismiss(notice.id)}><X aria-hidden="true" className="size-4"/></button>
  </div>
}

export default function NotificationsProvider({ children }: { children: ReactNode }) {
  const [notices, setNotices] = useState<Entry[]>([])
  const [host, setHost] = useState<Element>(document.body)
  const sequence = useRef(0)
  const notify = useCallback((notice: Notification) => {
    const entry = { ...notice, id: ++sequence.current }
    setNotices(current => [...current.filter(item => !notice.key || item.key !== notice.key), entry].slice(-3))
  }, [])
  const dismiss = useCallback((id: number) => setNotices(current => current.filter(item => item.id !== id)), [])
  useEffect(() => {
    // Native modal dialogs make the rest of the document inert. Keep notifications in the active top layer.
    const update = () => setHost(Array.from(document.querySelectorAll('dialog[open]')).at(-1) ?? document.body)
    update()
    const observer = new MutationObserver(update)
    observer.observe(document.body, { subtree: true, childList: true, attributes: true, attributeFilter: ['open'] })
    return () => observer.disconnect()
  }, [])
  const value = useMemo(() => ({ available: true, notify }), [notify])
  return <NotificationContext.Provider value={value}>{children}{notices.length>0&&createPortal(<div role="region" aria-label="Notifications" className="pointer-events-none fixed inset-x-4 bottom-[max(1rem,env(safe-area-inset-bottom))] z-[100] flex flex-col gap-3 sm:left-auto sm:right-6 sm:w-96">{notices.map(notice => <Toast key={notice.id} notice={notice} dismiss={dismiss}/>)}</div>, host)}</NotificationContext.Provider>
}
