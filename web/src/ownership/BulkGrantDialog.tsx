import { groupGrants } from './grants'
import { useState } from 'react'
import { api, type ToolGrant, type BulkToolGrantResult } from '../api'
import Button from '../Button'
import { Modal, ErrorBox } from '../ui'
import { input } from '../styles'

export default function BulkGrantDialog({ items, status, onClose, onComplete }: {
 items: ToolGrant[]; status: ToolGrant['status']; onClose: () => void; onComplete: () => Promise<void>
}) {
 const [reason, setReason] = useState(''), [busy, setBusy] = useState(false)
 const [result, setResult] = useState<BulkToolGrantResult>(), [error, setError] = useState('')
 const label = status === 'pending' ? 'Request access' : status === 'approved' ? 'Approve access' : status === 'rejected' ? 'Reject requests' : 'Revoke access'
 const groups = groupGrants(items)
 const submit = async () => {
  setBusy(true); setError('')
  try {
   const outcome = await api.bulkToolGrants(items.map(item => ({ ...item, status, reason: reason.trim() || item.reason })))
   setResult(outcome)
   await onComplete()
  } catch (failure) { setError(`${failure instanceof Error ? failure.message : String(failure)} Refresh before trying again; some updates may have completed.`) }
  finally { setBusy(false) }
 }
 return <Modal title={label} description="Review the selected tools before submitting. Each tool keeps its own approval and audit record." size="wide" busy={busy} onClose={onClose}
  footer={<div className="flex justify-end gap-2"><Button disabled={busy} onClick={onClose}>{result || error ? 'Close' : 'Cancel'}</Button>{!result && !error && <Button variant={status === 'revoked' ? 'danger' : 'primary'} loading={busy} disabled={!items.length || items.length > 200 || ((status === 'rejected' || status === 'revoked') && !reason.trim())} onClick={() => void submit()}>{label} ({items.length})</Button>}</div>}>
  <div className="space-y-4">
   {error && <ErrorBox>{error}</ErrorBox>}
   {result && <div role="status" className="surface-inset rounded-xl p-4"><p>{result.changed} updated · {result.unchanged} unchanged · {result.failed} failed</p><p className="supporting-copy mt-2">Approval changes take effect after configuration publication. This does not confirm data-plane propagation.</p>{result.items.filter(item => item.result === 'failed').map(item => <p key={JSON.stringify([item.virtualServer,item.upstream,item.tool])} className="mt-2 text-sm">{item.virtualServer} / {item.upstream} / {item.tool}: {item.code} — {item.message}</p>)}</div>}
   {!result && <><p>{items.length} selected tools in {groups.size} server/upstream groups. Maximum 200 per operation.</p><label className="block text-sm font-medium">{status === 'pending' ? 'Request reason (optional)' : 'Decision reason'}<textarea className={`${input} mt-2`} maxLength={2000} rows={3} value={reason} disabled={busy || !!error} onChange={event => setReason(event.target.value)} required={status === 'rejected' || status === 'revoked'}/></label>{(status === 'rejected' || status === 'revoked') && <p className="supporting-copy">A reason is required. Successful items are not automatically retried.</p>}</>}
   {[...groups].map(([group, tools]) => <section key={group} className="surface-inset rounded-xl p-4"><h3 className="font-medium break-all">{group} · {tools.length} tools</h3>{tools[0].consumerProject && <p className="supporting-copy mt-2">Project: {tools[0].consumerProject} · Consumer team: {tools[0].consumerTeam || 'Default — unmanaged'} · Upstream owner: {tools[0].upstreamOwnerTeam || 'Default — unmanaged'}</p>}<ul className="mt-2 space-y-1 text-sm">{tools.map(tool => <li key={tool.tool} className="break-all">{tool.tool} · {result ? result.items.find(item=>item.virtualServer===tool.virtualServer && item.upstream===tool.upstream && item.tool===tool.tool)?.result ?? 'outcome unknown' : tool.status}{tool.reason && <span className="text-ink-muted"> · {tool.reason}</span>}</li>)}</ul></section>)}
  </div>
 </Modal>
}
