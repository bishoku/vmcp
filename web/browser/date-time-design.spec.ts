import { auditSettings } from './audit-fixtures'
import { expect, test, type Page } from '@playwright/test'
import { server, key } from '../src/connections/fixtures'
test.use({viewport:{width:1280,height:1000},timezoneId:'Europe/Istanbul'})
async function setup(page:Page){
 const writes:{path:string;body:Record<string,unknown>}[]=[],queries:URL[]=[]
 await page.route('**/api/v1/**',route=>{
 if(new URL(route.request().url()).pathname.endsWith('/audit-settings'))return route.fulfill({json:auditSettings})

  const req=route.request(),url=new URL(req.url()),path=url.pathname
  if(['POST','PUT','PATCH','DELETE'].includes(req.method())){
   const body=req.postDataJSON();writes.push({path,body})
   return route.fulfill({json:path.includes('agent-keys')?{...key,...body}:{slug:'pipeline',token:'mock-once',scope:'read',version:1,...body}})
  }
  if(path.endsWith('/audit-events'))queries.push(url)
  const json=path.endsWith('/me')?{name:'Tester',role:'admin'}:path.endsWith('/info')?{version:'test',configRevision:1}:path.endsWith('/agent-keys')?{items:[{...key,expiresAt:'2027-02-03T04:05:06.789Z'}]}:path.endsWith('/virtual-servers')?{items:[server]}:{items:[]}
  return route.fulfill({json})
 })
 return {writes,queries}
}
async function openKey(page:Page){await page.goto('/agent-keys');await page.getByRole('button',{name:'team-key',exact:true}).click();await page.getByRole('button',{name:'Edit key',exact:true}).click()}

test('calendar navigation, cancellation and Escape preserve exact saved expiry until an explicit edit',async({page})=>{
 const {writes}=await setup(page);await openKey(page)
 const input=page.getByLabel('Expires',{exact:true}),trigger=page.getByRole('button',{name:'Choose expires date and time'})
 await expect(input).toHaveValue('2027-02-03T07:05')
 await trigger.click()
 const picker=page.getByRole('dialog',{name:'Expires date and time',exact:true})
 expect(await picker.evaluate(el=>el.matches(':popover-open'))).toBe(true)
 await expect(picker.getByRole('button',{name:'Wednesday, February 3, 2027'})).toBeFocused()
 await page.keyboard.press('ArrowRight');await expect(picker.getByRole('button',{name:'Thursday, February 4, 2027'})).toBeFocused();await page.keyboard.press('Enter')
 await picker.getByLabel('Time',{exact:true}).fill('14:45')
 await page.screenshot({path:test.info().outputPath('date-time-calendar-desktop.png'),animations:'disabled'})
 await page.keyboard.press('Escape');await expect(picker).toHaveCount(0);await expect(trigger).toBeFocused()
 await expect(page.getByRole('dialog',{name:'Edit team-key',exact:true})).toBeVisible()
 await expect(input).toHaveValue('2027-02-03T07:05')
 await trigger.click();await picker.getByRole('button',{name:'Next month',exact:true}).click();await expect(picker.getByText('March 2027',{exact:true})).toBeVisible()
 await picker.getByRole('button',{name:'Cancel',exact:true}).click()
 await page.getByLabel('Description',{exact:true}).fill('Keep original precision')
 expect(writes).toHaveLength(0)
 await page.getByRole('button',{name:'Save',exact:true}).click()
 expect(writes[0].body.expiresAt).toBe('2027-02-03T04:05:06.789Z')
})

test('mobile expiry picker stays inside the viewport, applies local time and saves only on request',async({page})=>{
 await page.setViewportSize({width:390,height:844})
 const {writes}=await setup(page);await openKey(page)
 await page.getByRole('button',{name:'Choose expires date and time'}).click()
 const picker=page.getByRole('dialog',{name:'Expires date and time',exact:true})
 await picker.getByRole('button',{name:'Thursday, February 4, 2027'}).click();await picker.getByLabel('Time',{exact:true}).fill('14:45')
 const bounds=await picker.boundingBox();expect(bounds!.x).toBeGreaterThanOrEqual(0);expect(bounds!.x+bounds!.width).toBeLessThanOrEqual(390);expect(bounds!.y+bounds!.height).toBeLessThanOrEqual(844)
 await page.screenshot({path:test.info().outputPath('date-time-calendar-mobile.png'),animations:'disabled'})
 await picker.getByRole('button',{name:'Apply',exact:true}).click()
 await expect(page.getByLabel('Expires',{exact:true})).toHaveValue('2027-02-04T14:45');expect(writes).toHaveLength(0)
 await page.getByRole('button',{name:'Save',exact:true}).click()
 expect(writes[0].body.expiresAt).toBe('2027-02-04T11:45:00.000Z')
})

