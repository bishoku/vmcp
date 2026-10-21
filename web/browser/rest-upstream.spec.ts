import { auditSettings } from './audit-fixtures'
import { expect, test } from '@playwright/test'
import { upstream } from '../src/connections/fixtures'
for(const width of [1280,390])test(`REST upstream draft and explicit preview at ${width}px`,async({page})=>{
 await page.setViewportSize({width,height:900})
 const writes:Record<string,unknown>[]=[];const actions:string[]=[]
 let saved:Record<string,unknown>|null=null
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',r=>{
 if(new URL(r.request().url()).pathname.endsWith('/audit-settings'))return r.fulfill({json:auditSettings})

 const req=r.request(),path=new URL(req.url()).pathname
 if(path.endsWith('/me'))return r.fulfill({status:req.headers().authorization?200:401,json:{role:'admin',name:'Tester'}})
 if(path.endsWith('/upstreams')&&req.method()==='POST'){writes.push(req.postDataJSON());saved={...upstream,...req.postDataJSON(),version:1};return r.fulfill({status:201,json:saved})}
 if(path.endsWith('/upstreams'))return r.fulfill({json:{items:saved?[saved]:[]}})
 if(path.includes('/rest/')){actions.push(path);return r.fulfill({json:{method:'GET',url:'https://api.example.com/orders',headers:{}}})}
 if(path.endsWith('/catalog'))return r.fulfill({json:{resources:[],resourceTemplates:[],prompts:[],capabilities:{}}})
 return r.fulfill({json:{items:[]}})
 })
 await page.goto('/upstreams/new');await page.getByLabel('Admin token').fill('test');await page.getByRole('button',{name:'Continue',exact:true}).click()
 await page.getByLabel('Upstream type',{exact:true}).selectOption('rest')
 await page.getByText('Connection & authentication · Base URL required',{exact:false}).click()
 await page.getByLabel('Slug',{exact:true}).fill('orders');await page.getByLabel('Name',{exact:true}).fill('Orders');await page.getByLabel('URL',{exact:true}).fill('https://api.example.com')
 await page.getByLabel('Authentication',{exact:true}).selectOption('basic');await page.getByLabel('Username',{exact:true}).fill('service');await page.getByLabel('Password',{exact:true}).fill('secret')
 await page.getByRole('button',{name:'Add operation',exact:true}).click()
 const dialog=page.getByRole('region',{name:'Request & tool',exact:true})
 await dialog.getByLabel('Tool name',{exact:true}).fill('list_orders');await dialog.getByLabel('Operation path',{exact:true}).fill('/orders')
 await dialog.getByLabel('Tool description',{exact:true}).fill('List orders for the current service account.')
 await dialog.getByLabel('Operation path',{exact:true}).fill('/orders/{id}')
 await dialog.getByLabel('Parameter 1 type',{exact:true}).selectOption('integer')
 await dialog.getByText('Advanced · body, response & headers',{exact:true}).click()
 await dialog.getByLabel('Response schema',{exact:true}).fill('{')
 await expect(page.getByRole('button',{name:'Save',exact:true})).toBeDisabled()
 await dialog.getByLabel('Response schema',{exact:true}).fill('')
 await expect(page.getByRole('button',{name:'Save',exact:true})).toBeEnabled()
 await dialog.getByText('Advanced · body, response & headers',{exact:true}).click()
 await dialog.getByLabel('Response example JSON',{exact:true}).fill('{"orders":[{"id":1,"customer":{"name":"Ada"}}],"internal":"private"}')
 await dialog.getByRole('button',{name:'Generate response schema',exact:true}).click()
 await dialog.getByLabel('Response.orders[].customer.name description',{exact:true}).fill('Customer display name')
 await dialog.getByLabel('Response.orders[].id required',{exact:true}).check()
 await expect(dialog.getByLabel('Response.orders[].customer.name type',{exact:true})).toHaveValue('string')
 await dialog.getByRole('button',{name:'Selected fields only',exact:true}).click()
 await dialog.getByLabel('Return field internal',{exact:true}).uncheck()
 await expect(dialog.getByLabel('Return field orders',{exact:true})).toBeDisabled()
 if(width<1280)await page.getByRole('button',{name:'MCP preview',exact:true}).click()
 await expect(page.getByLabel('MCP tool definition')).toContainText('"id"')
 await expect(page.getByLabel('MCP tool definition')).toContainText('"integer"')
 await expect(page.getByLabel('MCP tool definition')).not.toContainText('"internal"')
 await page.screenshot({path:`/tmp/vmcp-rest-workspace-${width}.png`,fullPage:true})
 if(width<1280)await page.getByRole('button',{name:'Editor',exact:true}).click()
 expect(writes).toHaveLength(0);expect(actions).toHaveLength(0)
 await page.getByRole('button',{name:'Save',exact:true}).click();await expect.poll(()=>writes.length).toBe(1)
 expect(writes[0]).toMatchObject({kind:'rest',rest:{auth:{type:'basic',username:'service',password:'secret'},operations:[{name:'list_orders',path:'/orders/{id}',responseFields:['orders'],parameters:[{name:'id',in:'path',required:true,schema:{type:'integer'}}]}]}})
 await expect(page.getByRole('button',{name:'Edit upstream',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'Preview request',exact:true}).click();await expect.poll(()=>actions.length).toBe(1);expect(actions[0]).toContain('/rest/preview')
 await expect(page.getByLabel('REST operation result')).toContainText('https://api.example.com/orders')
 await page.screenshot({path:`/tmp/vmcp-rest-${width}.png`,fullPage:true})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
})
