import { auditSettings } from './audit-fixtures'
import { withConfirmation } from './confirmation'
import { expect, test, type Page } from '@playwright/test'
async function login(page: Page, role: 'viewer'|'editor'|'admin'='admin') {
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',route=>{
 if(new URL(route.request().url()).pathname.endsWith('/audit-settings'))return route.fulfill({json:auditSettings})

  const request=route.request(),path=new URL(request.url()).pathname
  if(path.endsWith('/me'))return route.fulfill({status:request.headers().authorization?200:401,json:{name:'Tester',role,via:'admin-token'}})
  if(path.endsWith('/info'))return route.fulfill({json:{version:'test',configRevision:1}})
  return route.fulfill({json:{items:[]}})
 })
 await page.goto('/');await page.getByLabel('Admin token').fill('test-token');await page.getByRole('button',{name:'Continue',exact:true}).click()
 await expect(page.getByRole('link',{name:'Agent Keys',exact:true})).toBeVisible()
}
test('dirty navigation can be cancelled, accepted and guarded on browser Back',async({page})=>{
 await login(page)
 await page.getByRole('link',{name:'Agent Keys',exact:true}).click()
 await page.getByRole('button',{name:'New key',exact:true}).click()
 await page.getByLabel('Name',{exact:true}).fill('unsaved')
 await withConfirmation(page,false,()=>page.keyboard.press('Escape'))
 await expect(page.getByLabel('Name',{exact:true})).toHaveValue('unsaved')
 // Browser Back also goes through the router blocker, even while a modal is open.
 await withConfirmation(page,false,()=>page.goBack())
 await expect(page).toHaveURL(/agent-keys/)
 await withConfirmation(page,true,()=>page.goBack())
 await expect(page).toHaveURL(/virtual-servers/)
})
for(const role of ['viewer','editor','admin'] as const) {
 test(`${role} controls reflect API permissions`,async({page})=>{
  await login(page,role)
  if(role==='viewer') {
   await expect(page.getByText('Read-only access.',{exact:false})).toBeVisible()
   await expect(page.getByRole('button',{name:'New virtual server',exact:true})).toBeDisabled()
  } else await expect(page.getByRole('link',{name:'New virtual server',exact:true})).toHaveAttribute('href','/virtual-servers/new')
  await expect(page.getByRole('button',{name:'Refresh list',exact:true})).toBeEnabled()
  await page.getByRole('link',{name:'Settings',exact:true}).click()
  await expect(page.getByRole('button',{name:'New token',exact:true})).toHaveCount(role==='admin'?1:0)
 })
}
test('token sign-in is memory-only',async({page})=>{
 await login(page)
 expect(await page.evaluate(()=>localStorage.getItem('vmcp.adminToken'))).toBeNull()
 await page.reload()
 await expect(page.getByLabel('Admin token')).toBeVisible()
})
test('expired mutation requires sign-in, retains the draft and never replays the write',async({page})=>{
 await login(page)
 let writes=0
 await page.route('**/api/v1/agent-keys',route=>{
  if(route.request().method()==='POST'){writes++;return route.fulfill({status:401,json:{detail:'Session expired'}})}
  return route.fulfill({json:{items:[]}})
 })
 await page.getByRole('link',{name:'Agent Keys',exact:true}).click()
 await page.getByRole('button',{name:'New key',exact:true}).click()
 await page.getByLabel('Name',{exact:true}).fill('retained')
 await page.getByRole('button',{name:'Create',exact:true}).click()
 const expired=page.getByRole('dialog',{name:'Session expired',exact:true})
 await expect(expired).toBeVisible()
 await expired.getByLabel('Admin token').fill('replacement')
 await expired.getByRole('button',{name:'Continue',exact:true}).click()
 await expect(expired).toHaveCount(0)
 await expect(page.getByLabel('Name',{exact:true})).toHaveValue('retained')
 expect(writes).toBe(1)
})
