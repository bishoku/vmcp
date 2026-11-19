import assert from 'node:assert/strict'
const [url, profile, sdk] = process.argv.slice(2)
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
  if (!resp.ok) {
    const error = resp.headers.get('content-type')?.includes('application/json')
      ? JSON.parse(body).error
      : undefined
    return { status: resp.status, error, body }
  }
  const reply = resp.headers.get('content-type')?.includes('text/event-stream')
    ? body
        .split(/\r?\n/)
        .filter((line) => line.startsWith('data:'))
        .map((line) => JSON.parse(line.slice(5)))
        .find((message) => message.id === id)
    : JSON.parse(body)
  assert.equal(reply.id, id)
  assert.equal(reply.error, undefined)
  return { status: resp.status, result: reply.result, sid: resp.headers.get('Mcp-Session-Id') }
}
const clientInfo = { name: 'direct-profile-check', version: '1' }
const meta = {
  'io.modelcontextprotocol/protocolVersion': modern,
  'io.modelcontextprotocol/clientInfo': clientInfo,
  'io.modelcontextprotocol/clientCapabilities': {},
}
const headers = { 'MCP-Protocol-Version': modern, 'Mcp-Method': 'server/discover' }
const discovery = await rpc('server/discover', { _meta: meta }, headers)
if (profile === 'legacy') {
  assert.equal(discovery.status, 400)
  if (sdk === 'go') {
    assert.equal(discovery.error.code, -32022)
    assert.equal(discovery.error.data.requested, modern)
    assert.deepEqual(discovery.error.data.supported, [legacy])
  } else if (sdk === 'typescript') {
    assert.equal(discovery.error.code, -32000)
    assert.match(discovery.error.message, /Unsupported protocol version: 2026-07-28/)
  } else {
    assert.equal(sdk, 'fastmcp')
    assert.equal(discovery.error.code, -32600)
    assert.match(discovery.error.message, /Missing session ID/)
  }
} else {
  assert.equal(discovery.status, 200)
  assert.ok(discovery.result.supportedVersions.includes(modern))
  const list = await rpc('tools/list', { _meta: meta }, { ...headers, 'Mcp-Method': 'tools/list' })
  assert.ok(list.result.tools.some((t) => t.name === 'echo'))
}
const init = await rpc('initialize', { protocolVersion: legacy, capabilities: {}, clientInfo })
if (profile === 'modern') {
  if (sdk === 'go') {
    assert.equal(init.status, 200)
    assert.equal(init.result.protocolVersion, '2025-11-25')
  } else if (sdk === 'typescript') {
    assert.equal(init.status, 400)
    assert.equal(init.error.code, -32022)
    assert.equal(init.error.data.requested, legacy)
    assert.deepEqual(init.error.data.supported, [modern])
  } else {
    assert.equal(sdk, 'fastmcp')
    assert.equal(init.status, 400)
    assert.equal(init.body, 'Modern revision required')
  }
} else {
  assert.equal(init.status, 200)
  assert.equal(init.result.protocolVersion, legacy)
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
