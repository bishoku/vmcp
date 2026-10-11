import Connections from './pages/Connections'
import ConnectionSetup from './pages/ConnectionSetup'
import VirtualServerDetail from './pages/VirtualServerDetail'
import ManagementRoute from './connections/ManagementRoute'
import { createBrowserRouter, RouterProvider, NavLink, Navigate, Route, Routes, useSearchParams } from 'react-router-dom'
import { FileText, KeyRound, UserCheck, Layers, Network, Settings, type LucideIcon } from 'lucide-react'
import clsx from 'clsx'
import { useContext, useEffect, useState } from 'react'
import { api, authConfig, clearToken, setToken, signOut, type Me } from './api'
import { Modal } from './ui'
import InteractionProvider from './InteractionProvider'
import { RoleContext, Drafts, discardChanges, capabilities, useOperation } from './interaction'
import Upstreams from './pages/Upstreams'
import VirtualServers from './pages/VirtualServers'
import AgentKeys from './pages/AgentKeys'
import Identities from './pages/Identities'
import Audit from './pages/Audit'
import SettingsPage from './pages/Settings'

const nav: { to: string; label: string; icon: LucideIcon }[] = [
  { to: '/connections', label: 'Connections', icon: Network },
  { to: '/setup', label: 'Setup guide', icon: Layers },
  { to: '/upstreams', label: 'Upstreams', icon: Network },
  { to: '/virtual-servers', label: 'Virtual Servers', icon: Layers },
  { to: '/agent-keys', label: 'Agent Keys', icon: KeyRound },
  { to: '/identities', label: 'Identities', icon: UserCheck },
  { to: '/audit', label: 'Audit Log', icon: FileText },
  { to: '/settings', label: 'Settings', icon: Settings },
]

function Login({ onDone }: { onDone: (identity: Me) => void }) {
  const [t, setT] = useState('')
  const [err, setErr] = useState('')
  const [sso, setSso] = useState(false)
  useEffect(() => {
    void authConfig().then((c) => setSso(c.oidc))
  }, [])
  const { busy, run } = useOperation()
  const submit = () => run(async () => {
    setToken(t)
    try {
      const identity = await api.me()
      onDone(identity)
    } catch {
      clearToken()
      setErr('Token rejected or control plane unreachable.')
    }
  })
  return (
    <div className="grid h-full place-items-center">
      <div className="w-full max-w-96 rounded-card border border-line bg-surface p-8 shadow-card">
        <h1 className="text-lg font-semibold">Sign in to vMCP</h1>
        {sso && (
          <>
            <a href="/auth/login" className="mt-5 block w-full rounded-lg bg-accent-500 py-2 text-center text-sm font-medium text-white hover:bg-accent-700">
              Sign in with SSO
            </a>
            <div className="my-4 text-center text-xs text-ink-muted">or use an admin token</div>
          </>
        )}
        {!sso && <p className="mt-1 text-sm text-ink-muted">Enter the admin token.</p>}
        <input
          aria-label="Admin token"
          disabled={busy}
          type="password"
          autoFocus={!sso}
          className="mt-3 w-full rounded-lg border border-line px-3 py-2 text-sm outline-none focus:border-accent-500"
          value={t}
          onChange={(e) => setT(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && submit()}
        />
        <p className="mt-2 text-xs text-ink-muted">Token sign-in lasts until this tab reloads. Prefer SSO for regular use.</p>
        {err && <p className="mt-2 text-sm text-red-700">{err}</p>}
        <button disabled={busy || !t.trim()} onClick={submit} className={sso ? 'mt-3 w-full rounded-lg border border-line py-2 text-sm font-medium hover:bg-canvas' : 'mt-4 w-full rounded-lg bg-accent-500 py-2 text-sm font-medium text-white hover:bg-accent-700'}>
          Continue
        </button>
      </div>
    </div>
  )
}

function SignOut({onDone}:{onDone:()=>void}) {
  const drafts=useContext(Drafts)
  const {busy,run}=useOperation()
  return <button className="mt-2 text-xs text-accent-700 hover:underline" disabled={busy} onClick={()=>{
    if(drafts && [...drafts.values()].includes('busy'))return
    if(drafts && [...drafts.values()].includes('dirty') && !discardChanges())return
    void run(async()=>{const url=await signOut();if(url)window.location.assign(url);else onDone()})
  }}>Sign out</button>
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
  if (me === undefined) return <p role="status" className="p-10">Checking session…</p>
  if (me === null) return <Login onDone={signedIn} />
  return (
    <RoleContext.Provider value={me.role}><InteractionProvider>
      {expired && <Modal title="Session expired"><p role="alert">Sign in again to continue. Token sign-in retains your draft. Copy unsaved changes before using SSO, which reloads this page.</p><Login onDone={signedIn} /></Modal>}
      <div className="flex h-full flex-col md:flex-row">
        <aside className="flex w-full md:w-60 shrink-0 flex-col border-r border-line bg-surface px-3 py-5">
          <div className="mb-2 md:mb-6 flex items-center gap-2.5 px-3">
            <div className="grid size-8 place-items-center rounded-lg bg-accent-500 text-sm font-bold text-white">
              v
            </div>
            <span className="text-lg font-semibold tracking-tight">vMCP</span>
          </div>
          <nav aria-label="Main navigation" className="flex overflow-x-auto md:flex-col gap-1">
            {nav.map(({ to, label, icon: Icon }) => (
              <NavLink
                key={to}
                to={to}
                className={({ isActive }) =>
                  clsx(
                    'flex shrink-0 items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors',
                    isActive
                      ? 'bg-accent-50 text-accent-700'
                      : 'text-ink-muted hover:bg-canvas hover:text-ink',
                  )
                }
              >
                <Icon className="size-[18px]" />
                {label}
              </NavLink>
            ))}
          </nav>
          <div className="mt-2 md:mt-auto border-t border-line px-3 pt-4 text-sm">
            <div className="truncate font-medium">{me.name}</div>
            <div className="text-xs text-ink-muted">{me.role}</div>
            <SignOut onDone={()=>setMe(null)} />
          </div>
        </aside>
        <main className="min-w-0 flex-1 overflow-y-auto">
          {!capabilities(me.role).edit && <p role="status" className="px-10 pt-4 text-sm text-ink-muted">Read-only access. Ask an administrator for an editor role to change configuration.</p>}

          <Routes>
            <Route path="/connections" element={<Connections/>}/>
            <Route path="/setup" element={<ConnectionSetup/>}/>
            <Route path="/virtual-servers/:slug/:view?" element={<VirtualServerDetail/>}/>
            <Route path="/" element={<Navigate to="/virtual-servers" replace />} />
            <Route
              path="/upstreams"
              element={<ManagementRoute page={Upstreams} />}
            />
            <Route
              path="/virtual-servers"
              element={<ManagementRoute page={VirtualServers} />}
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
    </InteractionProvider></RoleContext.Provider>
  )
}

const router = createBrowserRouter([{ path: '*', element: <Application /> }])
export default function App() { return <RouterProvider router={router} /> }

function AuditRoute() {
  const [params] = useSearchParams()
  const server = params.get('virtualServer') ?? undefined
  const allowed = new Set(['virtualServer','operationId','requestId','actorId','stage','agentKey','subject','tool','status','from','to','incomplete'])
  const initialQuery = Object.fromEntries([...params].filter(([key])=>allowed.has(key)))
  return <Audit initialQuery={initialQuery} key={params.toString()} initialServer={server} initialOperation={params.get('operationId') ?? undefined}/>
}
