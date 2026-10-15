import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { render } from '../test-utils'
import { api, ApiError } from '../api'
import { RoleContext } from '../interaction'
import { server, upstream } from '../connections/fixtures'
import Upstreams from './Upstreams'
import VirtualServers from './VirtualServers'

beforeEach(()=>{
 vi.spyOn(api,'listUpstreams').mockResolvedValue([upstream])
 vi.spyOn(api,'listVirtualServers').mockResolvedValue([server])
 vi.spyOn(api,'refreshTools').mockResolvedValue({} as Awaited<ReturnType<typeof api.refreshTools>>)
 vi.spyOn(api,'deleteUpstream').mockResolvedValue(undefined)
 vi.spyOn(api,'deleteVirtualServer').mockResolvedValue(undefined)
})
afterEach(()=>{cleanup();vi.restoreAllMocks()})

test('upstream discovery and configuration filters combine without inferring health from discovery',async()=>{
 vi.mocked(api.listUpstreams).mockResolvedValue([{...upstream,health:{state:'unhealthy',nodes:[]}},{...upstream,slug:'legacy',name:'Legacy',transport:'sse',enabled:false,lastRefreshError:'Refresh unavailable'}])
 render(<MemoryRouter><Upstreams/></MemoryRouter>)
 const card=await screen.findByRole('article',{name:'Alpha'})
 expect(within(card).getByText('Snapshot available')).toBeTruthy()
 expect(within(card).getByText('unhealthy')).toBeTruthy()
 fireEvent.change(screen.getByLabelText('Upstream status'),{target:{value:'disabled'}})
 fireEvent.change(screen.getByLabelText('Upstream transport'),{target:{value:'sse'}})
 fireEvent.change(screen.getByLabelText('Upstream discovery'),{target:{value:'failed'}})
 expect(screen.getByRole('article',{name:'Legacy'})).toBeTruthy()
 expect(screen.queryByRole('article',{name:'Alpha'})).toBeNull()
 fireEvent.change(screen.getByLabelText('Search upstreams'),{target:{value:'no match'}})
 expect(screen.getByText('No upstreams match your filters.')).toBeTruthy()
 fireEvent.click(screen.getByRole('button',{name:'Clear filters'}))
 expect(screen.getAllByRole('article')).toHaveLength(2)
 expect(api.refreshTools).not.toHaveBeenCalled()
})

test('upstream deletion preserves version and explicit force confirmation and refresh exposes orphaned selections',async()=>{
 vi.mocked(api.deleteUpstream).mockRejectedValueOnce(new ApiError(409,'Selected by Demo','in_use'))
 const confirm=vi.spyOn(window,'confirm').mockReturnValue(true)
 vi.mocked(api.refreshTools).mockResolvedValue({orphanedSelections:[{virtualServer:'demo',tool:'lookup'}]} as Awaited<ReturnType<typeof api.refreshTools>>)
 render(<MemoryRouter><Upstreams/></MemoryRouter>)
 fireEvent.click(await screen.findByRole('button',{name:'Refresh capabilities for Alpha'}))
 expect(await screen.findByText(/removed tools still selected by virtual servers: demo:lookup/)).toBeTruthy()
 await waitFor(()=>expect((screen.getByRole('button',{name:'Delete Alpha'}) as HTMLButtonElement).disabled).toBe(false))
 fireEvent.click(screen.getByRole('button',{name:'Delete Alpha'}))
 await waitFor(()=>expect(api.deleteUpstream).toHaveBeenCalledTimes(2))
 expect(api.deleteUpstream).toHaveBeenNthCalledWith(1,'alpha',false,2)
 expect(api.deleteUpstream).toHaveBeenNthCalledWith(2,'alpha',true,2)
 expect(confirm).toHaveBeenCalledTimes(2)
})

test('virtual server filters reset pagination and deletion uses the saved version',async()=>{
 const servers=Array.from({length:25},(_,i)=>({...server,slug:`server-${i}`,name:`Server ${i}`,authRequired:i!==24}))
 vi.mocked(api.listVirtualServers).mockResolvedValue(servers)
 vi.spyOn(window,'confirm').mockReturnValue(true)
 render(<MemoryRouter><VirtualServers/></MemoryRouter>)
 await screen.findByRole('article',{name:'Server 0'})
 expect(screen.getAllByRole('article')).toHaveLength(24)
 fireEvent.click(screen.getByRole('button',{name:'Next'}))
 expect(screen.getAllByRole('article')).toHaveLength(1)
 fireEvent.change(screen.getByLabelText('Virtual server authentication'),{target:{value:'required'}})
 expect(screen.getAllByRole('article')).toHaveLength(24)
 expect(screen.queryByRole('article',{name:'Server 24'})).toBeNull()
 fireEvent.change(screen.getByLabelText('Search virtual servers'),{target:{value:'Server 0'}})
 fireEvent.click(screen.getByRole('button',{name:'Delete Server 0'}))
 await waitFor(()=>expect(api.deleteVirtualServer).toHaveBeenCalledWith('server-0',7))
})

test('failed refresh retains cards and viewer mutation controls remain disabled',async()=>{
 vi.mocked(api.listUpstreams).mockResolvedValueOnce([upstream]).mockRejectedValueOnce(new Error('Inventory offline'))
 render(<RoleContext.Provider value="viewer"><MemoryRouter><Upstreams/></MemoryRouter></RoleContext.Provider>)
 await screen.findByRole('article',{name:'Alpha'})
 expect((screen.getByRole('button',{name:'Delete Alpha'}) as HTMLButtonElement).disabled).toBe(true)
 expect((screen.getByRole('button',{name:'Refresh capabilities for Alpha'}) as HTMLButtonElement).disabled).toBe(true)
 fireEvent.click(screen.getByRole('button',{name:'Refresh list'}))
 expect((await screen.findByRole('alert')).textContent).toContain('Inventory offline')
 expect(screen.getByRole('article',{name:'Alpha'})).toBeTruthy()
 expect(api.refreshTools).not.toHaveBeenCalled()
})