test('audit rejects reversed ranges before querying and clearing a bound recovers the filter',async({page})=>{
 const {queries,writes}=await setup(page);await page.goto('/audit')
 await expect.poll(()=>queries.length).toBe(1)
 await page.getByLabel('From',{exact:true}).fill('2027-02-04T14:45')
 await expect.poll(()=>queries.length).toBe(2)
 expect(queries.at(-1)!.searchParams.get('from')).toBe('2027-02-04T11:45:00.000Z')
 await page.getByLabel('To',{exact:true}).fill('2027-02-03T14:45')
 await expect(page.getByLabel('To',{exact:true})).toHaveAttribute('aria-invalid','true')
 await expect(page.getByText('End time must be on or after the start time.')).toBeVisible()
 await page.getByRole('button',{name:'Refresh',exact:true}).click()
 expect(queries).toHaveLength(2)
 await page.screenshot({path:test.info().outputPath('date-time-audit-error.png'),animations:'disabled'})
 await page.getByRole('button',{name:'Choose to date and time'}).click()
 await page.getByRole('dialog',{name:'To date and time',exact:true}).getByRole('button',{name:'Clear date',exact:true}).click()
 await expect(page.getByLabel('To',{exact:true})).toHaveValue('');await expect.poll(()=>queries.length).toBe(3)
 expect(queries.at(-1)!.searchParams.has('to')).toBe(false);expect(writes).toHaveLength(0)
})

test('token expiry errors stay linked to the input and explicit clearing removes the expiry',async({page})=>{
 await setup(page)
 let body:Record<string,unknown>|undefined
 await page.route('**/api/v1/admin-tokens',route=>{
  if(route.request().method()!=='POST')return route.fallback()
  body=route.request().postDataJSON()
  return route.fulfill({status:422,json:{detail:'Validation failed',code:'validation_failed',errors:[{field:'/expiresAt',message:'Expiry exceeds the permitted lifetime.'}]}})
 })
 await page.goto('/settings');await page.getByRole('button',{name:'New token',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'New admin token',exact:true})
 await dialog.getByLabel('Name',{exact:true}).fill('pipeline')
 await dialog.getByLabel('Expires at',{exact:true}).fill('2027-02-04T14:45')
 await dialog.getByRole('button',{name:'Create',exact:true}).click()
 await expect(dialog.getByLabel('Expires at',{exact:true})).toBeFocused();await expect(dialog.getByLabel('Expires at',{exact:true})).toHaveAttribute('aria-invalid','true')
 expect(body!.expiresAt).toBe('2027-02-04T11:45:00.000Z')
 await dialog.getByRole('button',{name:'Choose expires at date and time'}).click()
 await page.getByRole('dialog',{name:'Expires at date and time',exact:true}).getByRole('button',{name:'No expiry',exact:true}).click()
 await expect(dialog.getByLabel('Expires at',{exact:true})).toHaveValue('');await expect(dialog.getByLabel('Expires at',{exact:true})).not.toHaveAttribute('aria-invalid','true')
 await dialog.getByRole('button',{name:'Create',exact:true}).click();expect(body!.expiresAt).toBeNull()
})

test.describe('daylight saving time',()=>{
 test.use({timezoneId:'America/New_York'})
 test('nonexistent local times cannot be applied or saved',async({page})=>{
  const {writes}=await setup(page);await openKey(page)
  const input=page.getByLabel('Expires',{exact:true})
  await input.fill('2027-03-14T02:30');await page.getByRole('button',{name:'Save',exact:true}).click()
  await expect(input).toHaveAttribute('aria-invalid','true');expect(writes).toHaveLength(0)
  await input.fill('2027-03-14T01:30');await page.getByRole('button',{name:'Choose expires date and time'}).click()
  const picker=page.getByRole('dialog',{name:'Expires date and time',exact:true})
  await picker.getByLabel('Time',{exact:true}).fill('02:30')
  await expect(picker.getByRole('button',{name:'Apply',exact:true})).toBeDisabled()
  await expect(picker.getByText('Enter a valid local time. Some times do not exist when clocks change.')).toBeVisible()
  expect(writes).toHaveLength(0)
 })
})
