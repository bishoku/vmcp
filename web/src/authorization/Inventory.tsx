import { useMemo, useState } from 'react'
import { ShieldCheck, UserCheck, Fingerprint, Copy, Trash2, Pencil, Eye } from 'lucide-react'
import Button from '../Button'
import { Pill, CopyButton } from '../ui'
import {
  assignmentWarnings,
  identityName,
  mappingName,
  mappingConditions,
  scopeName,
  useAccessDirectory,
} from './presentation'
import { TagBadge } from '../Badge'
import { DetailRow } from '../connections/ServerPanels'
import {
  ServerListFilters,
  ServerListResults,
  ServerListSelect,
  serverPageSize,
} from '../connections/ServerList'
import { useTableSort } from '../table-sorting'
import { filterItem } from '../search-filters'
import type { AccessRole, RoleAssignment, IdentityMapping, AssignmentInput } from './client'
import type { AccessData } from './state'
export type AccessEditor =
  | { kind: 'role'; value?: AccessRole; copy?: AccessRole }
  | { kind: 'assignment'; value?: RoleAssignment; initial?: Partial<AssignmentInput> }
  | { kind: 'mapping'; value?: IdentityMapping }
type Row = {
  id: string
  name: string
  identifier: string
  description: string
  status: string
  attention?: boolean
  type: string
  version: number
  scope: string
  details: [string, string | number][]
  editor: AccessEditor
  copy?: AccessRole
  protected: boolean
  references: number
  search: string
}
export default function Inventory({
  section,
  data,
  loading,
  error,
  onEdit,
  onDelete,
  onDenied,
  onSimulate,
}: {
  onDenied: () => void
  onSimulate: (assignment: RoleAssignment) => void
  section: 'roles' | 'assignments' | 'mappings'
  data: AccessData
  loading: boolean
  error: string
  onEdit: (editor: AccessEditor) => void
  onDelete: (
    kind: 'role' | 'assignment' | 'mapping',
    item: AccessRole | RoleAssignment | IdentityMapping,
  ) => void
}) {
  const { directory, failed: namesFailed } = useAccessDirectory(
    [
      ...data.assignments.map((a) => a.scope.kind),
      ...(data.assignments.some((a) => a.principal?.kind === 'control_token')
        ? ['control_token']
        : []),
    ],
    onDenied,
  )
  const [query, setQuery] = useState(''),
    [status, setStatus] = useState(''),
    [type, setType] = useState(''),
    [scope, setScope] = useState(''),
    [page, setPage] = useState(0)
  const rows = useMemo<Row[]>(() => {
    if (section === 'roles')
      return data.roles.map((r) => ({
        id: r.id,
        name: r.name,
        identifier: r.slug,
        description: r.description || 'An explicit permission list for scoped administration.',
        status: '',
        type: r.builtin ? 'Builtin' : 'Custom',
        version: r.version,
        scope: '',
        details: [
          ['Permissions', r.permissions.length],
          ['Assignments', data.assignments.filter((a) => a.roleId === r.id).length],
        ],
        editor: { kind: 'role', value: r },
        copy: r,
        protected: r.builtin,
        references: data.assignments.filter((a) => a.roleId === r.id).length,
        search: r.permissions.join(' '),
      }))
    if (section === 'assignments')
      return data.assignments.map((a) => ({
        id: a.id,
        name: identityName(a, data, directory),
        identifier: a.id,
        description: `${data.roles.find((r) => r.id === a.roleId)?.name ?? a.roleId} → ${scopeName(a.scope, directory)}`,
        status: a.enabled ? 'enabled' : 'disabled',
        attention: a.enabled && assignmentWarnings(a, data).length > 0,
        type: a.mappingId ? 'Mapping' : 'Principal',
        version: a.version,
        scope: a.scope.kind,
        details: [
          ['Role', data.roles.find((r) => r.id === a.roleId)?.name ?? a.roleId],
          ['Where', scopeName(a.scope, directory)],
          [
            'Configuration',
            !a.enabled
              ? 'Disabled — grants no access'
              : assignmentWarnings(a, data).join(' ') ||
                'Enabled — identity and resource checks still apply',
          ],
          ['Ownership period', a.scope.epoch ?? 'Automatic / live'],
          ['Recorded owner', a.scope.ownerTeam ?? 'Not pinned'],
          ['Payload scopes', a.captureScopes.join(', ') || 'None'],
        ],
        editor: { kind: 'assignment', value: a },
        protected: false,
        references: 0,
        search: `${a.mappingId ?? ''} ${a.principal.id} ${a.principal.issuer ?? ''} ${a.scope.id ?? ''}`,
      }))
    return data.mappings.map((m) => ({
      id: m.id,
      name: mappingName(m),
      identifier: m.id,
      description: mappingConditions(m),
      status: m.enabled ? 'enabled' : 'disabled',
      type: m.issuer === data.status.trustedIssuer ? 'Configured issuer' : 'Draft issuer',
      version: m.version,
      scope: '',
      details: [
        ['Provider', m.issuer],
        ['Purpose', m.description || 'No description'],
        ['Conditions', m.conditions.length],
        ['Assignments', data.assignments.filter((a) => a.mappingId === m.id).length],
        ['Principal type', 'Verified OIDC'],
      ],
      editor: { kind: 'mapping', value: m },
      protected: false,
      references: data.assignments.filter((a) => a.mappingId === m.id).length,
      search: m.conditions.map((c) => `${c.path} ${c.value}`).join(' '),
    }))
  }, [section, data, directory])
  const shown = rows.filter(
    (r) =>
      `${r.name} ${r.identifier} ${r.description} ${r.search}`
        .toLowerCase()
        .includes(query.trim().toLowerCase()) &&
      (!status || r.status === status) &&
      (!type || r.type === type) &&
      (!scope || r.scope === scope),
  )
  const sorting = useTableSort(shown, {
    Name: (r) => r.name,
    Identifier: (r) => r.identifier,
    Status: (r) => r.status,
    Version: (r) => r.version,
  })
  const currentPage = Math.min(page, Math.max(0, Math.ceil(shown.length / serverPageSize) - 1))
  const reset = () => {
    setQuery('')
    setStatus('')
    setType('')
    setScope('')
    setPage(0)
  }
  const noun =
      section === 'roles' ? 'roles' : section === 'assignments' ? 'assignments' : 'mappings',
    label =
      section === 'roles'
        ? 'roles'
        : section === 'assignments'
          ? 'role assignments'
          : 'claim mappings',
    Icon = section === 'roles' ? ShieldCheck : section === 'assignments' ? UserCheck : Fingerprint
  return (
    <>
      {namesFailed && (
        <p role="status" className="supporting-copy">
          Some resource names could not be loaded. Identifiers are shown instead.
        </p>
      )}
      <ServerListFilters
        title={`Find ${label}`}
        description="Search saved access configuration and narrow this inventory"
        searchLabel="Search access configuration"
        query={query}
        onQuery={(value) => {
          setQuery(value)
          setPage(0)
        }}
        activeFilters={[
          filterItem('Status', status, () => {
            setStatus('')
            setPage(0)
          }),
          filterItem('Type', type, () => {
            setType('')
            setPage(0)
          }),
          filterItem('Scope', scope, () => {
            setScope('')
            setPage(0)
          }),
        ]}
        onReset={reset}
      >
        {section !== 'roles' && (
          <ServerListSelect
            label="Access status"
            value={status}
            onChange={(value) => {
              setStatus(value)
              setPage(0)
            }}
          >
            <option value="">All statuses</option>
            <option value="enabled">Enabled</option>
            <option value="disabled">Disabled</option>
          </ServerListSelect>
        )}
        <ServerListSelect
          label="Access type"
          value={type}
          onChange={(value) => {
            setType(value)
            setPage(0)
          }}
        >
          <option value="">All types</option>
          {[...new Set(rows.map((r) => r.type))].map((v) => (
            <option key={v}>{v}</option>
          ))}
        </ServerListSelect>
        {section === 'assignments' && (
          <ServerListSelect
            label="Access scope"
            value={scope}
            onChange={(value) => {
              setScope(value)
              setPage(0)
            }}
          >
            <option value="">All scopes</option>
            {[...new Set(rows.map((r) => r.scope))].map((v) => (
              <option key={v}>{v}</option>
            ))}
          </ServerListSelect>
        )}
      </ServerListFilters>
      <ServerListResults
        title={`Configured ${label}`}
        noun={noun}
        total={rows.length}
        matched={shown.length}
        page={currentPage}
        onPage={setPage}
        sorting={sorting}
        loading={loading}
        readError={error}
        empty={
          rows.length
            ? 'No access configuration matches your filters.'
            : 'No saved configuration in this inventory yet.'
        }
        onClear={reset}
      >
        {sorting.rows
          .slice(currentPage * serverPageSize, (currentPage + 1) * serverPageSize)
          .map((row) => (
            <article
              key={row.id}
              aria-label={row.name}
              className="min-w-0 flex flex-col overflow-hidden surface-card surface-interactive"
            >
              <div className="flex items-start gap-3 panel-body">
                <div className="grid size-10 shrink-0 place-items-center rounded-xl bg-accent-50 text-accent-700">
                  <Icon aria-hidden="true" className="icon-feature" />
                </div>
                <div className="min-w-0 flex-1">
                  <h2 className="panel-title break-words">
                    <button
                      className="text-left text-accent-700 hover:underline"
                      onClick={() => onEdit(row.editor)}
                    >
                      {row.name}
                    </button>
                  </h2>
                  <details className="mt-2 text-xs text-ink-muted">
                    <summary className="cursor-pointer">Identifier</summary>
                    <p className="break-all font-mono">{row.identifier}</p>
                    <CopyButton value={row.id} label="Copy ID" />
                  </details>
                </div>
                {row.status ? (
                  <Pill
                    status={row.attention ? 'warning' : row.status}
                    label={row.attention ? 'Enabled · needs attention' : undefined}
                  />
                ) : (
                  <TagBadge label={row.type} />
                )}
              </div>
              <div className="flex-1 panel-inset pb-5">
                <p className="min-h-10 break-words text-xs leading-relaxed text-ink-muted">
                  {row.description}
                </p>
                <div className="mt-4 surface-inset p-3">
                  <p className="text-xs font-medium">{row.type}</p>
                  <p className="mt-1 text-xs leading-relaxed text-ink-muted">
                    {row.protected
                      ? 'Builtin permission set. Copy it to create a custom role.'
                      : row.references
                        ? 'Referenced assignments must be removed before deletion.'
                        : 'Explicit permissions and resource scope govern this configuration.'}
                  </p>
                </div>
                <dl className="mt-2 divide-y divide-line">
                  {row.details.map(([key, value]) => (
                    <DetailRow key={key} label={key}>
                      <span className="break-all">{value}</span>
                    </DetailRow>
                  ))}
                </dl>
                {row.editor.kind !== 'assignment' && (
                  <details className="mt-3 text-xs">
                    <summary className="cursor-pointer font-medium">
                      Related assignments ({row.references})
                    </summary>
                    <div className="mt-3 space-y-3">
                      {data.assignments
                        .filter((a) =>
                          row.editor.kind === 'role' ? a.roleId === row.id : a.mappingId === row.id,
                        )
                        .map((a) => (
                          <div key={a.id} className="surface-inset p-3">
                            <p className="supporting-copy break-words">
                              {identityName(a, data, directory)} →{' '}
                              {data.roles.find((r) => r.id === a.roleId)?.name ?? a.roleId} →{' '}
                              {scopeName(a.scope, directory)}
                            </p>
                            <p className="supporting-copy">
                              {a.enabled
                                ? assignmentWarnings(a, data).join(' ') ||
                                  'Enabled; policy checks apply'
                                : 'Disabled'}
                            </p>
                            <Button onClick={() => onEdit({ kind: 'assignment', value: a })}>
                              Edit assignment
                            </Button>
                            <Button onClick={() => onSimulate(a)}>Test access</Button>
                          </div>
                        ))}
                      {!row.references && (
                        <p className="supporting-copy">
                          No access is granted through this {row.editor.kind} yet.
                        </p>
                      )}
                    </div>
                  </details>
                )}
              </div>
              <footer className="flex flex-wrap items-center justify-between gap-3 border-t border-line panel-footer">
                <span className="text-xs text-ink-muted">Saved version {row.version}</span>
                <div className="flex flex-wrap items-center gap-2">
                  {row.editor.kind === 'assignment' ? (
                    <Button onClick={() => onSimulate(row.editor.value as RoleAssignment)}>
                      Test access
                    </Button>
                  ) : (
                    <Button
                      onClick={() =>
                        onEdit({
                          kind: 'assignment',
                          initial:
                            row.editor.kind === 'role' ? { roleId: row.id } : { mappingId: row.id },
                        })
                      }
                    >
                      Grant access
                    </Button>
                  )}
                  <Button onClick={() => onEdit(row.editor)}>
                    {row.protected ? (
                      <Eye aria-hidden="true" className="icon-inline" />
                    ) : (
                      <Pencil aria-hidden="true" className="icon-inline" />
                    )}
                    {row.protected ? 'View' : 'Edit'}
                  </Button>
                  {row.copy && (
                    <Button onClick={() => onEdit({ kind: 'role', copy: row.copy })}>
                      <Copy aria-hidden="true" className="icon-inline" />
                      Copy
                    </Button>
                  )}
                  {!row.protected && (
                    <Button
                      iconOnly
                      variant="danger-outline"
                      aria-label="Delete"
                      disabled={row.references > 0}
                      onClick={() => onDelete(row.editor.kind, row.editor.value!)}
                    >
                      <Trash2 aria-hidden="true" className="icon-inline" />
                    </Button>
                  )}
                </div>
              </footer>
            </article>
          ))}
      </ServerListResults>
    </>
  )
}
