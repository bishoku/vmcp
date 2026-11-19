import assert from 'node:assert/strict'
const [url, profile] = process.argv.slice(2)
const modern = '2026-07-28',
  legacy = '2025-06-18'
let id = 0
async function rpc(method, params, headers = {}) {
  const resp = await fetch(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Accept: 'application/json, text/event-stream',
      ...headers,
    },
    body: JSON.stringify({ jsonrpc: '2.0', id: ++id, method, params }),
  })
  const body = await resp.text()
  if (!resp.ok) return { ok: false }
  const reply = resp.headers.get('content-type')?.includes('text/event-stream')
    ? body
        .split(/\r?\n/)
        .filter((line) => line.startsWith('data:'))
        .map((line) => JSON.parse(line.slice(5)))
        .find((message) => message.id === id)
    : JSON.parse(body)
  assert.equal(reply.id, id)
  return { ok: !reply.error, result: reply.result, sid: resp.headers.get('Mcp-Session-Id') }
}
const clientInfo = { name: 'direct-profile-check', version: '1' }
const meta = {
  'io.modelcontextprotocol/protocolVersion': modern,
  'io.modelcontextprotocol/clientInfo': clientInfo,
  'io.modelcontextprotocol/clientCapabilities': {},
}
const headers = { 'MCP-Protocol-Version': modern, 'Mcp-Method': 'server/discover' }
const discovery = await rpc('server/discover', { _meta: meta }, headers)
assert.equal(
  Boolean(discovery.ok && discovery.result.supportedVersions.includes(modern)),
  profile !== 'legacy',
  'modern offering',
)
if (discovery.ok) {
  const list = await rpc('tools/list', { _meta: meta }, { ...headers, 'Mcp-Method': 'tools/list' })
  assert.ok(list.result.tools.some((t) => t.name === 'echo'))
}
const init = await rpc('initialize', { protocolVersion: legacy, capabilities: {}, clientInfo })
const acceptsLegacy = init.ok && init.result.protocolVersion === legacy
assert.equal(acceptsLegacy, profile !== 'modern', 'legacy offering')
if (acceptsLegacy) {
  const h = { 'MCP-Protocol-Version': legacy }
  if (init.sid) h['Mcp-Session-Id'] = init.sid
  const ack = await fetch(url, {
    method: 'POST',
    headers: {
      ...h,
      'Content-Type': 'application/json',
      Accept: 'application/json, text/event-stream',
    },
    body: JSON.stringify({ jsonrpc: '2.0', method: 'notifications/initialized' }),
  })
  assert.ok(ack.ok)
  const list = await rpc('tools/list', {}, h)
  assert.ok(list.result.tools.some((t) => t.name === 'echo'))
  if (init.sid) assert.ok((await fetch(url, { method: 'DELETE', headers: h })).ok)
}
