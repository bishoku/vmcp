import { auditSettings } from './audit-fixtures'
import { withConfirmation } from './confirmation'
import { expect, test } from '@playwright/test'
import { server as originalServer, upstream as originalUpstream } from '../src/connections/fixtures'

test.beforeEach(async({page})=>{
  let server = {...originalServer}
  let upstream = {...originalUpstream}
  await page.route('**/auth/config',route=>route.fulfill({json:{oidc:false}}))
  await page.route('**/api/v1/**',route=>{
 if(new URL(route.request().url()).pathname.endsWith('/audit-settings'))return route.fulfill({json:auditSettings})

    const request=route.request(), path=new URL(request.url()).pathname
    if(path.endsWith('/me'))return route.fulfill({status:request.headers().authorization?200:401,json:{name:'Admin',role:'admin'}})
    if(request.method()==='PUT'){
      expect(request.headers()['if-match']).toBe('"7"')
      server={...server,...request.postDataJSON(),version:8}
      return route.fulfill({json:server})
    }
    if(request.method()==='PATCH'){
      expect(request.headers()['if-match']).toBe('"2"')
      upstream={...upstream,...request.postDataJSON(),version:3}
      return route.fulfill({json:upstream})
    }
    if(path.endsWith('/virtual-servers'))return route.fulfill({json:{items:[server]}})
    if(path.endsWith('/upstreams'))return route.fulfill({json:{items:[upstream]}})
    if(path.endsWith('/catalog'))return route.fulfill({json:{resources:[],resourceTemplates:[],prompts:[],capabilities:{}}})
    return route.fulfill({json:{items:[]}})
  })
  await page.goto('/');await page.getByLabel('Admin token').fill('test-token');await page.getByRole('button',{name:'Continue',exact:true}).click()
})

test('virtual server card opens read-only details; Save and Cancel return to details',async({page})=>{
  await expect(page.getByRole('article',{name:'Demo',exact:true})).toBeVisible()
  await page.getByRole('link',{name:'Demo',exact:true}).click()
  await expect(page).toHaveURL(/virtual-servers\/demo\/overview$/)
  await expect(page.getByRole('textbox',{name:'Name',exact:true})).toHaveCount(0)
  await page.getByRole('link',{name:'Edit configuration',exact:true}).click()
  await expect(page).toHaveURL(/virtual-servers\/demo\/edit$/)
  await page.getByLabel('Name',{exact:true}).fill('Updated server')
  await page.getByRole('button',{name:'Save',exact:true}).click()
  await expect(page.getByRole('heading',{name:'Updated server',exact:true})).toBeVisible()
  await expect(page).toHaveURL(/virtual-servers\/demo\/overview$/)
  await page.getByRole('link',{name:'Edit configuration',exact:true}).click()
  await page.getByLabel('Name',{exact:true}).fill('Discarded')
  await withConfirmation(page,true,()=>page.getByRole('button',{name:'Cancel',exact:true}).click())
  await expect(page.getByRole('heading',{name:'Updated server',exact:true})).toBeVisible()
})

test('upstream opens read-only; Cancel restores saved state and Save exits edit mode',async({page})=>{
  await page.getByRole('link',{name:'Upstreams',exact:true}).click()
  await page.getByRole('link',{name:'Alpha',exact:true}).click()
  await expect(page).toHaveURL(/upstreams\/alpha$/)
  await expect(page.getByLabel('Name',{exact:true})).toBeDisabled()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await page.getByRole('button',{name:'Edit upstream'}).click()
  await page.getByLabel('Name',{exact:true}).fill('Discarded')
  await withConfirmation(page,true,()=>page.getByRole('button',{name:'Cancel',exact:true}).click())
  await expect(page.getByLabel('Name',{exact:true})).toHaveValue('Alpha')
  await expect(page.getByLabel('Name',{exact:true})).toBeDisabled()
  await page.getByRole('button',{name:'Edit upstream'}).click()
  await page.getByLabel('Name',{exact:true}).fill('Updated upstream')
  await page.getByRole('button',{name:'Save',exact:true}).click()
  await expect(page.getByRole('heading',{name:'Updated upstream',exact:true})).toBeVisible()
  await expect(page.getByLabel('Name',{exact:true})).toBeDisabled()
})

test('legacy inspection URLs open dedicated read-only details',async({page})=>{
  await page.goto('/upstreams?inspect=alpha')
  await page.getByLabel('Admin token').fill('test-token')
  await page.getByRole('button',{name:'Continue',exact:true}).click()
  await expect(page).toHaveURL(/upstreams\/alpha$/)
  await expect(page.getByLabel('Name',{exact:true})).toBeDisabled()
})


