import { auditSettings } from './audit-fixtures'
import { withConfirmation } from './confirmation'
import { expect, test, type Page } from '@playwright/test'
import { key, binding, server } from '../src/connections/fixtures'

async function login(page:Page,role:'admin'|'viewer'='admin'){
 let keys=[{...key,description:'Support agent credential',expiresAt:'2027-02-03T04:05:06.789Z'},{...key,slug:'expired-key',description:'Expired credential',expiresAt:'2020-01-01T00:00:00Z'}]
 let bindings=[{...binding,description:'Team agents with verified claims'},{...binding,slug:'disabled-binding',enabled:false}]
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',route=>{
 if(new URL(route.request().url()).pathname.endsWith('/audit-settings'))return route.fulfill({json:auditSettings})

  const request=route.request(),path=new URL(request.url()).pathname
  if(path.endsWith('/me'))return route.fulfill({status:request.headers().authorization?200:401,json:{name:'Tester',role}})
  if(request.method()==='PATCH' && path.endsWith('/agent-keys/team-key')){
   expect(request.headers()['if-match']).toBe(`"${keys[0].version}"`)
   const changes=request.postDataJSON();expect(changes.expiresAt).toBe('2027-02-03T04:05:06.789Z')
   keys=[{...keys[0],...changes,version:keys[0].version+1},keys[1]]
   return route.fulfill({json:keys[0]})
  }
  if(request.method()==='PATCH' && path.endsWith('/identity-bindings/team-jwt')){
   expect(request.headers()['if-match']).toBe(`"${bindings[0].version}"`)
   const changes=request.postDataJSON()
   expect(changes.conditions[0]).toEqual({claim:'azp',op:'exists'})
   bindings=[{...bindings[0],...changes,version:bindings[0].version+1},bindings[1]]
   return route.fulfill({json:bindings[0]})
  }
  if(path.endsWith('/team-key/rotate'))return route.fulfill({json:{...keys[0],key:'rotated-test-secret'}})
  if(path.endsWith('/identity-bindings/evaluate'))return route.fulfill({json:{decision:{allowed:true,principal:'team-jwt'},matches:[{slug:'team-jwt',virtualServers:['demo']}]}})
  if(path.endsWith('/agent-keys'))return route.fulfill({json:{items:keys}})
  if(path.endsWith('/identity-bindings'))return route.fulfill({json:{items:bindings}})
  if(path.endsWith('/virtual-servers'))return route.fulfill({json:{items:[server]}})
  return route.fulfill({json:{items:[]}})
 })
 await page.goto('/agent-keys');await page.getByLabel('Admin token').fill('test-token');await page.getByRole('button',{name:'Continue',exact:true}).click()
 await expect(page.getByRole('button',{name:'team-key',exact:true})).toBeVisible()
}

test('agent key list inspection and edit are responsive and preserve exact expiry',async({page})=>{
 await login(page)
 await page.screenshot({path:test.info().outputPath('agent-keys-list.png'),animations:'disabled'})
 await page.getByLabel('Agent key status').selectOption('expired')
 await expect(page.getByRole('button',{name:'expired-key',exact:true})).toBeVisible()
 await expect(page.getByRole('button',{name:'team-key',exact:true})).toHaveCount(0)
 await page.getByLabel('Agent key status').selectOption('')
 await page.getByRole('button',{name:'team-key',exact:true}).click()
 let dialog=page.getByRole('dialog',{name:'team-key',exact:true})
 await expect(dialog.getByRole('button',{name:'Save',exact:true})).toHaveCount(0)
 await expect(dialog.getByRole('region',{name:'Virtual server access'})).toBeVisible()
 await page.screenshot({path:test.info().outputPath('agent-key-detail.png'),animations:'disabled'})
 await dialog.getByRole('button',{name:'Edit key'}).click()
 dialog=page.getByRole('dialog',{name:'Edit team-key',exact:true})
 await dialog.getByLabel('Description',{exact:true}).fill('Updated key purpose')
 await page.screenshot({path:test.info().outputPath('agent-key-edit.png'),animations:'disabled'})
 await page.setViewportSize({width:390,height:844})
 expect(await dialog.locator('section').first().evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath('agent-key-edit-mobile.png'),animations:'disabled'})
 await dialog.getByRole('button',{name:'Save',exact:true}).click()
 await expect(page.getByRole('dialog')).toHaveCount(0)
 await expect(page.getByText('Updated key purpose',{exact:true})).toBeVisible()
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
})

