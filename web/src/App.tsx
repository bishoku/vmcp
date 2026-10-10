import { BrowserRouter, NavLink, Navigate, Route, Routes } from 'react-router-dom'
import { FileText, KeyRound, UserCheck, Layers, Network, Settings, type LucideIcon } from 'lucide-react'
import clsx from 'clsx'
import { useEffect, useState } from 'react'
import { api, authConfig, clearToken, setToken, signOut, type Me } from './api'
import Upstreams from './pages/Upstreams'
import VirtualServers from './pages/VirtualServers'
import AgentKeys from './pages/AgentKeys'
import Identities from './pages/Identities'
import Audit from './pages/Audit'
import SettingsPage from './pages/Settings'

const nav: { to: string; label: string; icon: LucideIcon }[] = [
  { to: '/upstreams', label: 'Upstreams', icon: Network },
  { to: '/virtual-servers', label: 'Virtual Servers', icon: Layers },
  { to: '/agent-keys', label: 'Agent Keys', icon: KeyRound },
  { to: '/identities', label: 'Identities', icon: UserCheck },
  { to: '/audit', label: 'Audit Log', icon: FileText },
  { to: '/settings', label: 'Settings', icon: Settings },
]

function Login({ onDone }: { onDone: () => void }) {
  const [t, setT] = useState('')
  const [err, setErr] = useState('')
  const [sso, setSso] = useState(false)
  useEffect(() => {
    void authConfig().then((c) => setSso(c.oidc))
  }, [])
  const submit = async () => {
    setToken(t)
    try {
      await api.me()
      onDone()
    } catch {
      clearToken()
      setErr('Token rejected or control plane unreachable.')
    }
  }
  return (
    <div className="grid h-full place-items-center">
      <div className="w-96 rounded-card border border-line bg-surface p-8 shadow-card">
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
          type="password"
          autoFocus={!sso}
          className="mt-3 w-full rounded-lg border border-line px-3 py-2 text-sm outline-none focus:border-accent-500"
          value={t}
          onChange={(e) => setT(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && submit()}
        />
        {err && <p className="mt-2 text-sm text-red-700">{err}</p>}
        <button onClick={submit} className={sso ? 'mt-3 w-full rounded-lg border border-line py-2 text-sm font-medium hover:bg-canvas' : 'mt-4 w-full rounded-lg bg-accent-500 py-2 text-sm font-medium text-white hover:bg-accent-700'}>
          Continue
        </button>
      </div>
    </div>
  )
}

export default function App() {
  // undefined = still checking; null = signed out.
  const [me, setMe] = useState<Me | null | undefined>(undefined)
  const refresh = () => api.me().then(setMe, () => setMe(null))
  useEffect(() => {
    void refresh()
  }, [])
  if (me === undefined) return null
  if (me === null) return <Login onDone={refresh} />
  return (
    <BrowserRouter>
      <div className="flex h-full">
        <aside className="flex w-60 shrink-0 flex-col border-r border-line bg-surface px-3 py-5">
          <div className="mb-6 flex items-center gap-2.5 px-3">
            <div className="grid size-8 place-items-center rounded-lg bg-accent-500 text-sm font-bold text-white">
              v
            </div>
            <span className="text-lg font-semibold tracking-tight">vMCP</span>
          </div>
          <nav className="flex flex-col gap-1">
            {nav.map(({ to, label, icon: Icon }) => (
              <NavLink
                key={to}
                to={to}
                className={({ isActive }) =>
                  clsx(
                    'flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors',
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
          <div className="mt-auto border-t border-line px-3 pt-4 text-sm">
            <div className="truncate font-medium">{me.name}</div>
            <div className="text-xs text-ink-muted">{me.role}</div>
            <button
              className="mt-2 text-xs text-accent-700 hover:underline"
              onClick={async () => {
                const idpLogout = await signOut()
                if (idpLogout) {
                  window.location.assign(idpLogout)
                  return
                }
                setMe(null)
              }}
            >
              Sign out
            </button>
          </div>
        </aside>
        <main className="flex-1 overflow-y-auto">
          <Routes>
            <Route path="/" element={<Navigate to="/virtual-servers" replace />} />
            <Route
              path="/upstreams"
              element={<Upstreams />}
            />
            <Route
              path="/virtual-servers"
              element={<VirtualServers />}
            />
            <Route
              path="/audit"
              element={<Audit />}
            />
            <Route path="/agent-keys" element={<AgentKeys />} />
            <Route path="/identities" element={<Identities />} />
            <Route path="/settings" element={<SettingsPage role={me.role} />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  )
}
