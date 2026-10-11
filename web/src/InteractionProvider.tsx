import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useBlocker } from 'react-router-dom'
import { Drafts, discardChanges } from './interaction'
export default function InteractionProvider({ children }: { children: ReactNode }) {
  const [drafts] = useState(() => new Map<symbol, 'dirty' | 'busy'>())
  const [notice, setNotice] = useState('')
  const handled = useRef<string|null>(null)
  const blocker = useBlocker(() => [...drafts.values()].some(Boolean))
  useEffect(() => {
    if (blocker.state === 'blocked') {
      if(handled.current===blocker.location.key)return
      handled.current=blocker.location.key
      if ([...drafts.values()].includes('busy')) { queueMicrotask(()=>setNotice('Wait for the current operation to finish before leaving.')); blocker.reset() }
      else if (discardChanges()) blocker.proceed()
      else blocker.reset()
    } else handled.current=null
  }, [blocker, drafts])
  return <Drafts.Provider value={drafts}>{notice && <div role="status" className="fixed right-4 top-4 z-50 rounded border bg-surface p-3">{notice}<button className="ml-3" onClick={()=>setNotice('')}>Dismiss</button></div>}{children}</Drafts.Provider>
}

