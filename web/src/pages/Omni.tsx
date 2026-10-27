import { useContext, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Network, Search, RefreshCw, ShieldCheck } from 'lucide-react'
import { api, type OmniStatus, type OmniHit } from '../api'
import { RoleContext, useOperation } from '../interaction'
import { source, useSources } from '../requests'
import { useNotifications } from '../notifications'
import { ServerPanel, SummaryTile } from '../connections/ServerPanels'
import { ErrorBox, CopyButton, Modal } from '../ui'
import { LoadingState, EmptyState } from '../Feedback'
import Button from '../Button'
import { btn, input } from '../styles'

export default function Omni() {
 const admin=useContext(RoleContext)==='admin'
 const [status,setStatus]=useState<OmniStatus|null>(null),[token,setToken]=useState(''),[query,setQuery]=useState('')
 const [hits,setHits]=useState<OmniHit[]|null>(null),[mode,setMode]=useState(''),[error,setError]=useState(''),[schema,setSchema]=useState<unknown>(null)
 const {busy,run}=useOperation(),{notify}=useNotifications()
 const {load,loading,readError}=useSources(()=>[source('Omni MCP',api.omniStatus,setStatus)])
 useEffect(()=>{void load()},[load])
 const execute=(fn:()=>Promise<void>)=>run(async()=>{setError('');try{await fn()}catch(e){setError(e instanceof Error?e.message:String(e))}})
 const endpoint=status?.endpoint?`${status.endpoint.replace(/\/$/,'')}/mcp/v1/omni`:''
 return <div className="page-layout">
  <header className="page-header"><div><h1 className="page-title">Omni MCP</h1><p className="page-description">Discover tools by intent, then use them within existing virtual server permissions.</p></div><div className="page-header-actions"><Button onClick={()=>void load()} loading={loading}><RefreshCw className="icon-control"/>Refresh</Button>{admin&&status?.configured&&<Button disabled={busy} onClick={()=>void execute(async()=>{await api.omniReindex();notify({title:'Index refresh requested'});await load()})}>Refresh index</Button>}</div></header>
  {readError&&<ErrorBox onRetry={load}>{readError}</ErrorBox>}{error&&<ErrorBox>{error}</ErrorBox>}
  {loading&&!status?<LoadingState label="Loading Omni configuration…"/>:status&&!status.configured?<EmptyState title="Omni MCP is not configured" description="Deploy the optional Omni component and configure its connection on the control plane. Existing virtual MCP endpoints remain available."/>:status&&<>
   {status.index&&<><div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4"><SummaryTile label="Index" value={status.index.ready?'Ready':'Preparing'} detail={status.index.refreshing?'Refresh in progress':'Latest published catalog'} icon={Network}/><SummaryTile label="Search mode" value={status.index.mode||'Starting'} detail="Hybrid search requires a configured embedding model" icon={Search}/><SummaryTile label="Indexed tools" value={status.index.documents} detail="Published virtual tool entries" icon={Network}/><SummaryTile label="Authorization" value="Runtime" detail="Checked for every discovery, schema and invocation request" icon={ShieldCheck}/></div>{status.index.error&&<ErrorBox title="Index refresh needs attention">{status.index.error}</ErrorBox>}<p className="text-xs text-ink-muted">Last refresh: {status.index.ready?new Date(status.index.updatedAt).toLocaleString():'Not completed'} · Catalog revision {status.index.revision}</p></>}
   <ServerPanel title="Connect an agent" description="Use an existing agent key or a gateway OAuth access token. Only explicitly published virtual servers are available." icon={Network}><div className="panel-body space-y-4">{endpoint?<div className="flex items-center gap-3 surface-inset p-3"><code className="min-w-0 flex-1 break-all text-xs">{endpoint}</code><CopyButton value={endpoint}/></div>:<p className="text-xs text-ink-muted">Set the public Omni URL to display connection instructions.</p>}{endpoint&&<pre className="overflow-auto surface-inset p-3 text-xs">{JSON.stringify({mcpServers:{vmcpOmni:{url:endpoint,headers:{Authorization:'Bearer <runtime-token>'}}}},null,2)}</pre>}<p className="text-xs text-ink-muted">Available tools: discover_server, discover_tools, get_tool_schema, invoke_tool.</p><Link className={btn} to="/virtual-servers">Manage virtual server publication</Link></div></ServerPanel>
   <ServerPanel title="Try discovery" description="Test with an explicit runtime identity. Your management session does not grant tool access. Credentials stay in this page's memory and are not saved." icon={Search}><form className="panel-body space-y-4" onSubmit={e=>{e.preventDefault();void execute(async()=>{setHits(null);const r=await api.omniPreview(token,query);setHits(r.tools??[]);setMode(r.search_mode??'')})}}>
    <label className="block text-xs font-medium" htmlFor="omni-token">Agent key or OAuth access token<input id="omni-token" disabled={busy} type="password" autoComplete="off" className={`${input} mt-2`} value={token} onChange={e=>{setToken(e.target.value);setHits(null);setSchema(null)}} required/></label>
    <label className="block text-xs font-medium" htmlFor="omni-query">Intent<input id="omni-query" disabled={busy} className={`${input} mt-2`} placeholder="Find a customer's open orders" maxLength={4096} value={query} onChange={e=>setQuery(e.target.value)} required/></label>
    <div className="flex flex-wrap gap-3"><Button type="submit" loading={busy} disabled={!token||!query.trim()}>Find tools</Button><Button type="button" disabled={busy} onClick={()=>{setToken('');setHits(null);setSchema(null)}}>Clear credential</Button></div>
   </form>{hits&&<div className="border-t border-line panel-body"><p className="mb-3 text-xs text-ink-muted">Search mode: {mode}</p>{hits.length===0?<EmptyState title="No accessible matches" description="Check the intent, virtual server publication and this runtime identity's permissions."/>:<ul className="divide-y divide-line">{hits.map(hit=><li key={hit.tool_ref} className="py-4"><div className="flex flex-wrap items-start justify-between gap-3"><div className="min-w-0 flex-1"><p className="break-all text-sm font-medium">{hit.name}</p><p className="mt-1 text-xs text-ink-muted">{hit.server_name} · {hit.project}</p><p className="mt-2 break-words text-xs">{hit.description}</p></div><Button disabled={busy} onClick={()=>void execute(async()=>setSchema(await api.omniPreview(token,'',hit.tool_ref)))}>View schema</Button></div></li>)}</ul>}</div>}</ServerPanel>
  </>}
  {schema!==null&&<Modal title="Published tool schema" variant="drawer" onClose={()=>setSchema(null)}><pre className="overflow-auto whitespace-pre-wrap break-all text-xs">{JSON.stringify(schema,null,2)}</pre></Modal>}
 </div>
}
