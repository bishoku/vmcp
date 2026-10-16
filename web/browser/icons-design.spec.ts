import { expect, test, type Page } from '@playwright/test'
import { upstream, server, key } from '../src/connections/fixtures'

async function setup(page:Page){
 const writes:string[]=[]
 await page.route('**/api/v1/**',route=>{
  const req=route.request(),path=new URL(req.url()).pathname
  if(['POST','PATCH','PUT','DELETE'].includes(req.method()))writes.push(path)
  const json=path.endsWith('/me')?{name:'Tester',role:'admin'}:path.endsWith('/info')?{version:'test',configRevision:1}:path.endsWith('/upstreams')?{items:[upstream]}:path.endsWith('/virtual-servers')?{items:[server]}:path.endsWith('/agent-keys')?{items:[key]}:path.endsWith('/catalog')?{resources:[],resourceTemplates:[],prompts:[]}:{items:[]}
  return route.fulfill({json})
 })
 return writes
}
async function verifyIcons(page:Page){
 const icons=page.locator('.lucide:visible')
 await expect.poll(()=>icons.count()).toBeGreaterThan(5)
 for(const icon of await icons.all()){
  await expect(icon).toHaveAttribute('aria-hidden','true')
  await expect(icon).toHaveCSS('stroke-width','1.75px')
  await expect(icon).toHaveCSS('flex-shrink','0')
  const size=await icon.boundingBox()
  expect([12,14,16,18,20,24]).toContain(size!.width)
  expect(size!.height).toBe(size!.width)
 }
}

test('desktop icons share optical weight, role sizes and action colors',async({page})=>{
 await page.setViewportSize({width:1280,height:1000});const writes=await setup(page)
 await page.goto('/upstreams');await verifyIcons(page)
 await expect(page.getByRole('navigation',{name:'Main navigation'}).locator('.lucide').first()).toHaveCSS('width','18px')
 const add=page.getByRole('button',{name:'Add upstream',exact:true})
 await expect(add.locator('.lucide')).toHaveCSS('width','16px')
 await expect(add.locator('.lucide')).toHaveCSS('color',await add.evaluate(el=>getComputedStyle(el).color))
 const remove=page.getByRole('button',{name:'Delete Alpha',exact:true})
 await expect(remove.locator('.lucide')).toHaveCSS('color',await remove.evaluate(el=>getComputedStyle(el).color))
 await expect(page.getByRole('article',{name:'Alpha',exact:true}).locator('.icon-feature')).toHaveCSS('width','20px')
 await page.screenshot({path:test.info().outputPath('icons-desktop.png'),animations:'disabled'})
 await page.goto('/settings');await verifyIcons(page)
 const panel=page.getByRole('region',{name:'Gateway information',exact:true})
 const heading=await panel.getByRole('heading',{name:'Gateway information',exact:true}).boundingBox()
 const icon=await panel.locator('.panel-header .lucide').boundingBox()
 expect(Math.abs(heading!.y+heading!.height/2-icon!.y-icon!.height/2)).toBeLessThanOrEqual(2)
 expect(writes).toHaveLength(0)
})

test('mobile icons stay aligned with labels and keep large touch targets in menus and dialogs',async({page})=>{
 await page.setViewportSize({width:390,height:844});const writes=await setup(page)
 await page.goto('/settings');await verifyIcons(page)
 const menu=page.getByRole('button',{name:'Open navigation',exact:true})
 expect((await menu.boundingBox())!.height).toBeGreaterThanOrEqual(44)
 await menu.click()
 const navigation=page.getByRole('dialog',{name:'Navigation',exact:true})
 await expect(navigation.getByRole('navigation',{name:'Mobile navigation'}).locator('.lucide').first()).toHaveCSS('width','18px')
 await navigation.getByRole('button',{name:'Close',exact:true}).click()
 await page.getByRole('button',{name:'New token',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'New admin token',exact:true})
 await verifyIcons(page)
 const close=dialog.getByRole('button',{name:'Close',exact:true})
 expect((await close.boundingBox())!.height).toBeGreaterThanOrEqual(44)
 await expect(close.locator('.lucide')).toHaveCSS('width','16px')
 expect(await dialog.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath('icons-mobile.png'),animations:'disabled'})
 expect(writes).toHaveLength(0)
})
