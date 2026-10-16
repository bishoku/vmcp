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

test('shared motion animates overlays locally and dismissal restores focus without mutations',async({page})=>{
 await page.emulateMedia({reducedMotion:'no-preference'});const writes=await setup(page)
 await page.goto('/settings')
 const open=page.getByRole('button',{name:'New token',exact:true});await open.click()
 const dialog=page.getByRole('dialog',{name:'New admin token',exact:true}),frame=dialog.locator('.surface-dialog')
 await expect(frame).toHaveCSS('animation-name','motion-dialog-enter')
 await expect(frame).toHaveCSS('animation-duration','0.2s')
 await expect(open).toHaveCSS('transition-duration','0.14s')
 await page.keyboard.press('Escape');await expect(dialog).not.toBeVisible();await expect(open).toBeFocused()
 await open.click()
 await page.getByRole('button',{name:'Choose expires at date and time'}).click()
 const calendar=page.getByRole('dialog',{name:'Expires at date and time',exact:true})
 await expect(calendar).toHaveCSS('animation-name','motion-popup-enter')
 await page.keyboard.press('Escape');await expect(calendar).not.toBeVisible();await expect(dialog).toBeVisible()
 await expect(page.getByRole('button',{name:'Choose expires at date and time'})).toBeFocused()
 await dialog.getByRole('button',{name:'Close',exact:true}).click();await expect(dialog).not.toBeVisible()
 expect(writes).toHaveLength(0)
})

test('mobile reduced motion disables overlays, loading and flow transitions, including live preference changes',async({page})=>{
 await page.setViewportSize({width:390,height:844});await page.emulateMedia({reducedMotion:'reduce'});const writes=await setup(page)
 let release!:()=>void
 const responseReady=new Promise<void>(resolve=>{release=resolve})
 await page.route('**/api/v1/info',async route=>{await responseReady;await route.fulfill({json:{version:'test',configRevision:1}})})
 await page.goto('/settings');const spinner=page.locator('.motion-spin:visible').first()
 await expect(spinner).toBeVisible();await expect(spinner).toHaveCSS('animation-name','none')
 release();const open=page.getByRole('button',{name:'New token',exact:true});await open.click()
 const dialog=page.getByRole('dialog',{name:'New admin token',exact:true})
 await expect(dialog.locator('.surface-dialog')).toHaveCSS('animation-name','none')
 await expect(open).toHaveCSS('transition-duration','0s')
 await page.keyboard.press('Escape');await expect(dialog).not.toBeVisible();await expect(open).toBeFocused()
 await page.getByRole('button',{name:'Open navigation',exact:true}).click()
 const nav=page.getByRole('dialog',{name:'Navigation',exact:true})
 await expect(nav.locator('.surface-drawer')).toHaveCSS('animation-name','none')
 await nav.getByRole('button',{name:'Close',exact:true}).click();await expect(nav).not.toBeVisible()
 await page.goto('/connections');await expect(page.getByTestId('flow-ribbon').first()).toBeAttached()
 await expect(page.getByTestId('flow-ribbon').first()).toHaveCSS('transition-duration','0s')
 await page.emulateMedia({reducedMotion:'no-preference'})
 await expect(page.getByTestId('flow-ribbon').first()).toHaveCSS('transition-duration','0.14s')
 await page.getByRole('button',{name:'Open navigation',exact:true}).click()
 await expect(nav.locator('.surface-drawer')).toHaveCSS('animation-name','motion-drawer-enter')
 await page.emulateMedia({reducedMotion:'reduce'})
 await expect(nav.locator('.surface-drawer')).toHaveCSS('animation-name','none')
 await page.keyboard.press('Escape');await expect(nav).not.toBeVisible()
 expect(writes).toHaveLength(0)
})
