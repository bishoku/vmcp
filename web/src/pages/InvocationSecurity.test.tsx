import { cleanup, fireEvent, screen, waitFor } from '@testing-library/react'
import { beforeEach, afterEach, expect, test, vi } from 'vitest'
import { render } from '../test-utils'
import { api, type InvocationSettings } from '../api'
import { RoleContext } from '../interaction'
import { MemoryRouter } from 'react-router-dom'
import InvocationSecurity from './InvocationSecurity'
beforeEach(()=>{
 Object.defineProperty(HTMLDialogElement.prototype,'showModal',{configurable:true,value:function(this:HTMLDialogElement){this.open=true}})
 Object.defineProperty(HTMLDialogElement.prototype,'close',{configurable:true,value:function(this:HTMLDialogElement){this.open=false}})
})
afterEach(()=>{cleanup();vi.restoreAllMocks();Reflect.deleteProperty(HTMLDialogElement.prototype,'showModal');Reflect.deleteProperty(HTMLDialogElement.prototype,'close')})
test.each(['oauth_subject','caller_context'] as const)('stages explicit %s identity without changing inherited tool rules',async source=>{
 const initial:InvocationSettings={revision:1,services:{session:{url:'https://session.test/check',approvalOrigin:'https://session.test',hasToken:true}},upstreams:{up:{tools:{transfer:{approvalServices:['session']}}}}}
 vi.spyOn(api,'invocationSecurity').mockResolvedValue(initial)
 vi.spyOn(api,'invocationUpstreams').mockResolvedValue({items:[{id:'up',slug:'bank'}]})
 const save=vi.spyOn(api,'saveInvocationSecurity').mockImplementation(async s=>({...s,revision:2}))
 render(<MemoryRouter><InvocationSecurity/></MemoryRouter>);await screen.findByText('Upstream and tool policies');await waitFor(()=>expect((screen.getByRole('button',{name:'Edit upstream policy bank'}) as HTMLButtonElement).disabled).toBe(false));fireEvent.click(screen.getByRole('button',{name:'Edit upstream policy bank'}))
 fireEvent.change(screen.getByLabelText('User identity source'),{target:{value:source}})
 if(source==='caller_context'){
  fireEvent.change(screen.getByLabelText('Session verification service'),{target:{value:'session'}})
  fireEvent.change(screen.getByLabelText('Caller context fields sent for verification'),{target:{value:'customerId, sessionId'}})
 }
 fireEvent.click(screen.getByRole('button',{name:'Stage policy'}))
 fireEvent.click(screen.getByRole('button',{name:'Save security configuration'}))
 await waitFor(()=>expect(save).toHaveBeenCalledTimes(1))
 expect(save.mock.calls[0][0].upstreams.up).toEqual({identity:source==='oauth_subject'?{source}:{source,service:'session',fields:['customerId','sessionId']},tools:initial.upstreams.up.tools})
})

test('service errors retain entered secrets and only staged changes are saved',async()=>{
 vi.spyOn(api,'invocationSecurity').mockResolvedValue({revision:1,services:{},upstreams:{}})
 vi.spyOn(api,'invocationUpstreams').mockResolvedValue({items:[]})
 const save=vi.spyOn(api,'saveInvocationSecurity').mockRejectedValue(new Error('Revision changed. Reload and review.'))
 render(<MemoryRouter><InvocationSecurity/></MemoryRouter>)
 fireEvent.click(await screen.findByRole('button',{name:'Add service'}))
 fireEvent.change(screen.getByLabelText('Service name'),{target:{value:'approval'}})
 fireEvent.change(screen.getByLabelText('Service token'),{target:{value:'private-service-token'}})
 fireEvent.change(screen.getByLabelText('HTTPS decision endpoint'),{target:{value:'http://invalid.test'}})
 fireEvent.click(screen.getByRole('button',{name:'Stage service'}))
 expect(await screen.findByText('Use an HTTPS URL without embedded credentials.')).toBeTruthy()
 expect((screen.getByLabelText('Service token') as HTMLInputElement).value).toBe('private-service-token')
 expect(save).not.toHaveBeenCalled()
 fireEvent.change(screen.getByLabelText('HTTPS decision endpoint'),{target:{value:'https://approval.test/check'}})
 fireEvent.change(screen.getByLabelText('Trusted approval screen origin'),{target:{value:'https://approval.test'}})
 fireEvent.click(screen.getByRole('button',{name:'Stage service'}))
 expect(screen.queryByLabelText('Service token')).toBeNull()
 fireEvent.click(screen.getByRole('button',{name:'Save security configuration'}))
 expect(await screen.findByText('Revision changed. Reload and review.')).toBeTruthy()
 expect(screen.getByText('approval',{exact:true})).toBeTruthy()
 expect((screen.getByRole('button',{name:'Save security configuration'}) as HTMLButtonElement).disabled).toBe(false)
})

test('read-only access exposes summaries without mutation controls',async()=>{
 vi.spyOn(api,'invocationSecurity').mockResolvedValue({revision:1,services:{},upstreams:{}})
 vi.spyOn(api,'invocationUpstreams').mockResolvedValue({items:[]})
 render(<MemoryRouter><RoleContext.Provider value="viewer"><InvocationSecurity/></RoleContext.Provider></MemoryRouter>)
 await screen.findByText('You can view this configuration. Changes require invocation security management permission.')
 expect(screen.queryByRole('button',{name:'Add service'})).toBeNull()
 expect((screen.getByRole('button',{name:'Save security configuration'}) as HTMLButtonElement).disabled).toBe(true)
})
