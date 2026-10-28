import { expect,test } from '@playwright/test'
import {server,upstream} from '../src/connections/fixtures'

for (const width of [390,1440]) test(`saved server requests its missing approvals in bulk ${width}`,async({page})=>{
 await page.setViewportSize({width,height:1000})
 let requests=0
 let granted=false
 const tools=['lookup','search'].map(tool=>({upstream:'alpha',tool,exposedName:`alpha__${tool}`,approvalStatus:'required',orphaned:false}))
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',route=>{
  const request=route.request(),path=new URL(request.url()).pathname
  if(path.endsWith('/me'))return route.fulfill({json:{role:'admin',name:'Tester'}})
  if(path.endsWith('/tool-grants/bulk')) {
   const body=request.postDataJSON();expect(body.items).toHaveLength(2)
   expect(body.items.every((i:{status:string;version:number})=>i.status==='pending'&&i.version===0)).toBe(true)
   requests++;granted=true
   return route.fulfill({json:{items:body.items.map((i:object)=>({...i,result:'changed'})),changed:2,unchanged:0,failed:0}})
  }
  if(path.endsWith('/virtual-servers'))return route.fulfill({json:{items:[{...server,project:'default',policy:{default:'allow',tools:{}},tools:tools.map(t=>({...t,approvalStatus:granted?'pending':'required'})),catalog:[]}]}})
  if(path.endsWith('/upstreams'))return route.fulfill({json:{items:[upstream]}})
  if(path.endsWith('/tools'))return route.fulfill({json:{items:tools.map(t=>({name:t.tool,description:'Read records'}))}})
  if(path.endsWith('/catalog'))return route.fulfill({json:{resources:[],resourceTemplates:[],prompts:[]}})
  return route.fulfill({json:{items:[]}})
 })
 await page.goto('/virtual-servers/demo/edit')
 const approvals=page.getByRole('region',{name:'Upstream tool approvals'})
 await expect(approvals).toContainText('2 awaiting request')
 await approvals.getByRole('button',{name:'Request all missing approvals (2)'}).click()
 const dialog=page.getByRole('dialog',{name:'Request access',exact:true})
 await dialog.getByRole('textbox').fill('Banking assistant access')
 await dialog.getByRole('button',{name:'Request access (2)'}).click()
 await expect(dialog.getByRole('status')).toContainText('2 updated · 0 unchanged · 0 failed')
 await dialog.getByRole('button',{name:'Close',exact:true}).last().click()
 await expect(approvals).toContainText('0 awaiting request · 2 pending approval')
 expect(requests).toBe(1)
 await approvals.scrollIntoViewIfNeeded()
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath(`server-approval-${width}.png`),fullPage:true})
})
