import { expect, test, type Page } from '@playwright/test'
import { upstream, server, key } from '../src/connections/fixtures'
test.use({viewport:{width:1280,height:1000}})
async function setup(page:Page){
 const writes:string[]=[]
 const upstreams=[upstream,{...upstream,slug:'beta',name:'Beta',health:{...upstream.health!,state:'unhealthy'},lastRefreshError:'Stored discovery failed'},{...upstream,slug:'gamma',name:'Gamma',enabled:false,lastRefreshAt:null,health:undefined}]
 const keys=[key,{...key,slug:'expired-key',expiresAt:'2020-01-01T00:00:00Z'},{...key,slug:'revoked-key',status:'revoked',revokedAt:'2026-01-01T00:00:00Z'}]
 const events=[{id:1,eventId:'event-1',operationId:'pending',time:'2026-10-15T10:00Z',stage:'admission',status:'started',tool:'lookup',method:'tools/call',virtualServer:'demo'},{id:2,eventId:'event-2',operationId:'failed',time:'2026-10-15T10:00Z',stage:'outcome',status:'error',tool:'search',method:'tools/call',virtualServer:'demo'}]
 await page.route('**/api/v1/**',route=>{
  const req=route.request(),path=new URL(req.url()).pathname
  if(['POST','PUT','PATCH','DELETE'].includes(req.method()))writes.push(path)
  const json=path.endsWith('/me')?{name:'Tester',role:'admin'}:path.endsWith('/info')?{version:'test',configRevision:1}:path.endsWith('/upstreams')?{items:upstreams}:path.endsWith('/virtual-servers')?{items:[server]}:path.endsWith('/agent-keys')?{items:keys}:path.endsWith('/audit-events')?{items:events}:{items:[]}
  return route.fulfill({json})
 })
 return writes
}

test('inventory badges share semantic tones, decorative icons and readable contrast',async({page})=>{
 const writes=await setup(page);await page.goto('/upstreams')
 await expect(page.getByRole('article',{name:'Alpha',exact:true}).locator('[data-state="enabled"]')).toHaveAttribute('data-tone','success')
 await expect(page.getByRole('article',{name:'Beta',exact:true}).locator('[data-state="refresh failed"]')).toHaveAttribute('data-tone','danger')
 await expect(page.getByRole('article',{name:'Gamma',exact:true}).locator('[data-state="not refreshed"]')).toHaveAttribute('data-tone','neutral')
 await expect(page.getByRole('article',{name:'Alpha',exact:true}).locator('[data-state="snapshot available"]')).toHaveAttribute('data-tone','info')
 await page.screenshot({path:test.info().outputPath('status-badges-inventory.png'),animations:'disabled'})
 const contrast=await page.locator('.badge').evaluateAll(nodes=>{
  const luminance=(value:string)=>{const colors=value.match(/[\d.]+/g)!.slice(0,3).map(Number).map(c=>{const v=c/255;return v<=.04045?v/12.92:((v+.055)/1.055)**2.4});return colors[0]*.2126+colors[1]*.7152+colors[2]*.0722}
  return nodes.map(node=>{const style=getComputedStyle(node),a=luminance(style.color),b=luminance(style.backgroundColor);return (Math.max(a,b)+.05)/(Math.min(a,b)+.05)})
 })
 for(const value of contrast)expect(value).toBeGreaterThanOrEqual(4.5)
 for(const badge of await page.locator('.badge').all())await expect(badge.locator('svg')).toHaveAttribute('aria-hidden','true')
 expect(writes).toHaveLength(0)
})

test('mobile key states remain distinct and static badges never add live status regions',async({page})=>{
 await page.setViewportSize({width:390,height:844})
 const writes=await setup(page);await page.goto('/agent-keys')
 const table=page.getByRole('table',{name:'Configured agent keys',exact:true})
 for(const [state,tone] of [['active','success'],['expired','warning'],['revoked','danger']])await expect(table.locator(`[data-state="${state}"]`)).toHaveAttribute('data-tone',tone)
 await table.scrollIntoViewIfNeeded();await page.screenshot({path:test.info().outputPath('status-badges-mobile.png'),animations:'disabled'})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 expect(await table.locator('.badge').evaluateAll(nodes=>nodes.every(node=>node.scrollWidth<=node.clientWidth+1))).toBe(true)
 await expect(page.locator('.badge[role="status"]')).toHaveCount(0)
 expect(writes).toHaveLength(0)
})

test('capability discovery is informational and missing audit outcomes stay warnings rather than failures',async({page})=>{
 const writes=await setup(page);await page.goto('/virtual-servers/demo/capabilities')
 const capabilities=page.getByRole('region',{name:'Selected capabilities',exact:true})
 await expect(capabilities.locator('[data-state="discovered"]').first()).toHaveAttribute('data-tone','info')
 await expect(capabilities.locator('[data-state="missing object"]')).toHaveAttribute('data-tone','warning')
 await page.goto('/audit')
 await expect(page.locator('[data-state="outcome not loaded"]')).toHaveAttribute('data-tone','warning')
 await expect(page.locator('[data-state="error"]')).toHaveAttribute('data-tone','danger')
 await page.getByRole('region',{name:'Audit events',exact:true}).scrollIntoViewIfNeeded()
 await page.screenshot({path:test.info().outputPath('status-badges-audit.png'),animations:'disabled'})
 await page.getByRole('radio',{name:'Raw events',exact:true}).check()
 await expect(page.locator('[data-state="started"]')).toHaveAttribute('data-tone','info')
 expect(writes).toHaveLength(0)
})
