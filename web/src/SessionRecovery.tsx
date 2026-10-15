import { Clock3, FileCheck2, RefreshCw } from 'lucide-react'
import type { Me } from './api'
import { Modal } from './ui'
import SignIn from './SignIn'
import { ServerPanel } from './connections/ServerPanels'

export default function SessionRecovery({onDone}:{onDone:(identity:Me)=>void}) {
  return <Modal title="Session expired" size="wide">
    <div className="space-y-6">
      <div role="alert" className="flex items-start gap-3 rounded-xl border border-amber-200 bg-amber-50 p-4"><Clock3 className="mt-0.5 size-5 shrink-0 text-amber-700"/><div><p className="text-sm font-semibold text-amber-900">Sign in again to continue</p><p className="mt-1 text-xs leading-relaxed text-amber-800">Your session can no longer authorize requests. Restore access before continuing your work.</p></div></div>
      <div className="grid items-start gap-6 lg:grid-cols-2"><ServerPanel title="Continue your work" description="Your page stays open while you restore access" icon={RefreshCw}><div className="space-y-5 p-5"><div className="flex items-start gap-3"><FileCheck2 className="mt-0.5 size-4 shrink-0 text-accent-700"/><div><p className="text-xs font-semibold">Keep your draft</p><p className="mt-2 text-xs leading-relaxed text-ink-muted">Token sign-in retains the current page and unsaved changes. Failed requests are not retried automatically; review your draft and submit again when ready.</p></div></div><div className="rounded-xl border border-line bg-canvas p-4"><p className="text-xs font-medium">Using SSO?</p><p className="mt-2 text-xs leading-relaxed text-ink-muted">Copy unsaved changes before using SSO. The identity-provider sign-in reloads this page, so the in-memory draft may be lost.</p></div></div></ServerPanel><SignIn onDone={onDone} reauthenticate/></div>
    </div>
  </Modal>
}
