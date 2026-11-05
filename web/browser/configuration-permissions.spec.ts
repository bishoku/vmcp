import {expect,test,type Page} from '@playwright/test'
import {upstream,server,key,binding} from '../src/connections/fixtures'

async function routes(page:Page,profile:'metadata'|'credentials'|'bindings'|'create'|'vseditor'='metadata'){
 const dataCalls:{method:string;path:string;body?:Record<string,unknown>}[]=[]
 let revoked=false,claimsExpire:string|undefined
 await page.route('**/auth/config',route=>route.fulfill({json:{oidc:true}}))
 await page.route('**/api/v1/**',route=>{
  const req=route.request(),path=new URL(req.url()).pathname.replace('/api/v1',''),method=req.method(),body=req.postDataJSON()
  if(path==='/me')return route.fulfill({json:{name:'Scoped maintainer',role:'custom',via:'oidc',claimsExpire}})
  if(path==='/configuration-access'){
   const allowed=body.checks.map((check:{method:string;path:string;body?:Record<string,unknown>;permission?:string})=>{
    const target=check.path.split('?')[0]
    if(profile==='vseditor'){
     if(target==='/projects'||target==='/tool-catalog')return !check.permission&&check.method==='GET'
     if(!['/virtual-servers','/virtual-servers/demo'].includes(target))return false
     if(check.permission)return ['virtual_servers.read','virtual_servers.create','virtual_servers.update'].includes(check.permission)
     return check.method==='GET'||check.method==='PATCH'&&Object.keys(check.body??{}).every(key=>['name','description','tools'].includes(key))
    }
    if(profile==='create'){
     if(target==='/teams')return !check.permission&&check.method==='GET'
     if(target!=='/upstreams')return false
     if(check.permission)return check.permission==='upstreams.create'&&(!check.body||check.body.ownerTeam==='payments')||check.permission==='upstreams.connection.update'&&check.body?.ownerTeam==='payments'
     return check.method==='POST'&&check.body?.ownerTeam==='payments'&&Object.keys(check.body).every(key=>['ownerTeam','slug','name','url','fallbackUrls','transport','kind'].includes(key))
    }
    if(profile==='bindings'){
     if(target==='/virtual-servers')return !check.permission&&check.method==='GET'
     if(!['/identity-bindings','/identity-bindings/team-jwt'].includes(target))return false
     if(check.permission)return ['identity_bindings.read','identity_bindings.update'].includes(check.permission)
     return check.method==='GET'||check.method==='PATCH'&&target==='/identity-bindings/team-jwt'&&Object.keys(check.body??{}).every(key=>key==='description')
    }
    if(profile==='credentials'){
     if(target==='/virtual-servers')return !check.permission&&check.method==='GET'
     if(!['/agent-keys','/agent-keys/team-key','/agent-keys/shared'].includes(target))return false
     if(check.permission)return check.permission==='agent_keys.read'||target==='/agent-keys/team-key'&&['agent_keys.update','agent_keys.rotate'].includes(check.permission)
     return check.method==='GET'||check.method==='PATCH'&&target==='/agent-keys/team-key'&&Object.keys(check.body??{}).every(key=>key==='description')
    }
    if(!['/upstreams','/upstreams/alpha','/virtual-servers','/virtual-servers/demo'].includes(target))return false
    if(check.permission)return !revoked&&['upstreams.read','upstreams.update','virtual_servers.read','virtual_servers.update'].includes(check.permission)
    if(check.method==='GET')return true
    return !revoked&&check.method==='PATCH'&&Object.keys(check.body??{}).every(key=>['name','description'].includes(key))
   })
   return route.fulfill({json:{authorizationRevision:revoked?2:1,allowed}})
  }
  dataCalls.push({method,path,body})
  if(profile==='vseditor'&&path==='/projects')return route.fulfill({json:{items:[{slug:'default',name:'Default',ownerTeam:'payments',version:1}]}})
  if(profile==='vseditor'&&path==='/tool-catalog')return route.fulfill({json:{items:[]}})
  if(profile==='create'&&path==='/teams')return route.fulfill({json:{items:[{slug:'payments',name:'Payments',issuer:'https://idp',group:'payments',version:1}]}})
  if(profile==='create'&&path==='/upstreams'&&method==='POST')return route.fulfill({json:{...upstream,...body,version:1}})
  if(profile==='bindings'&&path==='/identity-bindings'&&method==='GET')return route.fulfill({json:{items:[binding]}})
  if(profile==='bindings'&&path==='/identity-bindings/team-jwt'&&method==='PATCH')return route.fulfill({json:{...binding,...body,version:2}})
  if(profile==='credentials'&&path==='/agent-keys'&&method==='GET')return route.fulfill({json:{items:[key,{...key,slug:'shared',virtualServers:['demo','outside']}]}})
  if(profile==='credentials'&&path==='/agent-keys/team-key'&&method==='PATCH')return route.fulfill({json:{...key,...body,version:2}})

  if(path==='/upstreams/alpha'&&method==='GET')return route.fulfill({json:upstream})
  if(path==='/upstreams/alpha'&&method==='PATCH')return route.fulfill({json:{...upstream,...body,version:3}})
  if(path==='/virtual-servers/demo'&&method==='GET')return route.fulfill({json:server})
  if(path==='/virtual-servers/demo'&&method==='PATCH')return route.fulfill({json:{...server,...body,version:8}})
  if(path==='/virtual-servers')return route.fulfill({json:{items:[server]}})
  if(path==='/upstreams')return route.fulfill({json:{items:[upstream]}})
  return route.fulfill({status:403,json:{detail:'Unexpected download'}})
 })
 return {dataCalls,revoke:()=>{revoked=true},expireAt:(deadline:string)=>{claimsExpire=deadline}}
}
test('custom upstream editor sends metadata only and never downloads denied related data',async({page})=>{
 const f=await routes(page)
 await page.goto('/upstreams/alpha')
 await page.getByRole('button',{name:'Edit upstream',exact:true}).click()
 await expect(page.getByLabel('Name',{exact:true})).toBeEnabled()
 await expect(page.getByLabel('URL',{exact:true})).toBeDisabled()
 await page.getByLabel('Name',{exact:true}).fill('Renamed')
 await page.getByRole('button',{name:'Save',exact:true}).click()
 await expect.poll(()=>f.dataCalls.filter(c=>c.method==='PATCH').length).toBe(1)
 expect(f.dataCalls.find(c=>c.method==='PATCH')?.body).toEqual({name:'Renamed'})
 expect(f.dataCalls.filter(c=>c.method==='GET').every(c=>c.path==='/upstreams/alpha')).toBe(true)
})
test('denied direct resource URL never downloads resource inventory or detail',async({page})=>{
 const f=await routes(page);await page.goto('/upstreams/hidden')
 await expect(page.getByText('Your current permissions do not allow this configuration view.')).toBeVisible()
 expect(f.dataCalls).toEqual([])
})
test('revocation after opening a draft blocks the mutation before PATCH',async({page})=>{
 const f=await routes(page);await page.goto('/upstreams/alpha')
 await page.getByRole('button',{name:'Edit upstream',exact:true}).click()
 await page.getByLabel('Name',{exact:true}).fill('Revoked edit');f.revoke()
 await page.getByRole('button',{name:'Save',exact:true}).click()
 await expect(page.getByText('Configuration permissions changed. The view was reloaded; no denied operation was submitted.')).toBeVisible()
 expect(f.dataCalls.filter(c=>c.method!=='GET')).toEqual([])
})
test('virtual server metadata editing cannot change publication or policy and uses PATCH',async({page})=>{
 const f=await routes(page);await page.goto('/virtual-servers/demo/edit')
 await expect(page.getByLabel('Name',{exact:true})).toBeEnabled()
 await expect(page.getByRole('switch',{name:'Enabled',exact:true})).toBeDisabled()
 await expect(page.getByRole('switch',{name:'Require an agent key',exact:true})).toBeDisabled()
 await page.getByLabel('Name',{exact:true}).fill('Scoped demo')
 await page.getByRole('button',{name:'Save',exact:true}).click()
 await expect.poll(()=>f.dataCalls.filter(c=>c.method==='PATCH').length).toBe(1)
 expect(f.dataCalls.find(c=>c.method==='PATCH')?.body).toEqual({name:'Scoped demo'})
 expect(f.dataCalls.some(c=>/agent-keys|identity-bindings|tool-catalog/.test(c.path))).toBe(false)
})

