import { useEffect, useId, useRef, useState } from 'react'
import { api } from '../api'
import Button from '../Button'
import Select from '../Select'
import { Checkbox } from '../SelectionControls'
import { Modal } from '../ui'
import type { AssignmentInput, RoleAssignment } from './client'
import { AccessError, AccessSection, Field, ModeSelect } from './common'
import { captureScopes, useEditor, uuid, type AccessData } from './state'
import { PrincipalFields, ScopeFields } from './Selectors'
import { describePermission } from './permission-help'
import MappingEditor from './MappingEditor'
import type { IdentityMapping } from './client'
import {
  assignmentWarnings,
  identityName,
  mappingName,
  mappingConditions,
  scopeName,
  useAccessDirectory,
} from './presentation'
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
  initial,
  data: loadedData,
  onClose,
  onSaved,
  onDenied,
}: {
  assignment?: RoleAssignment
  initial?: Partial<AssignmentInput>
  data: AccessData
  onClose: () => void
  onSaved: () => void
  onDenied: () => void
}) {
  const [createdMappings, setCreatedMappings] = useState<IdentityMapping[]>([])
  const [creatingMapping, setCreatingMapping] = useState(false)
  const [step, setStep] = useState(0)
  const stepNavigation = useRef<HTMLElement>(null)
  useEffect(() => {
    if (assignment || creatingMapping) return
    stepNavigation.current?.focus({ preventScroll: true })
    stepNavigation.current?.scrollIntoView?.({ block: 'start' })
  }, [step, assignment, creatingMapping])
  const data = { ...loadedData, mappings: [...loadedData.mappings, ...createdMappings] }
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
          ...initial,
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
  const { directory, failed: namesFailed } = useAccessDirectory(
    [e.draft.scope.kind, ...(e.draft.principal?.kind === 'control_token' ? ['control_token'] : [])],
    onDenied,
  )
  const warnings = assignmentWarnings(e.draft, data)
  const selectedMapping = data.mappings.find((m) => m.id === e.draft.mappingId)
  const steps = ['Identity', 'Role', 'Scope', 'Review']
  const next = () => {
    if (step === 0 && (mapped ? !selectedMapping : !e.draft.principal?.id.trim())) {
      e.setError('Choose a claim mapping or enter an identity.')
      return
    }
    if (step === 1 && !role) {
      e.setError('Choose a role.')
      return
    }
    if (
      step === 2 &&
      ((!['tenant', 'unmanaged'].includes(e.draft.scope.kind) && !uuid(e.draft.scope.id ?? '')) ||
        (capture && !e.draft.captureScopes.length))
    ) {
      e.setError('Choose a resource and any required payload capture scopes.')
      return
    }
    e.setError('')
    setStep((v) => Math.min(v + 1, 3))
  }
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
  if (creatingMapping)
    return (
      <MappingEditor
        data={data}
        onDenied={onDenied}
        onClose={() => setCreatingMapping(false)}
        onSaved={(mapping) => {
          setCreatedMappings((items) => [...items, mapping])
          const { principal: _, ...rest } = e.draft
          e.setDraft({ ...rest, mappingId: mapping.id })
          setCreatingMapping(false)
        }}
      />
    )
  return (
    <Modal
      title={base ? 'Edit role assignment' : 'Grant access'}
      description="Connect a role to an identity and limit where its permissions apply."
      size="wide"
      busy={e.busy}
      onClose={() => e.unsaved.guard(onClose)}
      footer={
        <div className="flex flex-wrap justify-end gap-2">
          <Button onClick={() => e.unsaved.guard(onClose)}>Cancel</Button>
          {!base && step > 0 && (
            <Button
              disabled={e.busy}
              onClick={() => {
                e.setError('')
                setStep((v) => v - 1)
              }}
            >
              Back
            </Button>
          )}
          <Button form={formId} type="submit" variant="primary" disabled={e.stale} loading={e.busy}>
            {!base && step < 3 ? 'Continue' : 'Save assignment'}
          </Button>
        </div>
      }
    >
      <form
        id={formId}
        className="space-y-5"
        onSubmit={(event) => {
          event.preventDefault()
          if (!base && step < 3) next()
          else save()
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
        {!base && (
          <nav
            ref={stepNavigation}
            tabIndex={-1}
            aria-label="Grant access steps"
            className="flex flex-wrap gap-2"
          >
            {steps.map((label, index) => (
              <span
                key={label}
                aria-current={index === step ? 'step' : undefined}
                className={index === step ? 'field-label text-accent-700' : 'supporting-copy'}
              >
                {index + 1}. {label}
              </span>
            ))}
          </nav>
        )}
        {(base || step === 0) && (
          <AccessSection
            title="Who receives access?"
            description="Choose a specific identity, or a rule matching verified OIDC claims. A matching rule does not grant access on its own."
          >
            <ModeSelect
              label="Identity source"
              value={mapped ? 'mapping' : 'principal'}
              options={['principal', 'mapping']}
              labels={{
                principal: 'Specific user or control token',
                mapping: 'Identities matching a claim rule',
              }}
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
              <div className="space-y-3">
                <Field label="Claim mapping">
                  <Select
                    required
                    value={e.draft.mappingId ?? ''}
                    onChange={(event) => e.setDraft({ ...e.draft, mappingId: event.target.value })}
                  >
                    <option value="">Choose a mapping</option>
                    {data.mappings.map((m) => (
                      <option key={m.id} value={m.id}>
                        {mappingName(m)}
                        {m.name?.trim() ? ` · ${mappingConditions(m)}` : ''}{' '}
                        {m.enabled ? '' : '(disabled)'}
                      </option>
                    ))}
                  </Select>
                </Field>
                {selectedMapping && (
                  <p className="supporting-copy">
                    {selectedMapping.description} All conditions must match:{' '}
                    {mappingConditions(selectedMapping)}. Provider: {selectedMapping.issuer}.
                  </p>
                )}
                <Button onClick={() => setCreatingMapping(true)}>Create a claim mapping</Button>
                <p className="supporting-copy">
                  A new mapping is saved separately and remains available if you cancel this
                  assignment.
                </p>
              </div>
            ) : (
              <PrincipalFields
                value={e.draft.principal!}
                status={data.status}
                onDenied={onDenied}
                onChange={(principal) => e.setDraft({ ...e.draft, principal })}
              />
            )}
          </AccessSection>
        )}
        {(base || step === 1) && (
          <AccessSection
            title="What can they do?"
            description="Choose a reusable role. Only its explicit permissions can be granted."
          >
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
                <ul className="mt-3 divide-y divide-line">
                  {role.permissions.map((p) => (
                    <li key={p} className="py-3">
                      <p className="field-label">{describePermission(p).title}</p>
                      <p className="meta-copy break-all">{p}</p>
                      <p className="supporting-copy text-ink-muted">
                        {describePermission(p).description}
                      </p>
                      {!applicable.includes(p) && (
                        <p className="supporting-copy text-amber-800">
                          Inactive for {e.draft.scope.kind.replaceAll('_', ' ')} scope
                        </p>
                      )}
                    </li>
                  ))}
                </ul>
              </details>
            )}
          </AccessSection>
        )}

        {(base || step === 2) && (
          <AccessSection
            title="Scope and payload access"
            description="Limit this assignment to the intended resources. Payload capture scopes are an additional restriction, not a payload permission."
          >
            <ScopeFields
              value={e.draft.scope}
              onChange={(scope) => e.setDraft({ ...e.draft, scope })}
              onDenied={onDenied}
            />
            {capture && (
              <fieldset className="space-y-2">
                <legend className="field-label">Payload capture allowlist</legend>
                <p className="supporting-copy">
                  Select the event capture scopes this assignment may open. This list alone does not
                  grant payload access.
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
            )}
          </AccessSection>
        )}
        {(base || step === 3) && (
          <AccessSection
            title="Review access"
            description="Confirm who receives the role and where it applies. Saving an enabled assignment changes subsequent policy decisions."
          >
            <dl className="space-y-3">
              <div>
                <dt className="field-label">Who</dt>
                <dd className="supporting-copy break-words">
                  {identityName(e.draft, data, directory)}
                </dd>
              </div>
              <div>
                <dt className="field-label">Role</dt>
                <dd className="supporting-copy">{role?.name || 'No role selected'}</dd>
              </div>
              <div>
                <dt className="field-label">Where</dt>
                <dd className="supporting-copy break-words">
                  {scopeName(e.draft.scope, directory)}
                </dd>
              </div>
              <div>
                <dt className="field-label">Applicable permissions</dt>
                <dd className="supporting-copy">
                  {applicable.map((id) => describePermission(id).title).join(', ') || 'None'}
                </dd>
              </div>
            </dl>
            {namesFailed && (
              <p role="status" className="supporting-copy">
                Some resource names could not be loaded. Identifiers are shown instead.
              </p>
            )}
            {selectedMapping && (
              <p className="supporting-copy">
                All conditions must match: {mappingConditions(selectedMapping)}.
              </p>
            )}
            {e.draft.scope.kind === 'tenant' && (
              <p className="supporting-copy text-amber-800">
                Deployment-wide access: this assignment is not limited to a single team or resource.
              </p>
            )}
            {base && (
              <p className="supporting-copy">
                Previously: {identityName(assignmentInput(base), data, directory)} →{' '}
                {data.roles.find((r) => r.id === base.roleId)?.name ?? base.roleId} →{' '}
                {scopeName(base.scope, directory)} ({base.enabled ? 'enabled' : 'disabled'}). This
                changes this assignment only; other grants can still provide access.
              </p>
            )}
            {warnings.map((warning) => (
              <p key={warning} role="status" className="supporting-copy text-amber-800">
                {warning}
              </p>
            ))}
            {capture && (
              <p className="supporting-copy">
                Payload allowlist: {e.draft.captureScopes.join(', ') || 'None'}. Request and
                response permissions are separate.
              </p>
            )}
            <p className="supporting-copy">
              {e.draft.enabled
                ? 'This assignment will be enabled. Matching identity, permission and resource checks still apply.'
                : 'This assignment will be saved disabled and will grant no access.'}{' '}
              After saving, use Test access to evaluate the stored policy with a sample identity.
            </p>
            <Checkbox
              description="Enable only after reviewing the role, identity and scope. Changes apply to subsequent requests."
              label="Assignment enabled"
              checked={e.draft.enabled}
              onChange={(event) => e.setDraft({ ...e.draft, enabled: event.target.checked })}
            />
          </AccessSection>
        )}
      </form>
    </Modal>
  )
}
