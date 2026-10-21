import { auditSettings } from './audit-fixtures'
import { expect, test, type Page } from '@playwright/test'
import { server, upstream } from '../src/connections/fixtures'

async function login(page:Page,path:string){
 const upstreams=[{...upstream}]
 const servers=[{...server}]
 const writes:{path:string;body:Record<string,unknown>;version?:string}[]=[]
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',route=>{
 if(new URL(route.request().url()).pathname.endsWith('/audit-settings'))return route.fulfill({json:auditSettings})

  const request=route.request(),path=new URL(request.url()).pathname
  if(path.endsWith('/me'))return route.fulfill({status:request.headers().authorization?200:401,json:{name:'Tester',role:'admin'}})
  if(['PUT','PATCH','POST'].includes(request.method())){
   const body=request.postDataJSON();writes.push({path,body,version:request.headers()['if-match']})
   if(path.endsWith('/upstreams')){
    const created={...upstream,...body,version:1,headerNames:Object.keys(body.headers??{}),toolCount:0,lastRefreshAt:null}
    upstreams.push(created);return route.fulfill({json:created})
   }
   if(path.includes('/virtual-servers/')){
    const slug=path.split('/').at(-1)!,saved={...server,...body,slug,version:1,endpoint:`/mcp/v1/virtual/${slug}`}
    servers.push(saved);return route.fulfill({json:saved})
   }
   return route.fulfill({json:{ok:true}})
  }
  if(path.endsWith('/upstreams'))return route.fulfill({json:{items:upstreams}})
  if(path.endsWith('/virtual-servers'))return route.fulfill({json:{items:servers}})
  if(path.endsWith('/tools'))return route.fulfill({json:{items:[{name:'lookup',description:'Find a customer record',annotations:{readOnlyHint:true}},{name:'search',description:'Search documents'}]}})
  if(path.endsWith('/catalog'))return route.fulfill({json:{resources:[{uri:'doc://guide',description:'Team handbook'}],resourceTemplates:[{uriTemplate:'doc://{name}'}],prompts:[{name:'review'}],capabilities:{}}})
  return route.fulfill({json:{items:[]}})
 })
 await page.goto(path);await page.getByLabel('Admin token').fill('test-token');await page.getByRole('button',{name:'Continue',exact:true}).click()
 return writes
}

test('virtual server creation exposes composed panels and saves explicit selections',async({page})=>{
 const writes=await login(page,'/virtual-servers/new')
 await expect(page.getByRole('heading',{name:'New virtual server',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'Save',exact:true}).click()
 await expect(page.getByLabel('Name',{exact:true})).toHaveAttribute('aria-invalid','true')
 expect(writes).toHaveLength(0)
 await page.getByLabel('Name',{exact:true}).fill('Support workspace')
 await expect(page.getByLabel('Slug',{exact:true})).toHaveValue('support-workspace')
 await page.getByLabel('Description',{exact:true}).fill('Tools and reference material for customer support agents.')
 await page.screenshot({path:test.info().outputPath('virtual-server-create.png'),animations:'disabled'})
 await page.getByLabel('Tool upstream').selectOption('alpha')
 await page.getByLabel('Filter tools').fill('customer')
 await page.getByRole('button',{name:'lookup Find a customer record',exact:true}).click()
 await page.getByLabel('Alias for alpha/lookup').fill('customer_lookup')
 await page.getByLabel('Policy',{exact:true}).selectOption('read_only')
 await page.getByLabel('Catalog kind').selectOption('resource')
 await page.getByLabel('Add catalog object').selectOption('alpha\0resource\0doc://guide')
 await page.getByLabel('Alias doc://guide').fill('doc://support')
 await page.getByRole('region',{name:'Available tools',exact:true}).evaluate(el=>el.scrollIntoView({block:'center'}))
 await page.screenshot({path:test.info().outputPath('virtual-server-tool-picker.png'),animations:'disabled'})
 await page.setViewportSize({width:390,height:844})
 await page.locator('summary').filter({hasText:'Access & protection'}).click()
 for(const name of ['Server identity','Resources and prompts','Exposed tools (1)','PII masking']){
  await page.getByRole('region',{name,exact:true}).evaluate(el=>el.scrollIntoView({block:'center'}))
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 }
 await page.getByLabel('Alias for alpha/lookup').evaluate(el=>el.scrollIntoView({block:'center'}))
 await page.screenshot({path:test.info().outputPath('virtual-server-edit-mobile.png'),animations:'disabled'})
 await expect(page.getByRole('button',{name:'Save',exact:true})).toBeInViewport()
 await page.getByRole('button',{name:'Save',exact:true}).click()
 await expect(page).toHaveURL(/virtual-servers\/support-workspace\/overview$/)
 expect(writes).toHaveLength(1)
 expect(writes[0].body.tools).toEqual([{upstream:'alpha',tool:'lookup',alias:'customer_lookup'}])
 expect(writes[0].body.policy).toEqual({default:'allow',tools:{customer_lookup:'read_only'}})
 expect(writes[0].body.catalog).toEqual([{upstream:'alpha',kind:'resource',name:'doc://guide',alias:'doc://support'}])
})

test('new upstream protects pending headers and saves connection settings without automatic testing',async({page})=>{
 const writes=await login(page,'/upstreams/new')
 await expect(page.getByRole('heading',{name:'New upstream',exact:true})).toBeVisible()
 await page.getByLabel('Slug',{exact:true}).fill('knowledge')
 await page.getByLabel('Name',{exact:true}).fill('Knowledge MCP')
 await page.getByLabel('URL',{exact:true}).fill('https://knowledge.example/mcp')
 await page.screenshot({path:test.info().outputPath('upstream-create.png'),animations:'disabled'})
 await page.locator('summary').filter({hasText:'Security & shared headers'}).click()
 await page.getByLabel('New header name',{exact:true}).fill('X-Workspace')
 await page.getByLabel('New header value',{exact:true}).fill('support')
 await page.getByRole('button',{name:'Save',exact:true}).click()
 await expect(page.getByRole('alert')).toContainText('Add or clear the pending header')
 expect(writes).toHaveLength(0)
 await page.getByRole('button',{name:'Add',exact:true}).click()
 await page.setViewportSize({width:390,height:844})
 await page.getByRole('region',{name:'Request headers',exact:true}).evaluate(el=>el.scrollIntoView({block:'center'}))
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath('upstream-create-mobile.png'),animations:'disabled'})
 await expect(page.getByRole('button',{name:'Save',exact:true})).toBeInViewport()
 await page.getByRole('button',{name:'Save',exact:true}).click()
 await expect(page).toHaveURL(/upstreams\/knowledge$/)
 await expect(page.getByRole('button',{name:'Edit upstream',exact:true})).toBeVisible()
 expect(writes).toHaveLength(1)
 expect(writes[0].body.headers).toEqual({'X-Workspace':'support'})
 expect(writes[0].body.url).toBe('https://knowledge.example/mcp')
})

