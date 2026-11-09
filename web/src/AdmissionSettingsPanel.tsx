import { useEffect, useEffectEvent, useState } from 'react'
import { Gauge } from 'lucide-react'
import { api, type AdmissionSettings } from './api'
import { useConfigurationPermissions } from './configuration-access'
import { useDraft, useOperation } from './interaction'
import { ServerPanel } from './connections/ServerPanels'
import { ErrorBox } from './ui'
import { btn, input, primary } from './styles'

const fields = [
  ['total', 'Total concurrent requests'],
  ['perServer', 'Concurrent requests per virtual server'],
  ['perPrincipal', 'Concurrent requests per principal'],
] as const
export default function AdmissionSettingsPanel() {
  const access = useConfigurationPermissions('admission', '', ['read', 'update'])
  const readable = access.can('read'),
    writable = access.can('update')
  const [draft, setDraft, resetDraft] = useDraft<AdmissionSettings | null>(null, (s) =>
    s ? 'admission' : null,
  )
  const [error, setError] = useState(''),
    [notice, setNotice] = useState('')
  const { busy, run } = useOperation()
  const received = useEffectEvent((s: AdmissionSettings) => resetDraft(s))
  useEffect(() => {
    if (!readable) return
    const controller = new AbortController()
    void api
      .admissionSettings(controller.signal)
      .then((s) => {
        if (!controller.signal.aborted) received(s)
      })
      .catch((e) => {
        if (!controller.signal.aborted) setError(String(e))
      })
    return () => controller.abort()
  }, [readable])
  if (!readable) return null
  const invalid =
    draft &&
    (!Number.isInteger(draft.total) ||
      draft.total < 1 ||
      draft.total > 65536 ||
      [draft.perServer, draft.perPrincipal].some(
        (v) => !Number.isInteger(v) || v < 1 || v > draft.total,
      ))
  const save = () =>
    run(async () => {
      if (!draft || !writable || invalid) return
      setError('')
      setNotice('')
      try {
        resetDraft(await api.saveAdmissionSettings(draft))
        setNotice('Runtime protection saved. Each pod applies changes after configuration refresh.')
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e))
      }
    })
  const reload = () =>
    run(async () => {
      setError('')
      try {
        resetDraft(await api.admissionSettings())
        setNotice('')
      } catch (e) {
        setError(String(e))
      }
    })
  return (
    <ServerPanel
      title="Runtime protection"
      description="Concurrency limits per data-plane pod"
      icon={Gauge}
    >
      <div className="panel-body space-y-4">
        <p className="text-sm text-ink-muted">
          Each pod protects its own capacity. Adding replicas increases the combined allowance.
          These limits are not tenant quotas or requests-per-second limits. Active requests and open
          streams share the allowance; disconnected tool calls retain a slot until their worker
          finishes.
        </p>
        {error && (
          <ErrorBox>
            {error}
            <button className={btn} disabled={busy} onClick={() => void reload()}>
              Reload runtime protection
            </button>
          </ErrorBox>
        )}
        {notice && (
          <p role="status" className="text-sm">
            {notice}
          </p>
        )}
        {!draft ? (
          <p className="text-sm">Loading runtime protection…</p>
        ) : (
          <>
            <fieldset disabled={!writable || busy} className="grid gap-4 md:grid-cols-3">
              {fields.map(([key, label]) => (
                <label key={key} className="text-sm">
                  {label}
                  <input
                    className={input}
                    type="number"
                    min={1}
                    max={key === 'total' ? 65536 : draft.total}
                    step={1}
                    value={draft[key]}
                    onChange={(e) => {
                      setNotice('')
                      setDraft({ ...draft, [key]: Number(e.target.value) })
                    }}
                  />
                </label>
              ))}
            </fieldset>
            {invalid && (
              <p role="alert" className="text-sm">
                Total must be an integer from 1 to 65,536. Server and principal limits must be
                integers from 1 to total.
              </p>
            )}
            <p className="text-xs text-ink-muted">
              Lowering a limit blocks new requests until capacity becomes available. Running
              operations continue.
            </p>
            {writable && (
              <button className={primary} disabled={busy || !!invalid} onClick={() => void save()}>
                Save runtime protection
              </button>
            )}
          </>
        )}
      </div>
    </ServerPanel>
  )
}
