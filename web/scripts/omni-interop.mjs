import assert from 'node:assert/strict'
import { Client } from '@modelcontextprotocol/sdk/client/index.js'
import { StreamableHTTPClientTransport } from '@modelcontextprotocol/sdk/client/streamableHttp.js'

const client = new Client({ name: 'vmcp-omni-interop', version: '1.0.0' })
const transport = new StreamableHTTPClientTransport(new URL(process.argv[2]), {
  requestInit: { headers: { Authorization: `Bearer ${process.env.VMCP_INTEROP_KEY}` } },
})
try {
  await client.connect(transport)
  const catalog = await client.listTools()
  assert.deepEqual(catalog.tools.map(t => t.name), ['discover_server', 'discover_tools', 'get_tool_schema', 'invoke_tool'])
  const found = await client.callTool({ name: 'discover_tools', arguments: { query: 'tables' } })
  const tool = found.structuredContent.tools.find(t => t.name === 'tables')
  assert.ok(tool)
  const schema = await client.callTool({ name: 'get_tool_schema', arguments: { tool_ref: tool.tool_ref } })
  assert.equal(schema.structuredContent.tool.name, 'tables')
  const result = await client.callTool({ name: 'invoke_tool', arguments: { tool_ref: tool.tool_ref, schema_version: schema.structuredContent.schema_version, arguments: {} } })
  assert.equal(result.content[0].text, 'pg:list_tables')
  console.log('Omni MCP TypeScript SDK interoperability passed')
} finally {
  await client.close()
}
