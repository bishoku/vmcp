import { afterEach, expect, test, vi } from 'vitest'
import { api, setToken } from './api'
afterEach(()=>{vi.unstubAllGlobals();localStorage.clear()})
test('management pagination traverses beyond 200 and preserves cursor',async()=>{
 const fetch=vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({items:Array.from({length:200},(_,i)=>({slug:`u${i}`})),nextCursor:'next cursor'}))).mockResolvedValueOnce(new Response(JSON.stringify({items:[{slug:'last'}],nextCursor:null})))
 vi.stubGlobal('fetch',fetch)
 expect(await api.listUpstreams()).toHaveLength(201)
 expect(fetch.mock.calls[1][0]).toContain('cursor=next+cursor')
})
test('repeated pagination cursors fail instead of looping',async()=>{
 vi.stubGlobal('fetch',vi.fn().mockImplementation(()=>Promise.resolve(new Response(JSON.stringify({items:[],nextCursor:'same'})))))
 await expect(api.listAgentKeys()).rejects.toThrow('repeated')
})
test('writes carry original version and cookie CSRF header; stale edits fail',async()=>{
 const fetch=vi.fn().mockResolvedValue(new Response(JSON.stringify({detail:'conflict'}),{status:412}))
 vi.stubGlobal('fetch',fetch)
 await expect(api.patchUpstream('u',{name:'changed'},7)).rejects.toThrow('another session')
 expect(fetch.mock.calls[0][1].headers).toMatchObject({'If-Match':'"7"','X-VMCP-CSRF':'1'})
})
test('token authentication is explicit and stored credential test never submits credentials',async()=>{
 const fetch=vi.fn().mockResolvedValue(new Response('{"ok":true}'));vi.stubGlobal('fetch',fetch);setToken('admin-token')
 await api.testStoredConnection('upstream')
 expect(fetch.mock.calls[0][1].headers.Authorization).toBe('Bearer admin-token')
 expect(fetch.mock.calls[0][1].body).toBeUndefined()
})
