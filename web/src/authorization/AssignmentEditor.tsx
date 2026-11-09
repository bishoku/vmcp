import { useId, useState } from 'react'
import { api } from '../api'
import Button from '../Button'
import Select from '../Select'
import { Checkbox } from '../SelectionControls'
import { Modal } from '../ui'
import type { AssignmentInput, RoleAssignment } from './client'
import { AccessError, Field, ModeSelect } from './common'
import { captureScopes, useEditor, uuid, type AccessData } from './state'
import { PrincipalFields, ScopeFields } from './Selectors'
function assignmentInput(item: RoleAssignment): AssignmentInput {
  return {
    roleId: item.roleId,
    ...(item.mappingId ? { mappingId: item.mappingId } : { principal: { ...item.principal } }),
    scope: { ...item.scope },
    captureScopes: [...item.captureScopes],
    enabled: item.enabled,
  }
}
export default function AssignmentEditor({
  assignment,
  data,
  onClose,
  onSaved,
  onDenied,
}: {
  assignment?: RoleAssignment
  data: AccessData
  onClose: () => void
  onSaved: () => void
  onDenied: () => void
}) {
  const [base, setBase] = useState(assignment),
    formId = useId()
  const e = useEditor<AssignmentInput>(
    assignment
      ? assignmentInput(assignment)
      : {
          roleId: '',
          principal: { kind: 'oidc', id: '', issuer: data.status.trustedIssuer },
          scope: { kind: 'tenant' },
          captureScopes: [],
          enabled: false,
        },
    onDenied,
  )
  const mapped = e.draft.mappingId !== undefined,
    role = data.roles.find((r) => r.id === e.draft.roleId)
  const applicable =
    role?.permissions.filter((id) =>
      data.permissions.find((p) => p.id === id)?.scopes.includes(e.draft.scope.kind),
    ) ?? []
  const capture = role?.permissions.some(
    (p) => data.permissions.find((v) => v.id === p)?.captureRestricted,
  )
  const save = () => {
    if (e.stale) return
    const p = e.draft.principal,
      s = e.draft.scope
    if (
      !role ||
      (mapped ? !e.draft.mappingId : !p?.id) ||
      (!['tenant', 'unmanaged'].includes(s.kind) && !uuid(s.id ?? '')) ||
      (s.epoch && !uuid(s.epoch)) ||
      (s.ownerTeam && !uuid(s.ownerTeam))
    ) {
      e.setError(
        'Select a role, an identity and a valid scope. Resource and ownership identifiers must be UUIDs.',
      )
      return
    }
    if (role.permissions.length && !applicable.length) {
      e.setError('None of the selected role permissions support this scope.')
      return
    }
    if (capture && !e.draft.captureScopes.length) {
      e.setError(
        'Payload permissions require at least one explicit capture scope. Request and response are separate permissions.',
      )
      return
    }
    const issuer = mapped
      ? data.mappings.find((m) => m.id === e.draft.mappingId)?.issuer
      : p?.issuer
    if (
      e.draft.enabled &&
      (mapped || p?.kind === 'oidc') &&
      (!data.status.trustedIssuer || issuer !== data.status.trustedIssuer)
    ) {
      e.setError('Enabled OIDC assignments require the configured issuer exactly.')
      return
    }
    void e.execute(async (signal) => {
      const body: AssignmentInput = {
        roleId: e.draft.roleId,
        ...(mapped
          ? { mappingId: e.draft.mappingId }
          : {
              principal:
                p?.kind === 'oidc'
                  ? { kind: 'oidc', issuer: p.issuer, id: p.id }
                  : { kind: 'control_token', id: p!.id },
            }),
        scope: {
          kind: s.kind,
          ...(s.id ? { id: s.id } : {}),
          ...(s.epoch ? { epoch: s.epoch } : {}),
          ...(s.ownerTeam ? { ownerTeam: s.ownerTeam } : {}),
        },
        captureScopes: [...e.draft.captureScopes],
        enabled: e.draft.enabled,
      }
      if (base) await api.updateAccessAssignment(base, body, signal)
      else await api.createAccessAssignment(body, signal)
      if (!signal.aborted) {
        e.unsaved.clean()
        onSaved()
      }
    })
  }
  const reload = () =>
    e.unsaved.guard(
      () =>
        void e.execute(async (signal) => {
          if (!base) return
          const latest = await api.accessAssignment(base.id, signal)
          if (signal.aborted) return
          setBase(latest)
          const fresh = assignmentInput(latest)
          e.setDraft(fresh)
          e.unsaved.clean(fresh)
          e.setStale(false)
        }),
    )
  return (
    <Modal
      title={base ? 'Edit role assignment' : 'Create role assignment'}
      size="wide"
      busy={e.busy}
      onClose={() => e.unsaved.guard(onClose)}
      footer={
        <div className="flex flex-wrap justify-end gap-2">
          <Button onClick={() => e.unsaved.guard(onClose)}>Cancel</Button>
          <Button form={formId} type="submit" variant="primary" disabled={e.stale} loading={e.busy}>
            Save assignment
          </Button>
        </div>
      }
    >
      <form
        id={formId}
        className="space-y-5"
        onSubmit={(event) => {
          event.preventDefault()
          save()
        }}
      >
        <AccessError error={e.error} />
        {e.stale && <Button onClick={reload}>Reload assignment and discard draft</Button>}
        <p className="supporting-copy">
          Assignments apply on subsequent requests and do not delegate Access Management
          administration.
        </p>
        {base && (
          <p className="text-xs text-ink-muted">
            Version {base.version} · {base.id}
          </p>
        )}
        <Field label="Assigned role">
          <Select
            required
            value={e.draft.roleId}
            onChange={(event) => e.setDraft({ ...e.draft, roleId: event.target.value })}
          >
            <option value="">Choose a role</option>
            {data.roles.map((r) => (
              <option key={r.id} value={r.id}>
                {r.name} ({r.slug})
              </option>
            ))}
          </Select>
        </Field>
        {role && (
          <details>
            <summary className="cursor-pointer text-xs font-medium">
              Assigned permissions · {role.permissions.length}
            </summary>
            <p className="supporting-copy">
              Request and response access are separate. Metadata rights do not reveal messages.
            </p>
            <ul className="mt-2 space-y-1 text-xs">
              {role.permissions.map((p) => (
                <li key={p}>
                  {p}
                  {!applicable.includes(p) && ` · Inactive for ${e.draft.scope.kind}`}
                </li>
              ))}
            </ul>
          </details>
        )}
        <ModeSelect
          label="Identity source"
          value={mapped ? 'mapping' : 'principal'}
          options={['principal', 'mapping']}
          onChange={(value) => {
            const { principal: _, mappingId: __, ...rest } = e.draft
            e.setDraft(
              value === 'mapping'
                ? { ...rest, mappingId: '' }
                : {
                    ...rest,
                    principal: { kind: 'oidc', id: '', issuer: data.status.trustedIssuer },
                  },
            )
          }}
        />
        {mapped ? (
          <Field label="Claim mapping">
            <Select
              required
              value={e.draft.mappingId ?? ''}
              onChange={(event) => e.setDraft({ ...e.draft, mappingId: event.target.value })}
            >
              <option value="">Choose a mapping</option>
              {data.mappings.map((m) => (
                <option key={m.id} value={m.id}>
                  {m.issuer} · {m.id} {m.enabled ? '' : '(disabled)'}
                </option>
              ))}
            </Select>
          </Field>
        ) : (
          <PrincipalFields
            value={e.draft.principal!}
            status={data.status}
            onDenied={onDenied}
            onChange={(principal) => e.setDraft({ ...e.draft, principal })}
          />
        )}
        <ScopeFields
          value={e.draft.scope}
          onChange={(scope) => e.setDraft({ ...e.draft, scope })}
          onDenied={onDenied}
        />
        <fieldset className="space-y-2">
          <legend className="field-label">Payload capture allowlist</legend>
          <p className="supporting-copy">
            Select the event capture scopes this assignment may open. This list alone does not grant
            payload access.
          </p>
          <div className="grid gap-2 sm:grid-cols-2">
            {captureScopes.map((scope) => (
              <Checkbox
                variant="row"
                key={scope}
                label={scope}
                checked={e.draft.captureScopes.includes(scope)}
                onChange={(event) =>
                  e.setDraft({
                    ...e.draft,
                    captureScopes: event.target.checked
                      ? [...e.draft.captureScopes, scope]
                      : e.draft.captureScopes.filter((v) => v !== scope),
                  })
                }
              />
            ))}
          </div>
        </fieldset>
        <Checkbox
          label="Assignment enabled"
          checked={e.draft.enabled}
          onChange={(event) => e.setDraft({ ...e.draft, enabled: event.target.checked })}
        />
      </form>
    </Modal>
  )
}