test('upstream detail panels remain responsive and discovery reads never trigger connection tests',async({page})=>{
 let storedTests=0
 await page.route('**/api/v1/upstreams/alpha/tools*',route=>route.fulfill({json:{items:[{name:'lookup',description:'Look up a record'}]}}))
 await page.route('**/api/v1/upstreams/alpha/catalog',route=>route.fulfill({json:{resources:[{uri:'doc://team',description:'Team docs'}],resourceTemplates:[],prompts:[{name:'review'}],capabilities:{}}}))
 await page.route('**/api/v1/upstreams/alpha/connection-test',route=>{storedTests++;return route.fulfill({json:{ok:true,toolCount:1}})})
 await page.getByRole('link',{name:'Upstreams',exact:true}).click()
 await page.getByRole('link',{name:'Alpha',exact:true}).click()
 await expect(page.getByRole('region',{name:'Connection settings'})).toBeVisible()
 await expect(page.getByRole('region',{name:'Connection verification'})).toBeVisible()
 await expect(page.getByRole('region',{name:'Recorded node health'})).toBeVisible()
 await expect(page.getByRole('table',{name:'Stored upstream capabilities'}).getByText('doc://team',{exact:true})).toBeVisible()
 expect(storedTests).toBe(0)
 await page.getByRole('heading',{name:'Alpha',exact:true}).scrollIntoViewIfNeeded()
 await page.screenshot({path:test.info().outputPath('upstream-overview.png'),fullPage:false,animations:'disabled'})
 await page.getByRole('button',{name:'Test stored credentials'}).click()
 await expect(page.getByText('Connected · 1 tools',{exact:true})).toBeVisible()
 expect(storedTests).toBe(1)
 await page.getByRole('radio',{name:'Resources (1)',exact:true}).click()
 const table=page.getByRole('table',{name:'Stored upstream capabilities'})
 await expect(table.getByText('lookup',{exact:true})).toHaveCount(0)
 await expect(table.getByText('doc://team',{exact:true})).toBeVisible()
 await page.getByRole('heading',{name:'Discovered capabilities',exact:true}).scrollIntoViewIfNeeded()
 await page.screenshot({path:test.info().outputPath('upstream-discovery.png'),fullPage:false,animations:'disabled'})
 await page.setViewportSize({width:390,height:844})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 await page.getByRole('heading',{name:'Alpha',exact:true}).scrollIntoViewIfNeeded()
 await page.screenshot({path:test.info().outputPath('upstream-mobile.png'),fullPage:false,animations:'disabled'})
 await page.getByRole('button',{name:'Edit upstream'}).click()
 await page.getByLabel('Name',{exact:true}).fill('Draft upstream')
 await page.getByText('TLS (custom CA, server name, mTLS)',{exact:true}).click()
 await expect(page.getByLabel('Upload CA PEM')).toBeVisible()
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath('upstream-edit-mobile.png'),fullPage:false,animations:'disabled'})
 await withConfirmation(page,true,()=>page.getByRole('button',{name:'Cancel',exact:true}).click())
 await expect(page.getByLabel('Name',{exact:true})).toHaveValue('Alpha')
})

test('capability descriptions can be inspected without editing the virtual server',async({page})=>{
 const text='Original description\n  source detail\n'+'Long source text\n'.repeat(120)
 const server={...originalServer,tools:[{...originalServer.tools[0],description:'Override\n  agent instruction',upstreamDescription:text,descriptionDrift:true}]}
 await page.route('**/api/v1/virtual-servers*',route=>route.fulfill({json:{items:[server]}}))
 await page.goto('/virtual-servers/demo/capabilities')
 await page.getByLabel('Admin token').fill('test-token')
 await page.getByRole('button',{name:'Continue',exact:true}).click()
 await page.getByRole('button',{name:'Inspect description for alpha/lookup'}).click()
 const dialog=page.getByRole('dialog',{name:'Capability description'})
 await expect(dialog.getByRole('region',{name:'Agent description'})).toContainText('Override')
 await expect(dialog.getByRole('textbox')).toHaveCount(0)
 await expect(dialog.getByRole('button',{name:'Apply to draft'})).toHaveCount(0)
 await dialog.getByRole('button',{name:'Upstream',exact:true}).click()
 await expect(dialog.getByRole('region',{name:'Upstream description'})).toContainText('source detail')
 await page.setViewportSize({width:390,height:844})
 await expect(dialog.getByRole('button',{name:'Close',exact:true}).last()).toBeInViewport()
 expect(await dialog.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
 await dialog.getByRole('button',{name:'Close',exact:true}).last().click()
 await expect(dialog).not.toBeVisible()
})
