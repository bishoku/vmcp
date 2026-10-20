import { Code2 } from 'lucide-react'
import type { RESTOperation } from '../api'
import { ServerPanel } from './ServerPanels'
import { restToolPreview } from './rest-tool-preview'

export default function RESTToolPreview({ operation, invalid = false }: { operation?: RESTOperation; invalid?: boolean }) {
  return <ServerPanel title="MCP preview" description="The tool definition your agent will receive" icon={Code2}>
    <div className="panel-body layout-stack">
      <p className="text-xs text-ink-muted">Local preview · no requests are sent. Save validates the configuration; virtual servers choose which tools to expose.</p>
      {invalid
        ? <p role="status" className="text-sm text-amber-800">Resolve the editor errors to see an up-to-date MCP preview.</p>
        : operation ? <>
          <div className="surface-inset p-3">
            <p className="break-all text-sm font-semibold">{operation.name || 'Name your tool'}</p>
            <p className="mt-2 whitespace-pre-wrap break-words text-xs text-ink-muted">{operation.description || 'Describe when an agent should use this tool.'}</p>
          </div>
          {!operation.enabled && <p className="text-xs text-amber-800">Disabled · this definition will not be included in the tool catalog.</p>}
          <pre aria-label="MCP tool definition" className="max-h-[36rem] overflow-auto surface-inset whitespace-pre-wrap break-all p-3 text-xs">{JSON.stringify(restToolPreview(operation), null, 2)}</pre>
        </> : <p className="text-sm text-ink-muted">Select an operation, import a definition or add your first request.</p>}
    </div>
  </ServerPanel>
}
