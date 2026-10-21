import { auditSettings } from './audit-fixtures'
import { expect, test, type Page } from '@playwright/test'
import { server, upstream, key, binding } from '../src/connections/fixtures'
test.use({viewport:{width:1280,height:1000}})
async function setup(page:Page){
 const writes:string[]=[],queries:URL[]=[]
 const servers=Array.from({length:55},(_,i)=>({...server,slug:`workspace-${i}`,name:`Workspace ${i}`,enabled:i%2===0}))
 const tokens=[{slug:'pipeline',scope:'read',status:'active',version:1,expiresAt:null,revokedAt:null,lastUsedAt:null,createdAt:'2026-10-15T10:00Z',tokenPrefix:'vmcp_admin_',description:'Build pipeline'}]
 const audit=[{id:1,eventId:'e-1',operationId:'op-1',stage:'admission',status:'started',time:'2026-10-15T10:00:00Z',method:'tools/call',tool:'lookup',virtualServer:'workspace-0'},{id:2,eventId:'e-2',operationId:'op-1',stage:'outcome',status:'ok',time:'2026-10-15T10:00:01Z',method:'tools/call',tool:'lookup',virtualServer:'workspace-0'}]
 await page.route('**/api/v1/**',route=>{
 if(new URL(route.request().url()).pathname.endsWith('/audit-settings'))return route.fulfill({json:auditSettings})

  const req=route.request(),url=new URL(req.url()),path=url.pathname
  if(['POST','PUT','PATCH','DELETE'].includes(req.method()))writes.push(path)
  if(path.endsWith('/audit-events'))queries.push(url)
  const json=path.endsWith('/me')?{name:'Tester',role:'admin'}:path.endsWith('/info')?{version:'test',configRevision:1}:path.endsWith('/upstreams')?{items:[upstream]}:path.endsWith('/virtual-servers')?{items:servers}:path.endsWith('/agent-keys')?{items:[{...key,virtualServers:['workspace-0','missing-server']}]}:path.endsWith('/identity-bindings')?{items:[binding]}:path.endsWith('/admin-tokens')?{items:tokens}:path.endsWith('/audit-events')?{items:audit,nextCursor:'older'}:{items:[]}
  return route.fulfill({json})
 })
 return {writes,queries}
}

test('inventory search clearing preserves status, filter removal resets pagination and counts cover all matches',async({page})=>{
 const {writes}=await setup(page);await page.goto('/virtual-servers')
 await page.getByRole('button',{name:'Next',exact:true}).click()
 const search=page.getByRole('searchbox',{name:'Search virtual servers',exact:true})
 await page.getByRole('combobox',{name:'Virtual server status',exact:true}).selectOption('enabled')
 await search.fill('Workspace 1')
 await expect(page.getByRole('status',{name:'servers result count'})).toHaveText('5 of 55 servers')
 await expect(page.getByRole('article')).toHaveCount(5)
 await page.getByRole('button',{name:'Clear virtual servers search',exact:true}).click()
 await expect(search).toBeFocused();await expect(search).toHaveValue('')
 await expect(page.getByRole('combobox',{name:'Virtual server status',exact:true})).toHaveValue('enabled')
 await expect(page.getByRole('status',{name:'servers result count'})).toHaveText('28 of 55 servers')
 await page.getByRole('button',{name:'Remove Status filter',exact:true}).click()
 await expect(search).toBeFocused();await expect(page.getByRole('status',{name:'servers result count'})).toHaveText('55 of 55 servers')
 await page.getByRole('button',{name:'Next',exact:true}).click();await search.fill('Workspace 1')
 await expect(page.getByRole('article')).toHaveCount(11)
 await page.getByRole('region',{name:'Find virtual servers',exact:true}).scrollIntoViewIfNeeded()
 await page.screenshot({path:test.info().outputPath('search-filters-desktop.png'),animations:'disabled'})
 await page.setViewportSize({width:390,height:844});await search.fill('An exceptionally long filter value '.repeat(8))
 const filters=page.getByRole('region',{name:'Find virtual servers',exact:true})
 await filters.scrollIntoViewIfNeeded()
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 const chip=page.getByRole('button',{name:'Remove Search filter',exact:true})
 expect((await chip.boundingBox())!.height).toBeGreaterThanOrEqual(44)
 expect((await chip.boundingBox())!.height).toBeLessThanOrEqual(80)
 await page.screenshot({path:test.info().outputPath('search-filters-mobile.png'),animations:'disabled'})
 await page.getByRole('button',{name:'Clear filters',exact:true}).click();await expect(search).toBeFocused()
 await expect(page.getByRole('status',{name:'servers result count'})).toHaveText('55 of 55 servers')
 expect(writes).toHaveLength(0)
})

