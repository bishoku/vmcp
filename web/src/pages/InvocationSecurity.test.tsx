import { cleanup, fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { render } from '../test-utils'
import { api, type InvocationSettings } from '../api'
import InvocationSecurity from './InvocationSecurity'
afterEach(()=>{cleanup();vi.restoreAllMocks()})
test.each(['oauth_subject','caller_context'] as const)('stages explicit %s identity without changing inherited tool rules',async source=>{
 const initial:InvocationSettings={revision:1,services:{session:{url:'https://session.test/check',approvalOrigin:'https://session.test',hasToken:true}},upstreams:{up:{tools:{transfer:{approvalServices:['session']}}}}}
 vi.spyOn(api,'invocationSecurity').mockResolvedValue(initial)
 vi.spyOn(api,'invocationUpstreams').mockResolvedValue({items:[{id:'up',slug:'bank'}]})
 const save=vi.spyOn(api,'saveInvocationSecurity').mockImplementation(async s=>({...s,revision:2}))
 render(<InvocationSecurity/>);await screen.findByText('Upstream and tool policy')
 fireEvent.change(screen.getByLabelText('Upstream'),{target:{value:'up'}})
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
