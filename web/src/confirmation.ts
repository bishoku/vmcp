import { createContext, useContext } from 'react'

export interface Confirmation {
  title:string
  description:string
  confirmLabel:string
  cancelLabel?:string
  tone?:'danger'|'warning'
}
export type Confirm = (request:string|Confirmation)=>boolean|Promise<boolean>
export const discardConfirmation:Confirmation={title:'Discard unsaved changes?',description:'Changes in this draft will be lost. Your saved configuration will stay unchanged.',confirmLabel:'Discard changes',cancelLabel:'Keep editing',tone:'warning'}
// Standalone consumers can supply their own confirmation service.
export const ConfirmationContext=createContext<Confirm>(request=>window.confirm(typeof request==='string'?request:`${request.title}\n\n${request.description}`))
export const useConfirm=()=>useContext(ConfirmationContext)
