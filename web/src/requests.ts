import { useCallback, useEffect, useRef, useState } from 'react'

interface Source { name: string; load: (signal: AbortSignal) => Promise<void> }
export function source<T>(name: string, read: (signal: AbortSignal) => Promise<T>, accept: (data: T) => void): Source {
  return { name, load: async signal => {
    const data = await read(signal)
    if (!signal.aborted) accept(data)
  } }
}

/** Read independent panels independently, retaining successful data on partial failure. */
export async function readSources(sources: Source[], signal: AbortSignal) {
  const results = await Promise.allSettled(sources.map(source => source.load(signal)))
  return results.flatMap((result, index) => result.status === 'rejected'
    ? [`${sources[index].name}: ${result.reason instanceof Error ? result.reason.message : String(result.reason)}`] : []).join('; ')
}

/** Page catalogs have a stable source set. Replaced reads and unmounted pages are cancelled. */
export function useSources(factory: () => Source[]) {
  const [sources] = useState(factory)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const control = useRef<AbortController | null>(null)
  const load = useCallback(async () => {
    control.current?.abort()
    const controller = new AbortController()
    control.current = controller
    setLoading(true)
    try {
      const failures = await readSources(sources, controller.signal)
      if (!controller.signal.aborted) setError(failures)
    } finally {
      if (!controller.signal.aborted) setLoading(false)
    }
  }, [sources, control])
  useEffect(() => () => control.current?.abort(), [control])
  return { load, loading, readError: error }
}
