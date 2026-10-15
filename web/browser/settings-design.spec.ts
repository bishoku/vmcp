import { withConfirmation } from './confirmation'
import { expect, test, type Page } from '@playwright/test'
import type { AdminToken } from '../src/api'

const token:AdminToken={slug:'deploy-bot',version:3,description:'Deployment automation',scope:'read',tokenPrefix:'vmcp_admin_',expiresAt:'2027-05-01T00:00:00Z',revokedAt:null,lastUsedAt:'2026-10-15T09:00:00Z',status:'active',createdAt:'2026-01-01T00:00:00Z'}
const document={apiVersion:'vmcp/v1',upstreams:[],virtualServers:[]}
async function login(page:Page,role:'admin'|'editor'|'viewer'='admin'){
 let tokens:AdminToken[]=[token,{...token,slug:'expired-admin',scope:'admin',expiresAt:'2020-01-01T00:00:00Z',lastUsedAt:null}]
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',route=>{
  const request=route.request(),url=new URL(request.url()),path=url.pathname
  if(path.endsWith('/me'))return route.fulfill({status:request.headers().authorization?200:401,json:{name:'Tester',role}})
  if(path.endsWith('/info'))return route.fulfill({json:{version:'dev-test',configRevision:17,tenantId:'demo-tenant'}})
  if(path.endsWith('/admin-tokens') && request.method()==='POST'){
   expect(role).toBe('admin')
   const body=request.postDataJSON();expect(body.scope).toBe('read')
   const created={...token,...body,version:1,tokenPrefix:'vmcp_new_'}
   tokens=[...tokens,created]
   return route.fulfill({json:{...created,token:'created-admin-test-secret'}})
  }
  if(path.endsWith('/rotate')){
   expect(role).toBe('admin');const slug=path.split('/').at(-2)!,old=tokens.find(t=>t.slug===slug)!
   expect(request.headers()['if-match']).toBe(`"${old.version}"`)
   const rotated={...old,version:old.version+1};tokens=tokens.map(t=>t.slug===slug?rotated:t)
   return route.fulfill({json:{...rotated,token:'rotated-admin-test-secret'}})
  }
  if(path.endsWith('/revoke')){
   expect(role).toBe('admin');const slug=path.split('/').at(-2)!,old=tokens.find(t=>t.slug===slug)!
   expect(request.headers()['if-match']).toBe(`"${old.version}"`)
   const revoked={...old,status:'revoked' as const,revokedAt:'2026-10-15T10:00:00Z',version:old.version+1};tokens=tokens.map(t=>t.slug===slug?revoked:t)
   return route.fulfill({json:revoked})
  }
  if(path.endsWith('/admin-tokens'))return route.fulfill({json:{items:tokens}})
  if(path.endsWith('/export'))return route.fulfill({json:document})
  if(path.endsWith('/apply')){
   expect(role).not.toBe('viewer');expect(request.headers()['if-match']).toBe('"17"')
   expect(url.searchParams.get('prune')).toBe('true')
   return route.fulfill({json:{changes:[{kind:'upstream',slug:'alpha',action:'delete'}],configRevision:17}})
  }
  if(path.endsWith('/openapi.json'))return route.fulfill({json:{openapi:'3.1.0'}})
  return route.fulfill({json:{items:[]}})
 })
 await page.goto('/settings');await page.getByLabel('Admin token').fill('test-token');await page.getByRole('button',{name:'Continue',exact:true}).click()
 await expect(page.getByRole('button',{name:'deploy-bot',exact:true})).toBeVisible()
}

test('settings token filters details and creation share responsive management panels',async({page})=>{
 await login(page)
 await expect(page.getByRole('region',{name:'Gateway information'}).getByText('demo-tenant',{exact:true})).toBeVisible()
 await page.getByRole('heading',{name:'Settings',exact:true}).scrollIntoViewIfNeeded()
 await page.screenshot({path:test.info().outputPath('settings-overview.png'),animations:'disabled'})
 await page.getByLabel('Admin token scope').selectOption('admin')
 await page.getByLabel('Admin token status').selectOption('expired')
 await expect(page.getByRole('button',{name:'expired-admin',exact:true})).toBeVisible()
 await expect(page.getByRole('button',{name:'deploy-bot',exact:true})).toHaveCount(0)
 await page.getByLabel('Admin token scope').selectOption('')
 await page.getByLabel('Admin token status').selectOption('')
 await page.getByRole('region',{name:'Stored tokens'}).evaluate(el=>el.scrollIntoView({block:'center'}))
 await page.screenshot({path:test.info().outputPath('admin-tokens.png'),animations:'disabled'})
 await page.getByRole('button',{name:'deploy-bot',exact:true}).click()
 let detail=page.getByRole('dialog',{name:'deploy-bot',exact:true})
 await expect(detail.getByRole('region',{name:'Token metadata'})).toBeVisible()
 await page.screenshot({path:test.info().outputPath('admin-token-detail.png'),animations:'disabled'})
 await detail.getByRole('button',{name:'Close',exact:true}).click()
 await page.getByRole('button',{name:'New token',exact:true}).click()
 const form=page.getByRole('dialog',{name:'New admin token',exact:true})
 await expect(form.getByRole('combobox',{name:'Scope',exact:true})).toHaveValue('read')
 await form.getByLabel('Name',{exact:true}).fill('release-agent')
 await page.screenshot({path:test.info().outputPath('admin-token-create.png'),animations:'disabled'})
 await page.setViewportSize({width:390,height:844})
 expect(await form.locator('section').first().evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath('admin-token-create-mobile.png'),animations:'disabled'})
 await form.getByRole('button',{name:'Create',exact:true}).click()
 const secret=page.getByRole('dialog',{name:'Admin token created',exact:true})
 await expect(secret.getByText('created-admin-test-secret',{exact:true})).toBeVisible()
 await page.keyboard.press('Escape');await expect(secret).toBeVisible()
 await secret.getByRole('button',{name:'I have saved it',exact:true}).click()
 await expect(page.getByText('created-admin-test-secret',{exact:true})).toHaveCount(0)
 const row=page.getByRole('row').filter({has:page.getByRole('button',{name:'release-agent',exact:true})})
 await withConfirmation(page,true,()=>row.getByRole('button',{name:'Revoke',exact:true}).click())
 await expect(row.getByText('revoked',{exact:true})).toBeVisible()
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
})

