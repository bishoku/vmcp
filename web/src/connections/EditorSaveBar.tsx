import { Save } from 'lucide-react'
import Button from '../Button'

export default function EditorSaveBar({status,detail,busy,disabled,onSave,onCancel,busyLabel='Saving…'}:{status:string;detail:string;busy:boolean;disabled:boolean;onSave:()=>void;onCancel:()=>void;busyLabel?:string}) {
  return <div className="sticky top-0 z-20 flex flex-wrap items-center justify-between gap-3 surface-card panel-inset py-4 shadow-floating">
    <div className="min-w-0"><p role="status" className="text-xs font-medium">{status}</p><p className="mt-1 text-xs text-ink-muted">{detail}</p></div>
    <div className="flex shrink-0 gap-2"><Button disabled={busy} onClick={onCancel}>Cancel</Button><Button variant="primary" disabled={disabled} loading={busy} loadingLabel={busyLabel} onClick={onSave}><Save aria-hidden="true" className="icon-inline"/>Save</Button></div>
  </div>
}