test('credential metadata, rotate and target grants are independent',async({page})=>{
 const f=await routes(page,'credentials');await page.goto('/agent-keys')
 const own=page.getByRole('row').filter({has:page.getByRole('button',{name:'team-key',exact:true})})
 const shared=page.getByRole('row').filter({has:page.getByRole('button',{name:'shared',exact:true})})
 await expect(own.getByRole('button',{name:'Rotate',exact:true})).toBeEnabled()
 await expect(shared.getByRole('button',{name:'Rotate',exact:true})).toBeDisabled()
 await expect(own.getByRole('button',{name:'Delete',exact:true})).toBeDisabled()
 await own.getByRole('button',{name:'team-key',exact:true}).click()
 await page.getByRole('button',{name:'Edit key',exact:true}).click()
 await expect(page.getByLabel('IP allowlist',{exact:true})).toBeDisabled()
 await expect(page.getByRole('checkbox',{name:'Demo',exact:true})).toBeDisabled()
 await page.getByLabel('Description',{exact:true}).fill('Scoped key')
 await page.getByRole('button',{name:'Save',exact:true}).click()
 await expect.poll(()=>f.dataCalls.filter(call=>call.method==='PATCH').length).toBe(1)
 expect(f.dataCalls.find(call=>call.method==='PATCH')?.body).toEqual({description:'Scoped key'})
})
test('scoped create selects an allowed owner and omits forbidden default fields',async({page})=>{
 const f=await routes(page,'create');await page.goto('/upstreams/new')
 await page.getByRole('combobox',{name:'Owner team',exact:true}).selectOption('payments')
 await page.getByLabel('Name',{exact:true}).fill('Payments')
 await page.getByLabel('Slug',{exact:true}).fill('payments')
 await page.getByLabel('URL',{exact:true}).fill('https://payments.example/mcp')
 await page.getByRole('button',{name:'Save',exact:true}).click()
 await expect.poll(()=>f.dataCalls.filter(call=>call.method==='POST').length).toBe(1)
 expect(f.dataCalls.find(call=>call.method==='POST')?.body).toEqual({ownerTeam:'payments',slug:'payments',name:'Payments',url:'https://payments.example/mcp',fallbackUrls:[],transport:'streamable_http',kind:'mcp'})
})

