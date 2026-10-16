import { expect, test, type Page } from '@playwright/test'
import { upstream, server, key } from '../src/connections/fixtures'
test.use({viewport:{width:1280,height:1000}})
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

test('static cards keep a shared surface while interactive cards respond to pointer and keyboard focus',async({page})=>{
 const writes=await setup(page);await page.goto('/settings')
 const panel=page.getByRole('region',{name:'Gateway information',exact:true})
 await expect(panel).toHaveClass(/surface-card/);await expect(panel).toHaveCSS('background-color','rgb(255, 255, 255)');await expect(panel).toHaveCSS('border-radius','16px');await expect(panel).toHaveCSS('border-width','1px')
 const resting=await panel.evaluate(el=>getComputedStyle(el).boxShadow)
 expect(resting).not.toBe('none');await panel.hover();await expect(panel).toHaveCSS('box-shadow',resting)
 await page.screenshot({path:test.info().outputPath('surfaces-desktop.png'),animations:'disabled'})
 await page.goto('/upstreams')
 const card=page.getByRole('article',{name:'Alpha',exact:true})
 await expect(card).toHaveClass(/surface-interactive/)
 const before=await card.evaluate(el=>getComputedStyle(el).boxShadow)
 await card.hover();await expect(card).toHaveCSS('border-color','rgb(224, 228, 255)')
 expect(await card.evaluate(el=>getComputedStyle(el).boxShadow)).not.toBe(before)
 await page.mouse.move(0,0);await card.getByRole('link',{name:'Alpha',exact:true}).focus()
 await expect(card).toHaveCSS('border-color','rgb(224, 228, 255)')
 const inset=card.locator('.surface-inset').first();await expect(inset).toHaveCSS('border-radius','12px');await expect(inset).toHaveCSS('box-shadow','none')
 expect(writes).toHaveLength(0)
})

test('mobile dialog surfaces retain elevation without overflow and state colors remain visible',async({page})=>{
 const writes=await setup(page);await page.setViewportSize({width:390,height:844});await page.goto('/settings')
 await page.getByRole('button',{name:'New token',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'New admin token',exact:true}),frame=dialog.locator('.surface-dialog')
 await expect(frame).toHaveCSS('border-radius','16px')
 expect(await frame.evaluate(el=>getComputedStyle(el).boxShadow)).not.toBe('none')
 await expect(dialog.getByRole('region',{name:'Token details',exact:true})).toHaveClass(/surface-card/)
 expect(await dialog.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath('surfaces-modal-mobile.png'),animations:'disabled'})
 await dialog.getByRole('button',{name:'Cancel',exact:true}).click()
 await page.goto('/virtual-servers/new')
 await page.getByRole('checkbox',{name:'roots',exact:true}).check()
 const selected=page.locator('.choice-field').filter({has:page.getByRole('checkbox',{name:'roots',exact:true})})
 await expect(selected).toHaveCSS('border-color','rgb(224, 228, 255)')
 await page.getByRole('button',{name:'Save',exact:true}).click()
 const errorSurface=await page.evaluate(()=>{
  const probe=document.createElement('span')
  probe.style.backgroundColor='var(--color-red-50)'
  document.body.append(probe)
  const color=getComputedStyle(probe).backgroundColor
  probe.remove()
  return color
 })
 await expect(page.getByRole('alert')).toHaveCSS('background-color',errorSurface)
 await page.getByRole('heading',{name:'New virtual server',exact:true}).scrollIntoViewIfNeeded()
 await page.screenshot({path:test.info().outputPath('surfaces-mobile-states.png'),animations:'disabled'})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 expect(writes).toHaveLength(0)
})
