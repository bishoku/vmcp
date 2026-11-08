import { cleanup, fireEvent, screen, waitFor } from '@testing-library/react'
import { beforeEach, afterEach, expect, test, vi } from 'vitest'
import { render } from '../test-utils'
import { api, type UpstreamInvocationSecurity } from '../api'
import { RoleContext } from '../interaction'
import { MemoryRouter } from 'react-router-dom'
import UpstreamSecurity, { ToolInvocationPolicy } from '../invocation/UpstreamSecurity'
import InvocationSecurity from './InvocationSecurity'

beforeEach(()=>{
  Object.defineProperty(HTMLDialogElement.prototype,'showModal',{configurable:true,value:function(this:HTMLDialogElement){this.open=true}})
  Object.defineProperty(HTMLDialogElement.prototype,'close',{configurable:true,value:function(this:HTMLDialogElement){this.open=false}})
})
afterEach(()=>{cleanup();vi.restoreAllMocks();Reflect.deleteProperty(HTMLDialogElement.prototype,'showModal');Reflect.deleteProperty(HTMLDialogElement.prototype,'close')})

const upstream:UpstreamInvocationSecurity={
  revision:0,upstreamId:'payments',sharedServices:['shared'],tools:['transfer'],callerContextFields:['sessionId'],
  policy:{identity:{source:'oauth_subject'},stepUp:{acceptedAcrValues:['mfa','hardware'],maxAgeSeconds:300}},
}

test('tool editor previews inherited policy and rejects disjoint step-up requirements before staging',async()=>{
  vi.spyOn(api,'upstreamInvocationSecurity').mockResolvedValue(upstream)
  const save=vi.spyOn(api,'saveUpstreamInvocationSecurity').mockImplementation(async(_path,doc)=>({...upstream,...doc,revision:2}))
  render(<UpstreamSecurity slug="payments" version={1}><section aria-label="Discovered tool"><ToolInvocationPolicy name="transfer"/></section></UpstreamSecurity>)
  const row=await screen.findByRole('region',{name:'Discovered tool'})
  await waitFor(()=>expect(row.textContent).toContain('Step-up: mfa, hardware'))
  fireEvent.click(row.querySelector('button')!)
  fireEvent.change(screen.getByLabelText('Accepted ACR values'),{target:{value:'password'}})
  fireEvent.click(screen.getByRole('button',{name:'Stage policy'}))
  expect(screen.getByRole('dialog')).toBeTruthy()
  expect(screen.getByText(/Choose at least one ACR accepted by the upstream policy/,{selector:'div[data-validation-summary] li button'})).toBeTruthy()
  fireEvent.change(screen.getByLabelText('Accepted ACR values'),{target:{value:'hardware'}})
  fireEvent.change(screen.getByLabelText('Maximum authentication age (seconds)'),{target:{value:'60'}})
  fireEvent.click(screen.getByRole('button',{name:'Stage policy'}))
  expect(row.textContent).toContain('Step-up: hardware · max 60s')
  expect(save).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button',{name:'Save invocation security'}))
  await waitFor(()=>expect(save).toHaveBeenCalledTimes(1))
  expect(save.mock.calls[0][1].policy.tools?.transfer).toEqual({stepUp:{acceptedAcrValues:['hardware'],maxAgeSeconds:60}})
})

test('read-only upstream access shows the effective rule without mutation controls',async()=>{
  vi.spyOn(api,'upstreamInvocationSecurity').mockResolvedValue(upstream)
  render(<RoleContext.Provider value="viewer"><UpstreamSecurity slug="payments" version={1}><p>Upstream details</p></UpstreamSecurity></RoleContext.Provider>)
  await screen.findByRole('heading',{name:'Tool invocation policies'})
  expect(screen.getByText('Uses upstream default')).toBeTruthy()
  expect(screen.queryByRole('button',{name:'Edit default'})).toBeNull()
  expect(screen.queryByRole('button',{name:'Save invocation security'})).toBeNull()
})

test('failed shared-service save retains staged credentials and submits only shared service settings',async()=>{
  const settings={revision:1,services:{},inUse:[]}
  vi.spyOn(api,'sharedInvocationServices').mockResolvedValue(settings)
  const save=vi.spyOn(api,'saveSharedInvocationServices').mockRejectedValue(new Error('Revision changed. Reload and review.'))
  render(<MemoryRouter><InvocationSecurity/></MemoryRouter>)
  fireEvent.click(await screen.findByRole('button',{name:'Add service'}))
  fireEvent.change(screen.getByLabelText('Service name'),{target:{value:'approval'}})
  fireEvent.change(screen.getByLabelText('Service token'),{target:{value:'private-service-token'}})
  fireEvent.change(screen.getByLabelText('HTTPS decision endpoint'),{target:{value:'http://invalid.test'}})
  fireEvent.click(screen.getByRole('button',{name:'Stage service'}))
  expect(await screen.findByText('Enter an HTTPS URL without embedded credentials or a fragment.')).toBeTruthy()
  expect((screen.getByLabelText('Service token') as HTMLInputElement).value).toBe('private-service-token')
  fireEvent.change(screen.getByLabelText('HTTPS decision endpoint'),{target:{value:'https://approval.test/check'}})
  fireEvent.change(screen.getByLabelText('Trusted approval screen origin'),{target:{value:'https://approval.test'}})
  fireEvent.click(screen.getByRole('button',{name:'Stage service'}))
  fireEvent.click(screen.getByRole('button',{name:'Save shared services'}))
  expect(await screen.findByText(/Revision changed/)).toBeTruthy()
  expect(screen.getByText('Token pending save')).toBeTruthy()
  expect(Object.keys(save.mock.calls[0][0]).sort()).toEqual(['revision','services'])
})

test('retry and read-only decisions stage independently and save the effective tool override',async()=>{
  const doc={...upstream,policy:{retry:'never' as const,readOnly:true}}
  vi.spyOn(api,'upstreamInvocationSecurity').mockResolvedValue(doc)
  const save=vi.spyOn(api,'saveUpstreamInvocationSecurity').mockImplementation(async(_path,value)=>({...doc,...value,revision:2}))
  render(<UpstreamSecurity slug="payments" version={1}><section aria-label="Discovered tool"><ToolInvocationPolicy name="transfer"/></section></UpstreamSecurity>)
  const row=await screen.findByRole('region',{name:'Discovered tool'})
  await waitFor(()=>expect(row.textContent).toContain('Retry: off'))
  fireEvent.click(row.querySelector('button')!)
  fireEvent.change(screen.getByLabelText('Automatic retry'),{target:{value:'once'}})
  fireEvent.change(screen.getByLabelText('Read-only classification'),{target:{value:'false'}})
  fireEvent.click(screen.getByRole('button',{name:'Stage policy'}))
  expect(save).not.toHaveBeenCalled()
  expect(row.textContent).toContain('Retry: once for eligible MCP calls')
  expect(row.textContent).toContain('Read-only: not classified')
  fireEvent.click(screen.getByRole('button',{name:'Save invocation security'}))
  await waitFor(()=>expect(save).toHaveBeenCalledTimes(1))
  expect(save.mock.calls[0][1].policy.tools?.transfer).toMatchObject({retry:'once',readOnly:false})
  expect(save.mock.calls[0][1].policy.retry).toBe('never')
})
