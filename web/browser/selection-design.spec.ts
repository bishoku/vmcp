import { auditSettings } from './audit-fixtures'
import { expect, test, type Page } from '@playwright/test'
import { server, upstream, key } from '../src/connections/fixtures'

test.use({viewport:{width:1280,height:1000}})
async function setup(page:Page,role='admin'){
 const writes:string[]=[]
 await page.route('**/api/v1/**',route=>{
 if(new URL(route.request().url()).pathname.endsWith('/audit-settings'))return route.fulfill({json:auditSettings})

  const request=route.request(),path=new URL(request.url()).pathname
  if(['POST','PATCH','PUT','DELETE'].includes(request.method()))writes.push(path)
  const json=path.endsWith('/me')?{name:'Tester',role}:path.endsWith('/info')?{version:'dev-test',configRevision:1}:path.endsWith('/upstreams')?{items:[upstream,{...upstream,slug:'beta',name:'Beta',enabled:false}]}:path.endsWith('/agent-keys')?{items:[key]}:path.endsWith('/virtual-servers')?{items:Array.from({length:60},(_,i)=>({...server,slug:`workspace-${i}`,name:`Workspace ${i} · Research and customer support gateway`,endpoint:`/mcp/v1/virtual/workspace-${i}`}))}:{items:[]}
  return route.fulfill({json})
 })
 return writes
}

test('filter pickers show selected options, support keyboard closing and keep their filter semantics',async({page})=>{
 const writes=await setup(page);await page.goto('/upstreams')
 const select=page.getByRole('combobox',{name:'Upstream status',exact:true})
 await select.click();await expect.poll(()=>select.evaluate(el=>el.matches(':open'))).toBe(true)
 const all=page.getByRole('option',{name:'All statuses',exact:true})
 await expect(all).toBeVisible();expect(await all.evaluate(el=>el.matches(':checked'))).toBe(true)
 await expect(all).toHaveCSS('background-color','rgb(238, 240, 255)')
 await page.screenshot({path:test.info().outputPath('select-filter-open.png'),animations:'disabled'})
 await page.getByRole('option',{name:'Enabled',exact:true}).click()
 await expect(select).toHaveValue('enabled')
 await expect(page.getByRole('article',{name:'Alpha',exact:true})).toBeVisible()
 await expect(page.getByRole('article',{name:'Beta',exact:true})).toHaveCount(0)
 await select.focus();await page.keyboard.press('Space')
 await expect.poll(()=>select.evaluate(el=>el.matches(':open'))).toBe(true)
 await page.keyboard.press('Escape')
 await expect.poll(()=>select.evaluate(el=>el.matches(':open'))).toBe(false)
 await expect(select).toBeFocused();await expect(select).toHaveValue('enabled')
 expect(writes).toEqual([])
})

test('selects and focus tooltips use the top layer inside a modal and Escape keeps the draft open',async({page})=>{
 const writes=await setup(page);await page.goto('/settings')
 await page.getByRole('button',{name:'New token',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'New admin token',exact:true})
 const select=dialog.getByRole('combobox',{name:'Scope',exact:true})
 await expect(select).toHaveAttribute('aria-required','true')
 const describedBy=await select.getAttribute('aria-describedby');expect(describedBy).toBeTruthy()
 await select.click();await expect.poll(()=>select.evaluate(el=>el.matches(':open'))).toBe(true)
 await expect(page.getByRole('option',{name:'admin (full access)',exact:true})).toBeVisible()
 await page.screenshot({path:test.info().outputPath('select-modal-open.png'),animations:'disabled'})
 await page.keyboard.press('Escape')
 await expect(dialog).toBeVisible();await expect(select).toHaveValue('read')
 const help=dialog.getByRole('button',{name:'About scope',exact:true})
 await help.focus()
 const tooltip=dialog.getByRole('tooltip')
 await expect(tooltip).toContainText('Use agent credentials')
 expect(await tooltip.evaluate(el=>el.matches(':popover-open'))).toBe(true)
 await expect(help).toBeFocused()
 const box=await tooltip.boundingBox();expect(box!.x).toBeGreaterThanOrEqual(0);expect(box!.x+box!.width).toBeLessThanOrEqual(1280)
 await page.screenshot({path:test.info().outputPath('tooltip-modal.png'),animations:'disabled'})
 await page.keyboard.press('Escape')
 await expect(tooltip).toHaveCount(0);await expect(dialog).toBeVisible();await expect(help).toBeFocused()
 await expect(select).toHaveAttribute('aria-describedby',describedBy!)
 expect(writes).toEqual([])
})

test('disabled actions explain access requirements without enabling mutations',async({page})=>{
 const writes=await setup(page,'viewer');await page.goto('/agent-keys')
 const rotate=page.getByRole('button',{name:'Rotate',exact:true})
 await expect(rotate).toBeDisabled();await rotate.hover()
 await expect(page.getByRole('tooltip')).toContainText('Editor access is required')
 await page.screenshot({path:test.info().outputPath('tooltip-disabled-action.png'),animations:'disabled'})
 await page.keyboard.press('Escape');await expect(page.getByRole('tooltip')).toHaveCount(0)
 expect(writes).toEqual([])
})

test.describe('touch selection',()=>{
 test.use({viewport:{width:390,height:844},hasTouch:true,isMobile:true})
 test('long pickers remain contained and help can be opened with a tap',async({page})=>{
  const writes=await setup(page);await page.goto('/setup')
  const select=page.getByRole('combobox',{name:'Guide for virtual server',exact:true})
  await select.tap();await expect.poll(()=>select.evaluate(el=>el.matches(':open'))).toBe(true)
  await page.screenshot({path:test.info().outputPath('select-long-mobile.png'),animations:'disabled'})
  const choice=page.getByRole('option',{name:'Workspace 59 · Research and customer support gateway',exact:true})
  await choice.scrollIntoViewIfNeeded();await choice.tap()
  await expect(select).toHaveValue('workspace-59')
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  await page.goto('/settings');await page.getByRole('button',{name:'New token',exact:true}).tap()
  const dialog=page.getByRole('dialog',{name:'New admin token',exact:true})
  await dialog.getByRole('button',{name:'About scope',exact:true}).tap()
  const tooltip=dialog.getByRole('tooltip');await expect(tooltip).toBeVisible()
  const box=await tooltip.boundingBox();expect(box!.x).toBeGreaterThanOrEqual(0);expect(box!.x+box!.width).toBeLessThanOrEqual(390);expect(box!.y+box!.height).toBeLessThanOrEqual(844)
  await page.screenshot({path:test.info().outputPath('tooltip-mobile.png'),animations:'disabled'})
  await dialog.getByRole('heading',{name:'New admin token',exact:true}).tap()
  await expect(tooltip).toHaveCount(0);await expect(dialog).toBeVisible()
  expect(writes).toEqual([])
 })
})
