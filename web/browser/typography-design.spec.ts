import { auditSettings } from './audit-fixtures'
import { expect, test, type Page } from '@playwright/test'
import { server, upstream, key, binding } from '../src/connections/fixtures'
test.use({viewport:{width:1280,height:1000}})
async function setup(page:Page,longNames=false){
 const name=longNames?'Support workspace · '+ 'Customer operations and research '.repeat(3):'Support workspace'
 const description='Shared tools and reference material for support agents. Review the configured access rules and upstream capabilities before connecting a client.'
 await page.route('**/api/v1/**',route=>{
 if(new URL(route.request().url()).pathname.endsWith('/audit-settings'))return route.fulfill({json:auditSettings})

  const path=new URL(route.request().url()).pathname
  const json=path.endsWith('/me')?{name:'Tester',role:'admin'}:path.endsWith('/info')?{version:'test',configRevision:1}:path.endsWith('/upstreams')?{items:[{...upstream,name,description}]}:path.endsWith('/virtual-servers')?{items:[{...server,name,description}]}:path.endsWith('/agent-keys')?{items:[key]}:path.endsWith('/identity-bindings')?{items:[binding]}:path.endsWith('/catalog')?{resources:[],resourceTemplates:[],prompts:[]}:path.endsWith('/tools')?{items:[{name:'lookup',description:'Find a customer record'}]}:{items:[]}
  return route.fulfill({json})
 })
}

test('page titles and descriptions share a readable scale across inventories, details and management',async({page})=>{
 await setup(page)
 for(const path of ['/connections','/setup','/upstreams','/virtual-servers','/agent-keys','/identities','/audit','/settings','/upstreams/alpha','/virtual-servers/demo/overview','/virtual-servers/new']){
  await page.goto(path)
  const title=page.getByRole('heading',{level:1})
  await expect(title).toHaveCount(1);await expect(title).toHaveClass(/page-title/)
  await expect(title).toHaveCSS('font-size','28px');await expect(title).toHaveCSS('line-height','35px')
  const description=page.locator('.page-description').first()
  await expect(description).toBeVisible();await expect(description).toHaveCSS('font-size','14px');await expect(description).toHaveCSS('line-height','24.5px')
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 }
 await page.goto('/settings');await page.getByRole('heading',{name:'Settings',exact:true}).scrollIntoViewIfNeeded()
 await page.screenshot({path:test.info().outputPath('typography-desktop.png'),animations:'disabled'})
 await expect(page.getByRole('heading',{name:'Stored tokens',exact:true})).toHaveAttribute('class',/panel-title/)
})

test('long names, descriptions and dialog headings remain readable on narrow screens with enlarged text',async({page})=>{
 await setup(page,true)
 for(const width of [320,390]){
  await page.setViewportSize({width,height:844})
  for(const path of ['/virtual-servers/demo/overview','/upstreams/alpha','/virtual-servers/new']){
   await page.goto(path);const title=page.getByRole('heading',{level:1})
   await expect(title).toBeVisible();await expect(title).toHaveCSS('font-size','24px')
   const overflow=await title.evaluate(el=>el.scrollWidth>el.clientWidth+1)
   expect(overflow,`${width}px · ${path}`).toBe(false)
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
   await page.addStyleTag({content:'html { font-size: 20px !important; }'})
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  }
 }
 await page.goto('/virtual-servers/demo/overview')
 await page.screenshot({path:test.info().outputPath('typography-mobile.png'),animations:'disabled'})
 await page.goto('/settings');await page.getByRole('button',{name:'New token',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'New admin token',exact:true})
 await expect(dialog.getByRole('heading',{name:'New admin token',exact:true})).toHaveClass(/dialog-title/)
 await expect(dialog.getByLabel('Name',{exact:true})).toBeVisible()
 await page.screenshot({path:test.info().outputPath('typography-modal-mobile.png'),animations:'disabled'})
 expect(await dialog.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
})
