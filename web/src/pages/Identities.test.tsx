import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, afterEach, expect, test, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { render } from '../test-utils'
import { RoleContext } from '../interaction'
import { api, ApiError } from '../api'
import { binding, server } from '../connections/fixtures'
import Identities from './Identities'

beforeEach(()=>{
 Object.defineProperty(HTMLDialogElement.prototype,'showModal',{configurable:true,value:function(this:HTMLDialogElement){this.open=true}})
 Object.defineProperty(HTMLDialogElement.prototype,'close',{configurable:true,value:function(this:HTMLDialogElement){this.open=false}})
 vi.spyOn(api,'listBindings').mockResolvedValue([binding])
 vi.spyOn(api,'listVirtualServers').mockResolvedValue([server])
 vi.spyOn(api,'patchBinding').mockResolvedValue(binding)
 vi.spyOn(api,'evaluateBindings').mockResolvedValue({decision:{allowed:true,principal:'team-jwt'},matches:[{slug:'team-jwt',virtualServers:['demo']}]})
})
afterEach(()=>{cleanup();vi.restoreAllMocks();Reflect.deleteProperty(HTMLDialogElement.prototype,'showModal');Reflect.deleteProperty(HTMLDialogElement.prototype,'close')})
const open=(viewer=false,initialSelection?:string)=>render(<MemoryRouter><RoleContext.Provider value={viewer?'viewer':'admin'}><Identities initialSelection={initialSelection}/></RoleContext.Provider></MemoryRouter>)

test('deep links inspect bindings before editing and exists conditions omit values on save',async()=>{
 open(false,'team-jwt')
 const dialog=await screen.findByRole('dialog',{name:'team-jwt'})
 expect(within(dialog).queryByRole('button',{name:'Save'})).toBeNull()
 expect(within(dialog).getByRole('region',{name:'Claim conditions'}).textContent).toContain('azp')
 fireEvent.click(within(dialog).getByRole('button',{name:'Edit binding'}))
 fireEvent.change(screen.getByLabelText('Operator 1'),{target:{value:'exists'}})
 expect((screen.getByLabelText('Value 1') as HTMLInputElement).disabled).toBe(true)
 fireEvent.change(screen.getByLabelText(/IP allowlist/),{target:{value:'192.0.2.1\n198.51.100.0/24'}})
 fireEvent.click(screen.getByRole('button',{name:'Save'}))
 await waitFor(()=>expect(api.patchBinding).toHaveBeenCalledWith('team-jwt',expect.objectContaining({conditions:[{claim:'azp',op:'exists'}],virtualServers:['demo'],ipAllowlist:['192.0.2.1','198.51.100.0/24']}),binding.version))
})
test('viewers inspect grants and conditions without mutation controls',async()=>{
 open(true)
 fireEvent.click(await screen.findByRole('button',{name:'team-jwt'}))
 const dialog=screen.getByRole('dialog',{name:'team-jwt'})
 expect(within(dialog).getByRole('link',{name:/Demo/}).getAttribute('href')).toBe('/virtual-servers/demo/overview')
 expect(within(dialog).queryByRole('button',{name:'Edit binding'})).toBeNull()
 expect((screen.getByRole('button',{name:'New binding'}) as HTMLButtonElement).disabled).toBe(true)
 expect((screen.getByRole('button',{name:'Evaluate claims'}) as HTMLButtonElement).disabled).toBe(true)
})
test('claim evaluation is explicit, clears stale results and rejects invalid JSON locally',async()=>{
 open();await screen.findByRole('button',{name:'team-jwt'})
 expect(api.evaluateBindings).not.toHaveBeenCalled()
 fireEvent.click(screen.getByRole('button',{name:'Evaluate claims'}))
 fireEvent.change(screen.getByLabelText('Token claims (JSON)'),{target:{value:'{"azp":"team"}'}})
 fireEvent.click(screen.getByRole('button',{name:'Evaluate'}))
 await screen.findByText('Allowed as team-jwt')
 fireEvent.change(screen.getByLabelText('Token claims (JSON)'),{target:{value:'invalid'}})
 expect(screen.queryByText('Allowed as team-jwt')).toBeNull()
 fireEvent.click(screen.getByRole('button',{name:'Evaluate'}))
 await waitFor(()=>expect(screen.getByRole('alert').textContent).toContain('Claims is not valid JSON'))
 expect(api.evaluateBindings).toHaveBeenCalledTimes(1)
})
test('stale writes retain the condition draft for explicit recovery',async()=>{
 vi.mocked(api.patchBinding).mockRejectedValue(new ApiError(412,'This item changed in another session.'))
 open(false,'team-jwt');await screen.findByRole('dialog',{name:'team-jwt'})
 fireEvent.click(screen.getByRole('button',{name:'Edit binding'}))
 fireEvent.change(screen.getByLabelText('Value 1'),{target:{value:'changed'}})
 fireEvent.click(screen.getByRole('button',{name:'Save'}))
 await screen.findByText('Reload latest and discard draft')
 expect((screen.getByLabelText('Value 1') as HTMLInputElement).value).toBe('changed')
})
test('filtering server choices retains missing saved grants',async()=>{
 vi.mocked(api.listBindings).mockResolvedValue([{...binding,virtualServers:['demo','missing-server']}])
 open(false,'team-jwt');await screen.findByRole('dialog',{name:'team-jwt'})
 fireEvent.click(screen.getByRole('button',{name:'Edit binding'}))
 fireEvent.change(screen.getByLabelText('Search virtual servers'),{target:{value:'missing-server'}})
 expect((screen.getByRole('checkbox',{name:/missing-server/}) as HTMLInputElement).checked).toBe(true)
 fireEvent.click(screen.getByRole('button',{name:'Save'}))
 await waitFor(()=>expect(api.patchBinding).toHaveBeenCalledWith('team-jwt',expect.objectContaining({virtualServers:['demo','missing-server']}),binding.version))
})
