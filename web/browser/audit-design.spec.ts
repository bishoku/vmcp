import { auditSettings } from './audit-fixtures'
import { expect, test, type Page } from '@playwright/test'

const base={eventId:'event-1',operationId:'operation-123',requestId:'request-123',traceId:'trace-123',schemaVersion:1,actorType:'agent_key',actorId:'team-key',agentKey:'team-key',subject:'team',issuer:'https://idp.example',action:'tools/call',target:'lookup',configRevision:7,id:5,time:'2026-10-15T10:00:00Z',virtualServer:'demo',method:'tools/call',tool:'lookup',upstream:'alpha',status:'started',stage:'admission',durationMs:0,redactions:0}
const outcome={...base,eventId:'event-2',id:6,stage:'outcome',status:'error',reason:'Upstream timeout',durationMs:250,redactions:2}
const pending={...base,eventId:'event-3',id:4,operationId:'operation-pending',requestId:'request-pending',tool:'search'}
const mutation={...base,eventId:'event-4',id:3,operationId:null,requestId:null,traceId:'mutation-trace',stage:'mutation',status:'ok',actorType:'admin_token',actorId:'admin-console',agentKey:null,subject:null,virtualServer:'',tool:null,action:'upstream.update',method:'',target:'upstream/alpha',durationMs:0}
async function login(page:Page){
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',route=>{
 if(new URL(route.request().url()).pathname.endsWith('/audit-settings'))return route.fulfill({json:auditSettings})

  const url=new URL(route.request().url())
  if(url.pathname.endsWith('/me'))return route.fulfill({status:route.request().headers().authorization?200:401,json:{name:'Reviewer',role:'viewer'}})
  if(url.pathname.endsWith('/audit-events')){
   if(url.searchParams.get('operationId'))return route.fulfill({json:{items:[outcome,base]}})
   if(url.searchParams.get('cursor'))return route.fulfill({json:{items:[pending,mutation]}})
   return route.fulfill({json:{items:[outcome,base,pending],nextCursor:'older'}})
  }
  return route.fulfill({json:{items:[]}})
 })
 await page.goto('/audit?virtualServer=demo&subject=team')
 await page.getByLabel('Admin token').fill('test-token');await page.getByRole('button',{name:'Continue',exact:true}).click()
 await expect(page.getByRole('button',{name:'Details',exact:true})).toHaveCount(2)
}

test('audit filters list and drawer share responsive panels and operation links preserve sign-in',async({page})=>{
 await login(page)
 await expect(page.getByText('outcome not loaded',{exact:true})).toBeVisible()
 await page.getByRole('heading',{name:'Audit Log',exact:true}).scrollIntoViewIfNeeded()
 await page.screenshot({path:test.info().outputPath('audit-filters.png'),animations:'disabled'})
 await page.getByRole('button',{name:'Hide filters',exact:true}).click()
 await page.getByRole('heading',{name:'Audit Log',exact:true}).scrollIntoViewIfNeeded()
 await page.getByRole('region',{name:'Audit events',exact:true}).evaluate(el=>el.scrollIntoView({block:'center'}))
 await page.screenshot({path:test.info().outputPath('audit-events.png'),animations:'disabled'})
 const details=page.getByRole('button',{name:'Details',exact:true}).first()
 await details.focus();await page.keyboard.press('Enter')
 const drawer=page.getByRole('dialog',{name:'Audit details',exact:true})
 await expect(drawer.getByRole('region',{name:'Event context'})).toBeVisible()
 await expect(drawer.getByRole('button',{name:'Copy operation ID'})).toBeVisible()
 await expect(drawer.locator('pre')).toHaveCount(2)
 await drawer.getByText('Raw stored metadata',{exact:true}).first().click()
 await expect(drawer.locator('pre').first()).toBeVisible()
 expect(JSON.parse(await drawer.locator('pre').first().innerText()).status).toBe('error')
 await drawer.getByRole('heading',{name:'Audit details',exact:true}).scrollIntoViewIfNeeded()
 await page.screenshot({path:test.info().outputPath('audit-detail.png'),animations:'disabled'})
 await page.setViewportSize({width:390,height:844})
 expect(await drawer.locator('section').first().evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
 await drawer.getByRole('heading',{name:'Audit details',exact:true}).scrollIntoViewIfNeeded()
 await page.screenshot({path:test.info().outputPath('audit-detail-mobile.png'),animations:'disabled'})
 await drawer.getByRole('link',{name:'Open operation',exact:true}).click()
 await expect(page).toHaveURL(/audit\?operationId=operation-123$/)
 await expect(page.getByLabel('Operation ID',{exact:true})).toHaveValue('operation-123')
 await expect(page.getByLabel('Admin token')).toHaveCount(0)
 await expect(page.getByRole('button',{name:'Details',exact:true})).toHaveCount(1)
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
})

test('pagination deduplicates records and raw mode preserves all stages',async({page})=>{
 await login(page)
 await page.getByRole('button',{name:'Load more',exact:true}).click()
 await expect(page.getByRole('button',{name:'Details',exact:true})).toHaveCount(3)
 await page.getByRole('radio',{name:'Raw events',exact:true}).click()
 await expect(page.getByRole('button',{name:'Details',exact:true})).toHaveCount(4)
 await page.getByRole('radio',{name:'Operations',exact:true}).click()
 await expect(page.getByRole('button',{name:'Details',exact:true})).toHaveCount(3)
 await page.setViewportSize({width:390,height:844})
 await page.getByRole('heading',{name:'Stored activity',exact:true}).scrollIntoViewIfNeeded()
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath('audit-list-mobile.png'),animations:'disabled'})
})

test('correlation filters copy exact query state and clear without losing the view mode',async({page,context})=>{
 await context.grantPermissions(['clipboard-read','clipboard-write'])
 await login(page)
 await page.getByRole('button',{name:'Correlation filters',exact:true}).click()
 await page.getByLabel('Request ID',{exact:true}).fill('request-123')
 await page.getByLabel('Stage',{exact:true}).selectOption('outcome')
 await page.getByLabel('From',{exact:true}).fill('2026-10-15T09:30')
 await page.getByRole('button',{name:'Copy audit filter link',exact:true}).click()
 const link=new URL(await page.evaluate(()=>navigator.clipboard.readText()))
 expect(link.searchParams.get('requestId')).toBe('request-123')
 expect(link.searchParams.get('stage')).toBe('outcome')
 expect(link.searchParams.get('subject')).toBe('team')
 const from=await page.evaluate(()=>new Date('2026-10-15T09:30').toISOString())
 expect(link.searchParams.get('from')).toBe(from)
 await page.getByRole('button',{name:'Remove Subject filter',exact:true}).click()
 await expect(page.getByLabel('Subject',{exact:true})).toHaveValue('')
 await page.getByRole('radio',{name:'Raw events',exact:true}).click()
 await page.getByRole('button',{name:'Hide filters',exact:true}).click()
 await expect(page.getByRole('button',{name:'Remove Request ID filter',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'Show filters',exact:true}).click()
 await expect(page.getByLabel('Request ID',{exact:true})).toHaveValue('request-123')
 await page.getByRole('button',{name:'Clear all filters',exact:true}).click()
 await expect(page.getByLabel('Request ID',{exact:true})).toHaveValue('')
 await expect(page.getByLabel('Stage',{exact:true})).toHaveValue('')
 await expect(page.getByRole('radio',{name:'Raw events',exact:true})).toBeChecked()
})
