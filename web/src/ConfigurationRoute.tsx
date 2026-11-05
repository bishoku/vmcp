import { type ReactNode } from 'react'
import { useLocation } from 'react-router-dom'
import { useConfigurationChecks } from './configuration-access'
import { ErrorBox } from './ui'
import { LoadingState } from './Feedback'
export function ConfigurationRoute({children}:{children:ReactNode}){
 const location=useLocation(),root=location.pathname.split('/')[1]
 const path=root==='connections'||root==='setup'?'/virtual-servers':root==='ownership'?'/teams':root==='identities'?'/identity-bindings':root==='settings'?'/admin-tokens':'/'+root
 const slug=['upstreams','virtual-servers'].includes(root)?location.pathname.split('/')[2]:undefined
 const probe=slug&&slug!=='new'?path+'/'+slug:path
 const alternatives=root==='settings'?['/admin-tokens','/info','/audit-settings','/export']:root==='ownership'?['/teams','/projects','/tool-grants']:[probe]
 const result=useConfigurationChecks(slug==='new'?[{method:'GET',path,permission:root==='upstreams'?'upstreams.create':'virtual_servers.create'}]:alternatives.map(path=>({method:'GET',path})))

 if(!result.ready)return <div className="page-layout"><LoadingState label="Checking configuration access…"/></div>
 if(!result.allowed.some(Boolean))return <div className="page-layout"><ErrorBox title="Configuration access denied">Your current permissions do not allow this configuration view.</ErrorBox></div>
 return children
}
