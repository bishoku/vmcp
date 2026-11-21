import type { RESTOperation } from '../api'
import { parameterBehavior } from './rest-parameters'
import RESTToolPreview from './RESTToolPreview'
import { restToolPreview, schemaObject } from './rest-tool-preview'

export function formatLabel(op: RESTOperation) {
  return op.responseFormat === 'text' ? 'Plain text' : 'JSON'
}

export function inputCounts(op: RESTOperation) {
  const fixed = op.parameters.filter((p) => p.exposeToMcp === false).length
  const visible = op.parameters.length - fixed + Number(op.bodySchema !== undefined)
  return `${visible} agent input${visible === 1 ? '' : 's'} · ${fixed} fixed`
}

export default function RESTOperationDetails({ operation: op }: { operation: RESTOperation }) {
  const shape = schemaObject(restToolPreview(op).outputSchema.properties.result)
  const selected = op.responseFieldPaths?.map((path) => path.join(' → '))
  const fields = selected?.length ? selected : op.responseFields
  return (
    <div className="layout-stack min-w-0">
      {op.description && <p>{op.description}</p>}
      <section className="surface-inset p-3 layout-stack">
        <h3 className="font-semibold">Agent inputs</h3>
        <p>{inputCounts(op)}</p>
        <ul>
          {op.parameters.map((p) => (
            <li key={p.name}>
              <code>{p.name}</code> · {p.in} · {String(p.schema.type)} · {parameterBehavior(p)}
            </li>
          ))}
          {op.bodySchema !== undefined && (
            <li>
              <code>body</code> · JSON request body · Required
            </li>
          )}
        </ul>
      </section>
      <section className="surface-inset p-3 layout-stack">
        <h3 className="font-semibold">HTTP request</h3>
        <code className="break-all">
          {op.method} {op.path}
        </code>
        <code>Accept: {op.responseFormat === 'text' ? 'text/plain' : 'application/json'}</code>
      </section>
      <section className="surface-inset p-3 layout-stack">
        <h3 className="font-semibold">Agent result</h3>
        <p>
          <code>result</code> · {formatLabel(op)} ·{' '}
          {String(shape.type ?? 'schema-defined JSON value')}
        </p>
        {!!fields?.length && <p>Selected fields: {fields.join(', ')}</p>}
      </section>
      <details>
        <summary>Full MCP tool definition</summary>
        <RESTToolPreview operation={op} />
      </details>
    </div>
  )
}
