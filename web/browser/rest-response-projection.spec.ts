import {expect,test} from '@playwright/test'
import {auditSettings} from './audit-fixtures'
import {upstream} from '../src/connections/fixtures'
import {readFileSync} from 'node:fs'
const fixtures=JSON.parse(readFileSync(new URL('../src/connections/testdata/response-projection.json',import.meta.url),'utf8'))
for(const fixture of fixtures.slice(0,2))for(const width of [390,1440])test(`${fixture.name} selection persists at ${width}px`,async({page})=>{
 await page.setViewportSize({width,height:900})
 let saved={...upstream,slug:'nested-rest',name:'Nested REST',kind:'rest',rest:{auth:{type:'none'},forwardIdentity:false,requireIdentity:false,allowInsecureHttp:false,timeoutSeconds:30,operations:[{name:'orders',description:'Read orders',method:'GET',path:'/orders',enabled:true,readOnly:true,parameters:[],responseSchema:fixture.schema}]}}
 const writes:Record<string,unknown>[]=[]
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',r=>{
  const req=r.request(),path=new URL(req.url()).pathname
  if(path.endsWith('/audit-settings'))return r.fulfill({json:auditSettings})
  if(path.endsWith('/me'))return r.fulfill({json:{role:'admin',name:'Tester'}})
  if(req.method()==='PATCH'){const body=req.postDataJSON();writes.push(body);saved={...saved,...body,version:3};return r.fulfill({json:saved})}
  if(path.endsWith('/upstreams'))return r.fulfill({json:{items:[saved]}})
  if(path.endsWith('/catalog'))return r.fulfill({json:{resources:[],resourceTemplates:[],prompts:[],capabilities:{}}})
  return r.fulfill({json:{items:[]}})
 })
 const open=async()=>{
  await page.getByRole('button',{name:'Edit upstream',exact:true}).click()
  await page.getByRole('button',{name:'Edit operation orders',exact:true}).click()
  const operation=page.getByRole('dialog',{name:'Edit REST operation',exact:true})
  await operation.getByRole('button',{name:'Request & response schemas',exact:true}).click()
  await operation.getByRole('button',{name:'Edit response schema',exact:true}).click()
  const schema=page.getByRole('dialog',{name:'Response schema',exact:true})
  await schema.getByRole('button',{name:'Fields',exact:true}).click()
  return {operation,schema}
 }
 await page.goto('/upstreams/nested-rest')
 const {operation,schema}=await open()
 await schema.getByRole('button',{name:'Selected fields only',exact:true}).click()
 if(fixture.name==='root-matrix'){
  await schema.getByLabel('Return field [][].hidden',{exact:true}).uncheck()
  await expect(schema.getByLabel('Return field [][].id',{exact:true})).toBeDisabled()
 }else{
  await schema.getByLabel('Return field private',{exact:true}).uncheck()
  await schema.getByLabel('Find response fields',{exact:true}).fill('order')
  await schema.getByLabel('Return field order.items[].price',{exact:true}).uncheck()
  await schema.getByLabel('Return field order.secret',{exact:true}).uncheck()
  await expect(schema.getByLabel('Return field order',{exact:true})).toHaveJSProperty('indeterminate',true)
  await schema.getByLabel('Find response fields',{exact:true}).fill('items')
 }
 await page.screenshot({path:test.info().outputPath(`nested-selection-${width}.png`)})
 expect(await schema.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
 await schema.getByRole('button',{name:'Apply schema',exact:true}).click()
 await operation.getByRole('button',{name:'Apply operation',exact:true}).click()
 expect(writes).toHaveLength(0)
 await page.getByRole('button',{name:'Save',exact:true}).click()
 await expect.poll(()=>writes.length).toBe(1)
 const body=writes[0] as {rest:{operations:{responseFields:string[];responseFieldPaths:string[][]}[]}}
 expect(body.rest.operations[0].responseFields).toEqual([])
 expect(body.rest.operations[0].responseFieldPaths).toEqual(fixture.paths)
 await page.reload()
 const reopened=await open()
 if(fixture.name==='root-matrix'){
  await expect(reopened.schema.getByLabel('Return field [][].id',{exact:true})).toBeChecked()
  await expect(reopened.schema.getByLabel('Return field [][].hidden',{exact:true})).not.toBeChecked()
 }else{
  await reopened.schema.getByLabel('Find response fields',{exact:true}).fill('order')
  await expect(reopened.schema.getByLabel('Return field order.items[].sku',{exact:true})).toBeChecked()
  await expect(reopened.schema.getByLabel('Return field order.items[].price',{exact:true})).not.toBeChecked()
  await expect(reopened.schema.getByLabel('Return field order.secret',{exact:true})).not.toBeChecked()
 }
})
