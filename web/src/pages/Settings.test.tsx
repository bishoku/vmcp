import { render } from '../test-utils'
import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { api, ApiError, type AdminToken } from '../api'
import Settings from './Settings'

beforeEach(()=>{
 Object.defineProperty(HTMLDialogElement.prototype,'showModal',{configurable:true,value:function(this:HTMLDialogElement){this.open=true}})
 Object.defineProperty(HTMLDialogElement.prototype,'close',{configurable:true,value:function(this:HTMLDialogElement){this.open=false}})
})
afterEach(() => { cleanup();vi.restoreAllMocks();Reflect.deleteProperty(HTMLDialogElement.prototype,'showModal');Reflect.deleteProperty(HTMLDialogElement.prototype,'close') })

test('viewers can download both specifications using the authenticated API', async () => {
  vi.spyOn(api, 'listAdminTokens').mockResolvedValue([])
  vi.spyOn(api, 'info').mockResolvedValue({ version: 'test', configRevision: 1 })
  const download = vi.spyOn(api, 'downloadOpenAPI').mockResolvedValue(new Blob(['spec']))
  const create = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:spec')
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  render(<Settings role="viewer" />)
  fireEvent.click(screen.getByRole('button', { name: 'Download OpenAPI JSON' }))
  await waitFor(() => expect(create).toHaveBeenCalledTimes(1))
  await waitFor(() => expect((screen.getByRole('button', { name: 'Download OpenAPI YAML' }) as HTMLButtonElement).disabled).toBe(false))
  fireEvent.click(screen.getByRole('button', { name: 'Download OpenAPI YAML' }))
  await waitFor(() => expect(download.mock.calls).toEqual([['json'], ['yaml']]))
})


const token:AdminToken={slug:'pipeline',version:4,description:'Build pipeline',scope:'read',tokenPrefix:'vmcp_admin_',expiresAt:null,revokedAt:null,lastUsedAt:null,status:'active',createdAt:'2026-01-01T00:00:00Z'}
function setupTokens(){
 vi.spyOn(api,'listAdminTokens').mockResolvedValue([token,{...token,slug:'expired-admin',scope:'admin',expiresAt:'2020-01-01T00:00:00Z'}])
 vi.spyOn(api,'info').mockResolvedValue({version:'test',configRevision:0,tenantId:'test-tenant'})
}

test.each(['viewer','editor'] as const)('%s can inspect token metadata without admin mutations',async(role)=>{
 setupTokens();render(<Settings role={role}/>)
 fireEvent.click(await screen.findByRole('button',{name:'pipeline'}))
 const detail=screen.getByRole('dialog',{name:'pipeline'})
 expect(within(detail).getByText('vmcp_admin_…')).toBeTruthy()
 expect(within(detail).getByRole('region',{name:'Token metadata'})).toBeTruthy()
 expect(screen.queryByRole('button',{name:'New token'})).toBeNull()
 expect(screen.queryByRole('button',{name:'Rotate'})).toBeNull()
 expect(screen.queryByRole('button',{name:'Revoke'})).toBeNull()
 expect(screen.queryByRole('button',{name:'Delete'})).toBeNull()
})

test('token scope and expiry filters combine without changing gateway metadata',async()=>{
 setupTokens();render(<Settings role="admin"/>)
 await screen.findByRole('button',{name:'pipeline'})
 fireEvent.change(screen.getByLabelText('Admin token scope'),{target:{value:'admin'}})
 fireEvent.change(screen.getByLabelText('Admin token status'),{target:{value:'expired'}})
 expect(screen.getByRole('button',{name:'expired-admin'})).toBeTruthy()
 expect(screen.queryByRole('button',{name:'pipeline'})).toBeNull()
 expect(screen.getByRole('region',{name:'Gateway information'}).textContent).toContain('test-tenant')
 fireEvent.change(screen.getByLabelText('Search admin tokens'),{target:{value:'no match'}})
 expect(screen.getByText('No matching admin tokens')).toBeTruthy()
})

test('token creation defaults to read scope, converts local expiry and shows the secret once',async()=>{
 setupTokens()
 const create=vi.spyOn(api,'createAdminToken').mockResolvedValue({...token,slug:'new-token',token:'created-secret'})
 render(<Settings role="admin"/>);await screen.findByRole('button',{name:'pipeline'})
 fireEvent.click(screen.getByRole('button',{name:'New token'}))
 expect((screen.getByLabelText('Scope') as HTMLSelectElement).value).toBe('read')
 fireEvent.change(screen.getByLabelText('Name'),{target:{value:'New Token'}})
 fireEvent.change(screen.getByLabelText('Expires at'),{target:{value:'2027-04-05T06:07'}})
 fireEvent.click(screen.getByRole('button',{name:'Create'}))
 const secret=await screen.findByRole('dialog',{name:'Admin token created'})
 expect(create).toHaveBeenCalledWith(expect.objectContaining({slug:'new-token',scope:'read',expiresAt:new Date('2027-04-05T06:07').toISOString()}))
 expect(within(secret).getByText('created-secret')).toBeTruthy()
 fireEvent.click(within(secret).getByRole('button',{name:'I have saved it'}))
 expect(screen.queryByText('created-secret')).toBeNull()
})

test('rotation requires confirmation and retains the token version precondition',async()=>{
 setupTokens()
 const rotate=vi.spyOn(api,'rotateAdminToken').mockResolvedValue({...token,token:'rotated-secret'})
 const confirm=vi.spyOn(window,'confirm').mockReturnValue(false)
 render(<Settings role="admin"/>);await screen.findByRole('button',{name:'pipeline'})
 const row=screen.getByRole('table',{name:'Configured admin API tokens'}).querySelector('tbody tr')!
 fireEvent.click(within(row as HTMLElement).getByRole('button',{name:'Rotate'}))
 expect(rotate).not.toHaveBeenCalled()
 confirm.mockReturnValue(true)
 fireEvent.click(within(row as HTMLElement).getByRole('button',{name:'Rotate'}))
 await screen.findByRole('dialog',{name:'Token rotated'})
 expect(rotate).toHaveBeenCalledWith('pipeline',4)
})

test('creation errors retain the form instead of displaying a secret',async()=>{
 setupTokens();vi.spyOn(api,'createAdminToken').mockRejectedValue(new ApiError(422,'Name already exists'))
 render(<Settings role="admin"/>);await screen.findByRole('button',{name:'pipeline'})
 fireEvent.click(screen.getByRole('button',{name:'New token'}))
 fireEvent.change(screen.getByLabelText('Name'),{target:{value:'duplicate'}})
 fireEvent.click(screen.getByRole('button',{name:'Create'}))
 await waitFor(()=>expect(screen.getByRole('alert').textContent).toContain('Name already exists'))
 expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('duplicate')
 expect(screen.queryByRole('dialog',{name:'Admin token created'})).toBeNull()
})
