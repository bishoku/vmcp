import assert from 'node:assert/strict'
import { performance } from 'node:perf_hooks'
import { Client, StreamableHTTPClientTransport } from '@modelcontextprotocol/client'
import { Client as Legacy } from '@modelcontextprotocol/sdk/client/index.js'
import { StreamableHTTPClientTransport as LegacyHTTP } from '@modelcontextprotocol/sdk/client/streamableHttp.js'

const [url, revision, endpoint] = process.argv.slice(2)
const modern = revision === 'modern'
const C = modern ? Client : Legacy
const T = modern ? StreamableHTTPClientTransport : LegacyHTTP
const transport = new T(new URL(url), {
  requestInit: { headers: { Authorization: 'Bearer runtime-key' } },
})
const client = new C(
  { name: 'matrix', version: '1' },
  modern ? { versionNegotiation: { mode: { pin: '2026-07-28' } } } : {},
)
try {
  await client.connect(transport)
  const list = await client.listTools()
  const omni = endpoint === 'omni'
  assert.ok(list.tools.some((t) => t.name === (omni ? 'invoke_tool' : 'tables')))
  if (omni) {
    const discovery = await client.callTool({
      name: 'discover_tools',
      arguments: { query: 'tables' },
    })
    assert.ok(discovery.structuredContent.tools.some((t) => t.tool_ref === 'support/tables'))
    const schema = await client.callTool({
      name: 'get_tool_schema',
      arguments: { tool_ref: 'support/tables' },
    })
    assert.ok(schema.structuredContent.schema_version)
  }
  const times = []
  for (let i = 0; i < 21; i++) {
    const start = performance.now()
    const result = await client.callTool(
      omni
        ? { name: 'invoke_tool', arguments: { tool_ref: 'support/tables', arguments: {} } }
        : { name: 'tables', arguments: {} },
    )
    times.push(performance.now() - start)
    assert.equal(result.content[0].text, 'list_tables')
  }
  const [cold, ...warm] = times
  warm.sort((a, b) => a - b)
  console.log(JSON.stringify({ firstMs: cold, warmP95Ms: warm[18], samples: warm.length }))
} finally {
  await client.close()
}
