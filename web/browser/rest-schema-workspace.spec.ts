import {expect,test} from '@playwright/test'
import {auditSettings} from './audit-fixtures'
import {upstream} from '../src/connections/fixtures'
for(const width of [390,1440])test(`large response schema, mismatch and nested Escape at ${width}px`,async({page})=>{
 await page.setViewportSize({width,height:900})
 const responseSchema={type:'object',properties:Object.fromEntries(Array.from({length:150},(_,i)=>[`record${i}`,{type:'object',properties:{amount:{type:'integer'},description:{type:'string'}}}]))}
 let saved={...upstream,slug:'large-rest',name:'Large REST',kind:'rest',rest:{auth:{type:'none'},forwardIdentity:false,requireIdentity:false,allowInsecureHttp:false,timeoutSeconds:30,operations:[{name:'orders',description:'Read orders',method:'GET',path:'/orders',enabled:true,readOnly:true,parameters:[],responseSchema}]}}
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
 await page.goto('/upstreams/large-rest');await page.getByRole('button',{name:'Edit upstream',exact:true}).click();await page.getByRole('button',{name:'Edit operation orders',exact:true}).click()
 const operation=page.getByRole('dialog',{name:'Edit REST operation',exact:true})
 await operation.getByRole('button',{name:'Request & response schemas',exact:true}).click();await operation.getByRole('button',{name:'Edit response schema',exact:true}).click()
 const schema=page.getByRole('dialog',{name:'Response schema',exact:true})
 await expect(schema).toBeVisible();await page.keyboard.press('Escape');await expect(schema).not.toBeVisible();await expect(operation).toBeVisible()
 await operation.getByRole('button',{name:'Edit response schema',exact:true}).click();await schema.getByLabel('Response example JSON',{exact:true}).fill('{"record149":{"amount":null}}');await schema.getByRole('button',{name:'Generate response schema',exact:true}).click()
 await expect(schema.getByText('/record149/amount: must be integer',{exact:true})).toBeVisible()
 await schema.getByRole('button',{name:'Apply schema',exact:true}).click();await expect(schema).toBeVisible()
 await expect(schema.getByLabel('Response.record0.amount type',{exact:true})).toHaveCount(0)
 await schema.getByLabel('Find schema fields',{exact:true}).fill('record149')
 await schema.getByLabel('Response.record149.amount nullable',{exact:true}).check()
 await schema.getByRole('button',{name:'Check schema & example',exact:true}).click();await expect(schema.getByText('Schema checks passed and the example matches.',{exact:true})).toBeVisible()
 await page.screenshot({path:test.info().outputPath(`large-schema-${width}.png`)})
 expect(await schema.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
 expect(writes).toHaveLength(0)
 await schema.getByRole('button',{name:'Apply schema',exact:true}).click();await expect(schema).not.toBeVisible();await expect(operation.getByRole('button',{name:'Edit response schema',exact:true})).toBeFocused()
 await operation.getByRole('button',{name:'Apply operation',exact:true}).click();await expect(operation).not.toBeVisible();expect(writes).toHaveLength(0)
 await page.getByRole('button',{name:'Save',exact:true}).click();await expect.poll(()=>writes.length).toBe(1)
 const body=writes[0] as {rest:{operations:{responseSchema:{properties:Record<string,{properties:{amount:{type:unknown}}}>}}[]}}
 expect(body.rest.operations[0].responseSchema.properties.record149.properties.amount.type).toEqual(['integer','null'])
 expect(Object.keys(body.rest.operations[0].responseSchema.properties)).toHaveLength(150)
})
