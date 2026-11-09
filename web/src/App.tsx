import { ConfigurationRoute } from './ConfigurationRoute'
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
import {
  createBrowserRouter,
  RouterProvider,
  Navigate,
  Route,
  Routes,
  useSearchParams,
} from 'react-router-dom'
import { LogOut } from 'lucide-react'
import AppNavigation from './AppNavigation'
import ConfirmationProvider from './ConfirmationProvider'
import { discardConfirmation, useConfirm } from './confirmation'
import { btn } from './styles'
import { useContext, useEffect, useRef, useState } from 'react'
import { api, signOut, resetConfigurationIdentity, type Me } from './api'
import SignIn from './SignIn'
import SessionRecovery from './SessionRecovery'
import ClaimSessionNotice from './ClaimSessionNotice'
import InteractionProvider from './InteractionProvider'
import { RoleContext, Drafts, useOperation } from './interaction'
import Upstreams from './pages/Upstreams'
import VirtualServers from './pages/VirtualServers'
import AgentKeys from './pages/AgentKeys'
import Identities from './pages/Identities'
import Audit from './pages/Audit'
import Ownership from './pages/Ownership'
import Authorization from './pages/Authorization'
import Omni from './pages/Omni'
import SettingsPage from './pages/Settings'
import InvocationSecurity from './pages/InvocationSecurity'

function SignOut({ onDone }: { onDone: () => void }) {
  const confirm = useConfirm()
  const drafts = useContext(Drafts)
  const { busy, run } = useOperation()
  return (
    <button
      className={`${btn} w-full justify-start border-transparent bg-transparent text-xs text-ink-muted`}
      disabled={busy}
      onClick={async () => {
        if (drafts && [...drafts.values()].includes('busy')) return
        if (
          drafts &&
          [...drafts.values()].includes('dirty') &&
          !(await confirm(discardConfirmation))
        )
          return
        void run(async () => {
          const url = await signOut()
          if (url) window.location.assign(url)
          else onDone()
        })
      }}
    >
      <LogOut aria-hidden="true" className="icon-inline" />
      {busy ? 'Signing out…' : 'Sign out'}
    </button>
  )
}

