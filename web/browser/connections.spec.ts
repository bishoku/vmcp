import { expect, test, type Page } from '@playwright/test'
import { server, upstream, key, binding } from '../src/connections/fixtures'
async function login(page:Page,role:'viewer'|'admin'='admin', path='/') {
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',route=>{
  const request=route.request(),path=new URL(request.url()).pathname
  if(path.endsWith('/me'))return route.fulfill({status:request.headers().authorization?200:401,json:{name:'Tester',role,via:'admin-token'}})
  if(path.endsWith('/upstreams'))return route.fulfill({json:{items:[upstream]}})
  if(path.endsWith('/virtual-servers'))return route.fulfill({json:{items:[server]}})
  if(path.endsWith('/agent-keys'))return route.fulfill({json:{items:[key]}})
  if(path.endsWith('/identity-bindings'))return route.fulfill({json:{items:[binding]}})
  if(path.endsWith('/catalog'))return route.fulfill({json:{resources:[],resourceTemplates:[],prompts:[],capabilities:{}}})
  return route.fulfill({json:{items:[]}})
 })
 await page.goto(path);await page.getByLabel('Admin token').fill('admin-login-token');await page.getByRole('button',{name:'Continue',exact:true}).click()
 await expect(page.getByRole('link',{name:'Connections',exact:true}).first()).toBeVisible()
}
test('keyboard inspection opens relationship health and a deep-linked upstream editor',async({page})=>{
 await login(page);await page.getByRole('link',{name:'Connections',exact:true}).click()
 await expect(page.getByText('Configured relationships (3)')).toBeVisible()
 await page.screenshot({path:test.info().outputPath('connections.png'),fullPage:true})
 const inspect=page.getByRole('button',{name:'Inspect upstream demo to alpha',exact:true})
 await inspect.focus();await page.keyboard.press('Enter')
 const dialog=page.getByRole('dialog',{name:'demo → alpha',exact:true})
 await expect(dialog).toBeVisible();await expect(dialog.getByText('Missing object',{exact:true})).toBeVisible();await expect(dialog.getByText(/Checked:/)).toBeVisible()
 await dialog.getByRole('link',{name:'Open configuration',exact:true}).click()
 await expect(page).toHaveURL(/upstreams\/alpha/)
 await expect(page.getByRole('heading',{name:'Alpha',exact:true})).toBeVisible()
 await expect(page.getByLabel('Name',{exact:true})).toBeDisabled()
})
test('saved views deep-link without losing draft protection and activity filters carry into audit',async({page})=>{
 await login(page,'admin','/virtual-servers/demo/capabilities')
 await expect(page.getByRole('heading',{name:'Demo',exact:true})).toBeVisible()
 await expect(page.getByRole('link',{name:'Capabilities',exact:true})).toHaveAttribute('aria-current','page')
 await page.getByRole('link',{name:'Edit configuration',exact:true}).click()
 await expect(page.getByLabel('Name',{exact:true})).toHaveValue('Demo')
 await page.getByLabel('Name',{exact:true}).fill('Draft name')
 await expect(page.getByText(/Unsaved changes/)).toBeVisible()
 const dismissed=new Promise<void>(resolve=>page.once('dialog',async d=>{await d.dismiss();resolve()}))
 await page.getByRole('link',{name:'View saved configuration',exact:true}).click()
 await dismissed
 await expect(page.getByLabel('Name',{exact:true})).toHaveValue('Draft name')
 const accepted=new Promise<void>(resolve=>page.once('dialog',async d=>{await d.accept();resolve()}))
 await page.getByRole('link',{name:'View saved configuration',exact:true}).click()
 await accepted
 await page.getByRole('link',{name:'Activity',exact:true}).click()
 await expect(page.getByText(/No stored events in this window/)).toBeVisible()
 await page.getByRole('link',{name:'Open audit log',exact:true}).click()
 await expect(page.getByPlaceholder('Virtual server', {exact:true})).toHaveValue('demo')
})
test('viewer explicitly verifies MCP discovery with a gateway key and no capability execution',async({page})=>{
 await login(page,'viewer','/virtual-servers/demo/connect')
 const methods:string[]=[]
 let deleted=false
 await page.route('**/mcp/v1/virtual/demo',route=>{
  const request=route.request()
  expect(request.headers().authorization).toBe('Bearer separate-agent-key')
  if(request.method()==='GET')return route.fulfill({status:405})
  if(request.method()==='DELETE'){deleted=true;return route.fulfill({status:204})}
  const message=request.postDataJSON();methods.push(message.method)
  if(message.method==='notifications/initialized')return route.fulfill({status:202})
  const results:Record<string,unknown>={initialize:{protocolVersion:'2025-06-18',capabilities:{tools:{},resources:{},prompts:{}},serverInfo:{name:'vMCP demo',version:'test'}},ping:{},'tools/list':{tools:[]},'resources/list':{resources:[]},'resources/templates/list':{resourceTemplates:[]},'prompts/list':{prompts:[]}}
  return route.fulfill({headers:{'Mcp-Session-Id':'browser-test'},json:{jsonrpc:'2.0',id:message.id,result:results[message.method]}})
 })
 await expect(page.getByRole('button',{name:'Verify connection',exact:true})).toBeDisabled()
 expect(methods).toEqual([])
 await page.getByLabel('Agent key or OAuth access token').fill('separate-agent-key')
 await page.getByRole('button',{name:'Verify connection',exact:true}).click()
 await expect(page.getByText(/Verified initialization and ping with vMCP demo/)).toBeVisible()
 await expect(page.getByLabel('Agent key or OAuth access token')).toHaveValue('')
 expect(methods).toEqual(['initialize','notifications/initialized','ping','tools/list','resources/list','resources/templates/list','prompts/list'])
 expect(deleted).toBe(true)
})
