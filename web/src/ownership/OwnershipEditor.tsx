import { useState } from 'react'
import { Pencil, Save } from 'lucide-react'
import { api, type Project, type Team } from '../api'
import Button from '../Button'
import Select from '../Select'
import { FormErrors, FormField, FormScope, FormGroup } from '../FormField'
import { requiredText, slugError, useFormValidation } from '../form-validation'
import { useOperation, useUnsaved } from '../interaction'
import { useConfirm } from '../confirmation'
import { btn, input, primary } from '../styles'
import { Modal, StaleRecovery } from '../ui'
import { DetailRow } from '../connections/ServerPanels'

export type OwnershipSelection = { kind: 'team'; value: Team } | { kind: 'project'; value: Project }
export default function OwnershipEditor({ selection, teams, admin, onClose, onSaved }: {
 selection: OwnershipSelection; teams: Team[]; admin: boolean; onClose: () => void; onSaved: (title: string) => Promise<void>
}) {
 const [draft, setDraft] = useState(selection)
 const [baseline, setBaseline] = useState(selection)
 const [editing, setEditing] = useState(selection.value.version === 0)
 const validation = useFormValidation(), confirm = useConfirm()
 const { busy, run } = useOperation(), unsaved = useUnsaved(draft)
 const isNew = selection.value.version === 0, kind = draft.kind, value = draft.value
 const close = () => unsaved.guard(onClose)
 const name = kind === 'team' ? 'team' : 'project'
 const save = () => run(async () => {
  if (!admin || !editing) return
  const fields = draft.kind === 'team' ? { issuer: requiredText(draft.value.issuer, 'Issuer'), group: requiredText(draft.value.group, 'Group claim value') } : {}
  if (!validation.check({ slug: slugError(value.slug), name: requiredText(value.name, 'Name'), ...fields })) return
  if (draft.kind === 'project' && baseline.kind === 'project' && !isNew && draft.value.ownerTeam !== baseline.value.ownerTeam) {
   if (!await confirm({ title: 'Change project ownership?', description: 'Existing explicit tool grants for this project’s virtual servers will be revoked. Review and reapprove their access after the transfer.', confirmLabel: 'Change owner', tone: 'warning' })) return
  }
  if (draft.kind === 'team' && baseline.kind === 'team' && !isNew && (draft.value.issuer !== baseline.value.issuer || draft.value.group !== baseline.value.group)) {
   if (!await confirm({ title: 'Change team identity mapping?', description: 'Access to this team’s resources will follow the new issuer and group claim. Existing members may lose access.', confirmLabel: 'Change mapping', tone: 'warning' })) return
  }
  try {
   if (draft.kind === 'team') await api.saveTeam(draft.value)
   else await api.saveProject(draft.value)
   unsaved.clean(draft)
   await onSaved(`${kind === 'team' ? 'Team' : 'Project'} ${isNew ? 'created' : 'saved'}`)
  } catch (error) { validation.fail(error) }
 })
 const reload = async () => {
  const result = kind === 'team' ? await api.teams() : await api.projects()
  const latest = result.items.find(item => item.slug === value.slug)
  if (!latest) throw new Error('This record is no longer available. Close this dialog and refresh the list.')
  const next = kind === 'team' ? { kind: 'team' as const, value: latest as Team } : { kind: 'project' as const, value: latest as Project }
  setDraft(next); setBaseline(next); unsaved.clean(next); validation.reset()
 }
 return <Modal title={isNew ? `New ${name}` : editing ? `Edit ${name}` : value.name} size="wide" busy={busy} onClose={close}
  description={kind === 'team' ? 'Connect an externally managed identity group to resource ownership.' : 'Group virtual servers under one owner team.'}
  footer={<div className="flex flex-wrap justify-end gap-2"><button type="button" className={btn} disabled={busy} onClick={close}>{editing ? 'Cancel' : 'Close'}</button>{editing ? <Button className={primary} loading={busy} type="submit" form="ownership-editor"><Save aria-hidden="true" className="icon-inline"/>Save {name}</Button> : admin && <button className={primary} onClick={() => setEditing(true)}><Pencil aria-hidden="true" className="icon-inline"/>Edit {name}</button>}</div>}>
  {editing ? <FormScope validation={validation}><form id="ownership-editor" noValidate onSubmit={event => { event.preventDefault(); void save() }} className="space-y-6">
   <FormErrors validation={validation}/><StaleRecovery message={validation.issues.map(issue => issue.message).join(' ')} reload={reload}/>
   <FormGroup name="identity" label={kind === 'team' ? 'Team identity' : 'Project identity'}><div className="grid gap-4 sm:grid-cols-2">
   <FormField name="name" label="Name" required><input className={input} value={value.name} onChange={event => setDraft({ ...draft, value: { ...value, name: event.target.value } } as OwnershipSelection)}/></FormField>
   <FormField name="slug" label="Identifier" required hint={isNew ? 'A stable identifier: lowercase letters, numbers and hyphens.' : 'Identifiers cannot be changed after creation.'}><input className={input} disabled={!isNew} value={value.slug} onChange={event => setDraft({ ...draft, value: { ...value, slug: event.target.value } } as OwnershipSelection)}/></FormField>
   </div></FormGroup>
   {draft.kind === 'team' ? <FormGroup name="mapping" label="Identity provider mapping"><div className="grid gap-4 sm:grid-cols-2">
    <FormField name="issuer" label="OIDC issuer" required hint="Match the issuer from your trusted identity provider exactly."><input className={input} placeholder="https://idp.example.com" value={draft.value.issuer} onChange={event => setDraft({ ...draft, value: { ...draft.value, issuer: event.target.value } })}/></FormField>
    <FormField name="group" label="Group claim value" required hint="Use the group ID or mail group value emitted in the configured groups claim. Membership stays in your identity provider."><input className={input} placeholder="crm-product-owners" value={draft.value.group} onChange={event => setDraft({ ...draft, value: { ...draft.value, group: event.target.value } })}/></FormField>
   </div><p className="supporting-copy mt-4 text-ink-muted">Users and group membership are managed in your identity provider. vMCP uses these claims to determine resource ownership.</p></FormGroup> : <FormGroup name="ownership" label="Resource ownership">
    <FormField name="ownerTeam" label="Owner team" hint={value.slug === 'default' ? 'The built-in Default project always remains unmanaged.' : 'All virtual servers in this project inherit this owner.'}><Select disabled={value.slug === 'default'} value={draft.value.ownerTeam} onChange={event => setDraft({ ...draft, value: { ...draft.value, ownerTeam: event.target.value } })}><option value="">Default — unmanaged</option>{teams.map(team => <option key={team.slug} value={team.slug}>{team.name}</option>)}</Select></FormField>
    <p className="supporting-copy rounded-xl surface-inset p-4 text-ink-muted">Unmanaged projects keep existing workspace permissions. Tools from managed upstreams still require the upstream owner’s approval.</p>
   </FormGroup>}
  </form></FormScope> : <dl className="divide-y divide-line"><DetailRow label="Identifier">{value.slug}</DetailRow><DetailRow label="Name">{value.name}</DetailRow>{draft.kind === 'team' ? <><DetailRow label="OIDC issuer"><span className="break-all">{draft.value.issuer}</span></DetailRow><DetailRow label="Group claim">{draft.value.group}</DetailRow></> : <DetailRow label="Owner team">{teams.find(team => team.slug === draft.value.ownerTeam)?.name || draft.value.ownerTeam || 'Default — unmanaged'}</DetailRow>}<DetailRow label="Saved version">{value.version}</DetailRow></dl>}
 </Modal>
}
