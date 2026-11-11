import { useState } from 'react'
import { KeyRound, Pencil, Plus, Trash2 } from 'lucide-react'
import type { InvocationService, InvocationServices } from '../api'
import Button from '../Button'
import { ServerPanel } from '../connections/ServerPanels'
import { useConfirm } from '../confirmation'
import { EmptyState } from '../Feedback'
import { FormErrors, FormField, FormScope } from '../FormField'
import { requiredText, useFormValidation } from '../form-validation'
import { useDraft } from '../interaction'
import { input } from '../styles'
import { Modal } from '../ui'

const serviceName = /^[A-Za-z0-9:._/-]{1,200}$/

function httpsError(value: string, origin = false) {
  try {
    const url = new URL(value)
    if (
      url.protocol === 'https:' &&
      !url.username &&
      !url.password &&
      !url.hash &&
      (!origin || (!url.search && url.pathname === '/'))
    )
      return
  } catch {
    /* Return the field-specific message below. */
  }
  return origin
    ? 'Enter an HTTPS origin without a path or query.'
    : 'Enter an HTTPS URL without embedded credentials or a fragment.'
}

export default function Services({
  services,
  sharedNames = [],
  used,
  writable,
  busy,
  scope,
  onChange,
}: {
  services: InvocationServices
  sharedNames?: string[]
  used: Set<string>
  writable: boolean
  busy: boolean
  scope: 'upstream' | 'shared'
  onChange: (services: InvocationServices) => void
}) {
  const [target, setTarget] = useState<string | null>(null)
  const confirm = useConfirm()
  return (
    <ServerPanel
      title={
        scope === 'shared'
          ? 'Shared verification & approval services'
          : 'Upstream verification & approval services'
      }
      description={
        scope === 'shared'
          ? 'These services can be selected by any upstream.'
          : 'Only policies on this upstream can select these service definitions.'
      }
      icon={KeyRound}
      actions={
        writable && (
          <Button disabled={busy} onClick={() => setTarget('')}>
            <Plus className="icon-inline" aria-hidden="true" />
            Add service
          </Button>
        )
      }
    >
      {Object.keys(services).length > 0 ? (
        <ul className="divide-y divide-line">
          {Object.entries(services)
            .sort(([a], [b]) => a.localeCompare(b))
            .map(([name, service]) => (
              <li
                key={name}
                className="flex flex-wrap items-center justify-between gap-3 panel-body"
              >
                <div className="min-w-0">
                  <p className="break-all text-sm font-semibold">{name}</p>
                  <p className="break-all text-xs text-ink-muted">{service.url}</p>
                  <p className="text-xs text-ink-muted">
                    <span>
                      {service.authType === 'none'
                        ? 'No authentication'
                        : service.authType === 'headers'
                          ? 'Custom authentication headers'
                          : service.authType === 'oauth_client_credentials'
                            ? 'OAuth2 client credentials'
                            : service.token
                              ? 'Token pending save'
                              : service.hasToken
                                ? 'Token stored'
                                : 'Token required'}
                    </span>{' '}
                    · {service.timeoutSeconds || 2}s timeout
                  </p>
                  {used.has(name) && <p className="text-xs text-ink-muted">Used by a policy</p>}
                </div>
                {writable && (
                  <div className="flex flex-wrap items-center gap-2">
                    <Button
                      disabled={busy}
                      aria-label={`Edit service ${name}`}
                      onClick={() => setTarget(name)}
                    >
                      <Pencil className="icon-inline" aria-hidden="true" />
                      Edit
                    </Button>
                    <Button
                      disabled={busy || used.has(name)}
                      aria-label={`Remove service ${name}`}
                      variant="danger-outline"
                      onClick={async () => {
                        if (
                          await confirm(
                            `Remove ${name}? Save the configuration to apply this change.`,
                          )
                        ) {
                          const next = { ...services }
                          delete next[name]
                          onChange(next)
                        }
                      }}
                    >
                      <Trash2 className="icon-inline" aria-hidden="true" />
                      Remove
                    </Button>
                  </div>
                )}
              </li>
            ))}
        </ul>
      ) : (
        <EmptyState
          icon={KeyRound}
          title="No services configured"
          description="Add a service for caller-context verification or human approval."
        />
      )}
      {target !== null && (
        <ServiceEditor
          name={target}
          services={services}
          sharedNames={sharedNames}
          onClose={() => setTarget(null)}
          onStage={(next) => {
            onChange(next)
            setTarget(null)
          }}
        />
      )}
    </ServerPanel>
  )
}

