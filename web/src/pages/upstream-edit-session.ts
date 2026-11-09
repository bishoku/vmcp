import { createContext, useContext, useEffect, useRef } from 'react'

export interface Section {
  label: string
  writable: boolean
  dirty: boolean
  blocked: boolean
  save: () => Promise<void>
  discard: () => void
}
interface EditSession {
  saving: boolean
  setSaving: (saving: boolean) => void
  editing: boolean
  setEditing: (editing: boolean) => void
  dirty: boolean
  blocked: boolean
  writable: boolean
  register: (id: string, section: Section) => () => void
  saveSections: () => Promise<string[]>
  discardSections: () => void
}
export const UpstreamEditContext = createContext<EditSession | null>(null)
export const useUpstreamEdit = () => useContext(UpstreamEditContext)

export function useUpstreamSection(id: string, section: Section) {
  const session = useUpstreamEdit()
  const register = session?.register
  const current = useRef(section)
  useEffect(() => {
    current.current = section
  })
  const { label, writable, dirty, blocked } = section
  useEffect(
    () =>
      register?.(id, {
        label,
        writable,
        dirty,
        blocked,
        save: () => current.current.save(),
        discard: () => current.current.discard(),
      }),
    [register, id, label, writable, dirty, blocked],
  )
  return session
}
