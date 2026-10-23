import {auditSettings} from './audit-fixtures'
import {expect,test} from '@playwright/test'
import {upstream} from '../src/connections/fixtures'
for(const width of [1280,390])test(`REST operation and schema modal drafts at ${width}px`,async({page})=>{
 await page.setViewportSize({width,height:900})
 const writes:Record<string,unknown>[]=[];const actions:string[]=[]
 let saved:Record<string,unknown>|null=null
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',r=>{
  const req=r.request(),path=new URL(req.url()).pathname
  if(path.endsWith('/audit-settings'))return r.fulfill({json:auditSettings})
  if(path.endsWith('/me'))return r.fulfill({status:req.headers().authorization?200:401,json:{role:'admin',name:'Tester'}})
  if(path.endsWith('/upstreams')&&req.method()==='POST'){writes.push(req.postDataJSON());saved={...upstream,...req.postDataJSON(),version:1};return r.fulfill({status:201,json:saved})}
  if(path.endsWith('/upstreams'))return r.fulfill({json:{items:saved?[saved]:[]}})
  if(path.includes('/rest/')){actions.push(path);return r.fulfill({json:{method:'GET',url:'https://api.example.com/orders',headers:{}}})}
  if(path.endsWith('/catalog'))return r.fulfill({json:{resources:[],resourceTemplates:[],prompts:[],capabilities:{}}})
  return r.fulfill({json:{items:[]}})
 })
 await page.goto('/upstreams/new');await page.getByLabel('Admin token').fill('test');await page.getByRole('button',{name:'Continue',exact:true}).click()
 await page.getByLabel('Upstream type',{exact:true}).selectOption('rest');await page.getByLabel('Slug',{exact:true}).fill('orders');await page.getByLabel('Name',{exact:true}).fill('Orders');await page.getByLabel('URL',{exact:true}).fill('https://api.example.com')
 await page.locator('summary').filter({hasText:'Security & shared headers'}).click();await page.getByLabel('Authentication',{exact:true}).selectOption('basic');await page.getByLabel('Username',{exact:true}).fill('service');await page.getByLabel('Password',{exact:true}).fill('secret')
 await page.getByRole('button',{name:'Add operation',exact:true}).click()
 const operation=page.getByRole('dialog',{name:'Edit REST operation',exact:true})
 await operation.getByLabel('Tool name',{exact:true}).fill('list_orders');await operation.getByLabel('Operation path',{exact:true}).fill('/orders/{id}')
 await operation.getByRole('button',{name:'Parameters (1)',exact:true}).click();await operation.getByLabel('Parameter 1 type',{exact:true}).selectOption('integer')
 await operation.getByRole('button',{name:'Advanced settings',exact:true}).click();await operation.getByLabel('Response schema',{exact:true}).fill('{');await expect(operation.getByRole('button',{name:'Apply operation',exact:true})).toBeDisabled();await operation.getByLabel('Response schema',{exact:true}).fill('')
 await operation.getByRole('button',{name:'Request & response schemas',exact:true}).click();await operation.getByRole('button',{name:'Edit response schema',exact:true}).click()
 const schema=page.getByRole('dialog',{name:'Response schema',exact:true})
 await schema.getByLabel('Response example JSON',{exact:true}).fill('{"orders":[{"id":1,"customer":{"name":"Ada"}}],"internal":"private"}')
 await schema.getByRole('button',{name:'Generate response schema',exact:true}).click();await schema.getByLabel('Find schema fields',{exact:true}).fill('customer')
 await schema.getByLabel('Response.orders[].customer.name description',{exact:true}).fill('Customer display name')
 await schema.getByLabel('Find schema fields',{exact:true}).fill('id');await schema.getByLabel('Response.orders[].id required',{exact:true}).check()
 await schema.getByRole('button',{name:'Selected fields only',exact:true}).click();await schema.getByLabel('Return field internal',{exact:true}).uncheck()
 expect(writes).toHaveLength(0);expect(actions).toHaveLength(0)
 await page.screenshot({path:test.info().outputPath(`response-schema-${width}.png`)})
 expect(await schema.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
 await schema.getByRole('button',{name:'Apply schema',exact:true}).click();await expect(schema).not.toBeVisible()
 await expect(operation.getByLabel('MCP tool definition')).not.toContainText('"internal"')
 await operation.getByRole('button',{name:'Apply operation',exact:true}).click();await expect(operation).not.toBeVisible()
 expect(writes).toHaveLength(0)
 await expect(page.getByRole('textbox',{name:'Tool name',exact:true})).toHaveCount(0)
 await page.getByRole('button',{name:'Save',exact:true}).click();await expect.poll(()=>writes.length).toBe(1)
 expect(writes[0]).toMatchObject({kind:'rest',rest:{auth:{type:'basic',username:'service',password:'secret'},operations:[{name:'list_orders',path:'/orders/{id}',responseFields:['orders'],parameters:[{name:'id',in:'path',required:true,schema:{type:'integer'}}]}]}})
 await page.getByRole('button',{name:'Preview request',exact:true}).click();await expect.poll(()=>actions.length).toBe(1)
 await expect(page.getByLabel('REST operation result')).toContainText('https://api.example.com/orders')
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
})
