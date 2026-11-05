import {afterEach,expect,test,vi} from 'vitest'
import {api,resetConfigurationIdentity,setToken} from './api'
import {configurationPatch} from './configuration-access'
afterEach(()=>{resetConfigurationIdentity();vi.unstubAllGlobals()})
test('denied configuration reads never download the target data',async()=>{
 resetConfigurationIdentity()
 const fetch=vi.fn().mockResolvedValue(new Response(JSON.stringify({allowed:[false]})))
 vi.stubGlobal('fetch',fetch)
 await expect(api.listAgentKeys()).rejects.toMatchObject({code:'configuration_access_denied'})
 expect(fetch).toHaveBeenCalledTimes(1)
 expect(fetch.mock.calls[0][0]).toBe('/api/v1/configuration-access')
 expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({checks:[{method:'GET',path:'/agent-keys?limit=200'}]})
})
test('a newly denied field edit never sends a PATCH',async()=>{
 resetConfigurationIdentity()
 const fetch=vi.fn().mockResolvedValue(new Response(JSON.stringify({allowed:[false]})))
 vi.stubGlobal('fetch',fetch)
 await expect(api.patchUpstream('private',{url:'https://example.com'},4)).rejects.toMatchObject({status:403})
 expect(fetch.mock.calls.map(call=>call[1].method)).toEqual(['POST'])
})
test('allowed reads retain authentication, no-store and cancellation on preflight',async()=>{
 resetConfigurationIdentity();setToken('reader')
 const controller=new AbortController()
 const fetch=vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({allowed:[true]}))).mockResolvedValueOnce(new Response(JSON.stringify({items:[]})))
 vi.stubGlobal('fetch',fetch)
 expect(await api.listUpstreams(controller.signal)).toEqual([])
 expect(fetch.mock.calls[0][1]).toMatchObject({signal:controller.signal,cache:'no-store',headers:{Authorization:'Bearer reader','X-VMCP-CSRF':'1'}})
 expect(fetch.mock.calls[1][0]).toBe('/api/v1/upstreams?limit=200')
})
test('missing positive decisions fail closed',async()=>{
 const fetch=vi.fn().mockResolvedValue(new Response(JSON.stringify({allowed:[]})))
 vi.stubGlobal('fetch',fetch)
 await expect(api.deleteVirtualServer('demo',7)).rejects.toMatchObject({status:403})
 expect(fetch).toHaveBeenCalledTimes(1)
})
test('restricted unchanged fields are omitted; TLS patch excludes unchanged client identity',()=>{
 const baseline={name:'Old',enabled:true,ownerTeam:'payments',tls:{caPem:'old-ca',clientCertPem:'saved-cert'},auth:{type:'none'}}
 expect(configurationPatch({...baseline,name:'New',tls:{...baseline.tls,caPem:'new-ca'}},baseline)).toEqual({name:'New',tls:{caPem:'new-ca'}})
})

test('omitting unchanged client credentials does not produce an empty protected TLS patch',()=>{
 expect(configurationPatch({name:'New',tls:{caPem:'',serverName:''}},{name:'Old',tls:{caPem:'',serverName:'',clientCertPem:''}})).toEqual({name:'New'})
})

test('identity changes during preflight never send the approved operation',async()=>{
 resetConfigurationIdentity();setToken('old-identity')
 let complete!:(value:Response)=>void
 const fetch=vi.fn().mockImplementation(()=>new Promise<Response>(resolve=>{complete=resolve}))
 vi.stubGlobal('fetch',fetch)
 const operation=api.deleteVirtualServer('demo',7)
 setToken('new-identity')
 complete(new Response(JSON.stringify({allowed:[true]})))
 await expect(operation).rejects.toMatchObject({code:'configuration_identity_changed'})
 expect(fetch).toHaveBeenCalledTimes(1)
})

test.each([
 ['runtime_access_denied',true,0],
 ['forbidden',true,1],
 ['runtime_access_denied',false,1],
] as const)('403 %s on Omni=%s preserves only the independent runtime boundary',async(code,omni,expected)=>{
 const denied=vi.fn()
 window.addEventListener('vmcp:configuration-denied',denied)
 try {
  const fetch=vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({allowed:[true]}))).mockResolvedValueOnce(new Response(JSON.stringify({code,detail:'Denied'}),{status:403}))
  vi.stubGlobal('fetch',fetch)
  await expect(omni?api.omniPreview('runtime-key','query'):api.getUpstream('private')).rejects.toMatchObject({status:403,code})
  expect(fetch).toHaveBeenCalledTimes(2)
  expect(denied).toHaveBeenCalledTimes(expected)
 } finally {window.removeEventListener('vmcp:configuration-denied',denied)}
})
