import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { ShieldCheck } from 'lucide-react'
import {
  api,
  ApiError,
  type InvocationRule,
  type UpstreamInvocationPolicy,
  type UpstreamInvocationSecurity as Document,
} from '../api'
import Button from '../Button'
import { useConfigurationChecks } from '../configuration-access'
import { useConfirm } from '../confirmation'
import { ServerPanel } from '../connections/ServerPanels'
import { EmptyState, LoadingState } from '../Feedback'
import { useDraft } from '../interaction'
import { SearchField } from '../SearchControls'
import { ErrorBox } from '../ui'
import { effectivePolicy, referencedServices, ruleSummary } from './policy'
import RuleEditor from './RuleEditor'
import PolicySummary from './PolicySummary'
import ConfigurationGroup from '../connections/ConfigurationGroup'
import LocalAuditSettingsPanel from '../LocalAuditSettingsPanel'
import { useUpstreamEdit, useUpstreamSection } from '../pages/upstream-edit-session'
import Services from './Services'

const ToolSecurity = createContext<{
  renderLink: (name: string) => ReactNode
  dirty: boolean
  open: () => void
} | null>(null)

export function InvocationSecurityLink() {
  const security = useContext(ToolSecurity)
  if (!security) return null
  return (
    <button className="action-button action-secondary" onClick={security.open}>
      <ShieldCheck className="icon-inline" aria-hidden="true" />
      Invocation security
    </button>
  )
}

export function ToolInvocationPolicy({ name }: { name: string }) {
  return useContext(ToolSecurity)?.renderLink(name) ?? null
}

export default function UpstreamSecurity({
  slug,
  version,
  children,
}: {
  slug: string
  version: number
  children: ReactNode
}) {
  const path = `/upstreams/${encodeURIComponent(slug)}/invocation-security`
  const { allowed } = useConfigurationChecks([
    { method: 'GET', path },
    { method: 'PUT', path },
  ])
  return (
    <Editor
      slug={slug}
      path={path}
      version={version}
      readable={allowed[0] === true}
      writable={allowed[1] === true}
    >
      {children}
    </Editor>
  )
}

function toolRule(policy: UpstreamInvocationPolicy, name: string): InvocationRule | undefined {
  return policy.tools && Object.hasOwn(policy.tools, name) ? policy.tools[name] : undefined
}

