import { auditSettings } from './audit-fixtures'
import { answerConfirmation } from './confirmation'
import { expect, test, type Page } from '@playwright/test'
import { server, upstream, key, binding } from '../src/connections/fixtures'

async function login(page:Page,path='/virtual-servers/demo/overview'){
 let logouts=0
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/auth/logout',r=>{logouts++;return r.fulfill({status:204})})
 await page.route('**/api/v1/**',route=>{
 if(new URL(route.request().url()).pathname.endsWith('/audit-settings'))return route.fulfill({json:auditSettings})

  const request=route.request(),path=new URL(request.url()).pathname
  if(path.endsWith('/me'))return route.fulfill({status:request.headers().authorization?200:401,json:{name:'Alexandra Morgan · Platform engineering',role:'admin'}})
  if(path.endsWith('/catalog'))return route.fulfill({json:{resources:[],resourceTemplates:[],prompts:[],capabilities:{}}})
  return route.fulfill({json:{items:path.endsWith('/virtual-servers')?[server]:path.endsWith('/upstreams')?[upstream]:path.endsWith('/agent-keys')?[key]:path.endsWith('/identity-bindings')?[binding]:[]}})
 })
 await page.goto(path);await page.getByLabel('Admin token').fill('test-token');await page.getByRole('button',{name:'Continue',exact:true}).click()
 await expect(page.getByRole('navigation',{name:'Main navigation',exact:true})).toBeVisible()
 return ()=>logouts
}

test('desktop sidebar groups links and retains active section on deep routes and short screens',async({page})=>{
 await login(page)
 const navigation=page.getByRole('navigation',{name:'Main navigation',exact:true})
 await expect(navigation.getByRole('link',{name:'Virtual Servers',exact:true})).toHaveAttribute('aria-current','page')
 await expect(navigation.getByRole('link')).toHaveCount(9)
 await expect(page.getByRole('region',{name:'Signed-in account'})).toContainText('Alexandra Morgan')
 await page.screenshot({path:test.info().outputPath('sidebar-desktop.png'),animations:'disabled'})
 await page.setViewportSize({width:1024,height:500})
 await navigation.getByRole('link',{name:'Settings',exact:true}).click()
 await expect(page.getByRole('heading',{name:'Settings',exact:true})).toBeVisible()
 await expect(navigation.getByRole('link',{name:'Settings',exact:true})).toHaveAttribute('aria-current','page')
 await expect(page.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
})

test('mobile drawer supports keyboard closing focus restoration route changes and sign out',async({page})=>{
 await page.setViewportSize({width:390,height:844})
 const logouts=await login(page)
 const menu=page.getByRole('button',{name:'Open navigation',exact:true})
 await expect(menu).toHaveAttribute('aria-expanded','false')
 await page.screenshot({path:test.info().outputPath('navigation-mobile-header.png'),animations:'disabled'})
 await menu.focus();await page.keyboard.press('Enter')
 const drawer=page.getByRole('dialog',{name:'Navigation',exact:true})
 await expect(drawer.getByRole('link',{name:'Virtual Servers',exact:true})).toHaveAttribute('aria-current','page')
 await expect(drawer.getByRole('link')).toHaveCount(9)
 await expect(menu).toHaveAttribute('aria-expanded','true')
 await page.screenshot({path:test.info().outputPath('navigation-mobile-drawer.png'),animations:'disabled'})
 await page.keyboard.press('Escape');await expect(drawer).toHaveCount(0);await expect(menu).toBeFocused()
 await menu.click();await drawer.getByRole('link',{name:'Connections',exact:true}).click()
 await expect(page).toHaveURL(/connections$/);await expect(drawer).toHaveCount(0)
 await expect(page.getByRole('navigation',{name:'Main navigation',exact:true})).toContainText('Connections')
 await menu.click();await page.setViewportSize({width:900,height:700})
 await expect(drawer).toHaveCount(0)
 await expect(page.getByRole('navigation',{name:'Main navigation',exact:true}).getByRole('link',{name:'Connections',exact:true})).toHaveAttribute('aria-current','page')
 await page.setViewportSize({width:320,height:568})
 await menu.click();await drawer.getByRole('button',{name:'Sign out',exact:true}).scrollIntoViewIfNeeded()
 expect(await drawer.locator('section').first().evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath('navigation-account-mobile.png'),animations:'disabled'})
 await drawer.getByRole('button',{name:'Sign out',exact:true}).click()
 await expect(page.getByRole('heading',{name:'Sign in to vMCP',exact:true})).toBeVisible()
 expect(logouts()).toBe(1)
})

test('mobile navigation and sign out respect rejected draft discard confirmation',async({page})=>{
 await page.setViewportSize({width:390,height:844})
 const logouts=await login(page,'/virtual-servers/demo/edit')
 await page.getByLabel('Name',{exact:true}).fill('Unsaved workspace')
 await page.getByRole('button',{name:'Open navigation',exact:true}).click()
 const drawer=page.getByRole('dialog',{name:'Navigation',exact:true})
 const discardNavigation=answerConfirmation(page,false)
 await drawer.getByRole('link',{name:'Settings',exact:true}).click()
 await discardNavigation
 await expect(page).toHaveURL(/virtual-servers\/demo\/edit$/)
 await expect(drawer).toBeVisible()
 const discardLogout=answerConfirmation(page,false)
 await drawer.getByRole('button',{name:'Sign out',exact:true}).click()
 await discardLogout
 expect(logouts()).toBe(0)
 await drawer.getByRole('button',{name:'Close',exact:true}).click()
 await expect(page.getByLabel('Name',{exact:true})).toHaveValue('Unsaved workspace')
 await page.getByRole('button',{name:'Open navigation',exact:true}).click()
 const acceptNavigation=answerConfirmation(page,true)
 await drawer.getByRole('link',{name:'Settings',exact:true}).click()
 await acceptNavigation
 await expect(page).toHaveURL(/settings$/);await expect(drawer).toHaveCount(0)
})
