import { expect, test } from '@playwright/test'
import { withConfirmation } from './confirmation'
import { configurationFixture } from './configuration-fixtures'
import { upstream } from '../src/connections/fixtures'
import type { UpstreamInvocationSecurity } from '../src/api'

test('upstream tool rows expose inheritance, stage stricter rules and allow tool-only removal on mobile',async({page})=>{
  let saves=0
  let doc:UpstreamInvocationSecurity={
    upstreamId:'payments-id',revision:1,tools:['transfer'],sharedServices:['corporate'],callerContextFields:[],
    policy:{identity:{source:'oauth_subject'},stepUp:{acceptedAcrValues:['mfa','hardware'],maxAgeSeconds:300}},
  }
  await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
  await page.route('**/api/v1/**',route=>{
    const capability=configurationFixture(route,'operator',true);if(capability)return capability
    const path=new URL(route.request().url()).pathname
    if(path.endsWith('/me'))return route.fulfill({status:route.request().headers().authorization?200:401,json:{name:'Payments owner',role:'team'}})
    if(path.endsWith('/upstreams/payments'))return route.fulfill({json:{...upstream,slug:'payments',name:'Payments',ownerTeam:'payments'}})
    if(path.endsWith('/upstreams/payments/invocation-security')){
      if(route.request().method()==='PUT'){
        saves++
        const data=route.request().postDataJSON()
        doc={...doc,...data,revision:doc.revision+1}
      }
      return route.fulfill({json:doc})
    }
    if(path.endsWith('/tools'))return route.fulfill({json:{items:[{name:'transfer',description:'Transfer funds'}]}})
    if(path.endsWith('/catalog'))return route.fulfill({json:{resources:[],resourceTemplates:[],prompts:[],capabilities:{}}})
    if(path.endsWith('/audit-settings'))return route.fulfill({json:{revision:1,rules:[]}})
    return route.fulfill({json:{items:[]}})
  })
  await page.goto('/upstreams/payments')
  await page.getByLabel('Admin token').fill('test')
  await page.getByRole('button',{name:'Continue',exact:true}).click()
  const discovery=page.getByRole('region',{name:'Upstream discovery'})
  const security=page.getByRole('region',{name:'Upstream invocation security'})
  await expect(discovery.getByText(/Step-up: mfa, hardware/)).toBeVisible()
  await discovery.getByRole('button',{name:'Add invocation rule for transfer'}).click()
  const modal=page.getByRole('dialog',{name:'Invocation policy for transfer'})
  await modal.getByLabel('Accepted ACR values',{exact:true}).fill('hardware')
  await modal.getByLabel('Maximum authentication age (seconds)',{exact:true}).fill('60')
  await modal.getByRole('checkbox',{name:'corporate'}).check()
  await page.setViewportSize({width:390,height:844})
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  await expect(modal.getByText(/Step-up: hardware · max 60s · Approval: corporate/)).toBeVisible()
  await page.screenshot({path:test.info().outputPath('upstream-invocation-mobile.png')})
  await withConfirmation(page,false,()=>modal.getByRole('button',{name:'Cancel',exact:true}).click())
  await expect(modal.getByLabel('Accepted ACR values',{exact:true})).toHaveValue('hardware')
  await modal.getByRole('button',{name:'Stage policy'}).click()
  expect(saves).toBe(0)
  await expect(security.getByText('Review changes · 1 catalog tool affected')).toBeVisible()
  await security.getByRole('button',{name:'Save invocation security'}).click()
  await expect(security.getByText('All invocation changes saved.')).toBeVisible()
  await withConfirmation(page,true,()=>security.getByRole('button',{name:'Remove invocation rule for transfer'}).click())
  await security.getByRole('button',{name:'Save invocation security'}).click()
  await expect(security.getByText('All invocation changes saved.')).toBeVisible()
  expect(saves).toBe(2)
  expect(doc.policy.tools?.transfer).toBeUndefined()
  expect(doc.policy.stepUp?.maxAgeSeconds).toBe(300)
})
