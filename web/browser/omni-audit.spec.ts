import {expect,test} from '@playwright/test'
import {auditSettings} from './audit-fixtures'

for(const width of [390,1440])test(`Omni discovery audit payload is loaded explicitly at ${width}px`,async({page})=>{
 await page.setViewportSize({width,height:900})
 let payloadRequests=0
 const base={time:'2026-10-30T10:00:00Z',schemaVersion:1,virtualServer:'',method:'tools/call',tool:'discover_tools',status:'ok',durationMs:10,stage:'outcome',actorType:'agent_key',actorId:'bot',redactions:0,payloadState:'captured'}
 const events=[{...base,id:2,eventId:'diag',scope:'omni_diagnostics',boundary:'omni_discovery',operationId:'diag',parentOperationId:'root',direction:'response',reason:'eligible=4; unauthorized=2'},{...base,id:1,eventId:'root',scope:'omni',boundary:'omni',operationId:'root'}]
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',r=>{
  const url=new URL(r.request().url())
  if(url.pathname.endsWith('/me'))return r.fulfill({status:r.request().headers().authorization?200:401,json:{name:'Admin',role:'admin'}})
  if(url.pathname.endsWith('/audit-settings'))return r.fulfill({json:auditSettings})
  if(url.pathname.endsWith('/audit-events'))return r.fulfill({json:{items:events,nextCursor:null}})
  if(url.pathname.endsWith('/payload')){payloadRequests++;return r.fulfill({json:{state:'captured',content:{counts:{eligible:4,unauthorized:2},stages:[{name:'reranker_scores',status:'ok',count:4,durationMs:8,details:{min_score:0.2,candidates:[{tool_ref:'authorized-tool',rerank_score:0.1,below_min_score:true}]}}]}}})}
  return r.fulfill({json:{items:[]}})
 })
 await page.goto('/audit?activity=omni')
 await page.getByLabel('Admin token').fill('test')
 await page.getByRole('button',{name:'Continue',exact:true}).click()
 await expect(page.getByLabel('Activity',{exact:true})).toHaveValue('omni')
 await page.getByRole('button',{name:'Details',exact:true}).click()
 await expect(page.getByLabel('Discovery pipeline summary')).toBeVisible()
 await page.getByRole('button',{name:'Inspect discovery pipeline'}).click()
 expect(payloadRequests).toBe(0)
 await expect(page.getByLabel('Discovery pipeline stages')).toHaveCount(0)
 await page.getByRole('button',{name:'Load audit payload',exact:true}).click()
 await expect(page.getByLabel('Discovery pipeline stages')).toBeVisible()
 await page.getByText('reranker_scores · ok · 4 candidates · 8 ms',{exact:true}).click()
 await expect(page.getByLabel('Discovery pipeline stages')).toContainText('below_min_score')
 expect(payloadRequests).toBe(1)
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath(`omni-audit-${width}.png`),animations:'disabled'})
})
