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
test('skip navigation and keyboard route changes land on content with a shared focus ring',async({page})=>{
 await page.setViewportSize({width:1280,height:1000});const writes=await setup(page)
 await page.goto('/upstreams');await expect(page.getByRole('heading',{name:'Upstreams',exact:true})).toBeVisible()
 await page.keyboard.press('Tab')
 const skip=page.getByRole('link',{name:'Skip to content',exact:true})
 await expect(skip).toBeFocused();await expect(skip).toHaveCSS('outline-color','rgb(61, 61, 168)')
 await page.keyboard.press('Enter');await expect(page.getByRole('main')).toBeFocused()
 const settings=page.getByRole('navigation',{name:'Main navigation'}).getByRole('link',{name:'Settings',exact:true})
 await settings.focus();await page.keyboard.press('Enter')
 await expect(page.getByRole('heading',{name:'Settings',exact:true})).toBeFocused()
 const open=page.getByRole('button',{name:'New token',exact:true});await open.focus()
 await expect(open).toHaveCSS('outline-style','solid');await expect(open).toHaveCSS('outline-width','2px')
 await page.screenshot({path:test.info().outputPath('keyboard-desktop.png'),animations:'disabled'})
 expect(writes).toHaveLength(0)
})
test('mobile dialogs cycle focus, calendar Escape closes only the picker and restores its trigger',async({page})=>{
 await page.setViewportSize({width:390,height:844});const writes=await setup(page)
 await page.goto('/settings');const open=page.getByRole('button',{name:'New token',exact:true})
 await open.focus();await page.keyboard.press('Enter')
 const dialog=page.getByRole('dialog',{name:'New admin token',exact:true}),close=dialog.getByRole('button',{name:'Close',exact:true})
 await expect(close).toBeFocused();await page.keyboard.press('Shift+Tab')
 await expect(dialog.getByRole('button',{name:'Create',exact:true})).toBeFocused()
 await page.keyboard.press('Tab');await expect(close).toBeFocused()
 await page.keyboard.press('Tab');await expect(dialog.getByLabel('Name',{exact:true})).toBeFocused()
 await expect(dialog.getByLabel('Name',{exact:true})).toHaveCSS('outline-color','rgb(61, 61, 168)')
 await page.screenshot({path:test.info().outputPath('keyboard-mobile.png'),animations:'disabled'})
 const calendar=dialog.getByRole('button',{name:'Choose expires at date and time'})
 await calendar.focus();await page.keyboard.press('Enter')
 const picker=page.getByRole('dialog',{name:'Expires at date and time',exact:true})
 await expect(picker).toBeVisible();await page.keyboard.press('Escape');await expect(picker).not.toBeVisible();await expect(calendar).toBeFocused()
 await page.keyboard.press('Escape');await expect(dialog).not.toBeVisible();await expect(open).toBeFocused()
 expect(writes).toHaveLength(0)
})
test('Sankey keyboard highlighting and drill-down retain a visible focus inside the diagram',async({page})=>{
 const writes=await setup(page);await page.goto('/connections')
 const diagram=page.getByRole('region',{name:'Scrollable configuration diagram',exact:true})
 const node=diagram.getByRole('button',{name:server.name,exact:true})
 await node.focus();await page.keyboard.press('Tab');await page.keyboard.press('Shift+Tab')
 await expect(node).toBeFocused();await expect(node).toHaveCSS('outline-offset','-3px')
 await expect(page.getByTestId('flow-ribbon').first()).toHaveAttribute('data-highlighted','true')
 await page.keyboard.press('Enter')
 await expect(page.getByRole('heading',{name:'Tool routing',exact:true})).toBeVisible()
 expect(await diagram.evaluate(el=>el===document.activeElement||el.contains(document.activeElement))).toBe(true)
 await page.emulateMedia({forcedColors:'active'})
 const active=page.locator(':focus');await expect(active).toHaveCSS('outline-style','solid')
 expect(writes).toHaveLength(0)
})
