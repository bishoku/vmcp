import { LoaderCircle, Save } from 'lucide-react'
import { btn, primary } from '../styles'

export default function EditorSaveBar({status,detail,busy,disabled,onSave,onCancel,busyLabel='Saving…'}:{status:string;detail:string;busy:boolean;disabled:boolean;onSave:()=>void;onCancel:()=>void;busyLabel?:string}) {
  return <div className="sticky top-0 z-20 flex flex-wrap items-center justify-between gap-3 rounded-card border border-line bg-surface px-5 py-4 shadow-card">
    <div className="min-w-0"><p role="status" className="text-xs font-medium">{status}</p><p className="mt-1 text-xs text-ink-muted">{detail}</p></div>
    <div className="flex shrink-0 gap-2"><button className={btn} disabled={busy} onClick={onCancel}>Cancel</button><button className={primary} disabled={disabled||busy} onClick={onSave}>{busy?<LoaderCircle className="size-3.5 animate-spin"/>:<Save className="size-3.5"/>}{busy?busyLabel:'Save'}</button></div>
  </div>
}
