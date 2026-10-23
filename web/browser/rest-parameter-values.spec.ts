import {expect,test} from '@playwright/test'
import {auditSettings} from './audit-fixtures'
import {upstream} from '../src/connections/fixtures'
for(const width of [390,1440])test(`fixed and fallback query values persist at ${width}px`,async({page})=>{
 await page.setViewportSize({width,height:900})
 let saved={...upstream,slug:'sample-rest',name:'Sample REST',kind:'rest',rest:{auth:{type:'none'},forwardIdentity:false,requireIdentity:false,allowInsecureHttp:false,timeoutSeconds:30,operations:[{name:'sample',description:'Read sample',method:'GET',path:'/sample',enabled:true,readOnly:true,parameters:[{name:'type',in:'query',required:false,schema:{type:'integer',enum:[1,2]}},{name:'content',in:'query',required:true,schema:{type:'string'}}]}]}}
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
  await page.getByRole('button',{name:'Edit operation sample',exact:true}).click()
  const operation=page.getByRole('dialog',{name:'Edit REST operation',exact:true})
  await operation.getByRole('button',{name:'Parameters (2)',exact:true}).click()
  return operation
 }
 await page.goto('/upstreams/sample-rest')
 const operation=await open()
 await operation.getByLabel('Parameter 1 expose to MCP',{exact:true}).uncheck()
 await operation.getByLabel('Parameter 1 fixed value',{exact:true}).fill('3')
 await operation.getByRole('button',{name:'Apply operation',exact:true}).click()
 await expect(operation.getByText(/Parameter type:.*allowed values/)).toBeVisible()
 await operation.getByLabel('Parameter 1 fixed value',{exact:true}).fill('1')
 await operation.getByLabel('Parameter 2 use default value',{exact:true}).check()
 await operation.getByLabel('Parameter 2 default value',{exact:true}).fill('all & more')
 await expect(operation.getByLabel('Parameter 2 required',{exact:true})).not.toBeChecked()
 await expect(operation.getByLabel('Parameter 2 required',{exact:true})).toBeDisabled()
 const tool=JSON.parse(await operation.getByLabel('MCP tool definition',{exact:true}).textContent()??'{}')
 expect(tool.inputSchema).toEqual({type:'object',properties:{content:{type:'string',default:'all & more'}},required:[],additionalProperties:false})
 await page.screenshot({path:test.info().outputPath(`parameter-values-${width}.png`)})
 expect(await operation.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
 await operation.getByRole('button',{name:'Apply operation',exact:true}).click()
 expect(writes).toHaveLength(0)
 await page.getByRole('button',{name:'Save',exact:true}).click();await expect.poll(()=>writes.length).toBe(1)
 const body=writes[0] as {rest:{operations:{parameters:unknown[]}[]}}
 expect(body.rest.operations[0].parameters).toEqual([{name:'type',in:'query',required:false,schema:{type:'integer',enum:[1,2]},exposeToMcp:false,defaultValue:1},{name:'content',in:'query',required:true,schema:{type:'string'},defaultValue:'all & more'}])
 await page.reload();const reopened=await open()
 await expect(reopened.getByLabel('Parameter 1 expose to MCP',{exact:true})).not.toBeChecked()
 await expect(reopened.getByLabel('Parameter 1 fixed value',{exact:true})).toHaveValue('1')
 await expect(reopened.getByLabel('Parameter 2 default value',{exact:true})).toHaveValue('all & more')
})
