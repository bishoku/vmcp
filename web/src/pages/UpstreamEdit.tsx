import { useCallback, useRef, useState, type ReactNode } from 'react'
import { UpstreamEditContext, type Section } from './upstream-edit-session'

/** Separate revisioned endpoints share one explicit edit session, but are not atomic. */
export function UpstreamEditProvider({children}:{children:ReactNode}) {
  const [editing,setEditing]=useState(false)
  const [saving,setSaving]=useState(false)
  const sections=useRef(new Map<string,Section>())
  const [status,setStatus]=useState({dirty:false,blocked:false,writable:false})
  const register=useCallback((id:string,section:Section)=>{
    sections.current.set(id,section)
    const update=()=>{
      const values=[...sections.current.values()]
      const next={dirty:values.some(value=>value.dirty),blocked:values.some(value=>value.writable&&value.blocked),writable:values.some(value=>value.writable)}
      setStatus(previous=>previous.dirty===next.dirty&&previous.blocked===next.blocked&&previous.writable===next.writable?previous:next)
    }
    update()
    return ()=>{sections.current.delete(id);update()}
  },[])
  const saveSections=async()=>{
    const saved:string[]=[]
    for(const section of [...sections.current.values()]){
      if(!section.dirty)continue
      try{await section.save();saved.push(section.label)}catch(error){
        throw new Error(`${section.label} could not be saved. ${saved.length?`Already saved: ${saved.join(', ')}. `:''}Remaining changes are still in this draft. ${String(error instanceof Error?error.message:error)}`)
      }
    }
    return saved
  }
  return <UpstreamEditContext.Provider value={{editing,setEditing,saving,setSaving,...status,register,saveSections,discardSections:()=>{for(const section of sections.current.values())section.discard()}}}>{children}</UpstreamEditContext.Provider>
}

