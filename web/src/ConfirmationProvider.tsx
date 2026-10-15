import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { ConfirmationContext, discardConfirmation, type Confirmation } from './confirmation'
import { Modal } from './ui'
import Button from './Button'

function describe(request:string|Confirmation):Confirmation {
  if(typeof request!=='string')return request
  if(request.startsWith('Discard'))return {...discardConfirmation,title:request,confirmLabel:request.includes('latest')?'Load latest and discard':'Discard changes'}
  const action=request.startsWith('Rotate')?'Rotate':request.startsWith('Revoke')?'Revoke':'Delete'
  const [title,...detail]=request.split('?')
  return {title:`${title}?`,description:detail.join('?').trim()||(action==='Delete'?'The selected configuration will be removed.':action==='Revoke'?'This credential will no longer grant access.':'The old secret stops working. Save the replacement secret when it is shown.'),confirmLabel:action,tone:action==='Rotate'?'warning':'danger'}
}

export default function ConfirmationProvider({children}:{children:ReactNode}) {
  const [request,setRequest]=useState<(Confirmation&{returnFocus:HTMLElement|null})|null>(null)
  const pending=useRef<((accepted:boolean)=>void)|null>(null)
  const confirm=useCallback((value:string|Confirmation)=>new Promise<boolean>(resolve=>{
    if(pending.current){resolve(false);return}
    pending.current=resolve;setRequest({...describe(value),returnFocus:document.activeElement instanceof HTMLElement?document.activeElement:null})
  }),[])
  useEffect(()=>()=>{pending.current?.(false);pending.current=null},[])
  const settle=(accepted:boolean)=>{
    const resolve=pending.current;pending.current=null;setRequest(null);resolve?.(accepted)
  }
  return <ConfirmationContext.Provider value={confirm}>{children}{request&&<Modal title={request.title} description={request.description} tone={request.tone??'danger'} onClose={()=>settle(false)} initialFocus="[data-confirm-cancel]" returnFocus={request.returnFocus}>
    <div className="flex flex-col-reverse gap-3 sm:flex-row sm:justify-end" data-testid="confirmation-actions"><Button data-confirm-cancel data-testid="cancel-action" onClick={()=>settle(false)}>{request.cancelLabel??'Cancel'}</Button><Button data-testid="confirm-action" variant={request.tone==='warning'?'primary':'danger'} onClick={()=>settle(true)}>{request.confirmLabel}</Button></div>
  </Modal>}</ConfirmationContext.Provider>
}
