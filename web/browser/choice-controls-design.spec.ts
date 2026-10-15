import { expect, test, type Page } from '@playwright/test'
import { server, upstream, key } from '../src/connections/fixtures'

async function setup(page:Page){
 const writes:{path:string;body:Record<string,unknown>}[]=[]
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',route=>{
  const req=route.request(),path=new URL(req.url()).pathname
  if(['POST','PUT','PATCH','DELETE'].includes(req.method())){
   const body=req.postDataJSON();writes.push({path,body})
   if(path.includes('/virtual-servers/'))return route.fulfill({json:{...server,...body,slug:path.split('/').at(-1)}})
   return route.fulfill({json:{ok:true}})
  }
  const json=path.endsWith('/me')?{name:'Tester',role:'admin'}:path.endsWith('/info')?{version:'test',configRevision:1}:path.endsWith('/upstreams')?{items:[upstream]}:path.endsWith('/virtual-servers')?{items:[server]}:path.endsWith('/agent-keys')?{items:[{...key,virtualServers:['demo','missing-server']}]}:path.endsWith('/tools')?{items:[{name:'lookup',description:'Find records'}]}:path.endsWith('/catalog')?{resources:[{uri:'doc://guide'}],resourceTemplates:[],prompts:[]}:path.endsWith('/audit-events')?{items:[]}:{items:[]}
  return route.fulfill({json})
 })
 return writes
}

test('switches and checkboxes edit only the draft, work with Space, and save explicit values',async({page})=>{
 const writes=await setup(page);await page.goto('/virtual-servers/new')
 await page.getByLabel('Name',{exact:true}).fill('Support workspace')
 const enabled=page.getByRole('switch',{name:'Enabled',exact:true})
 await expect(enabled).toBeChecked();await enabled.focus();await page.keyboard.press('Space');await expect(enabled).not.toBeChecked()
 const roots=page.getByRole('checkbox',{name:'roots',exact:true})
 await roots.locator('..').click();await expect(roots).toBeChecked()
 const pii=page.getByRole('switch',{name:'Mask sensitive data in tool results',exact:true})
 await pii.check();await expect(page.getByRole('group',{name:'Built-in detectors'})).toBeVisible()
 const detector=page.getByRole('group',{name:'Built-in detectors'}).getByRole('checkbox').first();await detector.check()
 await page.getByRole('switch',{name:'Also mask tool arguments sent upstream'}).check()
 await page.getByRole('region',{name:'PII masking',exact:true}).scrollIntoViewIfNeeded()
 await page.screenshot({path:test.info().outputPath('selection-controls-desktop.png'),animations:'disabled'})
 expect(writes).toHaveLength(0)
 await page.getByRole('button',{name:'Save',exact:true}).click()
 await expect(page).toHaveURL(/support-workspace\/overview$/)
 expect(writes).toHaveLength(1)
 expect(writes[0].body.enabled).toBe(false)
 expect(writes[0].body.clientFeatures).toMatchObject({roots:true})
 expect(writes[0].body.pii).toMatchObject({enabled:true,maskArguments:true})
})

test('read-only switches stay disabled, capability radios support arrows, and editing stays explicit',async({page})=>{
 const writes=await setup(page);await page.goto('/upstreams/alpha')
 const enabled=page.getByRole('switch',{name:'Enabled',exact:true})
 await expect(enabled).toBeDisabled();await expect(enabled).toBeChecked()
 await enabled.scrollIntoViewIfNeeded();await page.screenshot({path:test.info().outputPath('selection-controls-disabled.png'),animations:'disabled'})
 const group=page.getByRole('radiogroup',{name:'Capability family'})
 await group.getByRole('radio',{name:/All/}).focus();await page.keyboard.press('ArrowRight')
 await expect(group.getByRole('radio',{name:/Tools/})).toBeChecked()
 await page.keyboard.press('ArrowRight');await expect(group.getByRole('radio',{name:/Resources/})).toBeChecked()
 await page.getByRole('button',{name:'Edit upstream',exact:true}).click()
 await expect(enabled).toBeEnabled();await enabled.focus();await page.keyboard.press('Space');await expect(enabled).not.toBeChecked()
 expect(writes).toHaveLength(0)
})

test('mobile grant rows preserve hidden selections and allow removing unavailable grants',async({page})=>{
 await page.setViewportSize({width:390,height:844})
 const writes=await setup(page);await page.goto('/agent-keys')
 await page.getByRole('button',{name:'team-key',exact:true}).click()
 await page.getByRole('button',{name:'Edit key',exact:true}).click()
 const dialog=page.getByRole('dialog')
 const missing=dialog.getByRole('checkbox',{name:'missing-server',exact:true})
 await expect(missing).toBeChecked();await expect(missing).toBeEnabled()
 await dialog.getByLabel('Search virtual servers').fill('Demo')
 await expect(missing).toHaveCount(0)
 const demo=dialog.getByRole('checkbox',{name:'Demo',exact:true});await expect(demo).toBeChecked()
 const bounds=await demo.locator('..').boundingBox();expect(bounds!.height).toBeGreaterThanOrEqual(44)
 await dialog.getByLabel('Search virtual servers').fill('')
 await expect(missing).toBeChecked();await missing.locator('..').click();await expect(missing).toHaveCount(0)
 await demo.scrollIntoViewIfNeeded();await page.screenshot({path:test.info().outputPath('selection-controls-mobile.png'),animations:'disabled'})
 expect(await dialog.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
 expect(writes).toHaveLength(0)
})

test('audit view radios change with arrows and the incomplete checkbox remains a read-only filter',async({page})=>{
 const writes=await setup(page);await page.goto('/audit')
 const operations=page.getByRole('radio',{name:'Operations',exact:true})
 await operations.focus();await page.keyboard.press('ArrowRight');await expect(page.getByRole('radio',{name:'Raw events',exact:true})).toBeChecked()
 await page.keyboard.press('ArrowLeft');await expect(operations).toBeChecked()
 const incomplete=page.getByRole('checkbox',{name:'Incomplete operations only'})
 await incomplete.locator('..').click();await expect(incomplete).toBeChecked()
 expect(writes).toHaveLength(0)
})

test('server field errors focus switches and clear on draft change without replaying the save',async({page})=>{
 await setup(page)
 let saves=0
 await page.route('**/api/v1/virtual-servers/support-workspace',route=>{
  saves++
  return route.fulfill({status:422,json:{detail:'Validation failed',code:'validation_failed',errors:[{field:'/authRequired',message:'This workspace requires client authentication.'}]}})
 })
 await page.goto('/virtual-servers/new');await page.getByLabel('Name',{exact:true}).fill('Support workspace')
 const auth=page.getByRole('switch',{name:'Require an agent key',exact:true})
 await auth.uncheck();await page.getByRole('button',{name:'Save',exact:true}).click()
 await expect(auth).toBeFocused();await expect(auth).toHaveAttribute('aria-invalid','true');await expect(auth).not.toBeChecked()
 await expect(page.getByLabel('Name',{exact:true})).toHaveValue('Support workspace')
 await expect(page.getByTestId('toast')).toHaveCount(0)
 await page.screenshot({path:test.info().outputPath('selection-controls-error.png'),animations:'disabled'})
 await page.keyboard.press('Space');await expect(auth).toBeChecked();await expect(auth).not.toHaveAttribute('aria-invalid','true')
 expect(saves).toBe(1)
})
