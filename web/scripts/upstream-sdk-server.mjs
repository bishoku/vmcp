import { createServer } from 'node:http'
import { writeFileSync, appendFileSync } from 'node:fs'
import { McpServer, createMcpHandler } from '@modelcontextprotocol/server'
import { toNodeHandler } from '@modelcontextprotocol/node'
import { McpServer as LegacyServer } from '@modelcontextprotocol/sdk/server/mcp.js'
import { StreamableHTTPServerTransport } from '@modelcontextprotocol/sdk/server/streamableHttp.js'
import { z } from 'zod'
const [profile, ready] = process.argv.slice(2)
const info = { name: 'interop-typescript', version: '1' }
const tool = { description: 'Echo a supplied value', inputSchema: z.object({ value: z.string() }) }
const echo = async ({ value }) => {
  appendFileSync(process.env.VMCP_EXECUTIONS, value + '\n')
  return { content: [{ type: 'text', text: value }] }
}
const modern = () => {
  const s = new McpServer(info)
  s.registerTool('echo', tool, echo)
  return s
}
async function legacy(req, res) {
  const s = new LegacyServer(info)
  s.registerTool('echo', tool, echo)
  const transport = new StreamableHTTPServerTransport({
    sessionIdGenerator: undefined,
    enableJsonResponse: true,
  })
  res.on('close', () => s.close())
  await s.connect(transport)
  await transport.handleRequest(req, res)
}
const handler =
  profile === 'legacy'
    ? legacy
    : toNodeHandler(
        createMcpHandler(modern, {
          legacy: profile === 'modern' ? 'reject' : 'stateless',
          responseMode: 'json',
        }),
      )
const server = createServer(handler)
server.listen(0, '127.0.0.1', () =>
  writeFileSync(ready, `http://127.0.0.1:${server.address().port}/mcp`),
)
