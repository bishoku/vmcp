import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from 'react'
import { discardConfirmation, useConfirm } from './confirmation'
import type { Me } from './api'

export const RoleContext = createContext<Me['role']>('viewer')
export const Drafts = createContext<Map<symbol, 'dirty' | 'busy'> | null>(null)
/** Tracks a draft baseline without retaining its secrets in browser storage. */
export function useDraft<T>(initial: T, identity: (value: T) => unknown) {
  const confirm = useConfirm()
  const [value, rawSet] = useState(initial)
  const [baseline, setBaseline] = useState(() => JSON.stringify(initial))
  const current = useRef(value)
  const savedBaseline = useRef(baseline)
  const savedValue = useRef(initial)
  // Imperative save/retry decisions must see resets before React effects settle.
  const isDirty = () => JSON.stringify(current.current) !== savedBaseline.current
  const dirty = JSON.stringify(value) !== baseline
  const registry = useContext(Drafts)
  const [id] = useState(() => Symbol())
  useEffect(() => {
    if (dirty) registry?.set(id, 'dirty')
    else registry?.delete(id)
    const unload = (event: BeforeUnloadEvent) => {
      if (dirty) {
        event.preventDefault()
        event.returnValue = ''
      }
    }
    window.addEventListener('beforeunload', unload)
    return () => {
      registry?.delete(id)
      window.removeEventListener('beforeunload', unload)
    }
  }, [dirty, registry, id])
  const reset = (next: T) => {
    registry?.delete(id)
    savedValue.current = next
    savedBaseline.current = JSON.stringify(next)
    setBaseline(savedBaseline.current)
    current.current = next
    rawSet(next)
  }
  const set: Dispatch<SetStateAction<T>> = (next) => {
    const resolved = typeof next === 'function' ? (next as (v: T) => T)(current.current) : next
    if (registry && [...registry.values()].includes('busy')) return
    if (identity(resolved) !== identity(current.current)) {
      if (isDirty()) {
        const before = JSON.stringify(current.current),
          answer = confirm(discardConfirmation)
        const apply = (accepted: boolean) => {
          if (
            accepted &&
            JSON.stringify(current.current) === before &&
            ![...(registry?.values() ?? [])].includes('busy')
          )
            reset(resolved)
        }
        if (typeof answer === 'boolean') apply(answer)
        else void answer.then(apply)
        return
      }
      reset(resolved)
    } else {
      current.current = resolved
      rawSet(resolved)
    }
  }
  const open = (next: T) => {
    if (registry && [...registry.values()].includes('busy')) return
    if (isDirty()) {
      const before = JSON.stringify(current.current),
        answer = confirm(discardConfirmation)
      const apply = (accepted: boolean) => {
        if (
          accepted &&
          JSON.stringify(current.current) === before &&
          ![...(registry?.values() ?? [])].includes('busy')
        )
          reset(next)
      }
      if (typeof answer === 'boolean') apply(answer)
      else void answer.then(apply)
      return
    }
    reset(next)
  }
  const discard = () => reset(savedValue.current)
  return [value, set, reset, open, dirty, isDirty, discard] as const
}

/** A synchronous latch also catches two clicks before React renders disabled controls. */
export function useOperation() {
  const registry = useContext(Drafts)
  const [id] = useState(() => Symbol())
  useEffect(
    () => () => {
      registry?.delete(id)
    },
    [registry, id],
  )
  const lock = useRef(false)
  const [busy, setBusy] = useState(false)
  const run = async (operation: () => Promise<void>) => {
    if (lock.current) return
    lock.current = true
    registry?.set(id, 'busy')
    setBusy(true)
    try {
      await operation()
    } finally {
      lock.current = false
      registry?.delete(id)
      setBusy(false)
    }
  }
  return { busy, run }
}

export function useUnsaved(value: unknown) {
  const confirm = useConfirm()
  const [baseline, setBaseline] = useState(() => JSON.stringify(value))
  const dirty = JSON.stringify(value) !== baseline
  const registry = useContext(Drafts)
  const [id] = useState(() => Symbol())
  useEffect(() => {
    if (dirty) registry?.set(id, 'dirty')
    else registry?.delete(id)
    const unload = (event: BeforeUnloadEvent) => {
      if (dirty) {
        event.preventDefault()
        event.returnValue = ''
      }
    }
    window.addEventListener('beforeunload', unload)
    return () => {
      registry?.delete(id)
      window.removeEventListener('beforeunload', unload)
    }
  }, [dirty, registry, id])
  return {
    dirty,
    guard: (action: () => void, otherDirty = false) => {
      if (!dirty && !otherDirty) action()
      else {
        const answer = confirm(discardConfirmation)
        if (typeof answer === 'boolean') {
          if (answer) action()
        } else
          void answer.then((accepted) => {
            if (accepted) action()
          })
      }
    },
    clean: (next: unknown = value) => {
      setBaseline(JSON.stringify(next))
      registry?.delete(id)
    },
  }
}
