import { auditSettings } from './audit-fixtures'
import { expect, test, type Page } from '@playwright/test'
import { server, upstream } from '../src/connections/fixtures'

test.use({viewport:{width:1280,height:1000}})

async function base(page:Page){
 const writes:string[]=[]
 page.on('request',request=>{if(['POST','PUT','PATCH','DELETE'].includes(request.method()))writes.push(request.url())})
 await page.route('**/api/v1/**',route=>new URL(route.request().url()).pathname.endsWith('/audit-settings')?route.fulfill({json:auditSettings}):route.fulfill({json:route.request().url().endsWith('/me')?{name:'Tester',role:'admin'}:{items:[]}}))
 return writes
}

test('first load resolves to a useful empty state with consistent mobile presentation',async({page})=>{
 const writes=await base(page)
 let release!:()=>void
 const pending=new Promise<void>(resolve=>{release=resolve})
 await page.route(/\/api\/v1\/upstreams(?:\?.*)?$/,async route=>{await pending;await route.fulfill({json:{items:[]}})})
 await page.goto('/upstreams')
 await expect(page.getByTestId('loading-state')).toContainText('Loading server inventory…')
 await expect(page.getByText(/No upstreams yet/)).toHaveCount(0)
 await page.screenshot({path:test.info().outputPath('inventory-loading.png'),animations:'disabled'})
 release()
 await expect(page.getByTestId('empty-state')).toContainText('No upstreams yet.')
 await expect(page.getByRole('button',{name:'Create an upstream',exact:true})).toBeVisible()
 await page.screenshot({path:test.info().outputPath('inventory-empty.png'),animations:'disabled'})
 await page.setViewportSize({width:390,height:844})
 await page.getByTestId('empty-state').evaluate(el=>el.scrollIntoView({block:'center'}))
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath('inventory-empty-mobile.png'),animations:'disabled'})
 expect(writes).toEqual([])
})

test('failed refresh retains server cards and explicit retry recovers without mutations',async({page})=>{
 const writes=await base(page)
 let fail=false
 await page.route(/\/api\/v1\/upstreams(?:\?.*)?$/,route=>route.fulfill({status:fail?503:200,json:fail?{detail:'Inventory temporarily offline'}:{items:[upstream]}}))
 await page.goto('/upstreams');await expect(page.getByRole('article',{name:'Alpha',exact:true})).toBeVisible()
 fail=true;await page.getByRole('button',{name:'Refresh list',exact:true}).click()
 await expect(page.getByRole('alert')).toContainText('Inventory temporarily offline')
 await expect(page.getByRole('article',{name:'Alpha',exact:true})).toBeVisible()
 await page.screenshot({path:test.info().outputPath('inventory-retained-error.png'),animations:'disabled'})
 fail=false;await page.getByRole('alert').getByRole('button',{name:'Try again',exact:true}).click()
 await expect(page.getByRole('alert')).toHaveCount(0)
 await expect(page.getByRole('article',{name:'Alpha',exact:true})).toBeVisible()
 expect(writes).toEqual([])
})

test('unavailable saved details retry in place and missing identifiers have a distinct empty state',async({page})=>{
 const writes=await base(page)
 let fail=true
 await page.route(/\/api\/v1\/virtual-servers(?:\?.*)?$/,route=>route.fulfill({status:fail?503:200,json:fail?{detail:'Configuration unavailable'}:{items:[server]}}))
 await page.goto('/virtual-servers/demo/overview')
 await expect(page.getByRole('alert')).toContainText('Configuration unavailable')
 await page.getByRole('alert').getByRole('button',{name:'Try again',exact:true}).click()
 await expect(page.getByRole('alert')).toBeVisible()
 fail=false;await page.getByRole('alert').getByRole('button',{name:'Try again',exact:true}).click()
 await expect(page.getByRole('region',{name:'Server overview',exact:true})).toBeVisible()
 await page.goto('/virtual-servers/missing/overview')
 await expect(page.getByTestId('empty-state')).toContainText('Virtual server “missing” was not found.')
 await expect(page.getByRole('link',{name:'Back to virtual servers',exact:true})).toBeVisible()
 expect(writes).toEqual([])
})

test('audit failure is not presented as no events and retry preserves query filters',async({page})=>{
 const writes=await base(page)
 let fail=true
 const operations:string[]=[]
 await page.route('**/api/v1/audit-events?*',route=>{
  operations.push(new URL(route.request().url()).searchParams.get('operationId')??'')
  return route.fulfill({status:fail?503:200,json:fail?{detail:'Audit storage offline'}:{items:[]}})
 })
 await page.goto('/audit?operationId=op-retry')
 await expect(page.getByTestId('empty-state')).toContainText('Audit events unavailable')
 await expect(page.getByText('No events match.',{exact:true})).toHaveCount(0)
 fail=false;await page.getByRole('alert').getByRole('button',{name:'Try again',exact:true}).click()
 await expect(page.getByTestId('empty-state')).toContainText('No events match.')
 await expect(page.getByLabel('Operation ID',{exact:true})).toHaveValue('op-retry')
 expect(operations).toEqual(['op-retry','op-retry'])
 expect(writes).toEqual([])
})

test('retrying failed editor catalog reads preserves an unsaved draft',async({page})=>{
 const writes=await base(page)
 let fail=true
 await page.route(/\/api\/v1\/virtual-servers(?:\?.*)?$/,route=>route.fulfill({json:{items:[server]}}))
 await page.route(/\/api\/v1\/upstreams(?:\?.*)?$/,route=>route.fulfill({status:fail?503:200,json:fail?{detail:'Catalog offline'}:{items:[upstream]}}))
 await page.route(/\/api\/v1\/upstreams\/alpha\/tools(?:\?.*)?$/,route=>route.fulfill({json:{items:[{name:'lookup'}]}}))
 await page.route('**/api/v1/upstreams/alpha/catalog',route=>route.fulfill({json:{resources:[],resourceTemplates:[],prompts:[],capabilities:{}}}))
 await page.goto('/virtual-servers/demo/edit')
 await page.getByLabel('Name',{exact:true}).fill('Retained draft')
 await expect(page.getByRole('alert')).toContainText('Catalog offline')
 fail=false;await page.getByRole('alert').getByRole('button',{name:'Try again',exact:true}).click()
 await expect(page.getByRole('alert')).toHaveCount(0)
 await expect(page.getByLabel('Name',{exact:true})).toHaveValue('Retained draft')
 await expect(page.getByLabel('Alias for alpha/lookup',{exact:true})).toHaveValue('lookup')
 expect(writes).toEqual([])
})
