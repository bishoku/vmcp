import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { useState } from 'react'
import { render } from '../test-utils'
import { api, type RESTConfig } from '../api'
import { upstream } from './fixtures'
import RESTUpstreamEditor from './RESTUpstreamEditor'
import { emptyREST } from './rest-config'
import { RoleContext } from '../interaction'
HTMLDialogElement.prototype.showModal=function(){this.setAttribute('open','')}
HTMLDialogElement.prototype.close=function(){this.removeAttribute('open')}
afterEach(()=>{cleanup();vi.restoreAllMocks()})
const config:RESTConfig={...emptyREST(),auth:{type:'basic',username:'service',hasPassword:true},operations:[{name:'lookup',description:'Look up an order',enabled:true,method:'GET',path:'/orders',parameters:[],readOnly:true,headerNames:['X-Key']}]}
function Draft(){const [value,setValue]=useState(config);return <RESTUpstreamEditor value={value} onChange={setValue} upstream={null}/>}
test('operation edits remain local and omitted credentials stay masked',async()=>{
 const save=vi.spyOn(api,'patchUpstream');render(<Draft/>);expect((screen.getByLabelText('Password') as HTMLInputElement).value).toBe('')
 fireEvent.click(screen.getByRole('button',{name:'Edit operation'}));const dialog=screen.getByRole('dialog',{name:'Edit REST operation'})
 fireEvent.change(within(dialog).getByLabelText('Tool name'),{target:{value:'find_order'}})
 fireEvent.click(within(dialog).getByRole('button',{name:'Use operation'}))
 await screen.findByText('find_order');expect(save).not.toHaveBeenCalled()
})
test('preview does not execute and admin execution requires confirmation',async()=>{
 const request=vi.spyOn(api,'restOperation').mockResolvedValue({method:'GET',url:'https://example.com/orders',headers:{}})
 const confirm=vi.spyOn(window,'confirm').mockReturnValue(false)
 render(<RoleContext.Provider value="admin"><RESTUpstreamEditor value={config} upstream={{...upstream,kind:'rest',rest:config}}/></RoleContext.Provider>)
 expect(request).not.toHaveBeenCalled();fireEvent.click(screen.getByRole('button',{name:'Preview request'}))
 await waitFor(()=>expect(request).toHaveBeenCalledWith(upstream.slug,'preview','lookup',{},upstream.version))
 fireEvent.click(screen.getByRole('button',{name:'Execute test request'}));await waitFor(()=>expect(confirm).toHaveBeenCalled());expect(request).toHaveBeenCalledTimes(1)
 confirm.mockReturnValue(true);fireEvent.click(screen.getByRole('button',{name:'Execute test request'}))
 await waitFor(()=>expect(request).toHaveBeenCalledWith(upstream.slug,'test','lookup',{},upstream.version))
})
test('viewers cannot execute and required identity prevents impersonation tests',()=>{
 render(<RoleContext.Provider value="viewer"><RESTUpstreamEditor value={config} upstream={{...upstream,kind:'rest',rest:{...config,requireIdentity:true}}}/></RoleContext.Provider>)
 expect((screen.getByRole('button',{name:'Execute test request'}) as HTMLButtonElement).disabled).toBe(true)
 expect((screen.getByRole('button',{name:'Preview request'}) as HTMLButtonElement).disabled).toBe(true)
})
