import {useContext,useEffect,useState} from 'react'
import {api,type Team,type Project} from './api'
import {RoleContext} from './interaction'
import {input} from './styles'
import Select from './Select'
export function TeamSelect({value,onChange,existing=false}:{value:string;onChange:(v:string)=>void;existing?:boolean}){
 const role=useContext(RoleContext),[teams,setTeams]=useState<Team[]>([]),[error,setError]=useState('')
 useEffect(()=>{let active=true;const load=()=>{void api.teams().then(r=>{if(active){setTeams(r.items);setError('')}}).catch(()=>{if(active)setError('Teams could not be loaded. Reload before selecting an owner.')})};load();window.addEventListener('vmcp:ownership-changed',load);return()=>{active=false;window.removeEventListener('vmcp:ownership-changed',load)}},[])
 return <label className="block text-sm">Owner team<Select className={`${input} mt-2`} value={value} disabled={existing&&role!=='admin'} onChange={e=>onChange(e.target.value)}><option value="">Default — no team ownership or tool approval</option>{teams.filter(t=>role==='admin'||t.canManage||t.slug===value).map(t=><option key={t.slug} value={t.slug}>{t.name}</option>)}</Select>{error&&<span role="alert">{error}</span>}</label>
}
export function ProjectSelect({value,onChange,existing=false}:{value:string;onChange:(v:string)=>void;existing?:boolean}){
 const role=useContext(RoleContext),[projects,setProjects]=useState<Project[]>([]),[error,setError]=useState('')
 useEffect(()=>{let active=true;void api.projects().then(r=>{if(active)setProjects(r.items)}).catch(()=>{if(active)setError('Projects could not be loaded. Reload before selecting a project.')});return()=>{active=false}},[])
 return <label className="block text-sm">Project<Select className={`${input} mt-2`} value={value} disabled={existing&&role!=='admin'} onChange={e=>onChange(e.target.value)}><option value="default">Default — existing workspace permissions</option>{projects.filter(p=>p.slug!=='default').map(p=><option key={p.slug} value={p.slug}>{p.name} · {p.ownerTeam||'Default'}</option>)}</Select>{error&&<span role="alert">{error}</span>}</label>
}
