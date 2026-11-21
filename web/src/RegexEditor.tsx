import { useEffect, useRef, useState } from 'react'
import { api, type RegexPreview } from './api'
import { btn, input, primary } from './styles'
import { Modal } from './ui'

type Props = {
  kind: 'audit' | 'pii'
  name: string
  pattern: string
  disabled?: boolean
  onApply: (pattern: string) => void
}

export default function RegexEditor(props: Props) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <button type="button" className={btn} disabled={props.disabled} onClick={() => setOpen(true)}>
        Edit and test
      </button>
      {open && <Editor {...props} close={() => setOpen(false)} />}
    </>
  )
}

function Editor({ kind, name, pattern, onApply, close }: Props & { close: () => void }) {
  const [draft, setDraft] = useState(pattern)
  const [sample, setSample] = useState('')
  const [result, setResult] = useState<RegexPreview>()
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const pending = useRef<AbortController | null>(null)
  useEffect(() => () => pending.current?.abort(), [])
  function reset() {
    pending.current?.abort()
    setResult(undefined)
    setError('')
    setBusy(false)
  }
  async function test() {
    reset()
    const controller = new AbortController()
    pending.current = controller
    setBusy(true)
    try {
      const value = await api.previewRegex(kind, name, draft, sample, controller.signal)
      if (!controller.signal.aborted) setResult(value)
    } catch (err) {
      if (!controller.signal.aborted)
        setError(err instanceof Error ? err.message : 'Preview failed')
    } finally {
      if (!controller.signal.aborted) setBusy(false)
    }
  }
  return (
    <Modal
      title="Edit and test regex"
      initialFocus="textarea"
      onClose={close}
      footer={
        <>
          <button type="button" className={btn} onClick={close}>
            Cancel
          </button>
          <button
            type="button"
            className={primary}
            disabled={!result}
            onClick={() => {
              onApply(draft)
              close()
            }}
          >
            Use pattern
          </button>
        </>
      }
    >
      <div className="space-y-4">
        <label className="block">
          RE2 pattern
          <textarea
            className={input + ' font-mono'}
            value={draft}
            onChange={(e) => {
              reset()
              setDraft(e.target.value)
            }}
          />
        </label>
        <details>
          <summary className="cursor-pointer">Regex syntax help</summary>
          <p className="text-sm">
            Use <code>\d</code> for a digit, <code>{'{6}'}</code> for six repetitions and{' '}
            <code>\b</code> for a word boundary. Enter a single backslash in the pattern. Go RE2
            does not support lookaround or backreferences.
          </p>
          <button
            type="button"
            className={btn}
            onClick={() => {
              reset()
              setDraft(String.raw`\bEMP-\d{6}\b`)
              setSample('Employee EMP-123456, previous ID EMP-12345')
            }}
          >
            Load employee ID example
          </button>
        </details>
        <label className="block">
          Test text
          <textarea
            className={input + ' font-mono'}
            rows={4}
            value={sample}
            onChange={(e) => {
              reset()
              setSample(e.target.value)
            }}
          />
        </label>
        <button type="button" className={btn} disabled={busy} onClick={test}>
          {busy ? 'Testing…' : 'Test pattern'}
        </button>
        {error && <p role="alert">{error}</p>}
        {result && (
          <section aria-live="polite" className="space-y-2">
            <h3>
              {result.count} {result.count === 1 ? 'match' : 'matches'}
            </h3>
            <pre className="whitespace-pre-wrap break-all">
              {result.parts.map((p, i) =>
                p.match ? <mark key={i}>{p.text || '∅'}</mark> : p.text,
              )}
            </pre>
            <h3>Output with this rule</h3>
            <pre className="surface-inset whitespace-pre-wrap break-all p-2">{result.masked}</pre>
          </section>
        )}
      </div>
    </Modal>
  )
}
