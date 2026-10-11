import assert from 'node:assert/strict'
import { Client } from '@modelcontextprotocol/sdk/client/index.js'
import { StreamableHTTPClientTransport } from '@modelcontextprotocol/sdk/client/streamableHttp.js'
import { readFile } from 'node:fs/promises'
const pkg = JSON.parse(await readFile(new URL('../node_modules/@modelcontextprotocol/sdk/package.json', import.meta.url)))
assert.equal(pkg.version, '1.32.1')
const client = new Client({ name: 'vmcp-release-check', version: '1' }, { capabilities: {} })
const transport = new StreamableHTTPClientTransport(new URL(process.argv[2]))
try {
  await client.connect(transport)
  assert.equal(client.getServerCapabilities().tools !== undefined, true)
  await client.ping()
  const list = await client.listTools()
  assert.ok(list.tools.some(t => t.name === 'say'))
  const result = await client.callTool({ name: 'say', arguments: { message: 'interop' } })
  assert.equal(result.isError ?? false, false)
  assert.ok(result.content.some(c => c.type === 'text' && c.text.includes('interop')))
  assert.equal(transport.protocolVersion, '2025-06-18')
  await transport.terminateSession()
  console.log(`MCP SDK ${pkg.version}: negotiated 2025-06-18, ping, tools/list, tools/call and DELETE passed`)
} finally { await client.close() }
