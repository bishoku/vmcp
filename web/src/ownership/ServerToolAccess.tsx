import { approvalLabel } from './grants'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type ToolGrant, type VirtualServer } from '../api'
import Button from '../Button'
import { Checkbox } from '../SelectionControls'
import { ErrorBox } from '../ui'
import BulkGrantDialog from './BulkGrantDialog'


const id = (tool: {upstream:string;tool:string}) => JSON.stringify([tool.upstream,tool.tool])

export default function ServerToolAccess({ server, canEdit, onReload }: { server: VirtualServer; canEdit: boolean; onReload: () => Promise<unknown> }) {
 const [selected, setSelected] = useState<string[]>([]), [items,setItems] = useState<ToolGrant[]>()
 const [busy,setBusy] = useState(false), [error,setError] = useState('')
 const available = server.tools.filter(tool => ['approved','same_team','not_required'].includes(tool.approvalStatus ?? '')).length
 const missing = server.tools.filter(tool => tool.approvalStatus === 'required')
 const pending = server.tools.filter(tool => tool.approvalStatus === 'pending').length
 const candidates = server.tools.filter(tool => ['required','rejected','revoked'].includes(tool.approvalStatus ?? ''))
 const request = async (all: boolean) => {
  setBusy(true);setError('')
  try {
   const grants = (await api.toolGrants()).items
   const tools = all ? missing.slice(0,200) : candidates.filter(tool => selected.includes(id(tool)))
   const next = tools.flatMap(tool => {
    const grant = grants.find(g => g.virtualServer === server.slug && g.upstream === tool.upstream && g.tool === tool.tool)
    if (grant?.status === 'approved' || grant?.status === 'pending') return []
    return [{virtualServer:server.slug,upstream:tool.upstream,tool:tool.tool,status:grant?.status ?? 'pending',reason:'',version:grant?.version ?? 0} as ToolGrant]
   })
   if (!next.length) {setError('These tools no longer need a request. Refresh the server to see current approval states.');return}
   setItems(next)
  } catch (failure) {setError(failure instanceof Error ? failure.message : String(failure))}
  finally {setBusy(false)}
 }
 return <section className="border-b border-line panel-body space-y-3" aria-label="Upstream tool approvals">
  <p className="text-sm">{server.tools.length} saved tools · {available} approval-cleared · {missing.length} awaiting request · {pending} pending approval</p>
  <p className="supporting-copy text-ink-muted">Saving tool selections does not submit access requests. Approval-cleared tools can still be hidden by policy, disabled upstreams or discovery failures.</p>
  {server.tools.length > 0 && available === 0 && <p role="status" className="text-sm text-amber-700">No saved tools have cleared upstream approval. Agents cannot list these tools until approval is granted.</p>}
  <Link className="text-sm text-accent-700 underline" to={`/ownership?section=access&server=${encodeURIComponent(server.slug)}`}>View requests and decisions</Link>
  {error && <ErrorBox>{error}</ErrorBox>}
  {canEdit && <><div className="flex flex-wrap gap-2"><Button loading={busy} disabled={!missing.length || !!items} onClick={() => void request(true)}>{missing.length > 200 ? 'Request next 200 missing approvals' : 'Request all missing approvals'} ({missing.length})</Button><Button disabled={busy || !selected.length || selected.length>200 || !!items} onClick={() => void request(false)}>Request access for selected ({selected.length})</Button></div>
   {candidates.length > 0 && <details><summary className="cursor-pointer text-sm">Select tools to request or request again ({candidates.length})</summary><p className="supporting-copy my-2">Rejected and revoked tools are included only when explicitly selected. Maximum 200 tools per operation.</p><div className="max-h-72 overflow-auto space-y-2">{candidates.map(tool => <Checkbox key={id(tool)} label={`${tool.upstream} / ${tool.tool} — ${approvalLabel(tool.approvalStatus)}`} checked={selected.includes(id(tool))} disabled={busy || !!items} onChange={event => setSelected(previous => event.target.checked ? [...previous,id(tool)] : previous.filter(value => value!==id(tool)))}/>)}</div></details>}
  </>}
  {items && <BulkGrantDialog items={items} status="pending" onClose={() => {setItems(undefined);setSelected([])}} onComplete={async () => {setSelected([]);await onReload()}}/>}
 </section>
}
