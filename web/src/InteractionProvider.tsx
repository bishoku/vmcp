import { useNotifications } from './notifications'
import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { useBlocker } from 'react-router-dom'
import { discardConfirmation, useConfirm } from './confirmation'
import { Drafts } from './interaction'
export default function InteractionProvider({ children }: { children: ReactNode }) {
  const confirm=useConfirm()
  const [drafts] = useState(() => new Map<symbol, 'dirty' | 'busy'>())
  const {notify}=useNotifications()
  const handled = useRef<string|null>(null)
  const blocker = useBlocker(() => [...drafts.values()].some(Boolean))
  const latestBlocker=useRef(blocker)
  useLayoutEffect(()=>{latestBlocker.current=blocker},[blocker])
  useEffect(() => {
    if (blocker.state === 'blocked') {
      if(handled.current===blocker.location.key)return
      handled.current=blocker.location.key
      if ([...drafts.values()].includes('busy')) { queueMicrotask(()=>notify({title:'Operation in progress',description:'Wait for the current operation to finish before leaving.',tone:'warning',key:'operation-busy'})); blocker.reset() }
      else {
        const key=blocker.location.key
        void Promise.resolve(confirm(discardConfirmation)).then(accepted=>{const current=latestBlocker.current;if(current.state==='blocked'&&current.location.key===key){handled.current=null;if(accepted)current.proceed();else current.reset()}})
      }
    } else handled.current=null
  }, [blocker, drafts,confirm,notify])
  return <Drafts.Provider value={drafts}>{children}</Drafts.Provider>
}

