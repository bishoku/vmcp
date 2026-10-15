import { useEffect, useId, useState } from 'react'
import { ArrowRight, CircleAlert, Eye, EyeOff, KeyRound, Layers, LoaderCircle, Network, Server, ShieldCheck } from 'lucide-react'
import { api, ApiError, authConfig, clearToken, setToken, type Me } from './api'
import { useOperation } from './interaction'
import { input, primary, btn } from './styles'
import { ServerPanel } from './connections/ServerPanels'

export default function SignIn({onDone,reauthenticate=false}:{onDone:(identity:Me)=>void;reauthenticate?:boolean}) {
  const [token,setTokenInput]=useState('')
  const [visible,setVisible]=useState(false)
  const [error,setError]=useState('')
  const [method,setMethod]=useState<'loading'|'token'|'sso'>('loading')
  const errorId=useId(),helpId=useId()
  const {busy,run}=useOperation()
  useEffect(()=>{
    let active=true
    void authConfig().then(config=>{if(active)setMethod(config.oidc?'sso':'token')})
    return()=>{active=false}
  },[])
  const submit=()=>{
    if(busy||!token.trim())return
    void run(async()=>{
      setError('')
      setToken(token.trim())
      try {
        const identity=await api.me()
        setTokenInput('');setVisible(false)
        onDone(identity)
      } catch(e) {
        clearToken()
        setError(e instanceof ApiError&&(e.status===401||e.status===403)?'This token was not accepted. Check the admin token and try again.':'Sign-in is unavailable. Check your connection and control plane availability, then try again.')
      }
    })
  }
  const form=<ServerPanel title={reauthenticate?'Restore access':'Administrative access'} description={reauthenticate?'Verify your identity to continue in this workspace':method==='sso'?'Use SSO or an admin token to open the control plane':'Use an admin API token to open the control plane'} icon={ShieldCheck}>
    <div className="space-y-5 p-5 sm:p-6">
      {method==='loading'&&<p role="status" className="inline-flex items-center gap-2 text-xs text-ink-muted"><LoaderCircle className="size-3.5 animate-spin"/>Checking sign-in options…</p>}
      {method==='sso'&&<><div className="space-y-3"><a className={`${primary} w-full py-2.5 ${busy?'pointer-events-none opacity-50':''}`} aria-disabled={busy||undefined} tabIndex={busy?-1:undefined} href="/auth/login"><ShieldCheck className="size-4"/>Sign in with SSO<ArrowRight className="ml-auto size-4"/></a><p className="text-xs leading-relaxed text-ink-muted">Use your organization’s identity provider for regular access.</p></div><div className="flex items-center gap-3"><span className="h-px flex-1 bg-line"/><span className="text-[11px] text-ink-muted">or use an admin token</span><span className="h-px flex-1 bg-line"/></div></>}
      <form onSubmit={e=>{e.preventDefault();submit()}} className="space-y-4" aria-label="Token sign-in">
        <div><label className="block text-xs font-medium" htmlFor={`${helpId}-token`}>Admin token</label><div className="relative mt-2"><KeyRound aria-hidden="true" className="absolute left-3 top-3 size-4 text-ink-muted"/><input id={`${helpId}-token`} disabled={busy} type={visible?'text':'password'} autoComplete="off" autoCapitalize="none" spellCheck={false} className={`${input} py-2.5 pl-9 pr-12`} value={token} onChange={e=>{setTokenInput(e.target.value);setError('')}} aria-invalid={!!error} aria-describedby={`${helpId}${error?` ${errorId}`:''}`}/><button type="button" className="absolute right-2 top-2 rounded-md p-1.5 text-ink-muted hover:bg-canvas" disabled={busy} aria-label={visible?'Hide token':'Show token'} aria-pressed={visible} onClick={()=>setVisible(value=>!value)}>{visible?<EyeOff className="size-4"/>:<Eye className="size-4"/>}</button></div><p id={helpId} className="mt-2 text-xs leading-relaxed text-ink-muted">Use an admin API token. Your role determines which settings you can change.</p></div>
        {error&&<div id={errorId} role="alert" className="flex items-start gap-2 rounded-xl border border-red-200 bg-red-50 p-3 text-xs leading-relaxed text-red-700"><CircleAlert className="mt-0.5 size-4 shrink-0"/><p>{error}</p></div>}
        <button type="submit" disabled={busy||!token.trim()} className={`${method==='sso'?btn:primary} w-full py-2.5`}>{busy?<><LoaderCircle className="size-4 animate-spin"/>Signing in…</>:<>Continue<ArrowRight className="size-4"/></>}</button>
        {busy&&<p role="status" className="text-xs text-ink-muted">Verifying your session…</p>}
      </form>
      <div className="flex items-start gap-2 rounded-xl border border-line bg-canvas p-3 text-xs leading-relaxed text-ink-muted"><ShieldCheck className="mt-0.5 size-4 shrink-0"/><p>Token sign-in lasts until this tab reloads. {reauthenticate?'Signing in with a token keeps your current draft.':'The token is held in memory for this tab.'}</p></div>
    </div>
  </ServerPanel>
  if(reauthenticate)return form
  return <main className="h-full overflow-y-auto px-4 py-8 sm:px-6 sm:py-12">
    <div className="mx-auto flex min-h-full max-w-5xl flex-col justify-center gap-8 lg:grid lg:grid-cols-[1fr_1fr] lg:items-center lg:gap-16">
      <div className="space-y-7"><div className="flex items-center gap-3"><div className="grid size-11 place-items-center rounded-xl bg-accent-500 text-xl font-semibold text-white shadow-card">v</div><div><p className="text-lg font-semibold tracking-tight">vMCP</p><p className="text-xs text-ink-muted">MCP control plane</p></div></div><div><h1 className="text-2xl font-semibold tracking-tight sm:text-3xl">Sign in to vMCP</h1><p className="mt-3 max-w-md text-sm leading-relaxed text-ink-muted">Manage upstream connections, compose virtual servers and configure access from one workspace.</p></div><div className="hidden space-y-3 lg:block">{[{icon:Network,title:'Configured access',description:'Agent keys and identity bindings'},{icon:Layers,title:'Virtual MCP servers',description:'Selected capabilities and access protection'},{icon:Server,title:'Upstream connections',description:'Physical MCP servers and discovery'}].map(({icon:Icon,title,description})=><div key={title} className="flex items-center gap-3 rounded-xl border border-line bg-surface p-4"><div className="grid size-9 shrink-0 place-items-center rounded-lg bg-accent-50 text-accent-700"><Icon className="size-4"/></div><div><p className="text-xs font-semibold">{title}</p><p className="mt-1 text-xs text-ink-muted">{description}</p></div></div>)}</div></div>
      <div className="min-w-0">{form}<p className="mt-4 text-center text-[11px] text-ink-muted">Access follows the role assigned to your identity.</p></div>
    </div>
  </main>
}
