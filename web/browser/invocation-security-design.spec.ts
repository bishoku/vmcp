import { expect,test } from '@playwright/test'
import { withConfirmation } from './confirmation'
import { configurationFixture } from './configuration-fixtures'

test('invocation editors preserve drafts and fit narrow screens',async({page})=>{
 let saves=0
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',route=>{
  const capability=configurationFixture(route,'administrator');if(capability)return capability
  const path=new URL(route.request().url()).pathname
  if(path.endsWith('/me'))return route.fulfill({status:route.request().headers().authorization?200:401,json:{name:'Admin',role:'admin'}})
  if(path.endsWith('/invocation-security')){
   if(route.request().method()==='PUT'){saves++;return route.fulfill({json:{...route.request().postDataJSON(),revision:2}})}
   return route.fulfill({json:{revision:1,services:{approval:{url:'https://approval.example.test/check',approvalOrigin:'https://approval.example.test',hasToken:true}},upstreams:{up:{identity:{source:'oauth_subject'},tools:{transfer:{approvalServices:['approval']}}}}}})
  }
  if(path.endsWith('/access/scopes'))return route.fulfill({json:{items:[{id:'up',slug:'payments'}]}})
  return route.fulfill({json:{items:[]}})
 })
 await page.goto('/invocation-security');await page.getByLabel('Admin token').fill('test');await page.getByRole('button',{name:'Continue',exact:true}).click()
 await expect(page.getByRole('heading',{name:'Invocation security',exact:true})).toBeVisible()
 await page.screenshot({path:test.info().outputPath('invocation-overview.png'),fullPage:true})
 await page.getByRole('button',{name:'Edit tool policy transfer'}).click()
 const modal=page.getByRole('dialog',{name:'Edit invocation policy'})
 await expect(modal.getByLabel('Original tool name',{exact:true})).toHaveValue('transfer')
 await modal.getByLabel('Accepted ACR values',{exact:true}).fill('2')
 await modal.getByLabel('Maximum authentication age (seconds)',{exact:true}).fill('60')
 await page.setViewportSize({width:390,height:844})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath('invocation-policy-mobile.png'),fullPage:true})
 await modal.getByRole('button',{name:'Stage policy'}).click()
 expect(saves).toBe(0)
 await page.getByRole('button',{name:'Save security configuration'}).click()
 await expect(page.getByText('All changes saved.',{exact:true})).toBeVisible()
 expect(saves).toBe(1)
 await page.getByRole('button',{name:'Edit service approval'}).click()
 const service=page.getByRole('dialog',{name:'Edit verification service'})
 await expect(service.getByLabel('Service token',{exact:true})).toHaveValue('')
 await service.getByLabel('HTTPS decision endpoint',{exact:true}).fill('http://invalid.test')
 await service.getByRole('button',{name:'Stage service'}).click()
 await expect(service.getByText('Use an HTTPS URL without embedded credentials.',{exact:true})).toBeVisible()
 await withConfirmation(page,false,()=>service.getByRole('button',{name:'Cancel',exact:true}).click())
 await expect(service.getByLabel('HTTPS decision endpoint',{exact:true})).toHaveValue('http://invalid.test')
 await withConfirmation(page,true,()=>service.getByRole('button',{name:'Cancel',exact:true}).click())
 await expect(service).toHaveCount(0)
 expect(saves).toBe(1)
})
