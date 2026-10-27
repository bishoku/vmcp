import { useContext, useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { Clock3, FolderKanban, Layers3, Plus, RefreshCw, ShieldCheck, Users } from 'lucide-react'
import { api, type Project, type Team, type ToolGrant, type VirtualServer } from '../api'
import { RoleContext, useOperation } from '../interaction'
import { useNotifications } from '../notifications'
import { useConfirm } from '../confirmation'
import { source, useSources } from '../requests'
import { btn, input, primary } from '../styles'
import Button from '../Button'
import Select from '../Select'
import { Badge } from '../Badge'
import { DataTable, LoadMore, RowActions } from '../DataTable'
import { EmptyState, LoadingState } from '../Feedback'
import { SearchField, ResultCount } from '../SearchControls'
import { useTableSort } from '../table-sorting'
import { DetailRow, ServerPanel, SummaryTile } from '../connections/ServerPanels'
import { ErrorBox, Modal } from '../ui'
import OwnershipEditor, { type OwnershipSelection } from '../ownership/OwnershipEditor'
import AccessRequest from '../ownership/AccessRequest'

const sections = [
 { id: 'teams', label: 'Teams', icon: Users, description: 'Map identity-provider groups to resource owners.' },
 { id: 'projects', label: 'Projects', icon: FolderKanban, description: 'Organize virtual servers under a single owner team.' },
 { id: 'access', label: 'Tool access', icon: ShieldCheck, description: 'Review permissions to expose upstream tools through virtual servers.' },
] as const
const grantTone = { pending: 'info', approved: 'success', rejected: 'warning', revoked: 'danger' } as const
const grantKey = (grant: ToolGrant) => JSON.stringify([grant.virtualServer, grant.upstream, grant.tool])
const newTeam: Team = { slug: '', name: '', issuer: '', group: '', version: 0 }
const newProject: Project = { slug: '', name: '', ownerTeam: '', version: 0 }

export default function Ownership() {
 const role = useContext(RoleContext), admin = role === 'admin', canEdit = role !== 'viewer'
 const [params, setParams] = useSearchParams()
 const section = sections.find(item => item.id === params.get('section')) ?? sections[0]
 const [teams, setTeams] = useState<Team[]>([]), [projects, setProjects] = useState<Project[]>([])
 const [grants, setGrants] = useState<ToolGrant[]>([]), [servers, setServers] = useState<VirtualServer[]>([])
 const [query, setQuery] = useState(''), [status, setStatus] = useState(''), [limit, setLimit] = useState(50)
 const [selection, setSelection] = useState<OwnershipSelection | null>(null), [requesting, setRequesting] = useState(false)
 const [review, setReview] = useState<ToolGrant | null>(null), [error, setError] = useState('')
 const { busy, run } = useOperation(), { notify } = useNotifications(), confirm = useConfirm()
 const { load, loading, readError } = useSources(() => [
  source('Teams', api.teams, response => setTeams(response.items)),
  source('Projects', api.projects, response => setProjects(response.items)),
  source('Tool access', api.toolGrants, response => setGrants(response.items)),
  source('Virtual servers', api.listVirtualServers, setServers),
 ])
 useEffect(() => { const timer = setTimeout(() => { void load() }, 0); return () => clearTimeout(timer) }, [load])
 const teamName = (slug: string) => teams.find(team => team.slug === slug)?.name || slug || 'Default — unmanaged'
 const matches = (...values: string[]) => values.join(' ').toLowerCase().includes(query.trim().toLowerCase())
 const shownTeams = teams.filter(team => matches(team.name, team.slug, team.group, team.issuer))
 const shownProjects = projects.filter(project => matches(project.name, project.slug, teamName(project.ownerTeam)))
 const shownGrants = grants.filter(grant => (!status || grant.status === status) && matches(grant.virtualServer, grant.upstream, grant.tool, grant.reason))
 const teamSort = useTableSort(shownTeams, { Team: team => team.name, 'Identity group': team => team.group, Projects: team => projects.filter(project => project.ownerTeam === team.slug).length })
 const projectSort = useTableSort(shownProjects, { Project: project => project.name, 'Owner team': project => teamName(project.ownerTeam), 'Virtual servers': project => servers.filter(server => (server.project || 'default') === project.slug).length })
 const grantSort = useTableSort(shownGrants, { Tool: grant => grant.tool, 'Virtual server': grant => grant.virtualServer, Status: grant => grant.status })
 const total = section.id === 'teams' ? teams.length : section.id === 'projects' ? projects.length : grants.length
 const count = section.id === 'teams' ? shownTeams.length : section.id === 'projects' ? shownProjects.length : shownGrants.length
 const clear = () => { setQuery(''); setStatus(''); setLimit(50) }
 const create = () => {
  setError('')
  if (section.id === 'teams') setSelection({ kind: 'team', value: newTeam })
  else if (section.id === 'projects') setSelection({ kind: 'project', value: newProject })
  else setRequesting(true)
 }
 const createLabel = section.id === 'teams' ? 'New team' : section.id === 'projects' ? 'New project' : 'Request access'
 const canCreate = section.id === 'access' ? canEdit : admin
 const saved = async (title: string) => {
  setSelection(null); setRequesting(false); setReview(null); setError('')
  notify({ title }); window.dispatchEvent(new Event('vmcp:ownership-changed')); await load()
 }
 const decide = (grant: ToolGrant, next: ToolGrant['status']) => run(async () => {
  if (!canEdit || !(admin || grant.canDecide === true)) return
  if (!await confirm({ title: `${next === 'approved' ? 'Approve' : next === 'rejected' ? 'Reject' : 'Revoke'} tool access?`, description: `${grant.virtualServer} → ${grant.upstream} / ${grant.tool}. ${next === 'approved' ? 'This grants permission to expose the tool on this virtual server.' : 'The tool will no longer be available through this grant after the configuration updates.'}`, confirmLabel: next === 'approved' ? 'Approve access' : next === 'rejected' ? 'Reject request' : 'Revoke access', tone: next === 'revoked' ? 'danger' : 'warning' })) return
  setError('')
  try { await api.saveToolGrant({ ...grant, status: next }); await saved(`Tool access ${next}`) }
  catch (failure) { setError(failure instanceof Error ? failure.message : String(failure)) }
 })
 const cell = 'panel-inset py-4'
 return <div className="page-layout">
  <header className="flex flex-wrap items-start justify-between gap-4"><div><h1 className="page-title">Teams &amp; projects</h1><p className="page-description mt-2 max-w-2xl text-sm leading-relaxed text-ink-muted">Manage resource ownership and tool access. Team membership stays in your identity provider.</p></div><div className="flex flex-wrap gap-2"><Button loading={loading} className={btn} disabled={loading || busy} onClick={() => { setError(''); void load() }}><RefreshCw aria-hidden="true" className="icon-inline"/>Refresh list</Button>{canCreate && <button className={primary} disabled={loading || busy || !!readError} onClick={create}><Plus aria-hidden="true" className="icon-control"/>{createLabel}</button>}</div></header>
  {loading && <LoadingState compact label="Loading ownership configuration…"/>}{readError && <ErrorBox title="Saved configuration could not be loaded" onRetry={load} busy={loading}>{readError}. Available data remains visible; some sections may be incomplete.</ErrorBox>}
  <div className="grid grid-cols-2 gap-3 xl:grid-cols-4"><SummaryTile label="Teams" value={teams.length} detail="Identity-provider group mappings" icon={Users}/><SummaryTile label="Projects" value={projects.length} detail="Including the Default workspace" icon={FolderKanban}/><SummaryTile label="Managed virtual servers" value={servers.filter(server => projects.some(project => project.slug === server.project && project.ownerTeam)).length} detail="Servers in team-owned projects" icon={Layers3}/><SummaryTile label="Pending requests" value={grants.filter(grant => grant.status === 'pending').length} detail="Visible requests awaiting a decision" icon={Clock3}/></div>
  <nav aria-label="Ownership sections" className="flex flex-wrap gap-2">{sections.map(item => <button key={item.id} type="button" className={`${btn} aria-[current=page]:border-accent-100 aria-[current=page]:bg-accent-50 aria-[current=page]:text-accent-700`} aria-current={section.id === item.id ? 'page' : undefined} onClick={() => { setParams({ section: item.id }); clear() }}><item.icon aria-hidden="true" className="icon-inline"/>{item.label}{item.id === 'access' && grants.some(grant => grant.status === 'pending') && <span className="tabular-nums">({grants.filter(grant => grant.status === 'pending').length})</span>}</button>)}</nav>
  <ServerPanel title={section.label} description={section.description} icon={section.icon}>
   <div className="flex flex-wrap items-center gap-3 border-b border-line panel-body"><SearchField label={`Search ${section.label.toLowerCase()}`} value={query} onChange={value => { setQuery(value); setLimit(50) }} placeholder={section.id === 'teams' ? 'Name, identifier, group or issuer' : section.id === 'projects' ? 'Name, identifier or owner' : 'Tool, upstream, virtual server or reason'} className="w-full sm:max-w-sm"/>{section.id === 'access' && <Select aria-label="Access request status" className={`${input} w-full sm:w-44`} value={status} onChange={event => { setStatus(event.target.value); setLimit(50) }}><option value="">All statuses</option>{(['pending', 'approved', 'rejected', 'revoked'] as const).map(value => <option key={value} value={value}>{value[0].toUpperCase() + value.slice(1)}</option>)}</Select>}<ResultCount matched={count} total={total} noun={section.id === 'access' ? 'requests' : section.id} busy={loading} note={readError ? 'last loaded configuration' : undefined}/>{(query || status) && <button className={btn} onClick={clear}>Clear filters</button>}</div>
   {section.id === 'teams' && count > 0 && <DataTable caption="Configured teams" columns={['Team', 'Identity group', 'Projects', 'Actions']} sorting={teamSort} actions={3}>{teamSort.rows.slice(0, limit).map(team => <tr key={team.slug} className="border-t border-line hover:bg-canvas/50"><td className={cell}><button className="text-left font-medium text-accent-700 hover:underline break-words" onClick={() => setSelection({ kind: 'team', value: team })}>{team.name}</button><p className="mt-1 font-mono text-xs text-ink-muted break-all">{team.slug}</p></td><td className={`${cell} max-w-sm`}><p className="break-all">{team.group}</p><p className="mt-1 text-xs text-ink-muted break-all">{team.issuer}</p></td><td className={cell}>{projects.filter(project => project.ownerTeam === team.slug).length}</td><td className={cell}><RowActions label={`Actions for ${team.name}`}><button className={btn} onClick={() => setSelection({ kind: 'team', value: team })}>View team</button></RowActions></td></tr>)}</DataTable>}
   {section.id === 'projects' && count > 0 && <DataTable caption="Configured projects" columns={['Project', 'Owner team', 'Virtual servers', 'Actions']} sorting={projectSort} actions={3}>{projectSort.rows.slice(0, limit).map(project => <tr key={project.slug} className="border-t border-line hover:bg-canvas/50"><td className={cell}><button className="text-left font-medium text-accent-700 hover:underline break-words" onClick={() => setSelection({ kind: 'project', value: project })}>{project.name}</button><p className="mt-1 font-mono text-xs text-ink-muted break-all">{project.slug}</p></td><td className={cell}>{project.ownerTeam ? <span className="break-words">{teamName(project.ownerTeam)}</span> : <Badge>Unmanaged</Badge>}</td><td className={cell}>{servers.filter(server => (server.project || 'default') === project.slug).length}</td><td className={cell}><RowActions label={`Actions for ${project.name}`}><button className={btn} onClick={() => setSelection({ kind: 'project', value: project })}>View project</button></RowActions></td></tr>)}</DataTable>}
   {section.id === 'access' && count > 0 && <DataTable caption="Tool access requests" columns={['Tool', 'Virtual server', 'Status', 'Actions']} sorting={grantSort} actions={3}>{grantSort.rows.slice(0, limit).map(grant => <tr key={grantKey(grant)} className="border-t border-line hover:bg-canvas/50"><td className={cell}><p className="font-medium break-all">{grant.tool}</p><p className="mt-1 text-xs text-ink-muted break-all">{grant.upstream}</p></td><td className={cell}><Link className="text-accent-700 hover:underline break-all" to={`/virtual-servers/${encodeURIComponent(grant.virtualServer)}`}>{grant.virtualServer}</Link></td><td className={cell}><Badge tone={grantTone[grant.status]}>{grant.status[0].toUpperCase() + grant.status.slice(1)}</Badge></td><td className={cell}><RowActions label={`Actions for ${grant.virtualServer} ${grant.tool}`}><button className={btn} onClick={() => { setError(''); setReview(grant) }}>{canEdit && (admin || grant.canDecide === true) ? 'Review request' : 'View request'}</button></RowActions></td></tr>)}</DataTable>}
   {!loading && count === 0 && <EmptyState icon={section.icon} title={readError ? `${section.label} unavailable` : query || status ? 'No matching results' : section.id === 'teams' ? 'No teams yet' : section.id === 'projects' ? 'No projects yet' : 'No access requests yet'} description={readError ? 'Retry loading before assuming there are no saved records.' : query || status ? 'Try another search or clear the filters.' : section.id === 'teams' ? 'Map an identity group to a team, then assign it to projects and upstreams.' : section.id === 'projects' ? 'Create a project to organize virtual servers and assign their owner.' : 'Requests let upstream owners review which virtual servers can expose their tools.'} actions={query || status ? <button className={btn} onClick={clear}>Clear filters</button> : canCreate && !readError ? <button className={btn} onClick={create}>{createLabel}</button> : undefined}/>}
   {count > 0 && <LoadMore shown={Math.min(limit, count)} total={count} noun={section.id === 'access' ? 'requests' : section.id} label="Show 50 more" onLoad={() => setLimit(value => value + 50)}/>}
  </ServerPanel>
  <p className="text-xs leading-relaxed text-ink-muted">{section.id === 'teams' ? 'Only platform admins can change identity mappings. Users and group membership are managed outside vMCP.' : section.id === 'projects' ? 'Default keeps existing workspace permissions. Upstream ownership is configured independently on each upstream.' : 'Approval allows a tool to be exposed on one virtual server; it does not approve each invocation.'}</p>
  {selection && <OwnershipEditor selection={selection} teams={teams} admin={admin} onClose={() => setSelection(null)} onSaved={saved}/>}
  {requesting && <AccessRequest servers={servers} grants={grants} onClose={() => setRequesting(false)} onSaved={saved}/>}
  {review && <Modal title="Tool access request" description="Review the consumer, upstream and reason before making a decision." variant="drawer" size="wide" busy={busy} onClose={() => setReview(null)} footer={<div className="flex flex-wrap justify-end gap-2"><button className={btn} onClick={() => setReview(null)}>Close</button>{canEdit && (admin || review.canDecide === true) && <>{review.status !== 'approved' && <Button className={primary} loading={busy} onClick={() => void decide(review, 'approved')}>Approve</Button>}{review.status === 'pending' && <Button className={btn} loading={busy} onClick={() => void decide(review, 'rejected')}>Reject</Button>}{review.status === 'approved' && <Button className={`${btn} action-danger-outline`} loading={busy} onClick={() => void decide(review, 'revoked')}>Revoke</Button>}</>}</div>}>
   {error && <ErrorBox>{error} Close this panel and refresh the list if another owner has updated the request.</ErrorBox>}<dl className="divide-y divide-line"><DetailRow label="Status"><Badge tone={grantTone[review.status]}>{review.status}</Badge></DetailRow><DetailRow label="Virtual server">{review.virtualServer}</DetailRow><DetailRow label="Upstream">{review.upstream}</DetailRow><DetailRow label="Tool">{review.tool}</DetailRow><DetailRow label="Reason"><span className="whitespace-pre-wrap">{review.reason || 'No reason provided.'}</span></DetailRow><DetailRow label="Saved version">{review.version}</DetailRow></dl>{!admin && review.canDecide !== true && <p className="mt-5 supporting-copy text-ink-muted">Only the upstream owner or a platform admin can decide this request.</p>}
  </Modal>}
 </div>
}