test('rotation retains one-time acknowledgement and versioned token mutation',async({page})=>{
 await login(page)
 const row=page.getByRole('row').filter({has:page.getByRole('button',{name:'deploy-bot',exact:true})})
 await withConfirmation(page,true,()=>row.getByRole('button',{name:'Rotate',exact:true}).click())
 const secret=page.getByRole('dialog',{name:'Token rotated',exact:true})
 await expect(secret.getByText('rotated-admin-test-secret',{exact:true})).toBeVisible()
 await page.keyboard.press('Escape');await expect(secret).toBeVisible()
 await secret.getByRole('button',{name:'I have saved it',exact:true}).click()
 await expect(page.getByText('rotated-admin-test-secret',{exact:true})).toHaveCount(0)
})

for(const role of ['viewer','editor'] as const)test(`${role} keeps token management restricted while configuration controls match its role`,async({page})=>{
 await login(page,role)
 await expect(page.getByRole('button',{name:'New token',exact:true})).toHaveCount(0)
 await expect(page.getByRole('button',{name:'Rotate',exact:true})).toHaveCount(0)
 await page.getByRole('button',{name:'deploy-bot',exact:true}).click()
 const detail=page.getByRole('dialog',{name:'deploy-bot',exact:true})
 await expect(detail.getByRole('region',{name:'Token metadata'})).toBeVisible()
 await detail.getByRole('button',{name:'Close',exact:true}).click()
 if(role==='viewer'){
  await expect(page.getByRole('button',{name:'Preview changes',exact:true})).toHaveCount(0)
  await page.getByRole('button',{name:'Export configuration',exact:true}).click()
  await expect(page.getByRole('button',{name:'Copy exported configuration',exact:true})).toBeVisible()
  const download=page.waitForEvent('download')
  await page.getByRole('button',{name:'Download OpenAPI JSON',exact:true}).click()
  expect((await download).suggestedFilename()).toBe('vmcp-openapi.json')
 }else await expect(page.getByRole('button',{name:'Preview changes',exact:true})).toBeDisabled()
 await page.setViewportSize({width:390,height:844})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
})

test('configuration preview preserves revision guards and invalidates on edits',async({page})=>{
 await login(page)
 const source=page.getByRole('textbox',{name:'Configuration document',exact:true})
 await source.fill(JSON.stringify(document,null,2))
 await page.getByRole('checkbox',{name:'Prune configuration absent from document'}).check()
 await page.getByRole('button',{name:'Preview changes',exact:true}).click()
 await expect(page.getByRole('table',{name:'Proposed configuration changes'}).getByText('alpha',{exact:true})).toBeVisible()
 await page.getByRole('region',{name:'Configuration as code'}).evaluate(el=>el.scrollIntoView({block:'start'}))
 await page.screenshot({path:test.info().outputPath('settings-configuration-preview.png'),animations:'disabled'})
 await source.fill(JSON.stringify({...document,bindings:[]}))
 await expect(page.getByRole('button',{name:'Apply preview',exact:true})).toHaveCount(0)
 await page.getByRole('button',{name:'Preview changes',exact:true}).click()
 await expect(page.getByRole('button',{name:'Apply preview',exact:true})).toBeVisible()
 await page.setViewportSize({width:390,height:844})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath('settings-configuration-mobile.png'),animations:'disabled'})
 await withConfirmation(page,true,()=>page.getByRole('button',{name:'Apply preview',exact:true}).click())
 await expect(page.getByRole('button',{name:'Apply preview',exact:true})).toHaveCount(0)
 await expect(source).toHaveValue(JSON.stringify(document,null,2))
})
