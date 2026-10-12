import { expect, test } from '@playwright/test'
import { server as originalServer, upstream as originalUpstream } from '../src/connections/fixtures'

test.beforeEach(async({page})=>{
  let server = {...originalServer}
  let upstream = {...originalUpstream}
  await page.route('**/auth/config',route=>route.fulfill({json:{oidc:false}}))
  await page.route('**/api/v1/**',route=>{
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

test('virtual server table opens read-only details; Save and Cancel return to details',async({page})=>{
  await expect(page.getByRole('table')).toBeVisible()
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
  page.once('dialog',dialog=>dialog.accept())
  await page.getByRole('button',{name:'Cancel',exact:true}).click()
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
  page.once('dialog',dialog=>dialog.accept())
  await page.getByRole('button',{name:'Cancel',exact:true}).click()
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
