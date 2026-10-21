import {expect,test} from '@playwright/test'
import {auditSettings} from './audit-fixtures'
test('settings helper previews both payloads using unsaved global rules on mobile',async({page})=>{
 const previews:unknown[]=[]
 await page.setViewportSize({width:390,height:844})
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',route=>{
 const path=new URL(route.request().url()).pathname
 if(path.endsWith('/me'))return route.fulfill({status:route.request().headers().authorization?200:401,json:{name:'Admin',role:'admin'}})
 if(path.endsWith('/audit-settings'))return route.fulfill({json:{...auditSettings,scopes:{control:{request:true,response:true}}}})
 if(path.endsWith('/audit-preview')){previews.push(route.request().postDataJSON());return route.fulfill({json:{state:'redacted',content:{email:'[REDACTED]'},originalBytes:40,storedBytes:22,redactions:1,revision:1,matches:[{source:'global',name:'email',count:1}]}})}
 if(path.endsWith('/info'))return route.fulfill({json:{version:'test',configRevision:1}})
 return route.fulfill({json:{items:[]}})
 })
 await page.goto('/settings');await page.getByLabel('Admin token').fill('test');await page.getByRole('button',{name:'Continue',exact:true}).click()
 await page.getByRole('button',{name:'Add masking rule',exact:true}).click();await page.getByLabel('Audit rule 1 name').fill('email');await page.getByLabel('Audit rule 1 regex').fill('[A-Za-z]+@example\\.com')
 await page.getByText('Test request / response audit payload',{exact:true}).click();await page.getByRole('button',{name:'Preview audit payloads',exact:true}).click()
 await expect(page.getByText('global / email: 1',{exact:true})).toHaveCount(2);expect(previews).toHaveLength(2)
 expect(previews[0]).toMatchObject({settings:{rules:[{name:'email'}]},direction:'request'})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath('audit-payload-mobile.png'),fullPage:true})
})
