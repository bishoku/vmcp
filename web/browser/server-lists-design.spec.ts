import { expect, test, type Page } from '@playwright/test'
import { server, upstream } from '../src/connections/fixtures'

async function login(page:Page,role:'admin'|'viewer'='admin'){
 const upstreams=[
  {...upstream,name:'Documentation MCP',description:'Search documentation and retrieve team knowledge.',toolCount:18},
  {...upstream,slug:'github',name:'GitHub',description:'Repository search and pull request workflows.',toolCount:32,auth:{type:'oauth_client_credentials',hasClientSecret:true,clientId:'gateway',tokenUrl:'https://idp.example/token'},health:{state:'degraded',nodes:[]}},
  {...upstream,slug:'legacy',name:'Legacy integrations',description:'Internal services exposed through the legacy transport.',transport:'sse',enabled:false,lastRefreshError:'Discovery endpoint is unavailable',health:{state:'unhealthy',nodes:[]}},
  {...upstream,slug:'new',name:'New workspace',description:'Newly registered connection awaiting discovery.',lastRefreshAt:null,toolCount:0,health:{state:'unknown',nodes:[]}},
  ...Array.from({length:22},(_,i)=>({...upstream,slug:`service-${i}`,name:`Service ${i}`}))
 ]
 const servers=[
  {...server,name:'Engineering workspace',description:'Curated tools and documentation for engineering agents.'},
  {...server,slug:'public',name:'Public documentation',description:'Anonymous discovery endpoint for public documentation.',authRequired:false,rateLimitPerMinute:60},
  {...server,slug:'review',name:'Code review',description:'Capabilities scoped to repository review and code analysis.',enabled:false},
  ...Array.from({length:23},(_,i)=>({...server,slug:`workspace-${i}`,name:`Workspace ${i}`}))
 ]
 const writes:string[]=[]
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',route=>{
  const request=route.request(),path=new URL(request.url()).pathname
  if(request.method()!=='GET')writes.push(path)
  if(path.endsWith('/me'))return route.fulfill({status:request.headers().authorization?200:401,json:{name:'Tester',role}})
  return route.fulfill({json:{items:path.endsWith('/upstreams')?upstreams:path.endsWith('/virtual-servers')?servers:[]}})
 })
 await page.goto('/upstreams');await page.getByLabel('Admin token').fill('test-token');await page.getByRole('button',{name:'Continue',exact:true}).click()
 await expect(page.getByRole('article',{name:'Documentation MCP',exact:true})).toBeVisible()
 return writes
}

test('server inventories share cards filters and responsive navigation',async({page})=>{
 const writes=await login(page)
 await expect(page.getByRole('article')).toHaveCount(24)
 await page.screenshot({path:test.info().outputPath('upstreams-list.png'),animations:'disabled'})
 await page.getByRole('region',{name:'Configured upstreams'}).evaluate(el=>el.scrollIntoView({block:'start'}))
 await page.screenshot({path:test.info().outputPath('upstreams-cards.png'),animations:'disabled'})
 await page.getByRole('button',{name:'Next',exact:true}).click()
 await expect(page.getByRole('article')).toHaveCount(2)
 await page.getByLabel('Upstream transport').selectOption('sse')
 await expect(page.getByRole('article',{name:'Legacy integrations',exact:true})).toBeVisible()
 await expect(page.getByRole('article')).toHaveCount(1)
 await page.getByLabel('Upstream discovery').selectOption('failed')
 await expect(page.getByText('Discovery endpoint is unavailable',{exact:true})).toBeVisible()
 await page.getByLabel('Search upstreams').fill('no match')
 await expect(page.getByText('No upstreams match your filters.',{exact:true})).toBeVisible()
 await page.getByRole('button',{name:'Clear filters',exact:true}).click()
 await expect(page.getByRole('article')).toHaveCount(24)
 await page.getByRole('link',{name:'Virtual Servers',exact:true}).click()
 await expect(page.getByRole('article',{name:'Engineering workspace',exact:true})).toBeVisible()
 await page.screenshot({path:test.info().outputPath('virtual-servers-list.png'),animations:'disabled'})
 await page.getByRole('region',{name:'Configured virtual servers'}).evaluate(el=>el.scrollIntoView({block:'start'}))
 await page.screenshot({path:test.info().outputPath('virtual-servers-cards.png'),animations:'disabled'})
 await page.getByLabel('Virtual server authentication').selectOption('anonymous')
 await expect(page.getByRole('article')).toHaveCount(1)
 await expect(page.getByRole('article',{name:'Public documentation',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'Clear filters',exact:true}).click()
 for(const path of ['/upstreams','/virtual-servers']){
  await page.setViewportSize({width:1440,height:844})
  await page.getByRole('link',{name:path==='/upstreams'?'Upstreams':'Virtual Servers',exact:true}).click()
  for(const width of [390,768,1440]){
   await page.setViewportSize({width,height:844})
   await expect(page.getByRole('article').first()).toBeVisible()
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  }
  await page.setViewportSize({width:390,height:844})
  await page.getByRole('article').first().evaluate(el=>el.scrollIntoView({block:'start'}))
  await page.screenshot({path:test.info().outputPath(path==='/upstreams'?'upstreams-mobile.png':'virtual-servers-mobile.png'),animations:'disabled'})
 }
 expect(writes).toEqual([])
})

test('viewer can open saved details while mutations stay unavailable',async({page})=>{
 const writes=await login(page,'viewer')
 await expect(page.getByRole('button',{name:'Add upstream',exact:true})).toBeDisabled()
 await expect(page.getByRole('button',{name:'Refresh capabilities for Documentation MCP',exact:true})).toBeDisabled()
 await expect(page.getByRole('button',{name:'Delete Documentation MCP',exact:true})).toBeDisabled()
 await page.getByRole('link',{name:'Documentation MCP',exact:true}).focus();await page.keyboard.press('Enter')
 await expect(page).toHaveURL(/upstreams\/alpha$/)
 await expect(page.getByLabel('Name',{exact:true})).toBeDisabled()
 await page.getByRole('link',{name:'Virtual Servers',exact:true}).click()
 await expect(page.getByRole('button',{name:'New virtual server',exact:true})).toBeDisabled()
 await expect(page.getByRole('button',{name:'Delete Engineering workspace',exact:true})).toBeDisabled()
 await page.getByRole('link',{name:'Engineering workspace',exact:true}).click()
 await expect(page).toHaveURL(/virtual-servers\/demo\/overview$/)
 await expect(page.getByRole('region',{name:'Server overview'})).toBeVisible()
 expect(writes).toEqual([])
})
