import { Client } from '@modelcontextprotocol/sdk/client/index.js'
import { StreamableHTTPClientTransport } from '@modelcontextprotocol/sdk/client/streamableHttp.js'

export interface VerificationResult {
  server: string
  lists: {kind:string; count:number; more:boolean}[]
}
/** Explicit, bounded discovery only. No tool calls, resource reads, or prompt execution. */
export async function verifyConnection(endpoint: string, credential: string, signal: AbortSignal): Promise<VerificationResult> {
  const url = new URL(endpoint)
  if (!['http:','https:'].includes(url.protocol) || url.username || url.password) throw new Error('Invalid gateway URL')
  const client = new Client({name:'vmcp-ui-verification',version:'1.0.0'},{capabilities:{}})
  const transport = new StreamableHTTPClientTransport(url, {
    fetch: (url, init) => {
      const combined = AbortSignal.any([signal,...(init?.signal?[init.signal]:[])])
      combined.throwIfAborted()
      return fetch(url, {...init, credentials:'omit',redirect:'error',signal:combined})
    },
    requestInit:{credentials:'omit',redirect:'error',signal,headers:credential ? {Authorization:`Bearer ${credential}`} : {}},
  })
  const options = {signal, timeout:10_000}
  const lists: VerificationResult['lists'] = []
  try {
    await client.connect(transport, options)
    await client.ping(options)
    const capabilities = client.getServerCapabilities()
    if (capabilities?.tools) {
      const result = await client.listTools({},options)
      lists.push({kind:'tools',count:result.tools.length,more:!!result.nextCursor})
    }
    if (capabilities?.resources) {
      const resources = await client.listResources({},options)
      lists.push({kind:'resources',count:resources.resources.length,more:!!resources.nextCursor})
      const templates = await client.listResourceTemplates({},options)
      lists.push({kind:'templates',count:templates.resourceTemplates.length,more:!!templates.nextCursor})
    }
    if (capabilities?.prompts) {
      const result = await client.listPrompts({},options)
      lists.push({kind:'prompts',count:result.prompts.length,more:!!result.nextCursor})
    }
    return {server:client.getServerVersion()?.name ?? 'MCP server',lists}
  } finally {
    try {
      if(transport.sessionId && !signal.aborted) await transport.terminateSession().catch(()=>undefined)
    } finally {
      await client.close().catch(()=>undefined)
    }
  }
}
