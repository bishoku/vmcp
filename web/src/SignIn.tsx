import Button from './Button'
import { ToggleButton } from './SelectionControls'
import { FormField, FormScope, FormErrors } from './FormField'
import { useFormValidation, requiredText } from './form-validation'
import { useEffect, useState } from 'react'
import { ArrowRight, Eye, EyeOff, KeyRound, Layers, LoaderCircle, Network, Server, ShieldCheck } from 'lucide-react'
import { api, ApiError, authConfig, clearToken, setToken, type Me } from './api'
import { useOperation } from './interaction'
import { input, primary } from './styles'
import { ServerPanel } from './connections/ServerPanels'

export default function SignIn({onDone,reauthenticate=false}:{onDone:(identity:Me)=>void;reauthenticate?:boolean}) {
  const [token,setTokenInput]=useState('')
  const [visible,setVisible]=useState(false)
  const validation=useFormValidation()
  const [method,setMethod]=useState<'loading'|'token'|'sso'>('loading')
  const {busy,run}=useOperation()
  useEffect(()=>{
    let active=true
    void authConfig().then(config=>{if(active)setMethod(config.oidc?'sso':'token')})
    return()=>{active=false}
  },[])
  const submit=()=>{
    if(busy||!validation.check({token:requiredText(token,'Admin token')}))return
    void run(async()=>{
      validation.reset()
      setToken(token.trim())
      try {
        const identity=await api.me()
        setTokenInput('');setVisible(false)
        onDone(identity)
      } catch(e) {
        clearToken()
        validation.fail(new ApiError(0,'Sign-in failed',undefined,[{field:'/token',message:e instanceof ApiError&&(e.status===401||e.status===403)?'This token was not accepted. Check the admin token and try again.':'Sign-in is unavailable. Check your connection and control plane availability, then try again.'}]))
      }
    })
  }
  const form=<FormScope validation={validation}><ServerPanel title={reauthenticate?'Restore access':'Administrative access'} description={reauthenticate?'Verify your identity to continue in this workspace':method==='sso'?'Use SSO or an admin token to open the control plane':'Use an admin API token to open the control plane'} icon={ShieldCheck}>
    <div className="space-y-5 p-5 sm:p-6">
      {method==='loading'&&<p role="status" className="inline-flex items-center gap-2 text-xs text-ink-muted"><LoaderCircle className="size-3.5 animate-spin"/>Checking sign-in options…</p>}
      {method==='sso'&&<><div className="space-y-3"><a className={`${primary} w-full py-2.5 ${busy?'pointer-events-none opacity-50':''}`} aria-disabled={busy||undefined} tabIndex={busy?-1:undefined} href="/auth/login"><ShieldCheck className="size-4"/>Sign in with SSO<ArrowRight className="ml-auto size-4"/></a><p className="text-xs leading-relaxed text-ink-muted">Use your organization’s identity provider for regular access.</p></div><div className="flex items-center gap-3"><span className="h-px flex-1 bg-line"/><span className="text-[11px] text-ink-muted">or use an admin token</span><span className="h-px flex-1 bg-line"/></div></>}
      <form onSubmit={e=>{e.preventDefault();submit()}} className="space-y-4" aria-label="Token sign-in">
        <FormErrors validation={validation} title="Unable to sign in"/>
        <FormField name="token" label="Admin token" required hint="Use an admin API token. Your role determines which settings you can change." renderControl={control=><div className="relative"><KeyRound aria-hidden="true" className="absolute left-3 top-3 size-4 text-ink-muted"/>{control}<ToggleButton variant="ghost" className="absolute right-1 top-1 p-1.5" disabled={busy} aria-label={visible?'Hide token':'Show token'} aria-pressed={visible} onClick={()=>setVisible(value=>!value)}>{visible?<EyeOff className="size-4"/>:<Eye className="size-4"/>}</ToggleButton></div>}><input disabled={busy} type={visible?'text':'password'} autoComplete="off" autoCapitalize="none" spellCheck={false} className={`${input} signin-token-input py-2.5 pl-9 pr-14`} value={token} onChange={e=>setTokenInput(e.target.value)}/></FormField>
        <Button type="submit" loading={busy} loadingLabel="Signing in…" variant={method==='sso'?'secondary':'primary'} className="w-full py-2.5">Continue<ArrowRight aria-hidden="true" className="size-4"/></Button>
        {busy&&<p role="status" className="text-xs text-ink-muted">Verifying your session…</p>}
      </form>
      <div className="flex items-start gap-2 rounded-xl border border-line bg-canvas p-3 text-xs leading-relaxed text-ink-muted"><ShieldCheck className="mt-0.5 size-4 shrink-0"/><p>Token sign-in lasts until this tab reloads. {reauthenticate?'Signing in with a token keeps your current draft.':'The token is held in memory for this tab.'}</p></div>
    </div>
  </ServerPanel></FormScope>
  if(reauthenticate)return form
  return <main className="h-full overflow-y-auto px-4 py-8 sm:px-6 sm:py-12">
    <div className="mx-auto flex min-h-full max-w-5xl flex-col justify-center gap-8 lg:grid lg:grid-cols-[1fr_1fr] lg:items-center lg:gap-16">
      <div className="space-y-7"><div className="flex items-center gap-3"><div className="grid size-11 place-items-center rounded-xl bg-accent-500 text-xl font-semibold text-white shadow-card">v</div><div><p className="text-lg font-semibold tracking-tight">vMCP</p><p className="text-xs text-ink-muted">MCP control plane</p></div></div><div><h1 className="page-title">Sign in to vMCP</h1><p className="page-description mt-3 max-w-md text-sm leading-relaxed text-ink-muted">Manage upstream connections, compose virtual servers and configure access from one workspace.</p></div><div className="hidden space-y-3 lg:block">{[{icon:Network,title:'Configured access',description:'Agent keys and identity bindings'},{icon:Layers,title:'Virtual MCP servers',description:'Selected capabilities and access protection'},{icon:Server,title:'Upstream connections',description:'Physical MCP servers and discovery'}].map(({icon:Icon,title,description})=><div key={title} className="flex items-center gap-3 rounded-xl border border-line bg-surface p-4"><div className="grid size-9 shrink-0 place-items-center rounded-lg bg-accent-50 text-accent-700"><Icon className="size-4"/></div><div><p className="text-xs font-semibold">{title}</p><p className="mt-1 text-xs text-ink-muted">{description}</p></div></div>)}</div></div>
      <div className="min-w-0">{form}<p className="mt-4 text-center text-[11px] text-ink-muted">Access follows the role assigned to your identity.</p></div>
    </div>
  </main>
}
