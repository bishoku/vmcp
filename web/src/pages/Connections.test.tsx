import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { api } from '../api'
import { server, upstream, key, binding } from '../connections/fixtures'
import Connections from './Connections'

beforeEach(()=>{
 vi.spyOn(api,'listVirtualServers').mockResolvedValue([server])
 vi.spyOn(api,'listUpstreams').mockResolvedValue([upstream])
 vi.spyOn(api,'listAgentKeys').mockResolvedValue([key])
 vi.spyOn(api,'listAudit')
 vi.spyOn(api,'listBindings').mockResolvedValue([binding])
})
afterEach(()=>{cleanup();vi.restoreAllMocks();Reflect.deleteProperty(HTMLDialogElement.prototype,'showModal');Reflect.deleteProperty(HTMLDialogElement.prototype,'close')})
const open=()=>render(<MemoryRouter><Connections/></MemoryRouter>)
test('configuration graph and accessible table show the same grants and selected upstream',async()=>{
 open();await screen.findByText('Configured relationships (3)')
 const flow=screen.getByRole('region',{name:'Connection flow'})
 expect(within(flow).getByRole('button',{name:/team-key/})).toBeTruthy()
 expect(within(flow).getByRole('link',{name:/Demo/}).getAttribute('href')).toBe('/virtual-servers/demo/overview')
 expect(screen.getByText(/not active clients or live traffic/)).toBeTruthy()
 expect(api.listAudit).not.toHaveBeenCalled() // no eager audit or gateway calls
})
test('search includes aliases and policies and dialog reveals health timestamps and missing objects',async()=>{
 HTMLDialogElement.prototype.showModal=function(){this.open=true}
 HTMLDialogElement.prototype.close=function(){this.open=false}
 open();await screen.findByText('Configured relationships (3)')
 fireEvent.change(screen.getByLabelText('Filter relationships'),{target:{value:'doc://team'}})
 await screen.findByText('Configured relationships (1)')
 fireEvent.click(screen.getByRole('button',{name:'Inspect upstream demo to alpha'}))
 const dialog=screen.getByRole('dialog',{name:'demo → alpha'})
 expect(within(dialog).getByText('Missing object')).toBeTruthy()
 expect(within(dialog).getByText(/Checked:/)).toBeTruthy()
 expect(within(dialog).getByRole('link',{name:'Open configuration'}).getAttribute('href')).toBe('/upstreams?inspect=alpha')
})
test('large tables render 50 rows at a time and filtering resets the visible page',async()=>{
 vi.mocked(api.listAgentKeys).mockResolvedValue(Array.from({length:120},(_,i)=>({...key,slug:`key-${i}`})))
 open();await screen.findByText('Configured relationships (122)')
 const table=screen.getByRole('table')
 expect(within(table).getAllByRole('row')).toHaveLength(51)
 fireEvent.click(screen.getByText('Show 50 more relationships'))
 expect(within(table).getAllByRole('row')).toHaveLength(101)
 fireEvent.change(screen.getByLabelText('Filter relationships'),{target:{value:'key-119'}})
 expect(within(table).getAllByRole('row')).toHaveLength(2)
})
test('partial catalog failure remains visible while successful relationships are retained',async()=>{
 vi.mocked(api.listBindings).mockRejectedValue(new Error('bindings offline'))
 open();await waitFor(()=>expect(screen.getByRole('alert').textContent).toContain('bindings offline'))
 expect(screen.getByText('Configured relationships (2)')).toBeTruthy()
})