function ServiceEditor({
  name,
  services,
  sharedNames,
  onClose,
  onStage,
}: {
  name: string
  services: InvocationServices
  sharedNames: string[]
  onClose: () => void
  onStage: (services: InvocationServices) => void
}) {
  const current = services[name]
  const [draft, setDraft, , , dirty] = useDraft(
    {
      name,
      url: current?.url ?? '',
      origin: current?.approvalOrigin ?? '',
      token: '',
      authType: current?.authType ?? 'bearer',
      timeout: String(current?.timeoutSeconds || 2),
      headers: Object.entries(current?.headers ?? {}).map(([name, value]) => ({ name, value })),
      tokenUrl: current?.oauth?.tokenUrl ?? '',
      clientId: current?.oauth?.clientId ?? '',
      clientSecret: '',
      scopes: current?.oauth?.scopes?.join(' ') ?? '',
      audience: current?.oauth?.audience ?? '',
      authStyle: current?.oauth?.authStyle ?? 'client_secret_basic',
      ca: current?.caPem ?? '',
    },
    () => name,
  )
  const validation = useFormValidation()
  const confirm = useConfirm()
  const close = async () => {
    if (!dirty || (await confirm('Discard changes to this service?'))) onClose()
  }
  const sameDestination =
    current?.url === draft.url &&
    (current?.caPem ?? '') === draft.ca &&
    (current?.authType ?? 'bearer') === draft.authType
  const tokenRequired = !sameDestination || !(current?.hasToken || current?.token)
  const secretRequired =
    !sameDestination ||
    current?.oauth?.tokenUrl !== draft.tokenUrl ||
    current?.oauth?.clientId !== draft.clientId ||
    (current?.oauth?.authStyle || 'client_secret_basic') !== draft.authStyle ||
    !(current?.oauth?.hasClientSecret || current?.oauth?.clientSecret)
  const headerError = () => {
    if (draft.headers.length < 1 || draft.headers.length > 16)
      return 'Add 1 to 16 authentication headers.'
    const names = new Set<string>()
    for (const row of draft.headers) {
      const key = row.name.toLowerCase()
      if (!/^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/.test(row.name) || names.has(key))
        return 'Use unique valid header names.'
      names.add(key)
      if (
        [
          'host',
          'content-type',
          'content-length',
          'connection',
          'transfer-encoding',
          'trailer',
          'upgrade',
          'te',
          'expect',
          'keep-alive',
        ].includes(key) ||
        key.startsWith('proxy-') ||
        key.startsWith('sec-')
      )
        return 'Transport and proxy headers cannot be used for authentication.'
      const stored = Object.keys(current?.hasHeaders ?? {}).some(
        (name) => name.toLowerCase() === key && current?.hasHeaders?.[name],
      )
      if (!row.value && !(sameDestination && stored))
        return 'Enter a value for each new or changed header.'
      if (/[\r\n]/.test(row.value) || row.value.length > 8192)
        return 'Header values must be a single line of at most 8192 characters.'
    }
  }
  const stage = () => {
    if (
      !validation.check({
        name: !serviceName.test(draft.name)
          ? 'Use 1 to 200 letters, numbers or : . _ / - characters.'
          : !name && (Object.hasOwn(services, draft.name) || sharedNames.includes(draft.name))
            ? 'This name is already in use.'
            : undefined,
        url: httpsError(draft.url),
        origin: httpsError(draft.origin, true),
        token:
          draft.authType === 'bearer' && tokenRequired
            ? requiredText(draft.token, 'Service token')
            : undefined,
        timeout:
          !Number.isInteger(Number(draft.timeout)) ||
          Number(draft.timeout) < 1 ||
          Number(draft.timeout) > 60
            ? 'Enter a timeout from 1 to 60 seconds.'
            : undefined,
        headers: draft.authType === 'headers' ? headerError() : undefined,
        tokenUrl:
          draft.authType === 'oauth_client_credentials' ? httpsError(draft.tokenUrl) : undefined,
        clientId:
          draft.authType === 'oauth_client_credentials'
            ? requiredText(draft.clientId, 'Client ID')
            : undefined,
        clientSecret:
          draft.authType === 'oauth_client_credentials' && secretRequired
            ? requiredText(draft.clientSecret, 'Client secret')
            : undefined,
      })
    )
      return
    onStage({
      ...services,
      [draft.name]: {
        url: draft.url,
        approvalOrigin: draft.origin,
        authType: draft.authType,
        timeoutSeconds: Number(draft.timeout),
        token:
          draft.authType === 'bearer'
            ? draft.token || (sameDestination ? current?.token : undefined)
            : undefined,
        hasToken: draft.authType === 'bearer' && sameDestination ? current?.hasToken : undefined,
        headers:
          draft.authType === 'headers'
            ? Object.fromEntries(draft.headers.map((row) => [row.name, row.value]))
            : undefined,
        hasHeaders:
          draft.authType === 'headers' && sameDestination ? current?.hasHeaders : undefined,
        oauth:
          draft.authType === 'oauth_client_credentials'
            ? {
                tokenUrl: draft.tokenUrl,
                clientId: draft.clientId,
                clientSecret:
                  draft.clientSecret ||
                  (!secretRequired ? current?.oauth?.clientSecret : undefined),
                hasClientSecret: !secretRequired && current?.oauth?.hasClientSecret,
                scopes: draft.scopes.split(/\s+/).filter(Boolean),
                audience: draft.audience || undefined,
                authStyle: draft.authStyle,
              }
            : undefined,
        caPem: draft.ca || undefined,
      },
    })
  }
  return (
    <Modal
      title={name ? 'Edit verification service' : 'Add verification service'}
      size="wide"
      onClose={() => void close()}
      footer={
        <>
          <Button onClick={() => void close()}>Cancel</Button>
          <Button variant="primary" onClick={stage}>
            Stage service
          </Button>
        </>
      }
    >
      <FormScope validation={validation}>
        <FormErrors validation={validation} />
        <div className="layout-stack">
          <div className="grid gap-4 md:grid-cols-2">
            <FormField name="name" label="Service name" required>
              <input
                className={input}
                disabled={!!name}
                value={draft.name}
                onChange={(e) => setDraft({ ...draft, name: e.target.value })}
              />
            </FormField>
            <FormField name="authType" label="Service authentication">
              <select
                className={input}
                value={draft.authType}
                onChange={(e) =>
                  setDraft({
                    ...draft,
                    authType: e.target.value as NonNullable<InvocationService['authType']>,
                  })
                }
              >
                <option value="bearer">Bearer token</option>
                <option value="oauth_client_credentials">OAuth2 client credentials</option>
                <option value="headers">Custom headers</option>
                <option value="none">No authentication</option>
              </select>
            </FormField>
            <FormField
              name="timeout"
              label="Request timeout (seconds)"
              hint="1–60 seconds per check or consume, including OAuth token acquisition."
            >
              <input
                className={input}
                type="number"
                min="1"
                max="60"
                value={draft.timeout}
                onChange={(e) => setDraft({ ...draft, timeout: e.target.value })}
              />
            </FormField>
            {draft.authType === 'bearer' && (
              <FormField
                name="token"
                label="Service token"
                required={tokenRequired}
                hint={
                  tokenRequired
                    ? 'Enter the write-only credential for this endpoint.'
                    : 'Leave blank to retain the existing token.'
                }
              >
                <input
                  className={input}
                  type="password"
                  autoComplete="new-password"
                  value={draft.token}
                  onChange={(e) => setDraft({ ...draft, token: e.target.value })}
                />
              </FormField>
            )}
            <FormField name="url" label="HTTPS decision endpoint" required>
              <input
                className={input}
                value={draft.url}
                onChange={(e) => setDraft({ ...draft, url: e.target.value })}
              />
            </FormField>
            <FormField
              name="origin"
              label="Trusted approval screen origin"
              required
              hint="Review links must use this HTTPS origin."
            >
              <input
                className={input}
                value={draft.origin}
                onChange={(e) => setDraft({ ...draft, origin: e.target.value })}
              />
            </FormField>
          </div>
          {draft.authType === 'oauth_client_credentials' && (
            <div className="grid gap-4 md:grid-cols-2">
              <FormField name="tokenUrl" label="OAuth token endpoint" required>
                <input
                  className={input}
                  value={draft.tokenUrl}
                  onChange={(e) => setDraft({ ...draft, tokenUrl: e.target.value })}
                />
              </FormField>
              <FormField name="clientId" label="OAuth client ID" required>
                <input
                  className={input}
                  value={draft.clientId}
                  onChange={(e) => setDraft({ ...draft, clientId: e.target.value })}
                />
              </FormField>
              <FormField
                name="clientSecret"
                label="OAuth client secret"
                required={secretRequired}
                hint={
                  secretRequired
                    ? 'Enter the write-only client secret.'
                    : 'Leave blank to retain the existing secret.'
                }
              >
                <input
                  className={input}
                  type="password"
                  autoComplete="new-password"
                  value={draft.clientSecret}
                  onChange={(e) => setDraft({ ...draft, clientSecret: e.target.value })}
                />
              </FormField>
              <FormField name="authStyle" label="Token endpoint authentication">
                <select
                  className={input}
                  value={draft.authStyle}
                  onChange={(e) =>
                    setDraft({
                      ...draft,
                      authStyle: e.target.value as 'client_secret_basic' | 'client_secret_post',
                    })
                  }
                >
                  <option value="client_secret_basic">Client secret in Basic header</option>
                  <option value="client_secret_post">Client secret in request body</option>
                </select>
              </FormField>
              <FormField name="scopes" label="OAuth scopes" hint="Space-separated scopes.">
                <input
                  className={input}
                  value={draft.scopes}
                  onChange={(e) => setDraft({ ...draft, scopes: e.target.value })}
                />
              </FormField>
              <FormField name="audience" label="OAuth audience (optional)">
                <input
                  className={input}
                  value={draft.audience}
                  onChange={(e) => setDraft({ ...draft, audience: e.target.value })}
                />
              </FormField>
            </div>
          )}
          {draft.authType === 'headers' && (
            <FormField
              name="headers"
              label="Authentication headers"
              hint="Values are write-only. Leave existing values blank to retain them; remove a row to delete it."
            >
              <div className="layout-stack">
                {draft.headers.map((row, index) => (
                  <div className="flex flex-wrap gap-2" key={index}>
                    <input
                      className={input}
                      aria-label={`Header ${index + 1} name`}
                      placeholder="X-API-Key"
                      value={row.name}
                      onChange={(e) =>
                        setDraft({
                          ...draft,
                          headers: draft.headers.map((item, i) =>
                            i === index ? { ...item, name: e.target.value } : item,
                          ),
                        })
                      }
                    />
                    <input
                      className={input}
                      type="password"
                      autoComplete="new-password"
                      aria-label={`Header ${index + 1} value`}
                      placeholder="Secret value"
                      value={row.value}
                      onChange={(e) =>
                        setDraft({
                          ...draft,
                          headers: draft.headers.map((item, i) =>
                            i === index ? { ...item, value: e.target.value } : item,
                          ),
                        })
                      }
                    />
                    <Button
                      aria-label={`Remove header ${index + 1}`}
                      onClick={() =>
                        setDraft({ ...draft, headers: draft.headers.filter((_, i) => i !== index) })
                      }
                    >
                      Remove
                    </Button>
                  </div>
                ))}
                <Button
                  disabled={draft.headers.length >= 16}
                  onClick={() =>
                    setDraft({ ...draft, headers: [...draft.headers, { name: '', value: '' }] })
                  }
                >
                  Add header
                </Button>
              </div>
            </FormField>
          )}
          {draft.authType === 'none' && (
            <p className="text-sm text-ink-muted">
              Requests to this service will not include authentication credentials.
            </p>
          )}
          <details className="surface-inset p-4">
            <summary className="cursor-pointer text-sm font-medium">
              Private certificate authority
            </summary>
            <FormField
              className="mt-4"
              name="ca"
              label="Custom CA certificate"
              hint="Optional PEM certificate."
            >
              <textarea
                className={`${input} min-h-32 font-mono`}
                value={draft.ca}
                onChange={(e) => setDraft({ ...draft, ca: e.target.value })}
              />
            </FormField>
          </details>
        </div>
      </FormScope>
    </Modal>
  )
}