test('claim expiry clears the draft and retains independent read access without an expiry loop',async({page})=>{
 await page.clock.install()
 const f=await routes(page)
 f.expireAt(new Date(Date.now()+60_000).toISOString())
 await page.goto('/upstreams/alpha')
 await page.getByRole('button',{name:'Edit upstream',exact:true}).click()
 await page.getByLabel('Name',{exact:true}).fill('Claim-based draft')
 f.revoke()
 await page.clock.fastForward(61_000)
 await expect(page.getByText('Configuration permissions changed. The view was reloaded; no denied operation was submitted.')).toBeVisible()
 await expect(page.getByLabel('Name',{exact:true})).toHaveValue('Alpha')
 await expect(page.getByRole('button',{name:'Save',exact:true})).toHaveCount(0)
 expect(f.dataCalls.some(call=>call.method!=='GET')).toBe(false)
 const reads=f.dataCalls.length
 await page.clock.fastForward(60_000)
 expect(f.dataCalls.length).toBe(reads)
})

test('identity binding metadata does not grant rule, constraint or target editing',async({page})=>{
 const f=await routes(page,'bindings');await page.goto('/identities')
 await page.getByRole('button',{name:'team-jwt',exact:true}).click()
 await page.getByRole('button',{name:'Edit binding',exact:true}).click()
 await expect(page.getByLabel('Description',{exact:true})).toBeEnabled()
 await expect(page.getByLabel('Claim 1',{exact:true})).toBeDisabled()
 await expect(page.getByLabel('IP allowlist',{exact:true})).toBeDisabled()
 await expect(page.getByRole('switch',{name:'Enabled',exact:true})).toBeDisabled()
 await expect(page.getByRole('checkbox',{name:'Demo',exact:true})).toBeDisabled()
 await page.getByLabel('Description',{exact:true}).fill('Scoped binding')
 await page.getByRole('button',{name:'Save',exact:true}).click()
 await expect.poll(()=>f.dataCalls.filter(call=>call.method==='PATCH').length).toBe(1)
 expect(f.dataCalls.find(call=>call.method==='PATCH')?.body).toEqual({description:'Scoped binding'})
 expect(f.dataCalls.some(call=>call.path==='/identity-bindings/evaluate')).toBe(false)
})

test('an admin label cannot bypass unavailable capability decisions',async({page})=>{
 const f=await routes(page)
 await page.route('**/api/v1/me',route=>route.fulfill({json:{name:'Platform administrator',role:'admin',via:'oidc'}}))
 await page.route('**/api/v1/configuration-access',route=>route.fulfill({status:503,json:{detail:'Authorization unavailable'}}))
 await page.goto('/upstreams/alpha')
 await expect(page.getByText('Your current permissions do not allow this configuration view.')).toBeVisible()
 expect(f.dataCalls).toEqual([])
})


test('virtual server template edits without downloading upstream configuration',async({page})=>{
 const f=await routes(page,'vseditor')
 await page.goto('/virtual-servers/demo/edit')
 await expect(page.getByLabel('Name',{exact:true})).toBeEnabled()
 await expect(page.getByText('Upstreams: You do not have permission for this configuration operation.')).toHaveCount(0)
 await page.getByLabel('Name',{exact:true}).fill('Template editor')
 await page.getByRole('button',{name:'Save',exact:true}).click()
 await expect.poll(()=>f.dataCalls.filter(c=>c.method==='PATCH').length).toBe(1)
 expect(f.dataCalls.some(c=>c.path.startsWith('/upstreams'))).toBe(false)
 expect(f.dataCalls.some(c=>c.path==='/tool-catalog')).toBe(true)
 expect(f.dataCalls.find(c=>c.method==='PATCH')?.body).toEqual({name:'Template editor'})
})
