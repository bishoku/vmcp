import { auditSettings } from './audit-fixtures'
import { expect, test } from '@playwright/test'
import { upstream } from '../src/connections/fixtures'
const definition='openapi: 3.1.0\ninfo: {title: Orders}\npaths: {}'
for(const width of [1280,390])test(`OpenAPI import stays in draft and preserves security at ${width}px`,async({page})=>{
 await page.setViewportSize({width,height:900})
 const writes:Record<string,unknown>[]=[];let analyzed=0
 const existing={name:'lookup',description:'Existing tool',method:'GET',path:'/existing',enabled:true,readOnly:true,parameters:[]}
 const imported={name:'lookup',description:'Imported orders',method:'GET',path:'/orders',enabled:true,readOnly:false,parameters:[],responseSchema:{type:'object'}}
 let saved={...upstream,kind:'rest',headerNames:['X-API-Key'],rest:{auth:{type:'basic',username:'service',hasPassword:true},forwardIdentity:true,requireIdentity:true,allowInsecureHttp:false,timeoutSeconds:30,operations:[existing]}}
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',r=>{
 if(new URL(r.request().url()).pathname.endsWith('/audit-settings'))return r.fulfill({json:auditSettings})

  const request=r.request(),path=new URL(request.url()).pathname
  if(path.endsWith('/me'))return r.fulfill({status:request.headers().authorization?200:401,json:{role:'admin',name:'Tester'}})
  if(path.endsWith('/rest-imports')){expect(request.postDataJSON()).toEqual({document:definition});analyzed++;return r.fulfill({json:{title:'Orders',version:'3.1.0',servers:['https://orders.example'],warnings:['Authentication and headers are preserved.'],operations:[{id:'GET /orders',method:'GET',path:'/orders',summary:'List orders',tags:['orders'],operation:imported,warnings:[],errors:[]},{id:'POST /upload',method:'POST',path:'/upload',summary:'Upload files',tags:[],warnings:[],errors:['Multipart is unsupported']}]}})}
  if(request.method()==='PATCH'){writes.push(request.postDataJSON());saved={...saved,...request.postDataJSON(),version:3};return r.fulfill({json:saved})}
  if(path.endsWith('/upstreams'))return r.fulfill({json:{items:[saved]}})
  if(path.endsWith('/catalog'))return r.fulfill({json:{resources:[],resourceTemplates:[],prompts:[],capabilities:{}}})
  return r.fulfill({json:{items:[]}})
 })
 await page.goto(`/upstreams/${upstream.slug}`);await page.getByLabel('Admin token').fill('test');await page.getByRole('button',{name:'Continue',exact:true}).click()
 await page.getByRole('button',{name:'Edit upstream',exact:true}).click();await page.getByRole('button',{name:'Import OpenAPI / Swagger',exact:true}).click()
 const dialog=page.getByRole('region',{name:'Import OpenAPI / Swagger',exact:true})
 await expect(page.getByRole('button',{name:'Save',exact:true})).toBeDisabled()
 await dialog.getByLabel('Definition file',{exact:true}).setInputFiles({name:'orders.yaml',mimeType:'application/yaml',buffer:Buffer.from(definition)})
 await expect(dialog.getByLabel('OpenAPI / Swagger definition',{exact:true})).toHaveValue(definition)
 expect(analyzed).toBe(0);await dialog.getByRole('button',{name:'Analyze definition',exact:true}).click()
 await expect(dialog.getByRole('checkbox',{name:'POST /upload',exact:true})).toBeDisabled()
 await expect(dialog.getByRole('checkbox',{name:'GET /orders',exact:true})).not.toBeChecked()
 await dialog.getByRole('checkbox',{name:'GET /orders',exact:true}).check();await expect(dialog.getByText('Tool: lookup_2',{exact:false})).toBeVisible()
 expect(writes).toHaveLength(0)
 await page.screenshot({path:`/tmp/vmcp-import-${width}.png`,fullPage:true})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await dialog.getByRole('button',{name:'Add 1 to draft',exact:true}).click();await expect(dialog).not.toBeVisible()
 const editor=page.getByRole('region',{name:'Request & tool',exact:true});await editor.getByLabel('Tool name',{exact:true}).fill('imported_orders')
 await expect(page.getByLabel('MCP tool definition')).toContainText('imported_orders')
 expect(writes).toHaveLength(0);await page.getByRole('button',{name:'Save',exact:true}).click();await expect.poll(()=>writes.length).toBe(1)
 expect(writes[0]).toMatchObject({kind:'rest',rest:{auth:{type:'basic',username:'service',hasPassword:true},forwardIdentity:true,requireIdentity:true,allowInsecureHttp:false,operations:[existing,{...imported,name:'imported_orders'}]}})
 expect(writes[0]).not.toHaveProperty('headers')
 await expect(page.getByRole('button',{name:'Edit upstream',exact:true})).toBeVisible()
})
