import { useEffect, useRef } from 'react'
/** Select a deep-linked record once, without replacing a subsequently edited draft on refresh. */
export function useInitialSelection<T extends {slug:string}>(slug: string | undefined, items: T[], select: (item:T)=>void) {
  const selected = useRef(false)
  useEffect(()=>{
    if (!slug || selected.current) return
    const item = items.find(item=>item.slug===slug)
    if (item) { selected.current=true; queueMicrotask(()=>select(item)) }
  },[slug,items,select])
}
