import { useEffect, useState, type ReactNode } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import { BookOpen, FileText, KeyRound, Layers, Menu, Network, Server, Settings, ShieldCheck, UserCheck } from 'lucide-react'
import clsx from 'clsx'
import type { Me } from './api'
import { Modal } from './ui'
import { btn } from './styles'

const groups = [
  {label:'Workspace',items:[{to:'/connections',label:'Connections',icon:Network},{to:'/setup',label:'Setup guide',icon:BookOpen}]},
  {label:'Servers',items:[{to:'/upstreams',label:'Upstreams',icon:Server},{to:'/virtual-servers',label:'Virtual Servers',icon:Layers}]},
  {label:'Access',items:[{to:'/agent-keys',label:'Agent Keys',icon:KeyRound},{to:'/identities',label:'Identities',icon:UserCheck}]},
  {label:'Administration',items:[{to:'/audit',label:'Audit Log',icon:FileText},{to:'/settings',label:'Settings',icon:Settings}]},
]

function Brand(){
  return <div className="flex min-w-0 items-center gap-3"><div className="grid size-10 shrink-0 place-items-center rounded-xl border border-accent-500/20 bg-accent-500 text-lg font-semibold text-white shadow-sm">v</div><div><p className="text-lg font-semibold tracking-tight">vMCP</p><p className="text-[11px] text-ink-muted">MCP control plane</p></div></div>
}

function MenuLinks({mobile=false}:{mobile?:boolean}){
  return <nav aria-label={mobile?'Mobile navigation':'Main navigation'} className="space-y-4">{groups.map(group=><div key={group.label}><p className="mb-1.5 px-3 text-[10px] font-semibold uppercase tracking-widest text-ink-muted">{group.label}</p><div className="space-y-1">{group.items.map(({to,label,icon:Icon})=><NavLink key={to} to={to} className={({isActive})=>clsx('group relative flex min-w-0 items-center gap-3 rounded-xl border px-3 py-2 text-sm font-medium transition-colors',isActive?'border-accent-100 bg-accent-50 text-accent-700 shadow-sm':'border-transparent text-ink-muted hover:border-line hover:bg-canvas hover:text-ink')}>
    {({isActive})=><><Icon aria-hidden="true" className="size-[18px] shrink-0"/><span className="min-w-0 flex-1 truncate">{label}</span>{isActive&&<span aria-hidden="true" className="size-1.5 shrink-0 rounded-full bg-accent-500"/>}</>}
  </NavLink>)}</div></div>)}</nav>
}

function UserArea({me,signOut}:{me:Me;signOut:ReactNode}){
  const initials=me.name.trim().split(/\s+/).slice(0,2).map(part=>part[0]).join('').toUpperCase()||'U'
  return <section aria-label="Signed-in account" className="rounded-xl border border-line bg-canvas p-3">
    <div className="flex items-center gap-3"><div aria-hidden="true" className="grid size-9 shrink-0 place-items-center rounded-lg border border-line bg-surface text-xs font-semibold text-accent-700">{initials}</div><div className="min-w-0 flex-1"><p className="truncate text-xs font-semibold" title={me.name}>{me.name}</p><p className="mt-1 inline-flex items-center gap-1 text-[11px] capitalize text-ink-muted"><ShieldCheck aria-hidden="true" className="size-3"/>{me.role}</p></div></div>
    <div className="mt-3 border-t border-line pt-2">{signOut}</div>
  </section>
}

export default function AppNavigation({me,signOut,sessionExpired=false}:{me:Me;signOut:ReactNode;sessionExpired?:boolean}){
  const location=useLocation()
  const [menu,setMenu]=useState({key:location.key,open:false})
  // A blocked navigation keeps its location key and leaves the menu open.
  if(menu.key!==location.key || (sessionExpired&&menu.open))setMenu({key:location.key,open:false})
  const open=menu.key===location.key&&menu.open&&!sessionExpired
  const setOpen=(value:boolean)=>setMenu({key:location.key,open:value})
  const current=groups.flatMap(group=>group.items).find(item=>location.pathname===item.to||location.pathname.startsWith(`${item.to}/`))
  useEffect(()=>{
    const media=window.matchMedia('(min-width: 768px)')
    const closeOnDesktop=()=>{if(media.matches)setMenu(previous=>({...previous,open:false}))}
    media.addEventListener('change',closeOnDesktop)
    return()=>media.removeEventListener('change',closeOnDesktop)
  },[])
  return <>
    <a href="#main-content" className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-50 focus:rounded-lg focus:border focus:border-line focus:bg-surface focus:px-4 focus:py-3 focus:text-sm">Skip to content</a>
    <aside aria-label="Application sidebar" className="hidden h-full w-64 shrink-0 flex-col border-r border-line bg-surface md:flex">
      <div className="shrink-0 border-b border-line px-6 py-6"><Brand/></div>
      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-5"><MenuLinks/></div>
      <div className="shrink-0 border-t border-line p-4"><UserArea me={me} signOut={signOut}/></div>
    </aside>
    <nav aria-label="Main navigation" className="flex shrink-0 items-center gap-3 border-b border-line bg-surface px-4 py-3 md:hidden">
      <div aria-hidden="true" className="grid size-9 shrink-0 place-items-center rounded-xl bg-accent-500 text-base font-semibold text-white">v</div>
      <div className="min-w-0 flex-1"><p className="text-[10px] font-medium text-ink-muted">vMCP</p><p className="truncate text-sm font-semibold">{current?.label??'Workspace'}</p></div>
      <button className={`${btn} shrink-0`} aria-label="Open navigation" aria-haspopup="dialog" aria-expanded={open} onClick={()=>setOpen(true)}><Menu className="size-4"/>Menu</button>
    </nav>
    {open&&<Modal title="Navigation" variant="drawer" onClose={()=>setOpen(false)}><div className="space-y-6"><Brand/><MenuLinks mobile/><UserArea me={me} signOut={signOut}/></div></Modal>}
  </>
}
