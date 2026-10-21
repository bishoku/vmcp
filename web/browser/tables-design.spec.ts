import { auditSettings } from './audit-fixtures'
import { expect, test, type Page } from '@playwright/test'
import { key, server, upstream } from '../src/connections/fixtures'
import { answerConfirmation } from './confirmation'

test.use({viewport:{width:1280,height:1000}})
async function inventory(page:Page){
 const writes:string[]=[]
 await page.route('**/api/v1/**',route=>{
 if(new URL(route.request().url()).pathname.endsWith('/audit-settings'))return route.fulfill({json:auditSettings})

  const request=route.request(),path=new URL(request.url()).pathname
  if(['POST','PATCH','PUT','DELETE'].includes(request.method()))writes.push(path)
  const json=path.endsWith('/me')?{name:'Tester',role:'admin'}:path.endsWith('/agent-keys')?{items:Array.from({length:110},(_,i)=>({...key,slug:`agent-${i}`,description:'Customer support workspace',expiresAt:i%2?'2027-06-01T00:00:00Z':null}))}:path.endsWith('/virtual-servers')?{items:Array.from({length:60},(_,i)=>({...server,slug:`workspace-${i}`,name:`Workspace ${i}`}))}:path.endsWith('/upstreams')?{items:[upstream]}:{items:[]}
  return route.fulfill({json})
 })
 return writes
}

test('inventory table sorts all filtered rows before limiting, restores order and preserves confirmations',async({page})=>{
 const writes=await inventory(page);await page.goto('/agent-keys')
 const table=page.getByRole('table',{name:'Configured agent keys'})
 await expect(table.getByRole('row')).toHaveCount(51)
 const keyHeader=table.getByRole('columnheader',{name:'Key',exact:true})
 await keyHeader.getByRole('button',{name:'Key',exact:true}).focus()
 await page.keyboard.press('Enter')
 await expect(keyHeader).toHaveAttribute('aria-sort','ascending')
 await keyHeader.getByRole('button',{name:'Key',exact:true}).click()
 await expect(keyHeader).toHaveAttribute('aria-sort','descending')
 await expect(table.getByRole('row').nth(1)).toContainText('agent-109')
 await page.getByRole('button',{name:'Show 50 more keys',exact:true}).click()
 await expect(table.getByRole('row')).toHaveCount(101)
 await expect(table.getByRole('row').nth(1)).toContainText('agent-109')
 await page.getByLabel('Search agent keys',{exact:true}).fill('agent-109')
 await expect(table.getByRole('row')).toHaveCount(2)
 await page.getByRole('button',{name:'Reset sorting',exact:true}).click()
 await table.getByRole('button',{name:'Delete',exact:true}).click()
 await answerConfirmation(page,false)
 expect(writes).toEqual([])
 await page.getByLabel('Search agent keys',{exact:true}).fill('')
 await page.locator('#main-content').evaluate(el=>el.scrollTop=0)
 await page.screenshot({path:test.info().outputPath('tables-agent-keys.png'),animations:'disabled'})
 await page.setViewportSize({width:390,height:844})
 await page.getByRole('combobox',{name:'Sort Configured agent keys',exact:true}).selectOption('Key|desc')
 const first=table.getByRole('row').first()
 await expect(first).toContainText('agent-109')
 await expect(first.getByRole('button',{name:'Rotate',exact:true})).toBeVisible()
 await expect(first.getByRole('button',{name:'Delete',exact:true})).toBeVisible()
 await first.scrollIntoViewIfNeeded()
 expect(await table.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath('tables-agent-keys-mobile.png'),animations:'disabled'})
})

test('card pagination shares numbered controls and sorting resets to the first page',async({page})=>{
 const writes=await inventory(page);await page.goto('/virtual-servers')
 const pagination=page.getByRole('navigation',{name:'Configured virtual servers pagination',exact:true})
 await pagination.getByRole('button',{name:'Next',exact:true}).click()
 await expect(pagination.getByRole('button',{name:'Page 2',exact:true})).toHaveAttribute('aria-current','page')
 await expect(page.getByRole('article').first()).toContainText('Workspace 24')
 await page.getByRole('combobox',{name:'Sort Configured virtual servers',exact:true}).selectOption('Name|desc')
 await expect(pagination.getByRole('button',{name:'Page 1',exact:true})).toHaveAttribute('aria-current','page')
 await expect(page.getByRole('article').first()).toContainText('Workspace 59')
 await pagination.scrollIntoViewIfNeeded()
 await page.screenshot({path:test.info().outputPath('pagination-servers.png'),animations:'disabled'})
 await page.setViewportSize({width:390,height:844})
 await pagination.getByRole('button',{name:'Next',exact:true}).click()
 await expect(pagination).toContainText('Page 2 of 3')
 await page.getByLabel('Search virtual servers',{exact:true}).fill('workspace-59')
 await expect(page.getByRole('article')).toHaveCount(1)
 await expect(page.getByRole('article').first()).toContainText('Workspace 59')
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 expect(writes).toEqual([])
})

test('audit sorting preserves operation groups and cursor loads without inventing a total',async({page})=>{
 let reads=0
 const base={virtualServer:'demo',agentKey:'team-key',time:'2026-10-15T12:00:00Z',tool:'lookup',redactions:0}
 await page.route('**/api/v1/**',route=>{
 if(new URL(route.request().url()).pathname.endsWith('/audit-settings'))return route.fulfill({json:auditSettings})

  const path=new URL(route.request().url()).pathname
  if(path.endsWith('/me'))return route.fulfill({json:{name:'Tester',role:'viewer'}})
  if(path.endsWith('/audit-events')){
   reads++
   const more=new URL(route.request().url()).searchParams.has('cursor')
   return route.fulfill({json:more?{items:[{...base,id:3,operationId:'op-b',stage:'outcome',status:'ok',durationMs:30}],nextCursor:null}:{items:[{...base,id:2,operationId:'op-a',stage:'outcome',status:'ok',durationMs:10},{...base,id:1,operationId:'op-b',stage:'admission',status:'started',durationMs:0}],nextCursor:'older'}})
  }
  return route.fulfill({json:{items:[]}})
 })
 await page.goto('/audit')
 const table=page.getByRole('table',{name:'Loaded audit operations'})
 await expect(table.getByRole('row')).toHaveCount(3)
 await table.getByRole('columnheader',{name:'Duration',exact:true}).getByRole('button').click()
 await table.getByRole('columnheader',{name:'Duration',exact:true}).getByRole('button').click()
 await expect(table.getByRole('row').nth(2)).toContainText('outcome not loaded')
 expect(reads).toBe(1)
 await page.getByRole('button',{name:'Load more',exact:true}).click()
 await expect(table.getByRole('row')).toHaveCount(3)
 await expect(table.getByRole('row').nth(1)).toContainText('30 ms')
 await expect(table.getByRole('row').nth(1)).toContainText('2 records')
 await expect(page.getByRole('button',{name:'Load more',exact:true})).toHaveCount(0)
 expect(reads).toBe(2)
 await page.setViewportSize({width:390,height:844})
 await table.getByRole('row').first().scrollIntoViewIfNeeded()
 expect(await table.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath('tables-audit-mobile.png'),animations:'disabled'})
 await table.getByRole('button',{name:'Details',exact:true}).first().click()
 await expect(page.getByRole('dialog',{name:'Audit details',exact:true})).toBeVisible()
})
