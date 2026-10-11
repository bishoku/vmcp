import { afterEach, expect, test, vi } from 'vitest'
import { setToken, clearToken } from '../api'
import { verifyConnection } from './verify'
afterEach(()=>{vi.unstubAllGlobals();clearToken()})

test('real SDK verification initializes, pings and lists negotiated catalogs without executing objects',async()=>{
 const methods:string[]=[]
 const fetch=vi.fn(async (_url:unknown,init?:RequestInit)=>{
  const headers=new Headers(init?.headers)
  expect(headers.get('Authorization')).toBe('Bearer agent-secret')
  expect(init?.credentials).toBe('omit');expect(init?.redirect).toBe('error')
  if(init?.method==='GET')return new Response('',{status:405})
  if(init?.method==='DELETE')return new Response(null,{status:204})
  const message=JSON.parse(String(init?.body));methods.push(message.method)
  if(message.method==='notifications/initialized')return new Response(null,{status:202})
  const results:Record<string,unknown>={initialize:{protocolVersion:'2025-06-18',capabilities:{tools:{},resources:{},prompts:{}},serverInfo:{name:'vMCP',version:'test'}},
   ping:{},'tools/list':{tools:[{name:'lookup',inputSchema:{type:'object'}}],nextCursor:'more'},'resources/list':{resources:[{name:'guide',uri:'doc://guide'}]},'resources/templates/list':{resourceTemplates:[]},'prompts/list':{prompts:[{name:'review'}]}}
  return new Response(JSON.stringify({jsonrpc:'2.0',id:message.id,result:results[message.method]}),{headers:{'Content-Type':'application/json','Mcp-Session-Id':'test-session'}})
 })
 setToken('admin-secret');vi.stubGlobal('fetch',fetch)
 const result=await verifyConnection('https://gateway.example/mcp','agent-secret',new AbortController().signal)
 expect(result.server).toBe('vMCP');expect(result.lists).toEqual([{kind:'tools',count:1,more:true},{kind:'resources',count:1,more:false},{kind:'templates',count:0,more:false},{kind:'prompts',count:1,more:false}])
 expect(methods).toEqual(['initialize','notifications/initialized','ping','tools/list','resources/list','resources/templates/list','prompts/list'])
 expect(fetch.mock.calls.some(([,init])=>init?.method==='DELETE')).toBe(true)
})
test('separate invalid gateway credentials do not expire the admin session or retry a mutation',async()=>{
 const expired=vi.fn();window.addEventListener('vmcp:session-expired',expired)
 const fetch=vi.fn().mockResolvedValue(new Response('Unauthorized',{status:401}))
 vi.stubGlobal('fetch',fetch)
 try{await expect(verifyConnection('https://gateway.example/mcp','bad-key',new AbortController().signal)).rejects.toThrow();expect(expired).not.toHaveBeenCalled();expect(fetch).toHaveBeenCalledTimes(1)}finally{window.removeEventListener('vmcp:session-expired',expired)}
})
test('verification honors cancellation and rejects non-HTTP endpoints before sending credentials',async()=>{
 const fetch=vi.fn();vi.stubGlobal('fetch',fetch)
 await expect(verifyConnection('file:///secret','key',new AbortController().signal)).rejects.toThrow('Invalid gateway')
 expect(fetch).not.toHaveBeenCalled()
 const control=new AbortController()
 fetch.mockImplementation(async(_url,init)=>{control.abort();expect(init.signal.aborted).toBe(true);throw new DOMException('Aborted','AbortError')})
 await expect(verifyConnection('https://gateway.example/mcp','key',control.signal)).rejects.toThrow()
 expect(fetch).toHaveBeenCalledTimes(1)
})
test('unadvertised primitive families are not probed',async()=>{
 const methods:string[]=[]
 vi.stubGlobal('fetch',vi.fn(async(_url,init)=>{
  if(init.method==='GET')return new Response('',{status:405})
  const message=JSON.parse(init.body);methods.push(message.method)
  if(message.method==='notifications/initialized')return new Response(null,{status:202})
  const result=message.method==='initialize'?{protocolVersion:'2025-06-18',capabilities:{},serverInfo:{name:'Minimal',version:'test'}}:{}
  return new Response(JSON.stringify({jsonrpc:'2.0',id:message.id,result}),{headers:{'Content-Type':'application/json'}})
 }))
 expect((await verifyConnection('https://gateway.example/mcp','',new AbortController().signal)).lists).toEqual([])
 expect(methods).toEqual(['initialize','notifications/initialized','ping'])
})
