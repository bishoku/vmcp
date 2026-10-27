import KeyboardNavigation from './KeyboardNavigation'
import TooltipProvider from './TooltipProvider'
import NotificationsProvider from './NotificationsProvider'
import { LoadingState } from './Feedback'
import Connections from './pages/Connections'
import ConnectionSetup from './pages/ConnectionSetup'
import VirtualServerDetail from './pages/VirtualServerDetail'
import VirtualServerEdit from './pages/VirtualServerEdit'
import UpstreamDetail from './pages/UpstreamDetail'
import ManagementRoute from './connections/ManagementRoute'
import { createBrowserRouter, RouterProvider, Navigate, Route, Routes, useSearchParams } from 'react-router-dom'
import { LogOut } from 'lucide-react'
import AppNavigation from './AppNavigation'
import ConfirmationProvider from './ConfirmationProvider'
import { discardConfirmation, useConfirm } from './confirmation'
import { btn } from './styles'
import { useContext, useEffect, useState } from 'react'
import { api, signOut, type Me } from './api'
import SignIn from './SignIn'
import SessionRecovery from './SessionRecovery'
import InteractionProvider from './InteractionProvider'
import { RoleContext, Drafts, capabilities, useOperation } from './interaction'
import Upstreams from './pages/Upstreams'
import VirtualServers from './pages/VirtualServers'
import AgentKeys from './pages/AgentKeys'
import Identities from './pages/Identities'
import Audit from './pages/Audit'
import Ownership from './pages/Ownership'
import SettingsPage from './pages/Settings'

function SignOut({onDone}:{onDone:()=>void}) {
  const confirm=useConfirm()
  const drafts=useContext(Drafts)
  const {busy,run}=useOperation()
  return <button className={`${btn} w-full justify-start border-transparent bg-transparent text-xs text-ink-muted`} disabled={busy} onClick={async()=>{
    if(drafts && [...drafts.values()].includes('busy'))return
    if(drafts && [...drafts.values()].includes('dirty') && !await confirm(discardConfirmation))return
    void run(async()=>{const url=await signOut();if(url)window.location.assign(url);else onDone()})
  }}><LogOut aria-hidden="true" className="icon-inline"/>{busy?'Signing out…':'Sign out'}</button>
}

function Application() {
  // undefined = still checking; null = signed out.
  const [me, setMe] = useState<Me | null | undefined>(undefined)
  const [expired, setExpired] = useState(false)
  const refresh = () => api.me().then(value => { setMe(value); setExpired(false) }, () => setMe(null))
  useEffect(() => {
    void refresh()
    const expired = () => setExpired(true)
    window.addEventListener('vmcp:session-expired', expired)
    return () => window.removeEventListener('vmcp:session-expired', expired)
  }, [])
  const signedIn = (identity: Me) => { setMe(identity); setExpired(false) }
  if (me === undefined) return <div className="grid h-full place-items-center p-6"><LoadingState label="Checking session…" detail="Verifying access to your workspace."/></div>
  if (me === null) return <SignIn onDone={signedIn} />
  return (
    <RoleContext.Provider value={me.role==='viewer' && me.teamMember?'team':me.role}><ConfirmationProvider><InteractionProvider>
      {expired && <SessionRecovery onDone={signedIn}/>}
      <div className="flex h-full min-h-0 flex-col md:flex-row">
        <AppNavigation me={me} sessionExpired={expired} signOut={<SignOut onDone={()=>setMe(null)}/>}/>
        <KeyboardNavigation/><main id="main-content" tabIndex={-1} className="min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-y-contain">
          {!capabilities(me.role).edit && <p role="status" className="mx-4 mt-4 surface-inset surface-white px-4 py-3 text-xs leading-relaxed text-ink-muted sm:mx-6 lg:mx-10">Read-only access. Ask an administrator for an editor role to change configuration.</p>}

          <Routes>
 <Route path="/ownership" element={<Ownership/>}/>
            <Route path="/connections" element={<Connections/>}/>
            <Route path="/setup" element={<ConnectionSetup/>}/>
            <Route path="/upstreams/new" element={<UpstreamDetail key="new" create/>}/>
            <Route path="/upstreams/:slug" element={<UpstreamDetail key="detail"/>}/>
            <Route path="/virtual-servers/new" element={<VirtualServerEdit create/>}/>
            <Route path="/virtual-servers/:slug/edit" element={<VirtualServerEdit/>}/>
            <Route path="/virtual-servers/:slug/:view?" element={<VirtualServerDetail/>}/>
            <Route path="/" element={<Navigate to="/virtual-servers" replace />} />
            <Route
              path="/upstreams"
              element={<ManagementRoute page={Upstreams} detailBase="/upstreams" />}
            />
            <Route
              path="/virtual-servers"
              element={<ManagementRoute page={VirtualServers} detailBase="/virtual-servers" />}
            />
            <Route
              path="/audit"
              element={<AuditRoute />}
            />
            <Route path="/agent-keys" element={<ManagementRoute page={AgentKeys} />} />
            <Route path="/identities" element={<ManagementRoute page={Identities} />} />
            <Route path="/settings" element={<SettingsPage role={me.role} />} />
          </Routes>

        </main>
      </div>
    </InteractionProvider></ConfirmationProvider></RoleContext.Provider>
  )
}

const router = createBrowserRouter([{ path: '*', element: <Application /> }])
export default function App() { return <TooltipProvider><NotificationsProvider><RouterProvider router={router} /></NotificationsProvider></TooltipProvider> }

function AuditRoute() {
  const [params] = useSearchParams()
  const server = params.get('virtualServer') ?? undefined
  const allowed = new Set(['activity','virtualServer','operationId','requestId','actorId','stage','agentKey','subject','tool','status','from','to','incomplete'])
  const initialQuery = Object.fromEntries([...params].filter(([key])=>allowed.has(key)))
  return <Audit initialQuery={initialQuery} key={params.toString()} initialServer={server} initialOperation={params.get('operationId') ?? undefined}/>
}
