import { expect, test } from '@playwright/test'
import { withConfirmation } from '../confirmation'

test('real upstream catalog policy survives save and supports tool-only removal',async({page,request})=>{
  const api=process.env.VMCP_LIVE_API+'/api/v1'
  const headers={Authorization:'Bearer demo-admin-token'}
  const slug='invocation-ui-'+Date.now().toString(36)
  const url=process.env.VMCP_INVOCATION_TEST_UPSTREAM??'http://mcp-calculator:9100/mcp'
  const tool=process.env.VMCP_INVOCATION_TEST_TOOL??'calculate'
  const created=await request.post(api+'/upstreams',{headers,data:{slug,name:'Invocation browser test',url,enabled:true,headers:{Authorization:'Bearer calculator-secret'}}})
  expect(created.ok(),await created.text()).toBe(true)
  const path=`${api}/upstreams/${slug}/invocation-security`
  try {
    const refresh=await request.post(`${api}/upstreams/${slug}/refresh-tools`,{headers})
    expect(refresh.ok(),await refresh.text()).toBe(true)
    await page.goto('/upstreams/'+slug)
    await page.getByLabel('Admin token').fill('demo-admin-token')
    await page.getByRole('button',{name:'Continue',exact:true}).click()
    const section=page.getByRole('region',{name:'Upstream invocation security'})
    await section.getByRole('button',{name:'Add service'}).click()
    const service=page.getByRole('dialog',{name:'Add verification service'})
    await service.getByLabel('Service name').fill('local-approval')
    await service.getByLabel('Service token').fill('local-test-credential')
    await service.getByLabel('HTTPS decision endpoint').fill('https://approvals.example.test/check')
    await service.getByLabel('Trusted approval screen origin').fill('https://approvals.example.test')
    await service.getByRole('button',{name:'Stage service'}).click()
    await section.getByRole('button',{name:'Configure default'}).click()
    let modal=page.getByRole('dialog',{name:'Upstream default invocation policy'})
    await modal.getByLabel('User identity source').selectOption('oauth_subject')
    await modal.getByLabel('Accepted ACR values').fill('mfa, hardware')
    await modal.getByRole('button',{name:'Stage policy'}).click()
    await section.getByRole('button',{name:'Save invocation security'}).click()
    await expect(section.getByText('All invocation changes saved.')).toBeVisible()

    await page.getByRole('region',{name:'Upstream discovery'}).getByRole('button',{name:`Add invocation rule for ${tool}`}).click()
    modal=page.getByRole('dialog',{name:`Invocation policy for ${tool}`})
    await modal.getByLabel('Accepted ACR values').fill('hardware')
    await modal.getByLabel('Maximum authentication age (seconds)').fill('60')
    await modal.getByRole('checkbox',{name:'local-approval'}).check()
    await expect(modal.getByText(/Step-up: hardware · max 60s/)).toBeVisible()
    await modal.getByRole('button',{name:'Stage policy'}).click()
    await section.getByRole('button',{name:'Save invocation security'}).click()
    await expect(section.getByText('All invocation changes saved.')).toBeVisible()
    let saved=await (await request.get(path,{headers})).json()
    expect(saved.policy.tools[tool].stepUp).toEqual({acceptedAcrValues:['hardware'],maxAgeSeconds:60})
    expect(saved.policy.tools[tool].approvalServices).toEqual(['local-approval'])
    expect(saved.policy.services['local-approval']).toMatchObject({hasToken:true})
    expect(saved.policy.services['local-approval'].token).toBeUndefined()
    await withConfirmation(page,true,()=>section.getByRole('button',{name:`Remove invocation rule for ${tool}`}).click())
    await section.getByRole('button',{name:'Save invocation security'}).click()
    await expect(section.getByText('All invocation changes saved.')).toBeVisible()
    saved=await (await request.get(path,{headers})).json()
    expect(saved.policy.tools).toBeUndefined()
    expect(saved.policy.stepUp.acceptedAcrValues).toEqual(['mfa','hardware'])
  } finally {
    const current=await (await request.get(path,{headers})).json()
    await request.put(path,{headers,data:{upstreamId:current.upstreamId,revision:current.revision,policy:{}}})
    await request.delete(`${api}/upstreams/${slug}`,{headers})
  }
})