function Editor({
  slug,
  path,
  version,
  readable,
  writable,
  children,
}: {
  slug: string
  path: string
  version: number
  readable: boolean
  writable: boolean
  children: ReactNode
}) {
  const session = useUpstreamEdit()
  const editable = writable && !!session?.editing && !session.saving
  const section = useRef<HTMLElement>(null)
  const [saved, setSaved] = useState<Document | null>(null)
  const [policy, setPolicy, resetPolicy, , dirty] = useDraft<UpstreamInvocationPolicy>(
    {},
    () => path,
  )
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [request, setRequest] = useState(0)
  const [target, setTarget] = useState<string | null | undefined>(undefined)
  const [query, setQuery] = useState('')
  const [selectedTool, setSelectedTool] = useState<string | null>(null)
  const [limit, setLimit] = useState(25)
  const confirm = useConfirm()
  const busy = !!session?.saving

  useEffect(() => {
    if (!readable) return
    const controller = new AbortController()
    void api
      .upstreamInvocationSecurity(path, controller.signal)
      .then((value) => {
        if (controller.signal.aborted) return
        setSaved((previous) => {
          if (
            !previous ||
            previous.revision !== value.revision ||
            previous.upstreamId !== value.upstreamId
          )
            return previous ?? value
          return {
            ...previous,
            tools: value.tools,
            callerContextFields: value.callerContextFields,
            sharedServices: value.sharedServices,
          }
        })
        setLoading(false)
      })
      .catch((reason) => {
        if (!controller.signal.aborted) {
          setError(String(reason))
          setLoading(false)
        }
      })
    return () => controller.abort()
  }, [path, version, request, readable])

  useEffect(() => {
    if (saved) resetPolicy(saved.policy)
    // Metadata refreshes keep the same policy reference and preserve staged changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [saved?.policy])

  const reload = async () => {
    if (dirty && !(await confirm('Discard unsaved invocation changes and reload?'))) return
    setError('')
    setLoading(true)
    setSaved(null)
    setRequest((value) => value + 1)
  }

  const names = saved
    ? [...new Set([...saved.tools, ...Object.keys(policy.tools ?? {})])].sort()
    : []
  const shown = names.filter((name) => name.toLowerCase().includes(query.trim().toLowerCase()))
  const save = async () => {
    if (!saved || !writable) throw new Error('Invocation security is unavailable.')
    setError('')
    try {
      const result = await api.saveUpstreamInvocationSecurity(path, {
        upstreamId: saved.upstreamId,
        revision: saved.revision,
        policy,
      })
      setSaved(result)
      resetPolicy(result.policy)
    } catch (reason) {
      const message =
        reason instanceof ApiError && reason.status === 412
          ? 'This policy changed since you opened it. Your draft is still visible. Review it before reloading.'
          : String(reason)
      setError(message)
      throw new Error(message)
    }
  }
  const remove = async (name: string | null) => {
    if (
      !(await confirm(
        name
          ? `Remove the rule for ${name}? This tool will use the upstream default after saving.`
          : 'Clear the upstream default? Existing tool rules will be kept.',
      ))
    )
      return
    if (name === null) setPolicy({ tools: policy.tools, services: policy.services })
    else {
      const next = { ...policy.tools }
      delete next[name]
      setPolicy({ ...policy, tools: next })
    }
  }
  const stage = (rule: InvocationRule) => {
    setPolicy(
      target
        ? { ...policy, tools: { ...policy.tools, [target]: rule } }
        : { ...rule, tools: policy.tools, services: policy.services },
    )
    setTarget(undefined)
  }
  const renderTool = (name: string) => {
    if (!saved || (!saved.tools.includes(name) && !toolRule(policy, name))) return null
    const specific = toolRule(policy, name)
    const inCatalog = saved.tools.includes(name)
    const effective = effectivePolicy(policy, specific)
    return (
      <div className="space-y-2">
        <h4 className="text-sm font-medium">Effective invocation policy</h4>
        {effective.error && <ErrorBox>{effective.error.message}</ErrorBox>}
        <PolicySummary rule={effective.rule} />
        <p className="break-words text-xs text-ink-muted">
          {specific ? `Tool-specific settings · ${ruleSummary(specific)}` : 'Uses upstream default'}
          {!inCatalog ? ' · Missing from saved catalog' : ''}
        </p>
        {editable && (
          <div className="flex flex-wrap gap-2">
            {inCatalog && (
              <Button
                disabled={busy || loading}
                aria-label={`${specific ? 'Edit' : 'Add'} invocation rule for ${name}`}
                onClick={() => setTarget(name)}
              >
                {specific ? 'Edit invocation rule' : 'Add invocation rule'}
              </Button>
            )}
            {specific && (
              <Button
                disabled={busy || loading}
                variant="danger-outline"
                aria-label={`Remove invocation rule for ${name}`}
                onClick={() => void remove(name)}
              >
                Remove tool rule
              </Button>
            )}
          </div>
        )}
      </div>
    )
  }
  const defaultConfigured = !!(
    policy.retry ||
    policy.readOnly !== undefined ||
    policy.identity ||
    policy.stepUp ||
    policy.approvalServices?.length ||
    policy.shareFields?.length
  )
  const changedServices = Object.keys(policy.services ?? {}).filter(
    (name) =>
      JSON.stringify(saved?.policy.services?.[name]) !== JSON.stringify(policy.services?.[name]),
  )
  const affected =
    saved && dirty
      ? saved.tools.flatMap((name) => {
          const before = effectivePolicy(saved.policy, toolRule(saved.policy, name)).rule
          const next = effectivePolicy(policy, toolRule(policy, name))
          const after = next.rule
          const recipients = referencedServices([after])
          return next.error ||
            JSON.stringify(before) !== JSON.stringify(after) ||
            changedServices.some((service) => recipients.has(service))
            ? [{ name, summary: next.error?.message ?? ruleSummary(after) }]
            : []
        })
      : []
  const invalid =
    Object.entries(policy.tools ?? {}).some(([, rule]) => !!effectivePolicy(policy, rule).error) ||
    !!effectivePolicy(policy).error

  useUpstreamSection('invocation', {
    label: 'Invocation security',
    writable: writable && readable,
    dirty,
    blocked: readable && (loading || !saved || invalid || target !== undefined),
    save,
    discard: () => {
      if (saved) resetPolicy(saved.policy)
      setTarget(undefined)
      setError('')
    },
  })
  const open = (tool?: string) => {
    const group = section.current?.querySelector('details')
    if (group) group.open = true
    requestAnimationFrame(() => {
      const row = tool
        ? [
            ...(section.current?.querySelectorAll<HTMLElement>('[data-invocation-tool]') ?? []),
          ].find((element) => element.dataset.invocationTool === tool)
        : undefined
      const destination = row ?? section.current
      destination?.scrollIntoView({ behavior: 'smooth', block: 'start' })
      destination?.querySelector('summary')?.focus({ preventScroll: true })
    })
  }
  const renderLink = (name: string) => {
    if (!saved || (!saved.tools.includes(name) && !toolRule(policy, name))) return null
    const specific = toolRule(policy, name)
    return (
      <div className="space-y-1">
        <p className="text-xs text-ink-muted">
          {specific ? 'Tool-specific invocation policy' : 'Uses upstream invocation default'}
        </p>
        <button
          className="text-xs font-medium text-accent-700 underline"
          onClick={() => {
            setQuery(name)
            setLimit(25)
            setSelectedTool(name)
            open(name)
          }}
        >
          View invocation policy for {name}
        </button>
      </div>
    )
  }
  return (
    <ToolSecurity.Provider value={readable ? { renderLink, dirty, open } : null}>
      {children}
      {readable && (
        <section
          ref={section}
          aria-label="Upstream invocation security"
          className="layout-stack scroll-mt-24"
          id="upstream-invocation-security"
        >
          {error && (
            <ErrorBox
              title="Invocation security could not be loaded or saved"
              onRetry={reload}
              retryLabel="Reload invocation security"
              busy={busy || loading}
            >
              {error}
            </ErrorBox>
          )}
          {loading && <LoadingState label="Loading upstream invocation security…" />}
          <ConfigurationGroup
            title="Invocation security"
            description={
              saved
                ? `${names.length} tools · ${Object.keys(policy.tools ?? {}).length} tool-specific policies · ${Object.keys(policy.services ?? {}).length} local services`
                : 'Loading policy configuration'
            }
          >
            {saved && (
              <>
                <ServerPanel
                  title="Upstream default"
                  description="Set requirements for operations routed to this upstream. Tools can add stricter identity requirements and override retry or read-only settings."
                  icon={ShieldCheck}
                  actions={
                    editable && (
                      <Button disabled={busy || loading} onClick={() => setTarget(null)}>
                        {defaultConfigured ? 'Edit default' : 'Configure default'}
                      </Button>
                    )
                  }
                >
                  <div className="panel-body space-y-3">
                    <PolicySummary rule={effectivePolicy(policy).rule} />
                    {editable && defaultConfigured && (
                      <Button
                        variant="danger-outline"
                        disabled={busy || loading}
                        onClick={() => void remove(null)}
                      >
                        Clear default
                      </Button>
                    )}
                    {!!saved.sharedServices.length && (
                      <p className="break-words text-xs text-ink-muted">
                        Available shared services · {saved.sharedServices.join(', ')}
                      </p>
                    )}
                  </div>
                </ServerPanel>

                <ServerPanel
                  title="Tool invocation policies"
                  description="Review the effective rule for any tool, including requirements inherited from the upstream."
                  icon={ShieldCheck}
                >
                  {names.length ? (
                    <>
                      <div className="panel-body">
                        <SearchField
                          label="Find an invocation tool"
                          value={query}
                          onChange={(value) => {
                            setQuery(value)
                            setLimit(25)
                          }}
                          placeholder="Search by original tool name"
                        />
                      </div>
                      <ul className="divide-y divide-line">
                        {shown.slice(0, limit).map((name) => (
                          <li
                            key={name}
                            data-invocation-tool={name}
                            className="panel-body scroll-mt-64"
                          >
                            <details open={selectedTool === name || undefined}>
                              <summary className="cursor-pointer text-sm font-semibold break-all">
                                {name}
                                <span className="ml-2 text-xs font-normal text-ink-muted">
                                  {toolRule(policy, name)
                                    ? 'Tool-specific settings'
                                    : 'Uses upstream default'}
                                </span>
                              </summary>
                              <div className="mt-3">{renderTool(name)}</div>
                            </details>
                          </li>
                        ))}
                      </ul>
                      {shown.length === 0 && <EmptyState title="No matching tools" compact />}
                      {shown.length > limit && (
                        <div className="panel-footer">
                          <Button onClick={() => setLimit((value) => value + 25)}>
                            Show more tools
                          </Button>
                        </div>
                      )}
                    </>
                  ) : (
                    <EmptyState
                      title="No tools in the saved catalog"
                      description="Refresh upstream discovery or save REST operations to configure tool rules."
                    />
                  )}
                </ServerPanel>
                <Services
                  scope="upstream"
                  services={policy.services ?? {}}
                  sharedNames={saved.sharedServices}
                  used={referencedServices([policy, ...Object.values(policy.tools ?? {})])}
                  writable={editable}
                  busy={busy || loading}
                  onChange={(services) => setPolicy({ ...policy, services })}
                />
                {editable && dirty && (
                  <div
                    id="invocation-review"
                    className="surface-card panel-body space-y-3 scroll-mt-24"
                  >
                    <details>
                      <summary className="cursor-pointer text-sm font-semibold">
                        Review changes · {affected.length} catalog{' '}
                        {affected.length === 1 ? 'tool' : 'tools'} affected
                      </summary>
                      <p className="mt-2 break-words text-xs text-ink-muted">
                        Staged upstream default · {ruleSummary(policy)}
                      </p>
                      {affected.length > 0 && (
                        <ul className="mt-2 max-h-60 space-y-2 overflow-auto text-xs">
                          {affected.map(({ name, summary }) => (
                            <li className="break-words" key={name}>
                              <strong>{name}</strong> · {summary}
                            </li>
                          ))}
                        </ul>
                      )}
                    </details>
                    {invalid && (
                      <ErrorBox title="The staged policies conflict">
                        Review the tool rules before saving. Restore the default or adjust the
                        affected tool rules.
                      </ErrorBox>
                    )}
                    <div className="flex flex-wrap items-center justify-between gap-3">
                      <p role="status" className="text-sm text-ink-muted">
                        Invocation changes are staged.
                      </p>
                      <span className="text-xs text-ink-muted">
                        Use Save at the top of the page to apply this draft.
                      </span>
                    </div>
                  </div>
                )}
                {editable && target !== undefined && (
                  <RuleEditor
                    key={target ?? ''}
                    doc={{ ...saved, policy }}
                    tool={target}
                    onClose={() => setTarget(undefined)}
                    onStage={stage}
                  />
                )}
              </>
            )}
          </ConfigurationGroup>
        </section>
      )}
      <LocalAuditSettingsPanel grouped scope="upstream" slug={slug} />
    </ToolSecurity.Provider>
  )
}
