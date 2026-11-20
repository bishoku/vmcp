import { useId, useState } from 'react'
import { api } from '../api'
import Button from '../Button'
import Select from '../Select'
import { Checkbox } from '../SelectionControls'
import { Modal } from '../ui'
import { input } from '../styles'
import { mappingName } from './presentation'
import type { IdentityMapping, MappingInput, MappingCondition } from './client'
import { AccessError, AccessSection, Field, TextInput } from './common'
import { useEditor, type AccessData } from './state'
export default function MappingEditor({
  mapping,
  data,
  onClose,
  onSaved,
  onDenied,
}: {
  mapping?: IdentityMapping
  data: AccessData
  onClose: () => void
  onSaved: (mapping: IdentityMapping) => void
  onDenied: () => void
}) {
  const [base, setBase] = useState(mapping),
    formId = useId()
  const e = useEditor<MappingInput>(
    {
      name: mapping?.name ?? '',
      description: mapping?.description ?? '',
      issuer: mapping?.issuer ?? data.status.trustedIssuer,
      principalKind: 'oidc',
      conditions: mapping?.conditions.map((c) => ({ ...c })) ?? [
        { path: 'groups', operator: 'array_contains', value: '' },
      ],
      enabled: mapping?.enabled ?? false,
    },
    onDenied,
  )
  const untrusted = !data.status.trustedIssuer || e.draft.issuer !== data.status.trustedIssuer
  const condition = (index: number, patch: Partial<MappingCondition>) =>
    e.setDraft({
      ...e.draft,
      conditions: e.draft.conditions.map((c, i) => (i === index ? { ...c, ...patch } : c)),
    })
  const save = () => {
    if (e.stale) return
    if (
      !e.draft.name?.trim() ||
      !e.draft.issuer.trim() ||
      !e.draft.conditions.length ||
      e.draft.conditions.some(
        (c) =>
          !c.path.trim() ||
          !c.value.trim() ||
          (c.operator === 'scope_contains' && /\s/.test(c.value)),
      )
    ) {
      e.setError(
        'Enter a mapping name, an issuer and complete every condition. Scope conditions require one exact scope token.',
      )
      return
    }
    if (e.draft.enabled && untrusted) {
      e.setError(
        'Enabled mappings must exactly match the configured OIDC issuer. Save a disabled draft or use the configured issuer.',
      )
      return
    }
    void e.execute(async (signal) => {
      const body: MappingInput = {
        name: e.draft.name?.trim(),
        description: e.draft.description?.trim(),
        issuer: e.draft.issuer,
        principalKind: 'oidc',
        enabled: e.draft.enabled,
        conditions: e.draft.conditions.map((c) => ({
          path: c.path,
          operator: c.operator,
          value: c.value,
        })),
      }
      const saved = base
        ? await api.updateAccessMapping(base, body, signal)
        : await api.createAccessMapping(body, signal)
      if (!signal.aborted) {
        e.unsaved.clean()
        onSaved(saved)
      }
    })
  }
  const reload = () =>
    e.unsaved.guard(
      () =>
        void e.execute(async (signal) => {
          if (!base) return
          const latest = await api.accessMapping(base.id, signal)
          if (signal.aborted) return
          setBase(latest)
          const fresh = {
            name: latest.name ?? '',
            description: latest.description ?? '',
            issuer: latest.issuer,
            principalKind: latest.principalKind,
            conditions: latest.conditions,
            enabled: latest.enabled,
          }
          e.setDraft(fresh)
          e.unsaved.clean(fresh)
          e.setStale(false)
        }),
    )
  return (
    <Modal
      title={base ? 'Edit claim mapping' : 'Create claim mapping'}
      description="Match verified identity claims before assigning a role. Every condition must match."
      size="wide"
      busy={e.busy}
      onClose={() => e.unsaved.guard(onClose)}
      footer={
        <div className="flex flex-wrap justify-end gap-2">
          <Button onClick={() => e.unsaved.guard(onClose)}>Cancel</Button>
          <Button form={formId} type="submit" variant="primary" loading={e.busy} disabled={e.stale}>
            Save mapping
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
        {e.stale && <Button onClick={reload}>Reload mapping and discard draft</Button>}
        <p className="supporting-copy">
          All conditions must match verified OIDC ID-token claims. Scope matching does not accept
          OAuth bearer tokens or add a trusted provider. New claim paths require a fresh sign-in.
        </p>
        {base && (
          <p className="text-xs text-ink-muted">
            Version {base.version} · {base.id}
          </p>
        )}
        <AccessSection
          title="Mapping details"
          description="Give this matching rule a recognizable name. Names do not change who matches."
        >
          <Field label="Mapping name">
            <TextInput
              required
              maxLength={200}
              value={e.draft.name ?? ''}
              onChange={(name) => e.setDraft({ ...e.draft, name })}
              placeholder="Support team"
            />
          </Field>
          <Field label="Description">
            <textarea
              className={input}
              maxLength={4000}
              rows={2}
              value={e.draft.description ?? ''}
              onChange={(event) => e.setDraft({ ...e.draft, description: event.target.value })}
            />
          </Field>
          {base && (
            <p className="supporting-copy">
              Changes to {mappingName(base)} affect{' '}
              {data.assignments.filter((a) => a.mappingId === base.id).length} linked assignments.
              Changing conditions changes who qualifies; disabling stops grants through this
              mapping. Renaming only changes its label.
            </p>
          )}
        </AccessSection>
        <AccessSection
          title="Identity provider"
          description="Use the configured OIDC issuer. A mapping matches identities; access is granted separately through an assignment."
        >
          <Field
            label="OIDC issuer"
            hint={`Configured issuer: ${data.status.trustedIssuer || 'No OIDC provider configured'}`}
          >
            <TextInput
              required
              maxLength={2048}
              value={e.draft.issuer}
              onChange={(issuer) => e.setDraft({ ...e.draft, issuer })}
            />
          </Field>
          <Checkbox
            label="Mapping enabled"
            checked={e.draft.enabled}
            onChange={(event) => e.setDraft({ ...e.draft, enabled: event.target.checked })}
          />
          {untrusted && (
            <p role="status" className="supporting-copy">
              This issuer can only be saved as a disabled draft. Enabled assignment references may
              also prevent an issuer change.
            </p>
          )}
        </AccessSection>
        <AccessSection
          title="Claim conditions"
          description="Match exact values from verified OIDC ID-token claims. Add conditions to narrow the identities that qualify."
        >
          <fieldset className="space-y-4">
            <legend className="field-label">AND conditions</legend>
            {e.draft.conditions.map((c, index) => (
              <div
                key={index}
                className="grid min-w-0 items-end gap-4 rounded-xl border border-line bg-canvas/40 p-4 sm:grid-cols-2"
              >
                <Field label={`Claim path ${index + 1}`}>
                  <TextInput
                    required
                    maxLength={512}
                    value={c.path}
                    onChange={(path) => condition(index, { path })}
                  />
                </Field>
                <Field label={`Operator ${index + 1}`}>
                  <Select
                    value={c.operator}
                    onChange={(event) =>
                      condition(index, {
                        operator: event.target.value as MappingCondition['operator'],
                      })
                    }
                  >
                    <option value="equals">String equals</option>
                    <option value="array_contains">Array contains string</option>
                    <option value="scope_contains">Scope contains exact token</option>
                  </Select>
                </Field>
                <Field label={`Claim value ${index + 1}`}>
                  <TextInput
                    required
                    maxLength={2048}
                    value={c.value}
                    onChange={(value) => condition(index, { value })}
                  />
                </Field>
                <Button
                  disabled={e.draft.conditions.length === 1}
                  onClick={() =>
                    e.setDraft({
                      ...e.draft,
                      conditions: e.draft.conditions.filter((_, i) => i !== index),
                    })
                  }
                >
                  Remove condition {index + 1}
                </Button>
              </div>
            ))}
            <Button
              disabled={e.draft.conditions.length >= 32}
              onClick={() =>
                e.setDraft({
                  ...e.draft,
                  conditions: [...e.draft.conditions, { path: '', operator: 'equals', value: '' }],
                })
              }
            >
              Add condition
            </Button>
          </fieldset>
        </AccessSection>
      </form>
    </Modal>
  )
}
