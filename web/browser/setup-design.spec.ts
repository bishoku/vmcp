import { auditSettings } from './audit-fixtures'
import { expect, test } from '@playwright/test'
import { server, upstream, key, binding } from '../src/connections/fixtures'

test('setup context and explicit verification work across desktop and mobile',async({page})=>{
 const methods:string[]=[]
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',route=>{
 if(new URL(route.request().url()).pathname.endsWith('/audit-settings'))return route.fulfill({json:auditSettings})

  const request=route.request(),path=new URL(request.url()).pathname
  if(path.endsWith('/me'))return route.fulfill({status:request.headers().authorization?200:401,json:{name:'Tester',role:'viewer'}})
  const items=path.endsWith('/virtual-servers')?[server,{...server,slug:'other',name:'Other',endpoint:'/mcp/v1/virtual/other',authRequired:false}]:path.endsWith('/upstreams')?[upstream]:path.endsWith('/agent-keys')?[key]:path.endsWith('/identity-bindings')?[binding]:[]
  return route.fulfill({json:{items}})
 })
 await page.route('**/mcp/v1/virtual/demo',route=>{
  const request=route.request()
  expect(request.headers().authorization).toBe('Bearer separate-agent-key')
  if(request.method()==='GET')return route.fulfill({status:405})
  if(request.method()==='DELETE')return route.fulfill({status:204})
  const message=request.postDataJSON();methods.push(message.method)
  if(message.method==='notifications/initialized')return route.fulfill({status:202})
  const results:Record<string,unknown>={initialize:{protocolVersion:'2025-06-18',capabilities:{tools:{},resources:{},prompts:{}},serverInfo:{name:'vMCP demo',version:'test'}},ping:{},'tools/list':{tools:[]},'resources/list':{resources:[]},'resources/templates/list':{resourceTemplates:[]},'prompts/list':{prompts:[]}}
  return route.fulfill({headers:{'Mcp-Session-Id':'setup-test'},json:{jsonrpc:'2.0',id:message.id,result:results[message.method]}})
 })
 await page.goto('/setup?server=demo');await page.getByLabel('Admin token').fill('test-token');await page.getByRole('button',{name:'Continue',exact:true}).click()
 await expect(page.getByRole('region',{name:'Saved connection path'}).getByRole('link',{name:'Demo',exact:true})).toBeVisible()
 await expect(page.getByRole('listitem')).toHaveCount(7)
 expect(methods).toEqual([])
 await page.screenshot({path:test.info().outputPath('setup-overview.png'),animations:'disabled'})
 await page.getByRole('region',{name:'Setup steps'}).evaluate(el=>el.scrollIntoView({block:'start'}))
 await page.screenshot({path:test.info().outputPath('setup-steps.png'),animations:'disabled'})
 const credential=page.getByLabel('Agent key or OAuth access token')
 await credential.fill('unsent-credential')
 await page.getByLabel('Guide for virtual server').selectOption('other')
 await expect(page).toHaveURL(/server=other/)
 await expect(credential).toHaveCount(0)
 await page.getByLabel('Guide for virtual server').selectOption('demo')
 await expect(credential).toHaveValue('')
 await credential.fill('separate-agent-key')
 await page.getByRole('button',{name:'Verify connection',exact:true}).click()
 await expect(page.getByText(/Verified initialization and ping with vMCP demo/)).toBeVisible()
 expect(methods).toEqual(['initialize','notifications/initialized','ping','tools/list','resources/list','resources/templates/list','prompts/list'])
 await page.getByRole('region',{name:'Client connection'}).evaluate(el=>el.scrollIntoView({block:'start'}))
 await page.screenshot({path:test.info().outputPath('setup-connection.png'),animations:'disabled'})
 for(const width of [390,768,1440]){
  await page.setViewportSize({width,height:900})
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 }
 await page.setViewportSize({width:390,height:844})
 await page.getByRole('heading',{name:'Connection setup guide',exact:true}).evaluate(el=>el.scrollIntoView({block:'start'}))
 await page.screenshot({path:test.info().outputPath('setup-mobile.png'),animations:'disabled'})
})