test('Escape and reset clear modal search without dismissing the modal or changing selected grants',async({page})=>{
 const {writes}=await setup(page);await page.goto('/agent-keys')
 await page.getByRole('button',{name:'team-key',exact:true}).click();await page.getByRole('button',{name:'Edit key',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'Edit team-key',exact:true}),search=dialog.getByRole('searchbox',{name:'Search virtual servers',exact:true})
 await search.fill('nomatch');await expect(dialog.getByRole('status',{name:'servers result count'})).toHaveText('0 of 56 servers')
 await search.press('Escape');await expect(search).toHaveValue('');await expect(search).toBeFocused();await expect(dialog).toBeVisible()
 await expect(dialog.getByRole('checkbox',{name:'Workspace 0',exact:true})).toBeChecked()
 await search.fill('Workspace 0');await dialog.getByRole('button',{name:'Clear filters',exact:true}).click()
 await expect(search).toBeFocused();await search.fill('missing-server');await expect(dialog.getByRole('checkbox',{name:'missing-server',exact:true})).toBeChecked()
 await page.setViewportSize({width:390,height:844});await search.fill('Workspace 0');await search.scrollIntoViewIfNeeded()
 await page.screenshot({path:test.info().outputPath('search-picker-mobile.png'),animations:'disabled'})
 expect(writes).toHaveLength(0)
})

test('audit chips remove only their filter and counts explicitly refer to loaded records',async({page})=>{
 const {writes,queries}=await setup(page);await page.goto('/audit?virtualServer=workspace-0&actorId=agent-1')
 await expect(page.getByRole('status',{name:'records result count'})).toHaveText('2 loaded records')
 const chips=page.getByRole('group',{name:'Active audit filters',exact:true})
 await chips.getByRole('button',{name:'Remove Virtual server filter',exact:true}).click()
 await expect.poll(()=>queries.at(-1)?.searchParams.has('virtualServer')).toBe(false)
 expect(queries.at(-1)!.searchParams.get('actorId')).toBe('agent-1')
 await page.getByRole('button',{name:'Clear all filters',exact:true}).click()
 await expect.poll(()=>queries.at(-1)?.searchParams.has('actorId')).toBe(false)
 await expect(chips).toHaveCount(0)
 await expect(page.getByRole('status',{name:'records result count'})).toHaveText('2 loaded records')
 expect(writes).toHaveLength(0)
})

test('Sankey search clearing retains its drill-down independently of relationship filter reset',async({page})=>{
 const {writes}=await setup(page);await page.goto('/connections')
 const flow=page.getByRole('region',{name:'Connection flow',exact:true})
 await flow.getByRole('combobox',{name:'Explore server',exact:true}).selectOption('["upstream","alpha"]')
 const search=flow.getByRole('searchbox',{name:'Find tool mapping',exact:true})
 await search.fill('nomatch');await expect(flow.getByRole('status',{name:'mappings result count'})).toContainText('0 of')
 await search.press('Escape');await expect(search).toHaveValue('');await expect(flow.getByRole('combobox',{name:'Explore server',exact:true})).toHaveValue('["upstream","alpha"]')
 await page.getByRole('searchbox',{name:'Filter relationships',exact:true}).fill('workspace')
 await page.getByRole('button',{name:'Clear relationship filters',exact:true}).click()
 await expect(flow.getByRole('combobox',{name:'Explore server',exact:true})).toHaveValue('["upstream","alpha"]')
 expect(writes).toHaveLength(0)
})
