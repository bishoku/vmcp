import { useCallback, useEffect, useLayoutEffect, useRef, type RefObject } from 'react'

type MotionKind = 'popup' | 'dialog' | 'drawer'

/** Animate an explicit dismissal before releasing the overlay and restoring focus. */
export function useMotionDismiss<T extends HTMLElement>(ref: RefObject<T | null>, onDismiss: (() => void) | undefined, kind: MotionKind = 'popup') {
 const pending = useRef<{ animation: Animation; cancel: () => void } | null>(null)
 const callback=useRef(onDismiss)
 useLayoutEffect(()=>{callback.current=onDismiss},[onDismiss])
 useEffect(() => () => pending.current?.cancel(), [])
 return useCallback(() => {
  const onDismiss=callback.current
  if (!onDismiss || pending.current) return
  const root=ref.current
  const target=kind==='popup'?root:root?.querySelector<HTMLElement>('.surface-dialog, .surface-drawer')
  const preference=window.matchMedia?.('(prefers-reduced-motion: reduce)')
  if (!target?.animate || preference?.matches) { onDismiss(); return }
  const duration=parseFloat(getComputedStyle(target).getPropertyValue('--motion-duration-exit')) || 120
  const transform=kind==='drawer'?'translateX(12px)':kind==='dialog'?'scale(.99)':'translateY(3px)'
  const animation=target.animate([{opacity:1,transform:'translate(0)'},{opacity:0,transform}],{duration,easing:'cubic-bezier(.4, 0, 1, 1)',fill:'forwards'})
  let cancelled=false
  const wasInert=target.hasAttribute('inert')
  target.setAttribute('inert','')
  target.setAttribute('data-motion-closing','true')
  const restore=()=>{target.toggleAttribute('inert',wasInert);target.removeAttribute('data-motion-closing');preference?.removeEventListener('change',reduce);pending.current=null}
  const finish=()=>{if(cancelled)return;cancelled=true;restore();onDismiss();animation.cancel()}
  const reduce=()=>{if(preference?.matches)finish()}
  pending.current={animation,cancel:()=>{cancelled=true;restore();animation.cancel()}}
  preference?.addEventListener('change',reduce)
  void animation.finished.then(finish,finish)
 },[ref,kind])
}
