import { auditSettings } from './audit-fixtures'
import { expect, test, type Page } from '@playwright/test'
import { upstream, server, key } from '../src/connections/fixtures'
test.use({viewport:{width:1280,height:1000}})
async function setup(page:Page,role='admin'){
 const writes:string[]=[]
 await page.route('**/api/v1/**',route=>{
 if(new URL(route.request().url()).pathname.endsWith('/audit-settings'))return route.fulfill({json:auditSettings})

  const req=route.request(),path=new URL(req.url()).pathname
  if(['POST','PUT','PATCH','DELETE'].includes(req.method()))writes.push(path)
  const json=path.endsWith('/me')?{name:'Tester',role}:path.endsWith('/info')?{version:'test',configRevision:1}:path.endsWith('/upstreams')?{items:[upstream]}:path.endsWith('/virtual-servers')?{items:[server]}:path.endsWith('/agent-keys')?{items:[key]}:path.endsWith('/catalog')?{resources:[],resourceTemplates:[],prompts:[]}:{items:[]}
  return route.fulfill({json})
 })
 return writes
}

test('save exposes busy feedback, blocks duplicate writes and recovers after a failed request',async({page})=>{
 await setup(page)
 let writes=0,release!:()=>void
 await page.route('**/api/v1/virtual-servers/support-workspace',async route=>{
  writes++;await new Promise<void>(resolve=>{release=resolve})
  return route.fulfill({status:500,json:{detail:'Temporary save failure'}})
 })
 await page.goto('/virtual-servers/new');await page.getByLabel('Name',{exact:true}).fill('Support workspace')
 const save=page.getByRole('button',{name:'Save',exact:true})
 await save.click()
 const saving=page.getByRole('button',{name:'Saving…',exact:true})
 await expect(saving).toBeDisabled();await expect(saving).toHaveAttribute('aria-busy','true')
 await page.keyboard.press('Enter');expect(writes).toBe(1)
 await page.screenshot({path:test.info().outputPath('buttons-loading.png'),animations:'disabled'})
 release();await expect(page.getByRole('alert')).toContainText('Temporary save failure')
 await expect(save).toBeEnabled();await expect(save).not.toHaveAttribute('aria-busy','true')
 await expect(page.getByLabel('Name',{exact:true})).toHaveValue('Support workspace')
})

test('danger and secondary actions have distinct treatments, visible focus, and keep confirmations explicit',async({page})=>{
 const writes=await setup(page);await page.goto('/upstreams')
 const remove=page.getByRole('button',{name:'Delete Alpha',exact:true})
 await expect(remove).toHaveClass(/action-danger-outline/)
 await remove.focus();await expect(remove).toHaveCSS('outline-style','solid')
 await page.keyboard.press('Enter')
 const dialog=page.getByRole('dialog',{name:'Delete upstream "alpha"?',exact:true})
 const confirm=dialog.getByTestId('confirm-action'),cancel=dialog.getByTestId('cancel-action')
 await expect(confirm).toHaveClass(/action-danger/);await expect(cancel).toHaveClass(/action-secondary/);await expect(cancel).toBeFocused()
 await page.screenshot({path:test.info().outputPath('buttons-danger-confirmation.png'),animations:'disabled'})
 await cancel.click();await expect(remove).toBeFocused();expect(writes).toHaveLength(0)
 await remove.hover();await expect(remove).toHaveCSS('background-color','rgb(254, 242, 242)')
 await page.setViewportSize({width:390,height:844});await remove.scrollIntoViewIfNeeded()
 const bounds=await remove.boundingBox();expect(bounds!.height).toBeGreaterThanOrEqual(44);expect(bounds!.width).toBeGreaterThanOrEqual(44)
 await page.screenshot({path:test.info().outputPath('buttons-mobile.png'),animations:'disabled'})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
})

test('disabled actions remain unavailable on hover and mobile table actions retain readable labels',async({page})=>{
 const writes=await setup(page,'viewer');await page.goto('/agent-keys')
 const remove=page.getByRole('button',{name:'Delete',exact:true})
 await expect(remove).toBeDisabled();await remove.hover()
 await expect(remove).toHaveCSS('cursor','not-allowed')
 await page.setViewportSize({width:390,height:844})
 const actions=page.getByRole('group',{name:'Actions for team-key',exact:true})
 await actions.scrollIntoViewIfNeeded()
 for(const name of ['Rotate','Revoke','Delete']){
  const button=actions.getByRole('button',{name,exact:true})
  expect((await button.boundingBox())!.height).toBeGreaterThanOrEqual(44)
  await expect(button.getByText(name,{exact:true})).toBeVisible()
 }
 expect(await actions.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath('buttons-disabled-mobile.png'),animations:'disabled'})
 expect(writes).toHaveLength(0)
})
