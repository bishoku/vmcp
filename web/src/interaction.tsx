import { createContext, useContext, useEffect, useRef, useState, type Dispatch, type SetStateAction } from 'react'
import type { Me } from './api'

export const RoleContext = createContext<Me['role']>('viewer')
export const capabilities = (role: Me['role']) => ({ edit: role !== 'viewer', manageTokens: role === 'admin' })
export const useCapabilities = () => capabilities(useContext(RoleContext))
export const Drafts = createContext<Map<symbol, 'dirty' | 'busy'> | null>(null)
export const discardChanges = () => window.confirm('Discard unsaved changes?')
/** Tracks a draft baseline without retaining its secrets in browser storage. */
export function useDraft<T>(initial: T, identity: (value: T) => unknown) {
  const [value, rawSet] = useState(initial)
  const [baseline, setBaseline] = useState(() => JSON.stringify(initial))
  const current = useRef(value)
  useEffect(() => { current.current = value }, [value])
  const dirty = JSON.stringify(value) !== baseline
  const registry = useContext(Drafts)
  const [id] = useState(() => Symbol())
  useEffect(() => {
    if (dirty) registry?.set(id, 'dirty'); else registry?.delete(id)
    const unload = (event: BeforeUnloadEvent) => { if (dirty) { event.preventDefault(); event.returnValue = '' } }
    window.addEventListener('beforeunload', unload)
    return () => { registry?.delete(id); window.removeEventListener('beforeunload', unload) }
  }, [dirty, registry, id])
  const reset = (next: T) => { setBaseline(JSON.stringify(next)); current.current = next; rawSet(next) }
  const set: Dispatch<SetStateAction<T>> = next => {
    const resolved = typeof next === 'function' ? (next as (v: T) => T)(current.current) : next
    if (registry && [...registry.values()].includes('busy')) return
    if (identity(resolved) !== identity(current.current)) {
      if (JSON.stringify(current.current) !== baseline && !discardChanges()) return
      reset(resolved)
    } else { current.current = resolved; rawSet(resolved) }
  }
  const open = (next: T) => {
    if (registry && [...registry.values()].includes('busy')) return
    if (JSON.stringify(current.current) !== baseline && !discardChanges()) return
    reset(next)
  }
  return [value, set, reset, open] as const
}

/** A synchronous latch also catches two clicks before React renders disabled controls. */
export function useOperation() {
  const registry = useContext(Drafts)
  const [id] = useState(() => Symbol())
  useEffect(() => () => { registry?.delete(id) }, [registry, id])
  const lock = useRef(false)
  const [busy, setBusy] = useState(false)
  const run = async (operation: () => Promise<void>) => {
    if (lock.current) return
    lock.current = true; registry?.set(id, 'busy'); setBusy(true)
    try { await operation() } finally { lock.current = false; registry?.delete(id); setBusy(false) }
  }
  return { busy, run }
}

export function useUnsaved(value: unknown) {
  const [baseline, setBaseline] = useState(() => JSON.stringify(value))
  const dirty = JSON.stringify(value) !== baseline
  const registry = useContext(Drafts)
  const [id] = useState(() => Symbol())
  useEffect(() => {
    if (dirty) registry?.set(id, 'dirty'); else registry?.delete(id)
    const unload = (event: BeforeUnloadEvent) => { if (dirty) { event.preventDefault(); event.returnValue = '' } }
    window.addEventListener('beforeunload', unload)
    return () => { registry?.delete(id); window.removeEventListener('beforeunload', unload) }
  }, [dirty, registry, id])
  return { guard: (action: () => void) => { if (!dirty || discardChanges()) action() }, clean: (next: unknown = value) => { setBaseline(JSON.stringify(next)); registry?.delete(id) } }
}
