import { expect, test, type Page } from '@playwright/test'
import { server, upstream, key, binding } from '../src/connections/fixtures'
test.use({viewport:{width:1280,height:1000}})
async function setup(page:Page){
 await page.route('**/api/v1/**',route=>{
  const path=new URL(route.request().url()).pathname
  const json=path.endsWith('/me')?{name:'Tester',role:'admin'}:path.endsWith('/info')?{version:'test',configRevision:1}:path.endsWith('/upstreams')?{items:[upstream]}:path.endsWith('/virtual-servers')?{items:[server]}:path.endsWith('/agent-keys')?{items:[key]}:path.endsWith('/identity-bindings')?{items:[binding]}:path.endsWith('/catalog')?{resources:[],resourceTemplates:[],prompts:[]}:path.endsWith('/tools')?{items:[{name:'lookup',description:'Look up records'}]}:{items:[]}
  return route.fulfill({json})
 })
}

test('page gutters and section rhythm stay consistent across desktop, tablet and mobile inventories and details',async({page})=>{
 await setup(page)
 for(const [width,gutter,block,gap] of [[1280,32,32,24],[768,24,24,24],[390,16,20,20]]){
  await page.setViewportSize({width,height:1000})
  for(const path of ['/connections','/setup','/upstreams','/virtual-servers','/settings','/audit','/agent-keys','/identities','/upstreams/alpha','/virtual-servers/demo/overview','/virtual-servers/new']){
   await page.goto(path)
   const shell=page.locator('.page-layout')
   await expect(shell).toHaveCount(1)
   await expect(shell).toHaveCSS('padding-left',`${gutter}px`);await expect(shell).toHaveCSS('padding-right',`${gutter}px`);await expect(shell).toHaveCSS('padding-top',`${block}px`)
   const spacing=await shell.evaluate(el=>Array.from(el.children).filter(child=>child.getBoundingClientRect().height>0).slice(1).map(child=>parseFloat(getComputedStyle(child).marginTop)))
   for(const value of spacing)expect(value,`${path} at ${width}px`).toBe(gap)
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
   expect(await page.evaluate(()=>window.scrollY)).toBe(0)
  }
 }
 await page.setViewportSize({width:1280,height:1000});await page.goto('/settings')
 const panel=page.getByRole('region',{name:'Control plane API',exact:true})
 const header=panel.locator('.panel-header'),body=panel.locator('.panel-body'),footer=panel.locator('.panel-footer')
 for(const part of [header,body,footer])await expect(part).toHaveCSS('padding-left','24px')
 await page.screenshot({path:test.info().outputPath('layout-desktop.png'),animations:'disabled'})
 await page.setViewportSize({width:390,height:844});await page.goto('/settings')
 await page.getByRole('region',{name:'Gateway information',exact:true}).scrollIntoViewIfNeeded()
 await page.screenshot({path:test.info().outputPath('layout-mobile.png'),animations:'disabled'})
})

test('mobile modal insets preserve useful form width and one vertical content scroller',async({page})=>{
 await setup(page);await page.setViewportSize({width:320,height:844});await page.goto('/settings')
 await page.getByRole('button',{name:'New token',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'New admin token',exact:true})
 const frame=dialog.locator('section').first()
 await expect(dialog).toHaveCSS('margin-top','0px')
 const box=await frame.boundingBox();expect(box!.y).toBeGreaterThanOrEqual(12);expect(box!.y+box!.height).toBeLessThanOrEqual(832);expect(box!.x).toBeGreaterThanOrEqual(12);expect(box!.x+box!.width).toBeLessThanOrEqual(308)
 const input=dialog.getByLabel('Name',{exact:true})
 expect((await input.boundingBox())!.width).toBeGreaterThanOrEqual(220)
 const scrolls=await frame.evaluate(el=>Array.from(el.querySelectorAll<HTMLElement>('*')).filter(child=>['auto','scroll'].includes(getComputedStyle(child).overflowY)&&child.scrollHeight>child.clientHeight+1&&child.tagName!=='TEXTAREA').length)
 expect(scrolls).toBe(1);expect(await page.evaluate(()=>window.scrollY)).toBe(0)
 expect(await dialog.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath('layout-modal-mobile.png'),animations:'disabled'})
 await input.fill('pipeline');await dialog.getByRole('button',{name:'Cancel',exact:true}).click()
 // Draft discard remains an explicit action after layout changes.
 await expect(page.getByTestId('confirmation-actions')).toBeVisible()
 await page.getByTestId('cancel-action').click();await expect(input).toHaveValue('pipeline')
})
