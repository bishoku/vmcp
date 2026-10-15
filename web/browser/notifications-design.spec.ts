import { expect, test, type Page } from '@playwright/test'
import { upstream } from '../src/connections/fixtures'

test.use({viewport:{width:1280,height:900}})
async function setup(page:Page,fail=false){
 let saved={...upstream}
 await page.route('**/api/v1/**',route=>{
  const req=route.request(),path=new URL(req.url()).pathname
  if(path.endsWith('/me'))return route.fulfill({json:{name:'Tester',role:'admin'}})
  if(req.method()==='PATCH'){
   if(fail)return route.fulfill({status:412,json:{code:'precondition_failed',detail:'stale'}})
   saved={...saved,...req.postDataJSON(),version:saved.version+1}
   return route.fulfill({json:saved})
  }
  if(path.endsWith('/admin-tokens')&&req.method()==='POST')return route.fulfill({json:{slug:'release-agent',token:'private-test-secret',version:1}})
  if(path.endsWith('/info'))return route.fulfill({json:{version:'dev-test',configRevision:1}})
  if(path.endsWith('/catalog'))return route.fulfill({json:{resources:[],resourceTemplates:[],prompts:[],capabilities:{}}})
  if(path.endsWith('/upstreams'))return route.fulfill({json:{items:[saved]}})
  return route.fulfill({json:{items:[]}})
 })
}

test('save notification survives remount and navigation without stealing focus',async({page})=>{
 await setup(page);await page.goto('/upstreams/alpha')
 await page.getByRole('button',{name:'Edit upstream',exact:true}).click()
 await page.getByLabel('Name',{exact:true}).fill('Alpha workspace')
 await page.getByRole('button',{name:'Save',exact:true}).click()
 const toast=page.getByTestId('toast')
 await expect(toast).toContainText('Upstream saved');await expect(toast).toContainText('Alpha workspace')
 await page.getByRole('link',{name:'Virtual Servers',exact:true}).click()
 await expect(page).toHaveURL(/\/virtual-servers$/)
 await expect(page.getByRole('heading',{name:'Virtual Servers',exact:true})).toBeVisible()
 await expect(toast).toBeVisible()
 await page.screenshot({path:test.info().outputPath('notification-saved.png'),animations:'disabled'})
 await toast.getByRole('button',{name:'Dismiss notification: Upstream saved'}).click()
 await expect(toast).toHaveCount(0)
})

test('copy toast remains interactive in native modal, keeps secrets private and fits mobile',async({page})=>{
 await setup(page)
 await page.addInitScript(()=>Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async()=>{}}}))
 await page.goto('/settings')
 await page.getByRole('button',{name:'New token',exact:true}).click()
 const form=page.getByRole('dialog',{name:'New admin token',exact:true})
 await form.getByLabel('Name',{exact:true}).fill('release-agent');await form.getByRole('button',{name:'Create',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'Admin token created',exact:true})
 await expect(dialog.getByTestId('toast')).toContainText('Admin token created')
 await dialog.getByRole('button',{name:'Copy',exact:true}).click()
 const copy=dialog.getByTestId('toast').filter({hasText:'Copied to clipboard'})
 await expect(copy).toBeVisible();await expect(copy).not.toContainText('private-test-secret')
 await expect(dialog.getByRole('button',{name:'Copy',exact:true})).toBeFocused()
 await copy.getByRole('button').click();await expect(copy).toHaveCount(0)
 await dialog.getByRole('button',{name:'I have saved it',exact:true}).click()
 await expect(page.getByTestId('toast')).toContainText('Admin token created')
 await page.setViewportSize({width:390,height:844})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath('notification-mobile.png'),animations:'disabled'})
})

test('failed save and failed copy never produce a success notification',async({page})=>{
 await setup(page,true)
 await page.addInitScript(()=>Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async()=>{throw new Error('denied')}}}))
 await page.goto('/upstreams/alpha')
 await page.getByRole('button',{name:'Edit upstream',exact:true}).click()
 await page.getByLabel('Name',{exact:true}).fill('Keep this draft')
 await page.getByRole('button',{name:'Save',exact:true}).click()
 await expect(page.getByRole('alert')).toContainText('another session')
 await expect(page.getByTestId('toast')).toHaveCount(0)
 await expect(page.getByLabel('Name',{exact:true})).toHaveValue('Keep this draft')
 await page.goto('/audit')
 await page.getByRole('button',{name:'Copy audit filter link'}).click()
 await expect(page.getByRole('alert')).toContainText('copy it manually')
 await expect(page.getByTestId('toast')).toHaveCount(0)
})
