import {expect,test} from '@playwright/test'
import {auditSettings} from './audit-fixtures'

for(const width of [390,1440])test(`audit activity filters retain virtual call chains at ${width}px`,async({page})=>{
 await page.setViewportSize({width,height:900})
 const base={time:'2026-10-15T10:00:00Z',schemaVersion:1,virtualServer:'same-server',method:'tools/call',tool:'createOrder',status:'ok',durationMs:10,stage:'outcome',actorType:'agent_key',actorId:'bot',redactions:0,payloadState:'disabled'}
 const events=[{...base,id:4,eventId:'child',scope:'upstream',boundary:'upstream',operationId:'child',parentOperationId:'root'},{...base,id:3,eventId:'root',scope:'virtual',boundary:'virtual',operationId:'root'}]
 const queries:URLSearchParams[]=[]
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',r=>{
  const url=new URL(r.request().url())
  if(url.pathname.endsWith('/me'))return r.fulfill({json:{name:'Viewer',role:'viewer'}})
  if(url.pathname.endsWith('/audit-settings'))return r.fulfill({json:auditSettings})
  if(url.pathname.endsWith('/audit-events')){
   queries.push(url.searchParams)
   const activity=url.searchParams.get('activity')
   return r.fulfill({json:{items:activity==='virtual'?events:activity==='system'?[{...base,id:2,eventId:'system',scope:'system',operationId:'system',tool:null,action:'upstream.discovery'}]:[],nextCursor:activity==='virtual'&&!url.searchParams.has('cursor')?'3':null}})
  }
  return r.fulfill({json:{items:[]}})
 })
 await page.goto('/audit?activity=virtual')
 await expect(page.getByLabel('Activity',{exact:true})).toHaveValue('virtual')
 await expect(page.getByRole('button',{name:'Details',exact:true})).toHaveCount(1)
 expect(queries.at(-1)?.get('activity')).toBe('virtual')
 await page.getByRole('radio',{name:'Raw events',exact:true}).click()
 await expect(page.getByRole('button',{name:'Details',exact:true})).toHaveCount(2)
 await page.context().grantPermissions(['clipboard-read','clipboard-write'])
 await page.getByRole('button',{name:'Copy audit filter link',exact:true}).click()
 await expect.poll(()=>page.evaluate(()=>navigator.clipboard.readText())).toContain('activity=virtual')
 await page.getByRole('button',{name:'Load more',exact:true}).click()
 await expect.poll(()=>queries.at(-1)?.get('cursor')).toBe('3')
 await page.getByLabel('Activity',{exact:true}).selectOption('system')
 await expect(page.getByRole('button',{name:'Details',exact:true})).toHaveCount(1)
 await expect.poll(()=>queries.at(-1)?.get('activity')).toBe('system')
 expect(queries.at(-1)?.has('cursor')).toBe(false)
 await expect(page.getByText('upstream.discovery',{exact:true})).toBeVisible()
 await page.screenshot({path:test.info().outputPath(`audit-activity-${width}.png`),animations:'disabled'})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.getByRole('button',{name:'Remove Activity filter',exact:true}).click()
 await expect(page.getByLabel('Activity',{exact:true})).toHaveValue('')
 await expect.poll(()=>queries.at(-1)?.has('activity')).toBe(false)
})
