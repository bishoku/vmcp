import { useEffect, useId, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Info } from 'lucide-react'

type Tip = { anchor: Element; content: string; source: 'pointer' | 'focus' }

/** Delegated hints keep buttons/links and their existing focus/disabled semantics intact. */
export default function TooltipProvider({ children }: { children: ReactNode }) {
  const [tip, setTip] = useState<Tip | null>(null)
  const active = useRef<Tip | null>(null)
  const popup = useRef<HTMLDivElement>(null)
  const id = useId()
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | undefined
    let recentPointer = 0
    let dismissed: Element | null = null
    const clearTimer = () => { clearTimeout(timer); timer = undefined }
    const dismiss = () => { clearTimer(); active.current = null; setTip(null) }
    const target = (node: EventTarget | null) => node instanceof Element ? node.closest('[data-tooltip]') : null
    const show = (anchor: Element, source: Tip['source']) => {
      clearTimer()
      const content = anchor.getAttribute('data-tooltip')?.trim()
      if (!content || !anchor.isConnected || dismissed === anchor) return
      const next = { anchor, content, source }
      active.current = next; setTip(next)
    }
    const over = (event: PointerEvent) => {
      if (popup.current?.contains(event.target as Node)) { clearTimer(); return }
      const anchor = target(event.target)
      if (!anchor || event.pointerType === 'touch') return
      clearTimer()
      if (active.current?.anchor === anchor) return
      timer = setTimeout(() => show(anchor, 'pointer'), 350)
    }
    const out = (event: PointerEvent) => {
      const anchor = target(event.target)
      const inPopup = popup.current?.contains(event.target as Node)
      if (!anchor && !inPopup) return
      if (event.relatedTarget instanceof Node && (anchor?.contains(event.relatedTarget) || popup.current?.contains(event.relatedTarget))) return
      clearTimer()
      if (dismissed === anchor) dismissed = null
      if (active.current?.source === 'focus' && active.current.anchor === document.activeElement) return
      timer = setTimeout(dismiss, 150)
    }
    const focus = (event: FocusEvent) => {
      const anchor = target(event.target)
      if (!anchor) { dismiss(); return }
      if (Date.now() - recentPointer < 100 && !anchor.hasAttribute('data-tooltip-help')) return
      show(anchor, 'focus')
    }
    const blur = (event: FocusEvent) => {
      dismissed = null
      if (event.relatedTarget instanceof Node && popup.current?.contains(event.relatedTarget)) return
      dismiss()
    }
    const down = (event: PointerEvent) => {
      recentPointer = Date.now()
      if (popup.current?.contains(event.target as Node)) return
      const anchor = target(event.target)
      if (anchor?.hasAttribute('data-tooltip-help')) { dismissed = null; show(anchor, 'pointer') }
      else dismiss()
    }
    const key = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      clearTimer()
      if (!active.current) return
      dismissed = active.current.anchor
      event.preventDefault(); event.stopPropagation(); dismiss()
    }
    const scroll = (event: Event) => { if (!popup.current?.contains(event.target as Node)) dismiss() }
    const observer = new MutationObserver(() => {
      const current = active.current
      if (!current) return
      const dialog = Array.from(document.querySelectorAll('dialog[open]')).at(-1)
      if (!current.anchor.isConnected || (dialog && !dialog.contains(current.anchor)) || current.anchor.getAttribute('data-tooltip')?.trim() !== current.content || (current.source === 'focus' && current.anchor.matches(':disabled'))) dismiss()
    })
    observer.observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ['open', 'disabled', 'data-tooltip'] })
    document.addEventListener('pointerover', over)
    document.addEventListener('pointerout', out)
    document.addEventListener('pointerdown', down, true)
    document.addEventListener('focusin', focus)
    document.addEventListener('focusout', blur)
    document.addEventListener('keydown', key, true)
    window.addEventListener('scroll', scroll, true)
    window.addEventListener('resize', dismiss)
    return () => {
      clearTimer(); observer.disconnect()
      document.removeEventListener('pointerover', over); document.removeEventListener('pointerout', out)
      document.removeEventListener('pointerdown', down, true); document.removeEventListener('focusin', focus)
      document.removeEventListener('focusout', blur); document.removeEventListener('keydown', key, true)
      window.removeEventListener('scroll', scroll, true); window.removeEventListener('resize', dismiss)
    }
  }, [])
  useLayoutEffect(() => {
    if (!tip || !popup.current) return
    const node = popup.current
    const previous = tip.anchor.getAttribute('aria-describedby')?.split(/\s+/).filter(Boolean) ?? []
    tip.anchor.setAttribute('aria-describedby', [...new Set([...previous, id])].join(' '))
    if(node.showPopover)node.showPopover()
    else node.removeAttribute('popover')
    const anchor = tip.anchor.getBoundingClientRect()
    const bounds = node.getBoundingClientRect()
    const width = document.documentElement.clientWidth || window.innerWidth
    const height = window.innerHeight
    const below = anchor.bottom + 8
    const top = below + bounds.height <= height - 12 ? below : anchor.top - bounds.height - 8
    node.style.left = `${Math.max(12, Math.min(anchor.left + anchor.width / 2 - bounds.width / 2, width - bounds.width - 12))}px`
    node.style.top = `${Math.max(12, Math.min(top, height - bounds.height - 12))}px`
    node.style.visibility = 'visible'
    return () => {
      if(node.isConnected)node.hidePopover?.()
      const remaining = (tip.anchor.getAttribute('aria-describedby') ?? '').split(/\s+/).filter(value => value && value !== id)
      if (remaining.length) tip.anchor.setAttribute('aria-describedby', remaining.join(' '))
      else tip.anchor.removeAttribute('aria-describedby')
    }
  }, [tip, id])
  const host = tip?.anchor.closest('dialog[open]') ?? document.body
  return <>{children}{tip && createPortal(<div ref={popup} id={id} role="tooltip" popover="manual" style={{ visibility: 'hidden' }} className="motion-popup fixed inset-auto z-[100] m-0 max-h-[min(20rem,calc(100dvh-1.5rem))] w-max max-w-[min(20rem,calc(100vw-1.5rem))] overflow-auto rounded-xl border border-slate-700 bg-ink px-3.5 py-3 text-xs leading-relaxed text-white shadow-popup"><div className="flex items-start gap-2"><Info aria-hidden="true" className="mt-0.5 icon-inline shrink-0 text-accent-100"/><p className="min-w-0 whitespace-pre-wrap break-words">{tip.content}</p></div></div>, host)}</>
}
