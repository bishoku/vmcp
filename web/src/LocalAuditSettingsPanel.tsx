import ConfigurationGroup from './connections/ConfigurationGroup'
import { useConfirm } from './confirmation'
import { useUpstreamSection } from './pages/upstream-edit-session'
import { useEffect, useState } from 'react'
import { api, type AuditRule, type AuditPreview, type LocalAuditSettings } from './api'
import { useConfigurationChecks, configurationPath } from './configuration-access'
import { useDraft, useOperation } from './interaction'
import { ServerPanel } from './connections/ServerPanels'
import { ErrorBox } from './ui'
import { ShieldCheck } from 'lucide-react'
import { btn, input, primary } from './styles'

export default function LocalAuditSettingsPanel({
  scope,
  slug,
  grouped = false,
}: {
  scope: 'virtual' | 'upstream'
  slug: string
  grouped?: boolean
}) {
  const path =
    configurationPath(scope === 'virtual' ? 'virtual_servers' : 'upstreams', slug) +
    '/audit-settings'
  const access = useConfigurationChecks([
    { method: 'GET', path },
    { method: 'PUT', path, body: { revision: 0, rules: [] } },
    { method: 'POST', path: path + '/preview' },
  ])
  const readable = access.allowed[0] === true
  if (!readable) return null
  const editor = (
    <LocalEditor
      key={path}
      path={path}
      writable={access.allowed[1] === true}
      previewable={access.allowed[2] === true}
    />
  )
  return grouped ? (
    <ConfigurationGroup
      title="Audit payload masking"
      description="Local masking rules add to mandatory and global protection"
    >
      {editor}
    </ConfigurationGroup>
  ) : (
    editor
  )
}
function LocalEditor({
  path,
  writable,
  previewable,
}: {
  path: string
  writable: boolean
  previewable: boolean
}) {
  const [draft, setDraft, resetDraft, , dirty, isDirty, discardDraft] =
    useDraft<LocalAuditSettings | null>(null, () => path)
  const [error, setError] = useState(''),
    [notice, setNotice] = useState(''),
    [generation, setGeneration] = useState(0)
  const [request, setRequest] = useState('{}'),
    [response, setResponse] = useState('{}')
  const [previews, setPreviews] = useState<AuditPreview[] | null>(null)
  const { busy, run } = useOperation()
  useEffect(() => {
    const controller = new AbortController()
    void api
      .localAuditSettings(path, controller.signal)
      .then((value) => {
        if (!controller.signal.aborted) {
          resetDraft(value)
          setError('')
          setPreviews(null)
        }
      })
      .catch((error) => {
        if (!controller.signal.aborted) setError(String(error))
      })
    return () => controller.abort()
    // A new source or explicit reload discards the old draft and samples.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [path, generation])
  const save = async () => {
    if (!draft || !writable) throw new Error('Audit masking is unavailable.')
    setError('')
    try {
      const result = await api.saveLocalAuditSettings(path, draft)
      resetDraft(result)
      setPreviews(null)
      setNotice(
        'Local audit rules saved. New operations use the updated policy after configuration refresh.',
      )
    } catch (error) {
      setError(String(error))
      throw error
    }
  }
  const session = useUpstreamSection('audit', {
    label: 'Audit payload masking',
    isDirty,
    writable,
    dirty,
    blocked: !draft || busy,
    save,
    discard: () => {
      discardDraft()
      setPreviews(null)
      setError('')
      setNotice('')
      setRequest('{}')
      setResponse('{}')
    },
  })
  const editable = writable && (!session || (session.editing && !session.saving))
  const confirm = useConfirm()
  const rules = draft?.rules ?? []
  const change = (rules: AuditRule[]) => {
    if (!draft || !editable) return
    setDraft({ ...draft, rules })
    setPreviews(null)
    setNotice('')
  }
  return (
    <ServerPanel
      title="Audit payload masking"
      description="Local rules add to global protection. Capture and global rules are managed separately."
      icon={ShieldCheck}
    >
      <div className="space-y-5 panel-body">
        {error && (
          <ErrorBox>
            {error}
            <button
              className={btn}
              disabled={busy}
              onClick={async () => {
                if (dirty && !(await confirm('Discard unsaved audit masking changes and reload?')))
                  return
                setDraft(null)
                setRequest('{}')
                setResponse('{}')
                setGeneration((value) => value + 1)
              }}
            >
              Reload local audit settings
            </button>
          </ErrorBox>
        )}
        {notice && (
          <p role="status" className="text-sm">
            {notice}
          </p>
        )}
        {!draft ? (
          <p role="status">Loading local audit settings…</p>
        ) : (
          <>
            <p className="text-xs text-ink-muted">
              Local rules apply to both request and response payloads. Global rules and mandatory
              secret filters always apply.
            </p>
            {session && !session.editing ? (
              <div className="space-y-3">
                <h3 className="text-sm font-medium">Local masking rules · {rules.length}</h3>
                {rules.length ? (
                  <dl className="divide-y divide-line">
                    {rules.map((rule, index) => (
                      <div key={index} className="py-3 space-y-1">
                        <dt className="text-sm font-medium break-words">{rule.name}</dt>
                        <dd className="font-mono text-xs break-all text-ink-muted">
                          {rule.pattern}
                        </dd>
                      </div>
                    ))}
                  </dl>
                ) : (
                  <p className="text-sm text-ink-muted">
                    No local masking rules. Global rules and mandatory secret filters still apply.
                  </p>
                )}
              </div>
            ) : (
              <fieldset disabled={!editable || busy} className="m-0 space-y-3 border-0 p-0">
                <legend className="text-sm font-medium">Local RE2 masking rules</legend>
                {rules.map((rule, index) => (
                  <div key={index} className="flex flex-wrap gap-2">
                    <input
                      aria-label={`Audit rule ${index + 1} name`}
                      className={input}
                      value={rule.name}
                      onChange={(event) =>
                        change(
                          rules.map((r, i) =>
                            i === index ? { ...r, name: event.target.value } : r,
                          ),
                        )
                      }
                    />
                    <input
                      aria-label={`Audit rule ${index + 1} regex`}
                      className={input}
                      value={rule.pattern}
                      onChange={(event) =>
                        change(
                          rules.map((r, i) =>
                            i === index ? { ...r, pattern: event.target.value } : r,
                          ),
                        )
                      }
                    />
                    <button
                      className={btn}
                      onClick={() => change(rules.filter((_, i) => i !== index))}
                    >
                      Remove rule {index + 1}
                    </button>
                  </div>
                ))}
                <button
                  className={btn}
                  disabled={rules.length >= 20 || busy}
                  onClick={() => change([...rules, { name: '', pattern: '' }])}
                >
                  Add masking rule
                </button>
              </fieldset>
            )}
            {previewable && (!session || session.editing) && (
              <details>
                <summary className="cursor-pointer text-sm font-medium">
                  Test request / response audit payload
                </summary>
                <div className="mt-4 space-y-4">
                  <p className="text-xs text-ink-muted">
                    Synthetic samples only. The server adds current global rules and fixes the
                    resource context; no stored messages are read.
                  </p>
                  <label className="block text-sm">
                    Request JSON
                    <textarea
                      aria-label="Example audit request"
                      className={input}
                      value={request}
                      onChange={(event) => {
                        setRequest(event.target.value)
                        setPreviews(null)
                      }}
                    />
                  </label>
                  <label className="block text-sm">
                    Response JSON
                    <textarea
                      aria-label="Example audit response"
                      className={input}
                      value={response}
                      onChange={(event) => {
                        setResponse(event.target.value)
                        setPreviews(null)
                      }}
                    />
                  </label>
                  <button
                    className={btn}
                    disabled={busy}
                    onClick={() =>
                      void run(async () => {
                        setError('')
                        setPreviews(null)
                        try {
                          const samples = [JSON.parse(request), JSON.parse(response)]
                          const results = await Promise.all(
                            (['request', 'response'] as const).map((direction, index) =>
                              api.previewLocalAudit(path, {
                                ...draft,
                                direction,
                                payload: samples[index],
                              }),
                            ),
                          )
                          setPreviews(results)
                        } catch (error) {
                          setError(String(error))
                        }
                      })
                    }
                  >
                    Preview audit payloads
                  </button>
                  {previews?.map((preview, index) => (
                    <pre className="whitespace-pre-wrap break-all text-xs" key={index}>
                      {index === 0 ? 'request' : 'response'} · {preview.state} ·{' '}
                      {JSON.stringify(preview.content)}
                    </pre>
                  ))}
                </div>
              </details>
            )}
            <div className="flex flex-wrap items-center justify-between gap-3">
              <span className="text-xs text-ink-muted">
                Policy revision {draft.revision}
                {dirty ? ' · Unsaved masking changes' : ''}
              </span>
              {!session && (
                <button
                  className={primary}
                  disabled={!writable || busy || !dirty}
                  onClick={() =>
                    void run(async () => {
                      try {
                        await save()
                      } catch {
                        /* Error is shown above. */
                      }
                    })
                  }
                >
                  Save audit settings
                </button>
              )}
            </div>
          </>
        )}
      </div>
    </ServerPanel>
  )
}
