import {expect,test} from '@playwright/test'
test('fresh stack discovers a real upstream and verifies a credentialed MCP browser connection',async({page,request})=>{
 const api=process.env.VMCP_LIVE_API+'/api/v1'
 const headers={Authorization:'Bearer demo-admin-token'}
 const upstream=await request.post(`${api}/upstreams`,{headers,data:{slug:'ui-smoke',name:'UI smoke knowledge',url:'http://mcp-knowledge:9100/mcp',transport:'streamable_http',enabled:true,headers:{Authorization:'Bearer knowledge-secret'}}})
 expect(upstream.ok(),await upstream.text()).toBe(true)
 const refresh=await request.post(`${api}/upstreams/ui-smoke/refresh-tools`,{headers})
 expect(refresh.ok(),await refresh.text()).toBe(true)
 const catalog=await request.get(`${api}/upstreams/ui-smoke/catalog`,{headers});expect(catalog.ok()).toBe(true)
 const discovered=await catalog.json();expect(discovered.resources.length).toBeGreaterThan(0);expect(discovered.prompts.length).toBeGreaterThan(0)
 const vs=await request.post(`${api}/virtual-servers`,{headers,data:{slug:'ui-smoke',name:'UI smoke',enabled:true,authRequired:true,tools:[],catalog:[{upstream:'ui-smoke',kind:'resource',name:discovered.resources[0].uri},{upstream:'ui-smoke',kind:'prompt',name:discovered.prompts[0].name}],policy:{default:'allow'}}})
 expect(vs.ok(),await vs.text()).toBe(true)
 const key=await request.post(`${api}/agent-keys`,{headers,data:{slug:'ui-smoke',virtualServers:['ui-smoke']}})
 expect(key.ok(),await key.text()).toBe(true)
 const issued=await key.json()
 await page.goto('/virtual-servers/ui-smoke/connect');await page.getByLabel('Admin token').fill('demo-admin-token');await page.getByRole('button',{name:'Continue',exact:true}).click()
 await expect(page.getByRole('heading',{name:'Connect a client'})).toBeVisible()
 // Both data planes receive the fresh configuration asynchronously.
 await expect(async()=>{
  await page.getByLabel('Agent key or OAuth access token').fill(issued.key)
  await page.getByRole('button',{name:'Verify connection',exact:true}).click()
  await expect(page.getByText(/Verified initialization and ping/)).toBeVisible({timeout:15_000})
 }).toPass({timeout:40_000})
 await expect(page.getByText('1 resources on the first page')).toBeVisible()
 await expect(page.getByText('1 prompts on the first page')).toBeVisible()
 await page.screenshot({path:test.info().outputPath('live-connection.png'),fullPage:true})
})

test('detail pages save real versioned upstream and virtual-server edits',async({page,request})=>{
 const api=process.env.VMCP_LIVE_API+'/api/v1'
 const headers={Authorization:'Bearer demo-admin-token'}
 const slug='detail-'+Date.now().toString(36)
 let upstreamCreated=false,serverCreated=false
 try {
  const upstream=await request.post(`${api}/upstreams`,{headers,data:{slug,name:slug,url:'http://mcp-calculator:9100/mcp',transport:'streamable_http',enabled:true,headers:{Authorization:'Bearer calculator-secret'}}})
  expect(upstream.ok(),await upstream.text()).toBe(true);upstreamCreated=true
  const refresh=await request.post(`${api}/upstreams/${slug}/refresh-tools`,{headers});expect(refresh.ok(),await refresh.text()).toBe(true)
  const server=await request.post(`${api}/virtual-servers`,{headers,data:{slug,name:slug,enabled:true,authRequired:true,tools:[{upstream:slug,tool:'calculate',alias:'sum'}]}})
  expect(server.ok(),await server.text()).toBe(true);serverCreated=true
  await page.goto('/virtual-servers');await page.getByLabel('Admin token').fill('demo-admin-token');await page.getByRole('button',{name:'Continue',exact:true}).click()
  await page.getByRole('link',{name:slug,exact:true}).click()
  await expect(page.getByRole('heading',{name:slug,exact:true})).toBeVisible()
  await page.getByRole('link',{name:'Edit configuration',exact:true}).click()
  await page.getByLabel('Name',{exact:true}).fill(slug+' saved')
  await page.getByRole('button',{name:'Save',exact:true}).click()
  await expect(page.getByRole('heading',{name:slug+' saved',exact:true})).toBeVisible()
  await page.screenshot({path:test.info().outputPath('virtual-server-detail.png'),fullPage:true})
  await page.getByRole('link',{name:'Upstreams',exact:true}).click()
  await page.getByRole('link',{name:slug,exact:true}).click()
  await expect(page.getByLabel('Name',{exact:true})).toBeDisabled()
  await page.getByRole('button',{name:'Edit upstream',exact:true}).click()
  await page.getByLabel('Name',{exact:true}).fill(slug+' saved')
  await page.getByRole('button',{name:'Save',exact:true}).click()
  await expect(page.getByRole('heading',{name:slug+' saved',exact:true})).toBeVisible()
  await expect(page.getByLabel('Name',{exact:true})).toBeDisabled()
  await page.screenshot({path:test.info().outputPath('upstream-detail.png'),fullPage:true})
  const result=await request.get(`${api}/virtual-servers`,{headers})
  const saved=(await result.json()).items.find((item:{slug:string})=>item.slug===slug)
  expect(saved.tools[0].alias).toBe('sum')
  const storedTest=await request.post(`${api}/upstreams/${slug}/connection-test`,{headers})
  expect(storedTest.ok(),await storedTest.text()).toBe(true)
  expect((await storedTest.json()).ok).toBe(true) // Unchanged write-only credentials survive editing.
 } finally {
  if(serverCreated)expect((await request.delete(`${api}/virtual-servers/${slug}`,{headers})).ok()).toBe(true)
  if(upstreamCreated)expect((await request.delete(`${api}/upstreams/${slug}`,{headers})).ok()).toBe(true)
 }
})