for (const width of [1280, 390]) {
 test(`long capability descriptions stay in the draft until Save at ${width}px`,async({page})=>{
  await page.setViewportSize({width,height:900})
  const writes=await login(page,'/virtual-servers/new')
  await page.getByLabel('Name',{exact:true}).fill('Description workspace')
  await page.getByRole('button',{name:'lookup Find a customer record',exact:true}).click()
  const trigger=page.getByRole('button',{name:'Edit description for alpha/lookup'})
  await trigger.click()
  let dialog=page.getByRole('dialog',{name:'Capability description'})
  await expect(dialog.getByLabel('Description override',{exact:true})).toBeFocused()
  await dialog.getByRole('button',{name:'Start from upstream'}).click()
  await expect(dialog.getByLabel('Description override',{exact:true})).toHaveValue('Find a customer record')
  const text='Instructions:\n  preserve indentation\n'+ 'Uzun açıklama 🙂\n'.repeat(500)
  await dialog.getByLabel('Description override',{exact:true}).fill(text)
  if (width >= 1024) {
   await dialog.getByRole('button',{name:'Compare',exact:true}).click()
   await expect(dialog.getByRole('region',{name:'Description comparison'})).toContainText('removed')
   await expect(dialog.getByRole('columnheader',{name:/Upstream/})).toBeVisible()
   await expect(dialog.getByRole('button',{name:'Apply to draft'})).toBeInViewport()
   await page.screenshot({path:test.info().outputPath('description-comparison-desktop.png'),animations:'disabled'})
   await page.setViewportSize({width:390,height:900})
   await expect(dialog.getByRole('button',{name:'Compare',exact:true})).toHaveCount(0)
   await expect(dialog.getByRole('region',{name:'Description comparison'})).toHaveCount(0)
   await expect(dialog.getByRole('button',{name:'Agent view'})).toHaveAttribute('aria-pressed','true')
   await page.setViewportSize({width,height:900})
  } else {
   await expect(dialog.getByRole('button',{name:'Compare',exact:true})).toHaveCount(0)
  }
  await dialog.getByRole('button',{name:'Upstream',exact:true}).click()
  await expect(dialog.getByRole('region',{name:'Upstream description'})).toContainText('Find a customer record')
  await dialog.getByRole('button',{name:'Agent view'}).click()
  await expect(dialog.getByRole('region',{name:'Agent description'})).toContainText('alpha__lookup')
  expect(writes).toHaveLength(0)
  await expect(dialog.getByRole('button',{name:'Apply to draft'})).toBeInViewport()
  expect(await dialog.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
  await page.screenshot({path:test.info().outputPath(`description-preview-${width}.png`),animations:'disabled'})
  await dialog.getByRole('button',{name:'Edit override'}).click()
  await page.keyboard.press('Escape')
  const confirmation=page.getByRole('dialog',{name:'Discard unsaved changes?'})
  await expect(confirmation).toBeVisible()
  await confirmation.getByRole('button',{name:'Keep editing'}).click()
  await expect(dialog.getByLabel('Description override',{exact:true})).toHaveValue(text)
  await dialog.getByRole('button',{name:'Apply to draft'}).click()
  await expect(dialog).not.toBeVisible()
  await expect(trigger).toBeFocused()
  for (const [kind,name] of [['resource','doc://guide'],['prompt','review']] as const) {
   await page.getByLabel('Add catalog object').selectOption(`alpha\0${kind}\0${name}`)
   await page.getByRole('button',{name:`Edit description for alpha/${kind}/${name}`}).click()
   dialog=page.getByRole('dialog',{name:'Capability description'})
   await dialog.getByLabel('Description override',{exact:true}).fill(`${kind}\n  ${text}`)
   await dialog.getByRole('button',{name:'Apply to draft'}).click()
  }
  expect(writes).toHaveLength(0)
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  await page.getByRole('button',{name:'Save',exact:true}).click()
  await expect(page).toHaveURL(/virtual-servers\/description-workspace\/overview$/)
  expect(writes).toHaveLength(1)
  expect(writes[0].body.tools).toEqual([{upstream:'alpha',tool:'lookup',description:text}])
  expect(writes[0].body.catalog).toEqual([{upstream:'alpha',kind:'resource',name:'doc://guide',description:`resource\n  ${text}`},{upstream:'alpha',kind:'prompt',name:'review',description:`prompt\n  ${text}`}])
 })
}
