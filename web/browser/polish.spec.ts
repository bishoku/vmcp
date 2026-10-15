import {expect,test} from '@playwright/test'
for (const width of [375,768,1440]) test(`navigation and audit at ${width}px`,async({page})=>{
 await page.setViewportSize({width,height:900})
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',r=>r.fulfill({json:r.request().url().endsWith('/me') ? {name:'Reviewer',role:'viewer'} : {items:[]}}))
 await page.goto('/audit?operationId=example');await expect(page).toHaveTitle(/vMCP/)
 await expect(page.getByRole('navigation',{name:'Main navigation'})).toBeVisible()
 await expect(page.getByLabel('Operation ID',{exact:true})).toHaveValue('example')
 await expect(page.getByText(/No events match/)).toBeVisible()
 await page.getByRole('button',{name:'Raw events',exact:true}).focus()
 await expect(page.getByRole('button',{name:'Raw events',exact:true})).toBeFocused()
 expect(await page.evaluate(()=>document.documentElement.scrollWidth <= innerWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath(`audit-${width}.png`),fullPage:true})
})

import {server,upstream,key,binding} from '../src/connections/fixtures'
for (const width of [375,768,1440]) test(`populated management layouts at ${width}px`,async({page})=>{
 await page.setViewportSize({width,height:1000})
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',r=>{
  const path=new URL(r.request().url()).pathname
  const json=path.endsWith('/me')?{name:'Admin',role:'admin'}:path.endsWith('/upstreams')?{items:[upstream]}:path.endsWith('/virtual-servers')?{items:[server]}:path.endsWith('/agent-keys')?{items:[key]}:path.endsWith('/identity-bindings')?{items:[binding]}:path.endsWith('/catalog')?{resources:[],resourceTemplates:[],prompts:[],capabilities:{}}:{items:[]}
  return r.fulfill({json})
 })
 for(const path of ['/connections','/setup','/virtual-servers/demo/connect','/virtual-servers?inspect=demo','/upstreams?inspect=alpha','/identities?inspect=team-jwt','/agent-keys','/settings']) {
  await page.goto(path);await expect(page.getByRole('navigation',{name:'Main navigation'})).toBeVisible()
  await page.waitForTimeout(100)
  expect(await page.locator('main').evaluate(el=>el.scrollWidth<=el.clientWidth),path).toBe(true)
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),path).toBe(true)
  const dialog=page.getByRole('dialog')
  if(await dialog.count())expect(await dialog.locator('section').first().evaluate(el=>el.scrollWidth<=el.clientWidth),path+' dialog').toBe(true)
  await page.screenshot({path:test.info().outputPath(`${path.replaceAll(/[^a-z]/g,'-')}-${width}.png`),fullPage:true})
 }
})
test('audit operation details are keyboard accessible and preserve both raw stages',async({page})=>{
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 const event={virtualServer:'demo',operationId:'op-123',requestId:'req-456',traceId:'trace-789',tool:'lookup',durationMs:3,time:'2026-10-11T12:00:00Z'}
 await page.route('**/api/v1/**',r=>r.fulfill({json:r.request().url().endsWith('/me')?{name:'Viewer',role:'viewer'}:{items:[{...event,id:2,stage:'outcome',status:'error',reason:'upstream timeout'},{...event,id:1,stage:'admission',status:'started'}]}}))
 await page.goto('/audit?virtualServer=demo&subject=team&from=2026-10-11T00%3A00%3A00Z')
 await expect(page.getByLabel('Subject',{exact:true})).toHaveValue('team')
 const details=page.getByRole('button',{name:'Details',exact:true});await expect(details).toHaveCount(1)
 await details.focus();await page.keyboard.press('Enter')
 const drawer=page.getByRole('dialog',{name:'Audit details'});await expect(drawer).toBeVisible()
 await expect(drawer.getByRole('button',{name:'Copy operation ID'})).toBeVisible()
 await expect(drawer.getByRole('link',{name:'Open operation'})).toHaveAttribute('href','/audit?operationId=op-123')
 await expect(drawer.locator('pre')).toHaveCount(2)
 await page.keyboard.press('Escape');await expect(details).toBeFocused()
 await page.getByRole('button',{name:'Raw events',exact:true}).click();await expect(page.getByRole('button',{name:'Details',exact:true})).toHaveCount(2)
})
test('theme text and keyboard focus colors meet targeted contrast thresholds',async({page})=>{
 await page.goto('/')
 const colors=await page.evaluate(()=>{
  const css=getComputedStyle(document.documentElement)
  return Object.fromEntries(['ink','ink-muted','canvas','accent-500','accent-700','accent-50'].map(name=>[name,css.getPropertyValue(`--color-${name}`).trim()]))
 })
 const luminance=(hex:string)=>{
  const values=hex.replace('#','').match(/../g)!.map(v=>parseInt(v,16)/255).map(v=>v<=0.04045?v/12.92:((v+0.055)/1.055)**2.4)
  return values[0]*0.2126+values[1]*0.7152+values[2]*0.0722
 }
 const contrast=(a:string,b:string)=>{const x=luminance(a),y=luminance(b);return (Math.max(x,y)+0.05)/(Math.min(x,y)+0.05)}
 for(const [a,b] of [[colors.ink,colors.canvas],[colors['ink-muted'],colors.canvas],['#ffffff',colors['accent-500']],[colors['accent-700'],colors['accent-50']]])expect(contrast(a,b)).toBeGreaterThanOrEqual(4.5)
 expect(contrast('#4338ca',colors.canvas)).toBeGreaterThanOrEqual(3)
})
