import { auditSettings } from './audit-fixtures'
import { expect, test, type Page } from '@playwright/test'
import { server, upstream } from '../src/connections/fixtures'

async function login(page:Page,path:string,headers:string[]=[]){
 const deletes:{path:string;version?:string}[]=[]
 let upstreams=[{...upstream,headerNames:headers}],servers=[{...server}]
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',route=>{
 if(new URL(route.request().url()).pathname.endsWith('/audit-settings'))return route.fulfill({json:auditSettings})

  const request=route.request(),url=new URL(request.url()),path=url.pathname
  if(path.endsWith('/me'))return route.fulfill({status:request.headers().authorization?200:401,json:{name:'Tester',role:'admin'}})
  if(request.method()==='DELETE'){
   deletes.push({path:path+url.search,version:request.headers()['if-match']})
   if(path.includes('/upstreams/')){
    if(!url.searchParams.has('force'))return route.fulfill({status:409,json:{code:'in_use',detail:'Selected by virtual server demo.'}})
    upstreams=[]
   }else servers=[]
   return route.fulfill({status:204})
  }
  if(path.endsWith('/catalog'))return route.fulfill({json:{resources:[],resourceTemplates:[],prompts:[],capabilities:{}}})
  return route.fulfill({json:{items:path.endsWith('/upstreams')?upstreams:path.endsWith('/virtual-servers')?servers:[]}})
 })
 await page.goto(path);await page.getByLabel('Admin token',{exact:true}).fill('test-token');await page.getByRole('button',{name:'Continue',exact:true}).click()
 return deletes
}

test('delete confirmation cancels safely restores focus and preserves versioned deletion',async({page})=>{
 const deletes=await login(page,'/virtual-servers')
 const remove=page.getByRole('button',{name:'Delete Demo',exact:true})
 await remove.click()
 const confirmation=page.getByRole('dialog',{name:'Delete virtual server "demo"?',exact:true})
 await expect(page.getByTestId('cancel-action')).toBeFocused()
 expect(deletes).toEqual([])
 await page.screenshot({path:test.info().outputPath('delete-confirmation.png'),animations:'disabled'})
 await page.keyboard.press('Escape');await expect(confirmation).toHaveCount(0);await expect(remove).toBeFocused()
 await remove.click();await page.setViewportSize({width:320,height:568})
 expect(await confirmation.locator('section').evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath('delete-confirmation-mobile.png'),animations:'disabled'})
 await page.getByTestId('confirm-action').click()
 await expect(page.getByRole('article',{name:'Demo',exact:true})).toHaveCount(0)
 expect(deletes).toEqual([{path:'/api/v1/virtual-servers/demo',version:'"7"'}])
})

test('in-use upstream requires a second explicit force confirmation',async({page})=>{
 const deletes=await login(page,'/upstreams')
 await page.getByRole('button',{name:'Delete Alpha',exact:true}).click();await page.getByTestId('confirm-action').click()
 const force=page.getByRole('dialog',{name:'Delete upstream and remove its selections?',exact:true})
 await expect(force).toBeVisible();await expect(force.getByText('Selected by virtual server demo.',{exact:true})).toBeVisible()
 await page.screenshot({path:test.info().outputPath('upstream-force-confirmation.png'),animations:'disabled'})
 await page.getByTestId('cancel-action').click()
 await expect(page.getByRole('article',{name:'Alpha',exact:true})).toBeVisible()
 expect(deletes).toHaveLength(1)
 await page.getByRole('button',{name:'Delete Alpha',exact:true}).click();await page.getByTestId('confirm-action').click();await expect(force).toBeVisible();await page.getByTestId('confirm-action').click()
 await expect(page.getByRole('article',{name:'Alpha',exact:true})).toHaveCount(0)
 expect(deletes.at(-1)).toEqual({path:'/api/v1/upstreams/alpha?force=true',version:'"2"'})
})

test('discard confirmation preserves modal draft on Escape and explicitly closes on acceptance',async({page})=>{
 await login(page,'/agent-keys')
 await page.getByRole('button',{name:'New key',exact:true}).click()
 const form=page.getByRole('dialog',{name:'New agent key',exact:true})
 await form.getByLabel('Name',{exact:true}).fill('unsaved-agent')
 await page.keyboard.press('Escape')
 const confirmation=page.getByRole('dialog',{name:'Discard unsaved changes?',exact:true})
 await expect(confirmation).toBeVisible();await expect(page.getByTestId('cancel-action')).toBeFocused()
 await page.screenshot({path:test.info().outputPath('discard-confirmation.png'),animations:'disabled'})
 await page.keyboard.press('Escape');await expect(confirmation).toHaveCount(0)
 await expect(form.getByLabel('Name',{exact:true})).toHaveValue('unsaved-agent')
 await page.setViewportSize({width:390,height:568})
 await form.getByRole('button',{name:'Cancel',exact:true}).scrollIntoViewIfNeeded()
 await expect(form.getByRole('heading',{name:'New agent key',exact:true})).toBeInViewport()
 await page.screenshot({path:test.info().outputPath('modal-form-mobile.png'),animations:'disabled'})
 await form.getByRole('button',{name:'Cancel',exact:true}).click();await page.getByTestId('confirm-action').click()
 await expect(form).toHaveCount(0)
})


test('header replacement uses a masked draft modal and does not write before saving',async({page})=>{
 await login(page,'/upstreams/alpha',['X-Workspace'])
 const writes:string[]=[]
 page.on('request',request=>{if(request.method()==='PATCH')writes.push(request.url())})
 await page.getByRole('button',{name:'Edit upstream',exact:true}).click()
 await page.getByRole('button',{name:'Replace',exact:true}).click()
 const replacement=page.getByRole('dialog',{name:'Replace X-Workspace',exact:true})
 await replacement.getByLabel('Replacement header value',{exact:true}).fill('test-replacement')
 await expect(replacement.getByLabel('Replacement header value',{exact:true})).toHaveAttribute('type','password')
 await replacement.getByRole('button',{name:'Use replacement',exact:true}).click()
 await expect(replacement).toHaveCount(0)
 await expect(page.getByText('(new value)',{exact:true})).toBeVisible()
 expect(writes).toEqual([])
 await page.getByRole('button',{name:'Cancel',exact:true}).click();await page.getByTestId('confirm-action').click()
 await expect(page.getByRole('button',{name:'Edit upstream',exact:true})).toBeVisible()
 await expect(page.getByText('(new value)',{exact:true})).toHaveCount(0)
 expect(writes).toEqual([])
})
