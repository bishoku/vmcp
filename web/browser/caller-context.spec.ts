import {auditSettings} from './audit-fixtures'
import {expect,test} from '@playwright/test'
import {upstream} from '../src/connections/fixtures'
for(const kind of ['rest','mcp'] as const) for(const width of [390,1440]) test(`caller context ${kind} save and reload at ${width}px`,async({page})=>{
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
 await page.getByLabel('Forward caller context',{exact:true}).check()
 await page.getByLabel('Metadata namespace',{exact:true}).fill('com.bank/caller-context')
 await page.getByRole('button',{name:'Add context field',exact:true}).click()
 await page.getByLabel('Context field 1',{exact:true}).fill('token')
 if(kind==='rest'){
  await page.getByLabel('Context header 1',{exact:true}).fill('Authorization')
  await page.getByLabel('Bearer prefix for field 1',{exact:true}).check()
 }
 await page.getByLabel('Require context field 1',{exact:true}).check()
 expect(writes).toHaveLength(0)
 await page.getByRole('button',{name:'Save',exact:true}).click()
 await expect.poll(()=>writes.length).toBe(1)
 expect(writes[0]).toMatchObject({callerContext:{enabled:true,metaKey:'com.bank/caller-context',fields:[{name:'token',required:true,...(kind==='rest'?{header:'Authorization',bearer:true}:{})}]}})
 await page.reload()
 await expect(page.getByLabel('Context field 1',{exact:true})).toHaveValue('token')
 await expect(page.getByLabel('Context field 1',{exact:true})).toBeDisabled()
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.getByRole('region',{name:'Caller context'}).scrollIntoViewIfNeeded()
 await page.screenshot({path:test.info().outputPath(`caller-context-${kind}-${width}.png`)})
})
