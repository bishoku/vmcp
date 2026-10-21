import {auditSettings} from './audit-fixtures'
import {expect,test} from '@playwright/test'
import {upstream} from '../src/connections/fixtures'
for(const kind of ['rest','mcp'] as const) for(const width of [390,1440]) test(`request header mapping ${kind} save and reload at ${width}px`,async({page})=>{
 await page.setViewportSize({width,height:900})
 let saved={...upstream,slug:'context',name:'Context',kind,rest:{auth:{type:'none'},forwardIdentity:false,requireIdentity:false,allowInsecureHttp:false,timeoutSeconds:30,operations:[]}} as Record<string,unknown>
 const writes:Record<string,unknown>[]=[]
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',r=>{
  const req=r.request(),path=new URL(req.url()).pathname
  if(path.endsWith('/audit-settings'))return r.fulfill({json:auditSettings})
  if(path.endsWith('/me'))return r.fulfill({json:{role:'admin',name:'Tester'}})
  if(path.endsWith('/upstreams/context')&&req.method()==='PATCH'){const body=req.postDataJSON();writes.push(body);saved={...saved,...body,version:2};return r.fulfill({json:saved})}
  if(path.endsWith('/upstreams/context'))return r.fulfill({json:saved})
  if(path.endsWith('/upstreams'))return r.fulfill({json:{items:[saved]}})
  if(path.endsWith('/catalog'))return r.fulfill({json:{resources:[],resourceTemplates:[],prompts:[],capabilities:{}}})
  return r.fulfill({json:{items:[]}})
 })
 await page.goto('/upstreams/context')
 await page.getByRole('button',{name:'Edit upstream',exact:true}).click()
 await page.locator('summary').filter({hasText:'Per-call context'}).click()
 await expect(page.locator('.summary-tile').getByText('Transport',{exact:true})).toBeVisible()
 await page.getByRole('button',{name:'Configure mappings',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'Configure request header mappings'})
 await dialog.getByRole('button',{name:'Add header mapping',exact:true}).click()
 await dialog.getByLabel('Source header 1',{exact:true}).fill('X-Agent-Token')
 await expect(dialog.getByLabel('Target header 1',{exact:true})).toHaveValue('X-Agent-Token')
 await dialog.getByLabel('Target header 1',{exact:true}).fill('X-Customer-Token')
 await dialog.getByLabel('Require header 1',{exact:true}).check()
 await page.screenshot({path:test.info().outputPath(`header-modal-${kind}-${width}.png`)})
 await dialog.getByRole('button',{name:'Use mappings',exact:true}).click()
 expect(writes).toHaveLength(0)
 await page.getByRole('button',{name:'Save',exact:true}).click()
 await expect.poll(()=>writes.length).toBe(1)
 expect(writes[0]).toMatchObject({requestHeaderMappings:[{source:'X-Agent-Token',target:'X-Customer-Token',required:true}]})
 await page.reload()
 await expect(page.getByText('X-Customer-Token',{exact:true})).toBeVisible()
 await expect(page.getByRole('button',{name:'Configure mappings'})).toHaveCount(0)
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath(`upstream-overview-${kind}-${width}.png`),fullPage:true})
})