test('identity list inspection editing and claim evaluation preserve semantics',async({page})=>{
 await login(page)
 await page.getByRole('link',{name:'Identities',exact:true}).click()
 await expect(page.getByRole('button',{name:'team-jwt',exact:true})).toBeVisible()
 await page.screenshot({path:test.info().outputPath('identities-list.png'),animations:'disabled'})
 await page.getByLabel('Identity binding status').selectOption('disabled')
 await expect(page.getByRole('button',{name:'team-jwt',exact:true})).toHaveCount(0)
 await page.getByLabel('Identity binding status').selectOption('')
 await page.getByRole('button',{name:'team-jwt',exact:true}).click()
 let dialog=page.getByRole('dialog',{name:'team-jwt',exact:true})
 await expect(dialog.getByRole('region',{name:'Claim conditions'})).toBeVisible()
 await page.screenshot({path:test.info().outputPath('identity-detail.png'),animations:'disabled'})
 await dialog.getByRole('button',{name:'Edit binding'}).click()
 dialog=page.getByRole('dialog',{name:'Edit team-jwt',exact:true})
 await dialog.getByLabel('Operator 1').selectOption('exists')
 await expect(dialog.getByLabel('Value 1')).toBeDisabled()
 await page.screenshot({path:test.info().outputPath('identity-edit.png'),animations:'disabled'})
 await page.setViewportSize({width:390,height:844})
 expect(await dialog.locator('section').first().evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath('identity-edit-mobile.png'),animations:'disabled'})
 await dialog.getByRole('button',{name:'Save',exact:true}).click()
 await expect(dialog).toHaveCount(0)
 await page.getByRole('button',{name:'Evaluate claims',exact:true}).click()
 await page.getByLabel('Token claims (JSON)').fill('{"azp":"team"}')
 await page.getByRole('button',{name:'Evaluate',exact:true}).click()
 await expect(page.getByText('Allowed as team-jwt',{exact:true})).toBeVisible()
 await page.getByLabel('Token claims (JSON)').fill('{"azp":"other"}')
 await expect(page.getByText('Allowed as team-jwt',{exact:true})).toHaveCount(0)
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
})

test('viewer inspection is available without edit or mutation permissions',async({page})=>{
 await login(page,'viewer')
 await expect(page.getByRole('button',{name:'New key',exact:true})).toBeDisabled()
 await page.getByRole('button',{name:'team-key',exact:true}).click()
 let dialog=page.getByRole('dialog',{name:'team-key',exact:true})
 await expect(dialog.getByRole('button',{name:'Edit key'})).toHaveCount(0)
 await dialog.getByRole('button',{name:'Close',exact:true}).click()
 await page.getByRole('link',{name:'Identities',exact:true}).click()
 await expect(page.getByRole('button',{name:'New binding',exact:true})).toBeDisabled()
 await page.getByRole('button',{name:'team-jwt',exact:true}).click()
 dialog=page.getByRole('dialog',{name:'team-jwt',exact:true})
 await expect(dialog.getByRole('button',{name:'Edit binding'})).toHaveCount(0)
 await expect(dialog.getByRole('region',{name:'Claim conditions'})).toBeVisible()
})

test('rotation still requires confirmation and acknowledgement of its one-time secret',async({page})=>{
 await login(page)
 await withConfirmation(page,true,()=>page.getByRole('row').filter({has:page.getByRole('button',{name:'team-key',exact:true})}).getByRole('button',{name:'Rotate',exact:true}).click())
 const secret=page.getByRole('dialog',{name:'Key rotated',exact:true})
 await expect(secret.getByText('rotated-test-secret',{exact:true})).toBeVisible()
 await page.keyboard.press('Escape')
 await expect(secret).toBeVisible()
 await secret.getByRole('button',{name:'I have saved it',exact:true}).click()
 await expect(secret).toHaveCount(0)
 await expect(page.getByText('rotated-test-secret',{exact:true})).toHaveCount(0)
})
