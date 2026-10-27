import { expect, test, type Page } from '@playwright/test'
async function fixture(page:Page,role='admin',configured=true,rerank=false) {
 const state={refresh:0,preview:0,token:'',fail:false,noMatch:false}
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',r=>{
  const req=r.request(),path=new URL(req.url()).pathname
  if(path.endsWith('/me'))return r.fulfill({json:{role,name:'Operator'}})
  if(path.endsWith('/omni'))return r.fulfill({json:{configured,endpoint:'https://gateway.example',...(role==='admin'?{index:{rerankEnabled:rerank,rerankCandidates:20,rerankNoMatchScore:0.0001,ready:true,mode:'hybrid',documents:6000,revision:82,updatedAt:'2026-10-27T09:00:00Z',refreshing:false}}:{})}})
  if(path.endsWith('/reindex')){state.refresh++;return r.fulfill({json:{accepted:true}})}
  if(path.endsWith('/preview')){
   state.preview++;state.token=req.headers()['x-vmcp-runtime-authorization'];expect(req.postData()).not.toContain('runtime-secret')
   if(state.fail)return r.fulfill({status:403,json:{detail:'runtime identity cannot access the requested Omni catalog'}})
   if(state.noMatch)return r.fulfill({json:{search_mode:'hybrid-reranked-no-match',tools:[]}})
   return r.fulfill({json:req.postDataJSON().tool_ref?{tool:{name:'list_orders',inputSchema:{type:'object'}},schema_version:'hash'}:{search_mode:'hybrid',tools:[{tool_ref:'ref',server_id:'chat',server_name:'Customer chat',project:'customer-app',name:'list_orders',description:'Find open customer orders',score:1.5}]}})
  }
  return r.fulfill({json:{items:[]}})
 });return state
}
for(const width of [390,1440])test(`Omni discovery and schema ${width}`,async({page})=>{
 await page.setViewportSize({width,height:1000});const state=await fixture(page);await page.goto('/omni')
 await expect(page.getByRole('heading',{name:'Omni MCP',exact:true})).toBeVisible()
 await expect(page.getByText('6000',{exact:true})).toBeVisible()
 await page.getByLabel('Agent key or OAuth access token').fill('runtime-secret')
 await page.getByLabel('Intent',{exact:true}).fill('customer orders')
 await page.getByRole('button',{name:'Find tools'}).click()
 await expect(page.getByText('list_orders',{exact:true})).toBeVisible();expect(state.token).toBe('Bearer runtime-secret')
 await page.screenshot({path:test.info().outputPath(`omni-${width}.png`),fullPage:true})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 await page.getByRole('button',{name:'View schema'}).click();await expect(page.getByRole('dialog')).toContainText('schema_version')
 await page.getByRole('button',{name:'Close',exact:true}).click()
 await page.getByRole('button',{name:'Clear credential'}).click();await expect(page.getByLabel('Agent key or OAuth access token')).toHaveValue('')
 await page.getByRole('button',{name:'Refresh index',exact:true}).click();expect(state.refresh).toBe(1)
})
test('Viewer preview requires runtime identity and cannot refresh index',async({page})=>{
 const state=await fixture(page,'viewer');await page.goto('/omni');await expect(page.getByRole('button',{name:'Refresh index',exact:true})).toHaveCount(0)
 await expect(page.getByRole('button',{name:'Find tools'})).toBeDisabled()
 state.fail=true;await page.getByLabel('Agent key or OAuth access token').fill('bad');await page.getByLabel('Intent',{exact:true}).fill('orders');await page.getByRole('button',{name:'Find tools'}).click();await expect(page.getByRole('alert')).toContainText('runtime identity')
})
test('Omni disabled state',async({page})=>{await fixture(page,'admin',false);await page.goto('/omni');await expect(page.getByText('Omni MCP is not configured',{exact:true})).toBeVisible();await expect(page.getByLabel('Agent key or OAuth access token')).toHaveCount(0)})

test('Configured reranker shows candidate and no-match settings',async({page})=>{
 await fixture(page,'admin',true,true);await page.goto('/omni')
 await expect(page.getByText('Reranker enabled · 20 candidates · no-match floor 0.0001',{exact:true})).toBeVisible()
})

test('Reranker no-match explains relevance rather than suggesting an access failure',async({page})=>{
 const state=await fixture(page,'admin',true,true);state.noMatch=true;await page.goto('/omni')
 await page.getByLabel('Agent key or OAuth access token').fill('runtime-secret')
 await page.getByLabel('Intent',{exact:true}).fill('unsupported intent')
 await page.getByRole('button',{name:'Find tools'}).click()
 await expect(page.getByText('No sufficiently relevant tools were found. Try a more specific intent.',{exact:true})).toBeVisible()
})
