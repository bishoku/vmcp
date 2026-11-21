import { useState } from 'react'
import type { RESTOperation } from '../api'
import { btn, primary } from '../styles'
import { Modal } from '../ui'

const format = 'vmcp.rest-operations.v1'

function portable(op: RESTOperation) {
  const {
    name,
    description,
    method,
    path,
    parameters,
    bodySchema,
    responseFormat,
    responseSchema,
    responseFields,
    responseFieldPaths,
    successStatuses,
    enabled,
    readOnly,
  } = op
  return {
    name,
    description,
    method,
    path,
    parameters,
    bodySchema,
    responseFormat,
    responseSchema,
    responseFields,
    responseFieldPaths,
    successStatuses,
    enabled,
    readOnly,
  }
}

function strings(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((v) => typeof v === 'string')
}

function readOperations(source: string): RESTOperation[] {
  const doc = JSON.parse(source)
  if (doc?.format !== format || !Array.isArray(doc.operations) || doc.operations.length > 200)
    throw Error('Choose a v1 vMCP REST operations file with up to 200 operations.')
  const names = new Set<string>()
  return doc.operations.map((op: RESTOperation) => {
    if (
      !op ||
      typeof op.name !== 'string' ||
      !/^[A-Za-z0-9_.-]{1,128}$/.test(op.name) ||
      names.has(op.name) ||
      typeof op.path !== 'string' ||
      !op.path.startsWith('/') ||
      !['GET', 'POST', 'PUT', 'PATCH', 'DELETE'].includes(op.method) ||
      typeof op.description !== 'string' ||
      typeof op.enabled !== 'boolean' ||
      typeof op.readOnly !== 'boolean' ||
      !Array.isArray(op.parameters) ||
      !op.parameters.every(
        (p) =>
          p &&
          typeof p.name === 'string' &&
          ['path', 'query'].includes(p.in) &&
          p.schema &&
          typeof p.schema === 'object',
      ) ||
      (op.responseFormat !== undefined && !['json', 'text'].includes(op.responseFormat)) ||
      (op.responseFormat === 'text' &&
        (op.responseSchema !== undefined ||
          op.responseFields?.length ||
          op.responseFieldPaths?.length)) ||
      (op.responseFields !== undefined && !strings(op.responseFields)) ||
      (op.responseFieldPaths !== undefined &&
        !(Array.isArray(op.responseFieldPaths) && op.responseFieldPaths.every(strings))) ||
      (op.successStatuses !== undefined &&
        !(Array.isArray(op.successStatuses) && op.successStatuses.every(Number.isInteger)))
    )
      throw Error('Invalid or duplicate operation in definition file.')
    names.add(op.name)
    return portable(op)
  })
}

export default function RESTDefinitionTransfer({
  operations,
  apply,
}: {
  operations: RESTOperation[]
  apply?: (ops: RESTOperation[]) => void
}) {
  const [incoming, setIncoming] = useState<RESTOperation[] | null>(null)
  const [error, setError] = useState('')
  const byName = new Map(operations.map((op) => [op.name, op]))
  const newOps = incoming?.filter((op) => !byName.has(op.name)) ?? []
  const updates =
    incoming?.filter(
      (op) =>
        byName.has(op.name) &&
        JSON.stringify(portable(byName.get(op.name)!)) !== JSON.stringify(op),
    ) ?? []
  return (
    <div className="flex flex-wrap items-center gap-2">
      <button
        className={btn}
        disabled={!operations.length}
        onClick={() => {
          const url = URL.createObjectURL(
            new Blob([JSON.stringify({ format, operations: operations.map(portable) }, null, 2)], {
              type: 'application/json',
            }),
          )
          const link = document.createElement('a')
          link.href = url
          link.download = 'rest-operations.json'
          link.click()
          setTimeout(() => URL.revokeObjectURL(url), 1000)
        }}
      >
        Export operations
      </button>
      {apply && (
        <label className={btn}>
          Import operations file
          <input
            type="file"
            className="sr-only"
            accept=".json,application/json"
            onChange={async (e) => {
              const file = e.target.files?.[0]
              e.target.value = ''
              if (!file) return
              try {
                setError('')
                if (file.size > 32 * 1024 * 1024)
                  throw Error('Choose a definition file up to 32 MiB.')
                setIncoming(readOperations(await file.text()))
              } catch (err) {
                setError(String(err))
              }
            }}
          />
        </label>
      )}
      {error && <p role="alert">{error}</p>}
      {incoming && apply && (
        <Modal
          title="Review operation changes"
          onClose={() => setIncoming(null)}
          footer={
            <button
              className={primary}
              disabled={
                operations.length + newOps.length > 200 || (!updates.length && !newOps.length)
              }
              onClick={() => {
                const replacements = new Map(updates.map((op) => [op.name, op]))
                apply([
                  ...operations.map((op) => ({ ...op, ...replacements.get(op.name) })),
                  ...newOps,
                ])
                setIncoming(null)
              }}
            >
              Apply to upstream draft
            </button>
          }
        >
          <p>
            Match by tool name. Destination-only operations, connection settings and operation
            headers are kept.
          </p>
          <p>
            Add ({newOps.length}): {newOps.map((op) => op.name).join(', ') || 'None'}
          </p>
          <p>
            Update ({updates.length}): {updates.map((op) => op.name).join(', ') || 'None'}
          </p>
          <p>Unchanged: {incoming.length - newOps.length - updates.length}</p>
          {operations.length + newOps.length > 200 && (
            <p role="alert">The resulting upstream exceeds 200 operations.</p>
          )}
        </Modal>
      )}
    </div>
  )
}
