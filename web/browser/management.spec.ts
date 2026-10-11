import { expect, test } from '@playwright/test'
test('login loads all management pages and rejects a concurrent edit',async({page})=>{
 let writes=0
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',async route=>{
  const req=route.request(),url=new URL(req.url())
  if(url.pathname.endsWith('/me'))return route.fulfill({status:req.headers().authorization?200:401,json:{name:'Admin',role:'admin',via:'bootstrap-token'}})
  if(req.method()==='PATCH'){writes++;expect(req.headers()['if-match']).toBe('"7"');return route.fulfill({status:412,json:{detail:'stale',code:'precondition_failed'}})}
  if(url.pathname.endsWith('/catalog'))return route.fulfill({json:{resources:[],resourceTemplates:[],prompts:[],capabilities:{}}})
  if(url.pathname.endsWith('/tools'))return route.fulfill({json:{items:[]}})
  const upstream=(i:number)=>({slug:`upstream-${i}`,name:`Upstream ${i}`,version:7,url:'https://example.com/mcp',transport:'streamable_http',enabled:true,headerNames:[],tls:{caPem:'',serverName:'',clientCertPem:'',hasClientKey:false,caCerts:[],clientCert:null},auth:{type:'none'},health:{state:'unknown',nodes:[]},toolCount:0,fallbackUrls:[]})
  if(url.pathname.endsWith('/upstreams'))return route.fulfill({json:url.searchParams.has('cursor')?{items:[upstream(200)],nextCursor:null}:{items:Array.from({length:200},(_,i)=>upstream(i)),nextCursor:'page2'}})
  return route.fulfill({json:{items:[]}})
 })
 await page.goto('/')
 await page.getByRole('textbox').fill('admin-test')
 await page.getByRole('button',{name:'Continue'}).click()
 await page.getByRole('link',{name:'Upstreams',exact:true}).click()
 await expect(page.getByText('Upstream 200',{exact:true})).toBeVisible()
 await page.getByText('Upstream 200',{exact:true}).click()
 await page.getByRole('button',{name:'Save',exact:true}).click()
 await expect(page.getByText('This item changed in another session. Reload before saving again.')).toBeVisible()
 expect(writes).toBe(1)
})
