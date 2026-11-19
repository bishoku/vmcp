import assert from 'node:assert/strict'
import { Client, StreamableHTTPClientTransport } from '@modelcontextprotocol/client'
import { Client as Legacy } from '@modelcontextprotocol/sdk/client/index.js'
import { StreamableHTTPClientTransport as LegacyHTTP } from '@modelcontextprotocol/sdk/client/streamableHttp.js'
const [url, revision, endpoint, value] = process.argv.slice(2)
const modern = revision === 'modern'
const client = modern
  ? new Client(
      { name: 'interop', version: '1' },
      { versionNegotiation: { mode: { pin: '2026-07-28' } } },
    )
  : new Legacy({ name: 'interop', version: '1' })
const HTTP = modern ? StreamableHTTPClientTransport : LegacyHTTP
const transport = new HTTP(new URL(url), {
  requestInit: { headers: { Authorization: `Bearer ${process.env.VMCP_INTEROP_KEY}` } },
})
try {
  await client.connect(transport)
  if (!modern)
    assert.equal(transport.protocolVersion, '2025-06-18', 'negotiated downstream revision')
  const tools = await client.listTools()
  const omni = endpoint === 'omni'
  assert.ok(tools.tools.some((t) => t.name === (omni ? 'invoke_tool' : 'say')))
  if (omni) {
    const found = await client.callTool({ name: 'discover_tools', arguments: { query: 'say' } })
    assert.ok(found.structuredContent.tools.some((t) => t.tool_ref === 'interop/say'))
    const schema = await client.callTool({
      name: 'get_tool_schema',
      arguments: { tool_ref: 'interop/say' },
    })
    assert.ok(schema.structuredContent.schema_version)
  }
  const args = { value }
  for (let i = 0; i < 2; i++) {
    const out = await client.callTool(
      omni
        ? { name: 'invoke_tool', arguments: { tool_ref: 'interop/say', arguments: args } }
        : { name: 'say', arguments: args },
    )
    assert.equal(out.isError ?? false, false)
    assert.equal(out.content[0].text, value)
  }
  if (!modern && !omni) await transport.terminateSession()
} finally {
  await client.close()
}