function Application() {
  // undefined = still checking; null = signed out.
  const [me, setMe] = useState<Me | null | undefined>(undefined)
  const [expired, setExpired] = useState(false)
  const [configurationRevision, setConfigurationRevision] = useState(0)
  const claimsExpirationHandled = useRef(false)
  const refresh = () =>
    api.me().then(
      (value) => {
        resetConfigurationIdentity()
        setMe(value)
        setExpired(false)
      },
      () => setMe(null),
    )
  useEffect(() => {
    void refresh()
    const expired = () => {
      resetConfigurationIdentity()
      setExpired(true)
    }
    window.addEventListener('vmcp:session-expired', expired)
    const denied = () => {
      setConfigurationRevision((value) => value + 1)
      void refresh()
    }
    window.addEventListener('vmcp:configuration-denied', denied)
    const claimsExpired = () => {
      if (claimsExpirationHandled.current) return
      claimsExpirationHandled.current = true
      setConfigurationRevision((value) => value + 1)
      void refresh()
    }
    window.addEventListener('vmcp:claims-expired', claimsExpired)
    return () => {
      window.removeEventListener('vmcp:session-expired', expired)
      window.removeEventListener('vmcp:configuration-denied', denied)
      window.removeEventListener('vmcp:claims-expired', claimsExpired)
    }
  }, [])
  const signedIn = (identity: Me) => {
    claimsExpirationHandled.current = false
    resetConfigurationIdentity()
    setMe(identity)
    setExpired(false)
  }
  if (me === undefined)
    return (
      <div className="grid h-full place-items-center p-6">
        <LoadingState label="Checking session…" detail="Verifying access to your workspace." />
      </div>
    )
  if (me === null) return <SignIn onDone={signedIn} />
  return (
    <RoleContext.Provider value={me.role === 'viewer' && me.teamMember ? 'team' : me.role}>
      <ConfirmationProvider>
        <InteractionProvider
          key={
            expired
              ? 'expired'
              : JSON.stringify([me.id, me.issuer, me.name, me.role, configurationRevision])
          }
        >
          {expired && <SessionRecovery onDone={signedIn} />}
          <div className="flex h-full min-h-0 flex-col md:flex-row">
            <AppNavigation
              me={me}
              sessionExpired={expired}
              signOut={<SignOut onDone={() => setMe(null)} />}
            />
            <KeyboardNavigation />
            <main
              id="main-content"
              tabIndex={-1}
              className="min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-y-contain"
            >
              <ClaimSessionNotice me={me} />
              {configurationRevision > 0 && (
                <p role="status" className="mx-4 mt-4 text-sm">
                  Configuration permissions changed. The view was reloaded; no denied operation was
                  submitted.
                </p>
              )}

              {expired ? (
                <p role="status" className="page-layout">
                  Sign in again to reload your permitted configuration.
                </p>
              ) : (
                <Routes>
                  <Route
                    path="/omni"
                    element={
                      <ConfigurationRoute>
                        <Omni />
                      </ConfigurationRoute>
                    }
                  />
                  <Route
                    path="/ownership"
                    element={
                      <ConfigurationRoute>
                        <Ownership />
                      </ConfigurationRoute>
                    }
                  />
                  <Route
                    path="/connections"
                    element={
                      <ConfigurationRoute>
                        <Connections />
                      </ConfigurationRoute>
                    }
                  />
                  <Route
                    path="/setup"
                    element={
                      <ConfigurationRoute>
                        <ConnectionSetup />
                      </ConfigurationRoute>
                    }
                  />
                  <Route
                    path="/upstreams/new"
                    element={
                      <ConfigurationRoute>
                        <UpstreamDetail key="new" create />
                      </ConfigurationRoute>
                    }
                  />
                  <Route
                    path="/upstreams/:slug"
                    element={
                      <ConfigurationRoute>
                        <UpstreamDetail key="detail" />
                      </ConfigurationRoute>
                    }
                  />
                  <Route
                    path="/virtual-servers/new"
                    element={
                      <ConfigurationRoute>
                        <VirtualServerEdit create />
                      </ConfigurationRoute>
                    }
                  />
                  <Route
                    path="/virtual-servers/:slug/edit"
                    element={
                      <ConfigurationRoute>
                        <VirtualServerEdit />
                      </ConfigurationRoute>
                    }
                  />
                  <Route
                    path="/virtual-servers/:slug/:view?"
                    element={
                      <ConfigurationRoute>
                        <VirtualServerDetail />
                      </ConfigurationRoute>
                    }
                  />
                  <Route path="/" element={<Navigate to="/virtual-servers" replace />} />
                  <Route
                    path="/upstreams"
                    element={
                      <ConfigurationRoute>
                        <ManagementRoute page={Upstreams} detailBase="/upstreams" />
                      </ConfigurationRoute>
                    }
                  />
                  <Route
                    path="/virtual-servers"
                    element={
                      <ConfigurationRoute>
                        <ManagementRoute page={VirtualServers} detailBase="/virtual-servers" />
                      </ConfigurationRoute>
                    }
                  />
                  <Route path="/audit" element={<AuditRoute />} />
                  <Route
                    path="/agent-keys"
                    element={
                      <ConfigurationRoute>
                        <ManagementRoute page={AgentKeys} />
                      </ConfigurationRoute>
                    }
                  />
                  <Route
                    path="/identities"
                    element={
                      <ConfigurationRoute>
                        <ManagementRoute page={Identities} />
                      </ConfigurationRoute>
                    }
                  />
                  <Route
                    path="/invocation-security"
                    element={
                      <ConfigurationRoute>
                        <InvocationSecurity />
                      </ConfigurationRoute>
                    }
                  />
                  <Route
                    path="/settings"
                    element={
                      <ConfigurationRoute>
                        <SettingsPage role={me.role} />
                      </ConfigurationRoute>
                    }
                  />
                  <Route
                    path="/access"
                    element={
                      <Authorization key={expired ? 'expired' : 'current'} active={!expired} />
                    }
                  />
                </Routes>
              )}
            </main>
          </div>
        </InteractionProvider>
      </ConfirmationProvider>
    </RoleContext.Provider>
  )
}

const router = createBrowserRouter([{ path: '*', element: <Application /> }])
export default function App() {
  return (
    <TooltipProvider>
      <NotificationsProvider>
        <RouterProvider router={router} />
      </NotificationsProvider>
    </TooltipProvider>
  )
}

function AuditRoute() {
  const [params] = useSearchParams()
  const server = params.get('virtualServer') ?? undefined
  const allowed = new Set([
    'conversationId',
    'rootConversationId',
    'turnId',
    'conversationGroupId',
    'activity',
    'virtualServer',
    'operationId',
    'requestId',
    'actorId',
    'stage',
    'agentKey',
    'subject',
    'tool',
    'status',
    'from',
    'to',
    'incomplete',
  ])
  const initialQuery = Object.fromEntries([...params].filter(([key]) => allowed.has(key)))
  return (
    <Audit
      initialQuery={initialQuery}
      key={params.toString()}
      initialServer={server}
      initialOperation={params.get('operationId') ?? undefined}
    />
  )
}
