import { useId, useState } from 'react'
import { api } from '../api'
import Button from '../Button'
import { Checkbox } from '../SelectionControls'
import { input } from '../styles'
import { Modal } from '../ui'
import { slugError } from '../form-validation'
import type { AccessRole, RoleInput } from './client'
import { AccessError, Field, TextInput } from './common'
import { useEditor, type AccessData } from './state'
export default function RoleEditor({
  role,
  copy,
  data,
  onClose,
  onSaved,
  onDenied,
}: {
  role?: AccessRole
  copy?: AccessRole
  data: AccessData
  onClose: () => void
  onSaved: () => void
  onDenied: () => void
}) {
  const [base, setBase] = useState(role),
    [query, setQuery] = useState(''),
    formId = useId()
  const e = useEditor<RoleInput>(
    {
      slug: role?.slug ?? (copy ? `${copy.slug.slice(0, 58)}-copy` : ''),
      name: role?.name ?? (copy ? `${copy.name.slice(0, 195)} copy` : ''),
      description: role?.description ?? copy?.description ?? '',
      permissions: [...(role?.permissions ?? copy?.permissions ?? [])],
    },
    onDenied,
  )
  const builtin = base?.builtin === true
  const filtered = data.permissions.filter((p) => p.id.toLowerCase().includes(query.toLowerCase()))
  const groups = [...new Set(filtered.map((p) => p.id.split('.')[0]))]
  const reload = () =>
    e.unsaved.guard(
      () =>
        void e.execute(async (signal) => {
          if (!base) return
          const latest = await api.accessRole(base.slug, signal)
          if (signal.aborted) return
          setBase(latest)
          const fresh = {
            slug: latest.slug,
            name: latest.name,
            description: latest.description,
            permissions: latest.permissions,
          }
          e.setDraft(fresh)
          e.unsaved.clean(fresh)
          e.setStale(false)
        }),
    )
  const save = () => {
    if (builtin || e.stale) return
    const invalid =
      slugError(e.draft.slug ?? '') || (!e.draft.name.trim() ? 'Name is required.' : undefined)
    if (invalid) {
      e.setError(invalid)
      return
    }
    void e.execute(async (signal) => {
      const body: RoleInput = {
        slug: e.draft.slug,
        name: e.draft.name,
        description: e.draft.description,
        permissions: [...(e.draft.permissions ?? [])],
      }
      if (base) await api.updateAccessRole(base, body, signal)
      else await api.createAccessRole(body, signal)
      if (!signal.aborted) {
        e.unsaved.clean()
        onSaved()
      }
    })
  }
  return (
    <Modal
      title={builtin ? 'Builtin role' : base ? 'Edit role' : copy ? 'Copy role' : 'Create role'}
      size="wide"
      busy={e.busy}
      onClose={() => e.unsaved.guard(onClose)}
      footer={
        <div className="flex flex-wrap justify-end gap-2">
          <Button onClick={() => e.unsaved.guard(onClose)}>Cancel</Button>
          {!builtin && (
            <Button
              form={formId}
              type="submit"
              variant="primary"
              disabled={e.stale}
              loading={e.busy}
            >
              Save role
            </Button>
          )}
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
        <p className="supporting-copy">
          Saved permissions apply to subsequent requests. Audit metadata, request and response
          access remain separate.
        </p>
        <AccessError error={e.error} />
        {e.stale && <Button onClick={reload}>Reload role and discard draft</Button>}
        {builtin && (
          <p className="supporting-copy">
            Builtin roles cannot be changed. Close this view and choose Copy to create an
            independent custom role.
          </p>
        )}
        {copy && (
          <p className="supporting-copy">
            Permissions copied from {copy.name}. This role keeps an explicit list and will not
            inherit later additions.
          </p>
        )}
        {base && <p className="text-xs text-ink-muted">Version {base.version}</p>}
        <fieldset disabled={builtin} className="space-y-5">
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Role identifier">
              <TextInput
                required
                readOnly={!!base}
                value={e.draft.slug ?? ''}
                onChange={(slug) => e.setDraft({ ...e.draft, slug })}
              />
            </Field>
            <Field label="Role name">
              <TextInput
                required
                maxLength={200}
                value={e.draft.name}
                onChange={(name) => e.setDraft({ ...e.draft, name })}
              />
            </Field>
          </div>
          <Field label="Description">
            <textarea
              className={input}
              maxLength={4000}
              rows={2}
              value={e.draft.description}
              onChange={(event) => e.setDraft({ ...e.draft, description: event.target.value })}
            />
          </Field>
        </fieldset>
        <Field label="Find permissions">
          <TextInput value={query} onChange={setQuery} />
        </Field>
        <p className="text-xs text-ink-muted">
          {e.draft.permissions?.length ?? 0} explicit permissions selected. Empty roles grant no
          access.
        </p>
        {groups.map((group) => (
          <fieldset
            disabled={builtin}
            key={group}
            className="space-y-2 rounded-xl border border-line p-4"
          >
            <legend className="px-2 text-xs font-semibold">{group.replaceAll('_', ' ')}</legend>
            {filtered
              .filter((p) => p.id.startsWith(`${group}.`))
              .map((p) => (
                <Checkbox
                  variant="row"
                  key={p.id}
                  label={p.id}
                  description={`Scopes: ${p.scopes.join(', ')}${p.captureRestricted ? ' · Requires an explicit payload capture allowlist' : ''}`}
                  checked={e.draft.permissions?.includes(p.id) ?? false}
                  onChange={(event) =>
                    e.setDraft({
                      ...e.draft,
                      permissions: event.target.checked
                        ? [...(e.draft.permissions ?? []), p.id]
                        : (e.draft.permissions ?? []).filter((id) => id !== p.id),
                    })
                  }
                />
              ))}
          </fieldset>
        ))}
      </form>
    </Modal>
  )
}
